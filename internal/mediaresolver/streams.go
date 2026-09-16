package mediaresolver

// Dashboard-facing stream monitoring: active proxy sessions with their
// viewer IPs, per-session byte counters, a rolling bandwidth history for the
// dashboard graphs, and the admin pause/stop/block controls.

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors surfaced by Proxy so the server layer can answer the
// player with a meaningful status while an admin holds or blocks a stream.
var (
	ErrStreamPaused  = errors.New("stream paused by administrator")
	ErrStreamBlocked = errors.New("viewer address blocked by administrator")
)

const (
	// bwBucketSeconds is the granularity of the bandwidth history graph.
	bwBucketSeconds = 5
	// bwHistoryMinutes is how far back the bandwidth history reaches.
	bwHistoryMinutes = 30
	// bwMaxBuckets is the steady-state bucket count of the rolling history.
	bwMaxBuckets = bwHistoryMinutes * 60 / bwBucketSeconds
	// unknownIP labels traffic recorded before a viewer address was learned.
	unknownIP = "unknown"
	// streamIdleAfter is how long without any player request a session is
	// still shown as live. HLS players poll every few seconds while playing,
	// so a closed tab or stopped player goes quiet almost immediately.
	streamIdleAfter = 30 * time.Second
	// streamHiddenAfter is how long a quiet, unpaused session lingers in the
	// dashboard table before it disappears (the session itself keeps its
	// normal TTL so a quick tab switch back resumes instantly).
	streamHiddenAfter = 2 * time.Minute
)

// bwBucket aggregates the traffic served to players within one time slice.
type bwBucket struct {
	total int64
	perIP map[string]int64
}

// streamInfo is one row of the dashboard's live-streams table: an active
// proxy session, its viewer and its playback identity.
type streamInfo struct {
	Token     string    `json:"token"`
	ClientIP  string    `json:"ip"`
	Provider  string    `json:"provider"`
	MediaType string    `json:"media_type"`
	MediaID   string    `json:"media_id"`
	Season    string    `json:"season,omitempty"`
	Episode   string    `json:"episode,omitempty"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Bytes     int64     `json:"bytes"`
	StartedAt time.Time `json:"started_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Active reports whether the player made a request recently — a closed
	// tab stops polling, so the dashboard shows Idle instead of Live.
	Active bool `json:"active"`
	// Expired reports whether the session outlived streamHiddenAfter since
	// its last activity; the dashboard hides such rows.
	Expired bool `json:"expired"`
	Paused  bool `json:"paused"`
}

// bwPoint is one slice of the bandwidth history: total bytes served in the
// slice and the breakdown per viewer IP.
type bwPoint struct {
	Time  int64            `json:"t"` // unix seconds of slice start
	Total int64            `json:"total"`
	PerIP map[string]int64 `json:"per_ip"`
}

// remoteIP extracts the bare viewer address from the proxy request.
func remoteIP(req *http.Request) string {
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}

// parseReqKey splits a resolution key (provider|type|id|season|episode) back
// into its playback identity for the dashboard.
func parseReqKey(key string) (provider, mediaType, id, season, episode string) {
	parts := strings.Split(key, "|")
	if len(parts) != 5 {
		return "", "", "", "", ""
	}
	return parts[0], parts[1], parts[2], parts[3], parts[4]
}

// highestVariantHeight returns the pixel height of the video variant the
// player is capped to. It mirrors forceHighestQuality's pick (largest pixel
// area, bandwidth as tiebreak) and its tag inference for playlists that omit
// RESOLUTION (4K/UHD → 2160, 2K/QHD → 1440, FHD → 1080, HD → 720). 0,0 when
// the master lists no usable resolution.
func highestVariantSize(master string) (width, height int) {
	maxPixels, maxBandwidth := -1, -1
	for _, line := range strings.Split(master, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			continue
		}
		bw := 0
		if m := bwRE.FindStringSubmatch(line); len(m) == 2 {
			bw, _ = strconv.Atoi(m[1])
		}
		pixels, w, h := 0, 0, 0
		if m := resRE.FindStringSubmatch(line); len(m) == 3 {
			w, _ = strconv.Atoi(m[1])
			h, _ = strconv.Atoi(m[2])
			pixels = w * h
		} else {
			switch {
			case strings.Contains(line, "4K") || strings.Contains(line, "2160") || strings.Contains(line, "UHD"):
				w, h, pixels = 3840, 2160, 3840*2160
			case strings.Contains(line, "1440") || strings.Contains(line, "2K") || strings.Contains(line, "QHD"):
				w, h, pixels = 2560, 1440, 2560*1440
			case strings.Contains(line, "1080") || strings.Contains(line, "FHD"):
				w, h, pixels = 1920, 1080, 1920*1080
			case strings.Contains(line, "720") || strings.Contains(line, "HD"):
				w, h, pixels = 1280, 720, 1280*720
			}
		}
		if pixels > maxPixels || (pixels == maxPixels && bw > maxBandwidth) {
			maxPixels, maxBandwidth, width, height = pixels, bw, w, h
		}
	}
	return width, height
}

