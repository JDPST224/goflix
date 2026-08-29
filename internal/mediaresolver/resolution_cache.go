package mediaresolver

// Resolution cache: remembers the validated upstream source of a successful
// resolve per (provider, media type, TMDB id, season, episode) so watching
// the same title again skips the resolve chain entirely — the player starts
// from a RAM-cached manifest before any upstream traffic happens, and the
// remembered link is re-validated in the background.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// resolveFlight is one in-flight fresh resolve shared by all concurrent
// callers of the same resolution key.
type resolveFlight struct {
	done chan flightResult
}

type flightResult struct {
	proxyURL string
	err      error
}

// resolutionStats counts what the resolution cache actually does, surfaced
// via Resolver.StatsSnapshot (exposed on /api/health).
type resolutionStats struct {
	hits, misses, heals, healFailures, prewarms, invalidations atomic.Int64
}

// StatsSnapshot returns a copy of the cache counters plus the current record
// count.
func (r *Resolver) StatsSnapshot() map[string]int64 {
	r.mu.Lock()
	records := len(r.resolutions)
	r.mu.Unlock()
	return map[string]int64{
		"hits":          r.stats.hits.Load(),
		"misses":        r.stats.misses.Load(),
		"heals":         r.stats.heals.Load(),
		"healFailures":  r.stats.healFailures.Load(),
		"prewarms":      r.stats.prewarms.Load(),
		"invalidations": r.stats.invalidations.Load(),
		"records":       int64(records),
	}
}

// logResolutionStats reports the cache counters — quiet until something has
// actually been resolved.
func (r *Resolver) logResolutionStats() {
	s := r.StatsSnapshot()
	if s["hits"]+s["misses"] == 0 {
		return
	}
	log.Printf("[MediaResolver] resolution cache stats: hits=%d misses=%d heals=%d healFailures=%d prewarms=%d invalidations=%d records=%d",
		s["hits"], s["misses"], s["heals"], s["healFailures"], s["prewarms"], s["invalidations"], s["records"])
}

// prewarmNextEpisode resolves the following episode in the background while
// the current one plays, so pressing Next episode replays the cached
// resolution instantly. Only the record is prepared — the session (and its
// read-ahead) is minted by the normal cache-hit path when playback actually
// starts, so nothing is held for an episode that is never watched. When the
// played episode is the last of its season, the first episode of the next
// season is prewarmed instead; at the series end (verified via
// HasEpisodeProvider, when set) nothing is attempted at all.
func (r *Resolver) prewarmNextEpisode(req MediaRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, ok := r.nextPrewarmTarget(ctx, req)
	if !ok {
		return // end of series (or non-numeric episode): nothing to prewarm
	}
	nextKey := resolutionKey(target)
	if r.lookupResolution(target) != nil {
		return // already replayable
	}
	if r.isClosed() {
		return
	}
	if _, busy := r.prewarms.LoadOrStore(nextKey, struct{}{}); busy {
		return
	}
	defer r.prewarms.Delete(nextKey)

	fresh, err := r.resolveDirectAny(ctx, target)
	if err != nil {
		// Common at season boundaries (episode N+1 does not exist) — keep
		// the log line cheap and non-alarming.
		log.Printf("[MediaResolver] next-episode prewarm skipped s%se%s: %v", target.Season, target.Episode, err)
		return
	}
	r.rememberResolution(target, newResolutionRecord(fresh))
	r.stats.prewarms.Add(1)
	log.Printf("[MediaResolver] prewarmed next episode s%se%s provider=%s", target.Season, target.Episode, target.Provider)
}

