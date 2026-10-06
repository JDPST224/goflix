package mediaresolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Direct movish resolution, reverse-engineered from vidcore.org's player
// bundle (which consumes movish as its "Rigel" upstream). The chain needs no
// browser, no encryption and no tokens:
//
//  1. GET https://movish.to/player-sources/rigel/movie/{tmdbId}
//     (or /tv/{id}/{s}/{e}) → JSON:
//     {"source":"rigel","streams":[{url,label,type,quality}, …]}
//     Each entry names one server (Rigel, Lyra, Algol, …); Rigel carries a
//     labelled "1080p" single-tier stream, Lyra and Algol serve multi-tier
//     ladders. Some entries carry no quality label but a "Spica"/"FHDp"
//     variant — those still validate as full ladders.
//
//  2. GET the stream URL (hosts api.dlproxy.com) → the master playlist.
//     Variants, the AES-128 (#EXT-X-KEY) URL and cdn.dlproxy.com segments
//     all answer plain GETs — no referer, cookies or client hints needed.
//     The master lists absolute variant URLs, and media playlists list
//     absolute segment URLs, so the generic proxy rewriter handles them
//     with no special casing.
//
// Servers are attempted in the order the API lists them (its own preference
// order); the first whose top tier validates wins. Failing servers fall
// through so a dead upstream node never blocks playback.

const (
	movishAPIBase = "https://movish.to/player-sources/rigel"
	movishReferer = "https://movish.to/"
	// movishResponseCap bounds the JSON source list; it is small.
	movishResponseCap = 1 << 20
)