// highestVariantHeight returns just the pixel height of the capped variant.
func highestVariantHeight(master string) int {
	_, h := highestVariantSize(master)
	return h
}

// noteStreamHeight records the top variant's dimensions the first time a
// playlist is seen for the session (read-ahead or live proxy path). Masters
// are parsed for RESOLUTION attributes; bare media playlists fall back to
// quality tokens in their URIs (vidking-style "init-s2160p-v1-a1.mp4"),
// which infer a standard 16:9 frame of that tier.
func (r *Resolver) noteStreamHeight(s *proxySession, text string) {
	if s.maxHeight.Load() != 0 {
		return
	}
	if w, h := highestVariantSize(text); h > 0 {
		s.maxWidth.Store(int32(w))
		s.maxHeight.Store(int32(h))
		return
	}
	if h := heightFromQualityToken(text); h > 0 {
		s.maxHeight.Store(int32(h))
	}
}

// heightFromTier parses a quality tier label ("2160p", "1080p") into a pixel
// height; 0 when the label carries no tier number ("auto", "").
func heightFromTier(tier string) int {
	tier = strings.TrimSpace(strings.ToLower(tier))
	if !strings.HasSuffix(tier, "p") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(tier, "p"))
	if err != nil || n <= 0 || n > 8640 {
		return 0
	}
	return n
}