// nextPrewarmTarget picks the episode to prewarm after req: the next episode
// in the same season, else the first episode of the next season. ok=false
// means neither exists (verified via HasEpisodeProvider when set), so there
// is nothing to prewarm.
func (r *Resolver) nextPrewarmTarget(ctx context.Context, req MediaRequest) (MediaRequest, bool) {
	next, ok := nextEpisodeNumber(req.Episode)
	if !ok {
		return MediaRequest{}, false
	}
	sameSeason := MediaRequest{Type: TV, ID: req.ID, Season: req.Season, Episode: next, Provider: req.Provider}
	if r.episodeMayExist(ctx, sameSeason) {
		return sameSeason, true
	}
	// Season rollover: first episode of the next season.
	nextSeason, ok := nextEpisodeNumber(req.Season)
	if !ok || nextSeason == req.Season {
		return MediaRequest{}, false
	}
	rollover := MediaRequest{Type: TV, ID: req.ID, Season: nextSeason, Episode: "1", Provider: req.Provider}
	if r.episodeMayExist(ctx, rollover) {
		return rollover, true
	}
	return MediaRequest{}, false
}

// episodeMayExist consults HasEpisodeProvider; when unset it assumes yes so
// behavior is unchanged for deployments without TMDB credentials.
func (r *Resolver) episodeMayExist(ctx context.Context, req MediaRequest) bool {
	if r.HasEpisodeProvider == nil {
		return true
	}
	return r.HasEpisodeProvider(ctx, req.ID, req.Season, req.Episode)
}

// nextEpisodeNumber returns the episode number following ep, preserving
// zero-padding width ("04" → "05"). Non-numeric episode ids report false.
func nextEpisodeNumber(ep string) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(ep))
	if err != nil || n < 0 {
		return "", false
	}
	next := strconv.Itoa(n + 1)
	if len(ep) > len(next) {
		next = strings.Repeat("0", len(ep)-len(next)) + next
	}
	return next, true
}

const (
	// resolutionTTL caps how long a remembered upstream link is trusted.
	// VOD CDNs eventually rotate paths; past this age the full resolve
	// chain runs again even if the link still answers.
	resolutionTTL = 24 * time.Hour
	// maxResolutionRecords bounds the cache's memory footprint (records
	// carry headers and at most one manifest each).
	maxResolutionRecords = 512
	// resolutionValidateTimeout bounds the background revalidation fetch so
	// a hung upstream cannot pin a healer goroutine forever.
	resolutionValidateTimeout = 15 * time.Second
	// maxCachedManifestBytes caps the manifest text kept inside a record.
	// Masters are normally a few KB; a pathological one only keeps its
	// fingerprint (validation re-fetches and compares either way), so 512
	// records can never balloon into hundreds of MB.
	maxCachedManifestBytes = 512 << 10
	// resolutionFileVersion guards the on-disk format so an older file is
	// discarded wholesale instead of half-interpreted.
	resolutionFileVersion = 1
)

// resolutionRecord is one remembered, validated upstream resolution.
type resolutionRecord struct {
	source  string
	headers http.Header
	allowed map[string]bool
	// manifest is the master playlist text as validated at resolve time.
	// nil for browser-resolved sources whose master was never captured.
	manifest []byte
	// manifestFP is the sha256 of manifest — the immutability fingerprint.
	// VOD manifests never change, so identical bytes on a later fetch
	// guarantee identical content. Empty when manifest is nil; those
	// records fall back to a prefix check.
	manifestFP string
	// noRevalidate marks sources whose manifest cannot be meaningfully
	// re-fetched (dynamic API endpoints, single-use tokens). Their records
	// trust the stored manifest until the TTL instead of validating.
	noRevalidate bool
	createdAt    time.Time
}

// newResolutionRecord captures a direct resolution for the cache. The
// manifest fingerprint is always recorded; the manifest text itself is only
// kept when small enough to seed the RAM cache on a replay.
func newResolutionRecord(vr *directResolution) *resolutionRecord {
	rec := &resolutionRecord{
		source:       vr.Source,
		headers:      cloneHeader(vr.Headers),
		allowed:      cloneAllowed(vr.Allowed),
		noRevalidate: vr.NoRevalidate,
	}
	if vr.MasterText != "" {
		sum := sha256.Sum256([]byte(vr.MasterText))
		rec.manifestFP = hex.EncodeToString(sum[:])
		if len(vr.MasterText) <= maxCachedManifestBytes {
			rec.manifest = []byte(vr.MasterText)
		}
	}
	return rec
}