type movishSource struct {
	URL     string `json:"url"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	Quality string `json:"quality"`
}

type movishPayload struct {
	Source  string         `json:"source"`
	Streams []movishSource `json:"streams"`
}

// tryMovishDirect resolves movish without a browser, registers the proxy
// session and starts the read-ahead warmup. It reports false so Resolve
// falls back to the browser scrape when the direct chain is unavailable.
func (r *Resolver) tryMovishDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	vr, tier, err := r.resolveMovishDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] movish direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] movish direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	// Dashboard resolution: dlproxy media playlists carry no RESOLUTION
	// attribute in their URIs, so the tier label from the source API is
	// authoritative for the Rigel single-tier entries; ladder servers get
	// their height from the master playlist itself.
	r.noteStreamHeightToken(token, vr.MasterText, tier)
	r.rememberResolution(req, newResolutionRecord(vr))
	if vr.MasterText != "" {
		// Admit the validated playlist under its canonical URL with a long
		// TTL: these CDN playlists are VOD manifests whose segment URLs
		// outlive the session by weeks.
		r.cache.put(&cacheEntry{
			key:         vr.Source,
			data:        []byte(vr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	log.Printf("[MediaResolver] movish resolved directly quality=%s source=%s", tier, redactQuery(vr.Source))
	r.attachAndWarm(token, req)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// movishCandidate is one stream entry queued for probing.
type movishCandidate struct {
	label string
	src   movishSource
}

// movishResult carries one candidate probe's outcome.
type movishResult struct {
	idx  int
	res  *directResolution
	tier string
	// height is the resolved master's top RESOLUTION height — the winner
	// picks the highest verified tier across the fleet, not merely the
	// first responding node (the API's listed order puts dead nodes first
	// just as often as good ones).
	height int
	err    error
}

// movishProbeTimeout caps a single candidate's depth validation. A node
// that cannot master+variant+segment inside 5 s is too slow to open a
// playback session on; the whole probe fleet runs in parallel, so the
// resolve's wall time is bounded by this cap, not the sum of failures.
const movishProbeTimeout = 5 * time.Second

// resolveMovishDirect walks the source API and returns the stream whose
// chain validates with the highest verified tier. Validation goes one level
// deeper than the master playlist: the dlproxy nodes rotate and some serve a
// well-formed master whose segment host no longer resolves, so the top
// variant's media playlist is fetched and its first segment is probed
// (bounded to 64 KiB) before a server is accepted.
//
// Candidates are probed concurrently and the winner is the best-height
// passing server; the wall time is bounded by movishProbeTimeout instead of
// the sum of the dead nodes, which kept the sequential chain at ~10 s on
// titles where half the fleet was down.
func (r *Resolver) resolveMovishDirect(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}

	payload, err := r.fetchMovishSources(ctx, client, req)
	if err != nil {
		return nil, "", err
	}

	cands := make([]movishCandidate, 0, len(payload.Streams))
	for _, st := range payload.Streams {
		u := strings.TrimSpace(st.URL)
		if u == "" {
			continue
		}
		pu, perr := url.Parse(u)
		if perr != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(st.Type), "hls") &&
			!strings.Contains(strings.ToLower(pu.Path), ".m3u8") {
			continue // mp4 Fastly rips and DASH are not proxiable here
		}
		cands = append(cands, movishCandidate{label: st.Label, src: st})
	}
	if len(cands) == 0 {
		return nil, "", errors.New("source API listed no proxiable streams")
	}

	results := make([]*movishResult, len(cands))
	// raceCtx carries a dedicated cancel so the losing probes' in-flight
	// upstream fetches are released the moment a winner is picked without
	// also cancelling the caller's context.
	raceCtx, raceCancel := context.WithCancel(ctx)
	defer raceCancel()
	var wg sync.WaitGroup
	for i, c := range cands {
		wg.Add(1)
		go func(i int, c movishCandidate) {
			defer wg.Done()
			res := &movishResult{idx: i}
			cctx, cancel := context.WithTimeout(raceCtx, movishProbeTimeout)
			defer cancel()
			res.tier, res.res, res.err = r.probeMovishCandidate(cctx, client, c.label, c.src)
			if res.res != nil {
				res.height = highestVariantHeight(res.res.MasterText)
			}
			results[i] = res
		}(i, c)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	var winner *movishResult
	var lastErr error
	for _, res := range results {
		if res == nil {
			continue
		}
		if res.err == nil && res.res != nil {
			// Highest verified height wins; ties keep the earlier listed
			// server (the API's preference order).
			if winner == nil || res.height > winner.height {
				winner = res
			}
		}
		if res.err != nil {
			lastErr = res.err
		}
	}
	if winner != nil {
		// Prefer a numeric tier label when the API supplied one; otherwise
		// derive it from the verified master height.
		tier := winner.tier
		if heightFromTier(tier) == 0 {
			tier = winnerHeightTier(winner)
		}
		return winner.res, tier, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no movish server attempted")
	}
	return nil, "", fmt.Errorf("all %d movish servers failed: %w", len(cands), lastErr)
}

// winnerHeightTier derives a numeric tier label from a winning resolution's
// verified master height ("816" → "816p").
func winnerHeightTier(winner *movishResult) string {
	if h := winner.height; h > 0 {
		return strings.ToLower(strconv.Itoa(h) + "p")
	}
	return ""
}

// probeMovishCandidate validates one source server: parse its URL, block-check
// the host, then validate the master→top variant→first segment depth.
func (r *Resolver) probeMovishCandidate(ctx context.Context, client *http.Client, label string, st movishSource) (string, *directResolution, error) {
	u := strings.TrimSpace(st.URL)
	pu, err := url.Parse(u)
	if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
		return "", nil, fmt.Errorf("server %q: bad stream URL", label)
	}
	if r.blockedUpstreamHost(ctx, pu.Hostname()) {
		return "", nil, fmt.Errorf("server %q host blocked", label)
	}

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", movishReferer)

	masterText, err := r.validateMovishDepth(ctx, client, u, headers)
	if err != nil {
		return "", nil, fmt.Errorf("server %q: %w", label, err)
	}
	res := &directResolution{
		Source:     u,
		Headers:    headers,
		Allowed:    map[string]bool{strings.ToLower(pu.Host): true},
		MasterText: masterText,
	}
	return movishQualityLabel(st.Quality), res, nil
}

// validateMovishDepth confirms a stream URL serves a master playlist, its
// top variant lists segments, and the first segment host actually answers
// (the dlproxy fleet rotates; dead nodes keep serving manifests). The master
// playlist text is returned so the caller can seed the body cache.
func (r *Resolver) validateMovishDepth(ctx context.Context, client *http.Client, masterURL string, headers http.Header) (string, error) {
	masterText, err := r.fetchManifestText(ctx, client, masterURL, headers)
	if err != nil {
		return "", fmt.Errorf("master playlist: %w", err)
	}
	if !strings.Contains(masterText, "#EXT-X-STREAM-INF") {
		// A bare media playlist: validate its own first segment directly.
		return masterText, r.probeFirstSegment(ctx, client, masterURL, masterText, headers)
	}
	variantURL, err := movishTopVariant(masterText, masterURL)
	if err != nil {
		return "", err
	}
	mediaText, err := r.fetchManifestText(ctx, client, variantURL, headers)
	if err != nil {
		return "", fmt.Errorf("top variant: %w", err)
	}
	return masterText, r.probeFirstSegment(ctx, client, variantURL, mediaText, headers)
}

// movishTopVariant parses the highest-resolution variant URI out of the
// master playlist text. Relative URIs are resolved against masterURL.
func movishTopVariant(master, masterURL string) (string, error) {
	base, err := url.Parse(masterURL)
	if err != nil {
		return "", fmt.Errorf("master URL: %w", err)
	}
	var bestURL string
	bestH := -1
	lines := strings.Split(master, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			continue
		}
		// URI is the next non-comment, non-empty line.
		uri := ""
		for j := i + 1; j < len(lines); j++ {
			cand := strings.TrimSpace(lines[j])
			if cand == "" || cand[0] == '#' {
				continue
			}
			uri, i = cand, j
			break
		}
		if uri == "" {
			continue
		}
		h := 0
		if m := vkHeightRE.FindStringSubmatch(line); m != nil {
			if n, e := strconv.Atoi(m[1]); e == nil {
				h = n
			}
		}
		if h > bestH {
			vu, verr := url.Parse(uri)
			if verr != nil {
				continue
			}
			bestURL, bestH = base.ResolveReference(vu).String(), h
		}
	}
	if bestURL == "" {
		return "", errors.New("master lists no parsable variant")
	}
	return bestURL, nil
}

// vkHeightRE extracts the RESOLUTION height from a STREAM-INF line.
var vkHeightRE = regexp.MustCompile(`RESOLUTION=\d+x(\d+)`)

// probeFirstSegment fetches the first segment named in a media playlist,
// bounded to 64 KiB, so a dead segment host fails the candidate server.
func (r *Resolver) probeFirstSegment(ctx context.Context, client *http.Client, playlistURL, mediaText string, headers http.Header) error {
	base, err := url.Parse(playlistURL)
	if err != nil {
		return fmt.Errorf("playlist URL: %w", err)
	}
	segURL := ""
	for _, line := range strings.Split(mediaText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		su, serr := url.Parse(line)
		if serr == nil {
			segURL = base.ResolveReference(su).String()
			break
		}
	}
	if segURL == "" {
		return errors.New("media playlist lists no segments")
	}
	segReq, err := http.NewRequestWithContext(ctx, http.MethodGet, segURL, nil)
	if err != nil {
		return err
	}
	for k, vals := range headers {
		for _, v := range vals {
			segReq.Header.Add(k, v)
		}
	}
	// Bounded probe: 64 KiB is enough to prove the host serves media without
	// downloading a full ~700 KiB TS chunk.
	segReq.Header.Set("Range", "bytes=0-65535")
	resp, err := client.Do(segReq)
	if err != nil {
		return fmt.Errorf("first segment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("first segment returned status %d", resp.StatusCode)
	}
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if n == 0 {
		return errors.New("first segment returned no data")
	}
	return nil
}

// fetchMovishSources calls the Rigel source API for the media id.
func (r *Resolver) fetchMovishSources(ctx context.Context, client *http.Client, req MediaRequest) (movishPayload, error) {
	var payload movishPayload
	rel := "movie/" + url.PathEscape(req.ID)
	if req.Type == TV {
		rel = "tv/" + url.PathEscape(req.ID) + "/" + url.PathEscape(req.Season) + "/" + url.PathEscape(req.Episode)
	}
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, movishAPIBase+"/"+rel, nil)
	if err != nil {
		return payload, err
	}
	apiReq.Header.Set("User-Agent", defaultUserAgent)
	apiReq.Header.Set("Referer", movishReferer)
	apiReq.Header.Set("Accept", "application/json")
	resp, err := r.resolveFetch(ctx, client, apiReq)
	if err != nil {
		return payload, fmt.Errorf("source API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return payload, fmt.Errorf("source API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, movishResponseCap))
	if err != nil {
		return payload, fmt.Errorf("source API read: %w", err)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, fmt.Errorf("source API returned invalid JSON: %w", err)
	}
	if payload.Source == "" || len(payload.Streams) == 0 {
		return payload, errors.New("source API listed no streams")
	}
	return payload, nil
}

// movishQualityRank extracts the numeric tier from quality labels like
// "1080p", "FHDp"; unranked labels rank 0 (unknown — ladder servers).
func movishQualityRank(q string) int {
	q = strings.TrimSpace(strings.ToLower(q))
	q = strings.TrimSuffix(q, "p")
	n, err := strconv.Atoi(q)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// movishQualityLabel normalizes a raw tier label ("FHDp" → "fhd", "" →
// "auto") for logs and the dashboard's quality column.
func movishQualityLabel(q string) string {
	if n := movishQualityRank(q); n > 0 {
		return strings.TrimSpace(strings.ToLower(q))
	}
	if q != "" {
		return strings.ToLower(strings.TrimSpace(q))
	}
	return "auto"
}