// qualityTokenRE matches explicit quality tokens in playlist URIs: vidking
// and similar CDNs name every rendition URL after its tier, e.g.
// "index-s2160p-v1-a1.m3u8" or "seg-1080p.mp4". Bounded on both sides so
// bitrates ("b2450000") and codec strings never match.
var qualityTokenRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9])s?(\d{3,4})p(?:[^A-Za-z0-9]|$)`)

// heightFromQualityToken extracts the best pixel height from quality tokens
// scattered through a playlist's URIs.
func heightFromQualityToken(text string) int {
	best := 0
	for _, m := range qualityTokenRE.FindAllStringSubmatch(text, 64) {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 144 || n > 8640 {
			continue
		}
		if n > best {
			best = n
		}
	}
	return best
}

// noteStreamHeightToken stamps the dashboard resolution onto the session
// behind token at resolve time: an explicit provider tier label wins, then
// the validated master playlist is parsed, then quality tokens in the
// playlist URIs. Callers race the read-ahead warmer, so an already-recorded
// height is never overwritten.
func (r *Resolver) noteStreamHeightToken(token, masterText, tier string) {
	h := heightFromTier(tier)
	w := 0
	if h == 0 && masterText != "" {
		w, h = highestVariantSize(masterText)
		if h == 0 {
			h = heightFromQualityToken(masterText)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[token]
	if s == nil || s.maxHeight.Load() != 0 {
		return
	}
	if h == 0 && s.source != "" {
		// Last resort: the source URL itself often names the tier
		// (".../index-s2160p-v1-a1.m3u8").
		h = heightFromQualityToken(s.source)
	}
	if h > 0 {
		if w > 0 {
			s.maxWidth.Store(int32(w))
		}
		s.maxHeight.Store(int32(h))
	}
}

// streamTouch records the viewer address and refreshes the activity stamp.
func (r *Resolver) streamTouch(s *proxySession, ip string) {
	if ip == "" {
		ip = unknownIP
	}
	s.clientIP.Store(ip)
	s.lastSeenAt.Store(time.Now().UnixNano())
}

// recordTraffic accounts n bytes served to a player: the per-session counter
// feeds the streams table while the rolling buckets feed the graphs.
func (r *Resolver) recordTraffic(s *proxySession, n int64) {
	if n <= 0 {
		return
	}
	s.bytesServed.Add(n)
	ip, _ := s.clientIP.Load().(string)
	if ip == "" {
		ip = unknownIP
	}
	slot := time.Now().Unix() / bwBucketSeconds
	r.bwMu.Lock()
	// Lazy init: tests and non-main paths can build a Resolver without
	// New(), so the maps cannot be assumed non-nil.
	if r.bwBuckets == nil {
		r.bwBuckets = make(map[int64]*bwBucket)
	}
	b := r.bwBuckets[slot]
	if b == nil {
		b = &bwBucket{perIP: map[string]int64{}}
		r.bwBuckets[slot] = b
		// Prune expired buckets on new-bucket creation so the cost is
		// amortized and a long-running server's history stays bounded.
		if cutoff := slot - int64(bwMaxBuckets); len(r.bwBuckets) > bwMaxBuckets+2 {
			for k := range r.bwBuckets {
				if k < cutoff {
					delete(r.bwBuckets, k)
				}
			}
		}
	}
	b.total += n
	b.perIP[ip] += n
	r.bwMu.Unlock()
}

// ipBlocked reports whether a viewer address is blocked by an administrator.
func (r *Resolver) ipBlocked(ip string) bool {
	if ip == "" {
		return false
	}
	r.mu.Lock()
	_, blocked := r.blockedIPs[ip]
	r.mu.Unlock()
	return blocked
}

// StreamsSnapshot returns every unexpired proxy session with its viewer and
// playback identity, newest first.
func (r *Resolver) StreamsSnapshot() []streamInfo {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]streamInfo, 0, len(r.sessions))
	for tok, s := range r.sessions {
		if now.After(s.expiresAt) {
			continue
		}
		si := streamInfo{
			Token:     tok,
			Width:     int(s.maxWidth.Load()),
			Height:    int(s.maxHeight.Load()),
			Bytes:     s.bytesServed.Load(),
			StartedAt: s.createdAt,
			Paused:    s.paused.Load(),
		}
		if v, ok := s.clientIP.Load().(string); ok {
			si.ClientIP = v
		}
		if ls := s.lastSeenAt.Load(); ls > 0 {
			si.LastSeen = time.Unix(0, ls)
		} else {
			si.LastSeen = s.createdAt
		}
		si.Active = now.Sub(si.LastSeen) < streamIdleAfter
		si.Expired = now.Sub(si.LastSeen) >= streamHiddenAfter && !si.Paused
		si.Provider, si.MediaType, si.MediaID, si.Season, si.Episode = parseReqKey(s.reqKey)
		out = append(out, si)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// BandwidthHistory returns the rolling per-IP traffic history for the
// dashboard graphs, one point per bucket covering the full window.
func (r *Resolver) BandwidthHistory() (points []bwPoint, bucketSec, windowSec int) {
	nowSlot := time.Now().Unix() / bwBucketSeconds
	span := int64(bwMaxBuckets)
	points = make([]bwPoint, 0, span)
	r.bwMu.Lock()
	defer r.bwMu.Unlock()
	for slot := nowSlot - span + 1; slot <= nowSlot; slot++ {
		p := bwPoint{Time: slot * bwBucketSeconds, PerIP: map[string]int64{}}
		if b, ok := r.bwBuckets[slot]; ok {
			p.Total = b.total
			for ip, n := range b.perIP {
				p.PerIP[ip] = n
			}
		}
		points = append(points, p)
	}
	return points, bwBucketSeconds, bwHistoryMinutes * 60
}

// findSession returns the unexpired session behind token.
func (r *Resolver) findSession(token string) *proxySession {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[token]
	if s != nil && time.Now().After(s.expiresAt) {
		return nil
	}
	return s
}

// PauseStream holds the stream behind token: every player request is refused
// until ResumeStream clears the hold, so playback stalls at the buffer.
func (r *Resolver) PauseStream(token string) bool {
	s := r.findSession(token)
	if s == nil {
		return false
	}
	s.paused.Store(true)
	return true
}

// ResumeStream clears an administrative pause.
func (r *Resolver) ResumeStream(token string) bool {
	s := r.findSession(token)
	if s == nil {
		return false
	}
	s.paused.Store(false)
	return true
}

// RetireStream ends the session behind token at the viewer's own request
// (the player retires its previous session when starting a new one). The
// resolution cache is left intact so a rewatch replays instantly. This keeps
// the dashboard honest: a viewer switching titles never counts as two
// devices while the abandoned session idles out.
func (r *Resolver) RetireStream(token string) bool {
	return r.StopStream(token)
}

// StopStream kills the session behind token: the read-ahead pipeline is
// cancelled and every following player request is refused, ending playback.
func (r *Resolver) StopStream(token string) bool {
	r.mu.Lock()
	s, ok := r.sessions[token]
	if ok {
		delete(r.sessions, token)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	if s.warmer != nil && s.warmer.cancel != nil {
		s.warmer.cancel()
	}
	return true
}

// BlockStream blocks the viewer address behind token — stopping every stream
// that address holds — and returns the blocked address.
func (r *Resolver) BlockStream(token string) (string, bool) {
	s := r.findSession(token)
	if s == nil {
		return "", false
	}
	ip, _ := s.clientIP.Load().(string)
	if ip == "" {
		return "", false
	}
	r.blockIP(ip)
	return ip, true
}

// BlockIP refuses every present and future stream from ip.
func (r *Resolver) BlockIP(ip string) {
	if ip != "" {
		r.blockIP(ip)
	}
}

// blockIP records the block and tears down every live session from ip.
func (r *Resolver) blockIP(ip string) {
	r.mu.Lock()
	if r.blockedIPs == nil {
		r.blockedIPs = make(map[string]time.Time)
	}
	r.blockedIPs[ip] = time.Now()
	var stops []*proxySession
	for tok, s := range r.sessions {
		if v, _ := s.clientIP.Load().(string); v == ip {
			stops = append(stops, s)
			delete(r.sessions, tok)
		}
	}
	r.mu.Unlock()
	for _, s := range stops {
		if s.warmer != nil && s.warmer.cancel != nil {
			s.warmer.cancel()
		}
	}
	r.persistBlockedIPs()
}

// UnblockIP lifts a viewer address block.
func (r *Resolver) UnblockIP(ip string) {
	if ip == "" {
		return
	}
	r.mu.Lock()
	delete(r.blockedIPs, ip)
	r.mu.Unlock()
	r.persistBlockedIPs()
}

// BlockedIPs lists every blocked viewer address, sorted.
func (r *Resolver) BlockedIPs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.blockedIPs))
	for ip := range r.blockedIPs {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

// ── Block list persistence ──────────────────────────────────────────────────
//
// Blocks are administrative decisions: they must survive a restart, so the
// blocked addresses are mirrored to a small JSON file following the same
// atomic-write pattern as the resolution cache.

const blockedFileVersion = 1

type blockedFile struct {
	Version int                  `json:"version"`
	Blocked map[string]time.Time `json:"blocked"` // address -> blocked at
}

func (r *Resolver) blockListPath() string {
	p := r.cfg.BlockListPath
	if p == "-" {
		return ""
	}
	if p == "" {
		p = "blocked.json"
	}
	return p
}

// loadBlockedIPs restores blocks saved by a previous run.
func (r *Resolver) loadBlockedIPs() {
	path := r.blockListPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run or unreadable file: start empty
	}
	var f blockedFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != blockedFileVersion {
		log.Printf("[MediaResolver] discarding unreadable/legacy block list %s", path)
		return
	}
	restored := 0
	r.mu.Lock()
	for ip, at := range f.Blocked {
		if ip == "" {
			continue
		}
		if r.blockedIPs == nil {
			r.blockedIPs = make(map[string]time.Time)
		}
		r.blockedIPs[ip] = at
		restored++
	}
	r.mu.Unlock()
	if restored > 0 {
		log.Printf("[MediaResolver] restored %d blocked address(es) from %s", restored, filepath.Base(path))
	}
}

// persistBlockedIPs snapshots the block list and writes it atomically. An
// empty list removes the file so the directory stays clean.
func (r *Resolver) persistBlockedIPs() {
	path := r.blockListPath()
	if path == "" {
		return
	}
	r.mu.Lock()
	blocked := make(map[string]time.Time, len(r.blockedIPs))
	for ip, at := range r.blockedIPs {
		if ip == "" {
			continue
		}
		blocked[ip] = at
	}
	r.mu.Unlock()
	if len(blocked) == 0 {
		_ = os.Remove(path)
		return
	}
	data, err := json.MarshalIndent(&blockedFile{Version: blockedFileVersion, Blocked: blocked}, "", "  ")
	if err != nil {
		log.Printf("[MediaResolver] block list marshal failed: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("[MediaResolver] block list write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		log.Printf("[MediaResolver] block list rename failed: %v", err)
	}
}