// resolutionKey identifies a request exactly: provider, media type, TMDB id
// and — for TV — season and episode. Exact strings mean a hit can only ever
// replay the source this precise request produced before.
func resolutionKey(req MediaRequest) string {
	provider := req.Provider
	if provider == "" {
		provider = "vixsrc"
	}
	season, episode := req.Season, req.Episode
	if req.Type != TV {
		season, episode = "-", "-"
	}
	return provider + "|" + string(req.Type) + "|" + req.ID + "|" + season + "|" + episode
}

// lookupResolution returns the remembered record for a request, or nil when
// none exists or it aged past resolutionTTL.
func (r *Resolver) lookupResolution(req MediaRequest) *resolutionRecord {
	key := resolutionKey(req)
	r.mu.Lock()
	rec, ok := r.resolutions[key]
	if ok && time.Now().After(rec.createdAt.Add(resolutionTTL)) {
		delete(r.resolutions, key)
		ok = false
	}
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return rec
}

// rememberResolution stores a validated resolution, refreshing its timestamp.
func (r *Resolver) rememberResolution(req MediaRequest, rec *resolutionRecord) {
	if rec == nil || rec.source == "" {
		return
	}
	rec.createdAt = time.Now()
	key := resolutionKey(req)
	r.mu.Lock()
	r.resolutions[key] = rec
	// Evict the oldest record when over cap so a long-running server's
	// memory stays bounded regardless of how many titles are watched.
	if len(r.resolutions) > maxResolutionRecords {
		oldestKey := ""
		var oldest time.Time
		first := true
		for k, v := range r.resolutions {
			if first || v.createdAt.Before(oldest) {
				oldestKey, oldest, first = k, v.createdAt, false
			}
		}
		delete(r.resolutions, oldestKey)
	}
	r.mu.Unlock()
	r.persistResolutions()
}

// forgetResolution drops the remembered resolution for a request so the next
// Resolve runs the full chain.
func (r *Resolver) forgetResolution(req MediaRequest) {
	key := resolutionKey(req)
	r.mu.Lock()
	delete(r.resolutions, key)
	r.mu.Unlock()
	r.persistResolutions()
}

// InvalidateResolution drops the remembered resolution behind a live proxy
// session (looked up by token) so the next source request re-resolves fully.
// The player calls this after fatal playback errors tied to a stale link.
func (r *Resolver) InvalidateResolution(token string) {
	r.mu.Lock()
	s := r.sessions[token]
	var key string
	if s != nil {
		key = s.reqKey
		delete(r.resolutions, key)
	}
	r.mu.Unlock()
	if key != "" {
		r.stats.invalidations.Add(1)
		r.persistResolutions()
	}
}

// tryCachedResolution replays a remembered resolution without upstream
// traffic: the manifest is served straight from RAM, so playback starts
// before the resolve chain would even have finished. The remembered link is
// re-validated in the background (validateAndHeal) — the user never waits
// on that check. Reports false when nothing is remembered.
func (r *Resolver) tryCachedResolution(req MediaRequest) (string, bool) {
	rec := r.lookupResolution(req)
	if rec == nil {
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), rec.source, rec.headers, rec.allowed)
	if err != nil {
		// Cannot serve (closed resolver, session cap) — fall through to a
		// full resolve, which surfaces the same error to the caller.
		return "", false
	}
	if len(rec.manifest) > 0 {
		// Re-seed the body cache so the proxy fast path serves the master
		// from RAM on the player's very first request.
		r.cache.put(&cacheEntry{
			key:         rec.source,
			data:        rec.manifest,
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	// Subtitles + read-ahead, exactly like a fresh resolve, so a cached
	// launch behaves identically to the original one.
	r.attachAndWarm(token, req)
	log.Printf("[MediaResolver] resolution cache hit (%s) — instant session", redactQuery(rec.source))
	go r.validateAndHeal(req, token, rec)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// validateAndHeal re-checks the remembered upstream in the background after
// playback has already started from the RAM-cached manifest. A fingerprint
// match means the stream being watched is exactly what validated at resolve
// time — done, invisibly. A mismatch (rotated path, error page, dead host)
// triggers a full re-resolve and hot-swaps the session's source so the proxy
// keeps serving under the same token.
func (r *Resolver) validateAndHeal(req MediaRequest, token string, rec *resolutionRecord) {
	// Serialize heals: they are rare and short, and a global lock is simpler
	// than per-key single-flight for a one-shot background check.
	r.healMu.Lock()
	defer r.healMu.Unlock()
	if r.isClosed() {
		return
	}
	if r.validateRecord(rec) {
		// Still legit — extend the trust window.
		r.mu.Lock()
		rec.createdAt = time.Now()
		r.mu.Unlock()
		return
	}
	log.Printf("[MediaResolver] cached resolution stale (%s); re-resolving in background", redactQuery(rec.source))
	r.forgetResolution(req)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fresh, err := r.resolveDirectAny(ctx, req)
	if err != nil {
		// Keep serving the old session — the RAM manifest still plays, and
		// the player's recovery ladder handles any segment that vanished.
		r.stats.healFailures.Add(1)
		log.Printf("[MediaResolver] background re-resolve failed: %v", err)
		return
	}
	r.rememberResolution(req, newResolutionRecord(fresh))
	r.hotSwapSession(token, fresh)
	r.stats.heals.Add(1)
}

// validateRecord re-fetches the remembered manifest and compares it with
// what validated at resolve time: byte-identical (sha256) when a fingerprint
// exists, otherwise a well-formed playlist. Anything else — an error page,
// a rotated playlist, a dead host — fails.
func (r *Resolver) validateRecord(rec *resolutionRecord) bool {
	if rec.noRevalidate {
		// The source cannot be re-fetched meaningfully (dynamic API
		// endpoint, single-use tokens) — the RAM manifest is authoritative.
		return true
	}
	body, err := r.fetchManifestForValidation(rec)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		return false
	}
	if rec.manifestFP == "" {
		return true
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) == rec.manifestFP
}

// fetchManifestForValidation fetches the remembered manifest with the
// session headers the CDN saw at resolve time.
func (r *Resolver) fetchManifestForValidation(rec *resolutionRecord) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), resolutionValidateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rec.source, nil)
	if err != nil {
		return nil, err
	}
	for _, k := range playbackHeaders {
		if v := rec.headers.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	client := &http.Client{Transport: r.transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
}

// hotSwapSession points an existing live session at a freshly resolved
// source under the same token: the player URL never changes.
func (r *Resolver) hotSwapSession(token string, fresh *directResolution) {
	if fresh == nil || fresh.Source == "" {
		return
	}
	r.mu.Lock()
	s := r.sessions[token]
	if s == nil {
		r.mu.Unlock()
		return
	}
	s.source = fresh.Source
	s.allowed = cloneAllowed(fresh.Allowed)
	if len(fresh.Headers) > 0 {
		s.headers = cloneHeader(fresh.Headers)
	}
	// Replace the read-ahead pipeline: it tracks the old playlist's
	// segment list and cursor.
	var oldCancel context.CancelFunc
	if s.warmer != nil && s.warmer.cancel != nil {
		oldCancel = s.warmer.cancel
	}
	s.warmer = nil
	r.mu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
	if fresh.MasterText != "" {
		r.cache.put(&cacheEntry{
			key:         fresh.Source,
			data:        []byte(fresh.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	// Re-warm the new source's manifest chain in the background so upcoming
	// segments are hot by the time the player reaches them.
	r.ensureWarmer(s)
	log.Printf("[MediaResolver] session hot-swapped to fresh source %s", redactQuery(fresh.Source))
}

// resolveDirectAny runs the provider's direct resolve chain, bypassing the
// resolution cache. Used by the background healer.
func (r *Resolver) resolveDirectAny(ctx context.Context, req MediaRequest) (*directResolution, error) {
	switch req.Provider {
	case "", "vixsrc":
		return r.resolveVixsrcDirect(ctx, req)
	case "cinesrc":
		return r.resolveCinesrcDirect(ctx, req)
	case "vidking":
		return r.resolveVidkingDirect(ctx, req)
	case "vidlove":
		return r.resolveVidloveDirect(ctx, req)
	case "vidsrcme", "vidsrc":
		return r.resolveVidsrcmeDirect(ctx, req)
	}
	return nil, errors.New("unknown provider")
}

// --- Disk persistence -------------------------------------------------------
//
// Upstream VOD links routinely outlive server restarts by weeks, so the
// resolution cache is written through to a small JSON file: a restart keeps
// its instant-rewatch behavior instead of re-running every resolve chain.

// storedResolutionRecord is the JSON form of resolutionRecord, keyed by
// resolution key in the file.
type storedResolutionRecord struct {
	Source     string              `json:"source"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Allowed    map[string]bool     `json:"allowed,omitempty"`
	Manifest   []byte              `json:"manifest,omitempty"`
	ManifestFP string              `json:"manifest_fp,omitempty"`
	// NoRevalidate: the source cannot be re-fetched for validation
	// (dynamic API endpoints, single-use tokens).
	NoRevalidate bool      `json:"no_revalidate,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type resolutionFile struct {
	Version int                                `json:"version"`
	Records map[string]*storedResolutionRecord `json:"records"`
}

// persistResolutions snapshots the cache and writes it to the configured
// path atomically (temp file + rename). Records are tiny and writes are
// rare (once per watch), so a synchronous write keeps the code simple and
// the file always consistent with memory.
func (r *Resolver) persistResolutions() {
	path := r.resolutionCachePath()
	if path == "" {
		return
	}
	r.mu.Lock()
	file := resolutionFile{Version: resolutionFileVersion, Records: make(map[string]*storedResolutionRecord, len(r.resolutions))}
	for k, rec := range r.resolutions {
		if rec.source == "" {
			continue
		}
		file.Records[k] = &storedResolutionRecord{
			Source:       rec.source,
			Headers:      rec.headers,
			Allowed:      rec.allowed,
			Manifest:     rec.manifest,
			ManifestFP:   rec.manifestFP,
			NoRevalidate: rec.noRevalidate,
			CreatedAt:    rec.createdAt,
		}
	}
	r.mu.Unlock()
	data, err := json.MarshalIndent(&file, "", "  ")
	if err != nil {
		log.Printf("[MediaResolver] resolution cache marshal failed: %v", err)
		return
	}
	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("[MediaResolver] resolution cache write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		log.Printf("[MediaResolver] resolution cache rename failed: %v", err)
	}
}

// loadPersistedResolutions restores records saved by a previous run,
// discarding anything already past its TTL.
func (r *Resolver) loadPersistedResolutions() {
	path := r.resolutionCachePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run or unreadable file: start empty
	}
	var file resolutionFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != resolutionFileVersion {
		log.Printf("[MediaResolver] discarding unreadable/legacy resolution cache %s", path)
		return
	}
	now := time.Now()
	loaded := 0
	r.mu.Lock()
	for k, sr := range file.Records {
		if sr == nil || sr.Source == "" || sr.CreatedAt.IsZero() {
			continue
		}
		if now.After(sr.CreatedAt.Add(resolutionTTL)) {
			continue
		}
		r.resolutions[k] = &resolutionRecord{
			source:       sr.Source,
			headers:      sr.Headers,
			allowed:      sr.Allowed,
			manifest:     sr.Manifest,
			manifestFP:   sr.ManifestFP,
			noRevalidate: sr.NoRevalidate,
			createdAt:    sr.CreatedAt,
		}
		loaded++
	}
	r.mu.Unlock()
	if loaded > 0 {
		log.Printf("[MediaResolver] restored %d resolution(s) from %s", loaded, filepath.Base(path))
	}
}

// resolutionCachePath returns the persistence path, applying the default,
// or "" when persistence is explicitly disabled.
func (r *Resolver) resolutionCachePath() string {
	p := r.cfg.ResolutionCachePath
	if p == "-" {
		return ""
	}
	if p == "" {
		p = "resolutions.json"
	}
	return p
}
