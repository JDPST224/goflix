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
	"strings"
	"time"
)

// vidloveAPIBase is the JSON API the vidlove web player itself calls (its
// serverConfigs bundle hardcodes this host). Resolving against it directly
// avoids the headless browser entirely — and matters for quality, because the
// embed hands its player the master manifest inline, so scraping the page's
// network traffic can never observe a master playlist and playback gets pinned
// to whichever low-tier variant the embed's own hls.js started on.
const vidloveAPIBase = "https://api.shows.st"

// vidloveServers lists the upstream sources in priority order. Active servers
// (vidapi, moviebox) are tried first to avoid multi-second sequential round-trips
// against defunct sub-sources.
var vidloveServers = []string{"vidapi", "moviebox", "warden", "cinefreak", "moviebox2", "ipcloud", "tcloud"}

// vidloveRaceServers is how many priority servers are probed in parallel
// before falling back to a sequential walk of the remainder. A dead first
// server would otherwise add its full HTTP timeout to every vidlove launch.
const vidloveRaceServers = 3

// vidloveResponseCap bounds how much of an API response is read. Responses
// embed the full TMDB metadata blob, so they are far larger than the source
// object we actually need.
const vidloveResponseCap = 1 << 20

// directResolution is a stream resolved without the browser, ready to become
// a proxy session.
type directResolution struct {
	Source  string
	Headers http.Header
	Allowed map[string]bool
	// MasterText is the full master playlist text when it is already known
	// (inline in an API response, or fetched during resolution). It seeds the
	// body cache so warmup and the proxy fast path serve the master without a
	// redundant upstream fetch.
	MasterText string
	// NoRevalidate marks sources whose manifest cannot be meaningfully
	// re-fetched later — e.g. vidlove's master arrives inline from a dynamic
	// API endpoint, and the bare source URL serves no playlist. Resolution
	// cache records built from these trust the stored manifest until their
	// TTL instead of background-revalidating (which always failed and
	// triggered pointless re-resolves).
	NoRevalidate bool
}

type vidloveSource struct {
	URL       *string          `json:"url"`
	Manifest  *string          `json:"manifest"`
	Qualities []vidloveQuality `json:"qualities"`
}

type vidloveQuality struct {
	Quality string `json:"quality"`
	Codec   string `json:"codec"`
	URL     string `json:"url"`
}

type vidloveAPIResponse struct {
	Source *vidloveSource `json:"source"`
}

// tryVidloveDirect resolves vidlove against its JSON API, registers the proxy
// session and starts the read-ahead warmup. It reports false when the direct
// path cannot produce a stream so Resolve can fall back to the browser scrape.
func (r *Resolver) tryVidloveDirect(parent context.Context, req MediaRequest) (string, bool) {
	vr, err := r.resolveVidloveDirect(parent, req)
	if err != nil {
		log.Printf("[MediaResolver] vidlove direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] vidlove direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	r.rememberResolution(req, newResolutionRecord(vr))
	if vr.MasterText != "" {
		// Admit the master under its canonical URL with a long TTL: this is a
		// VOD master whose variant list never changes. Warmup and the proxy
		// fast path both hit this entry instead of refetching the opaque
		// /api?d= endpoint (which may not even serve the master when re-fetched).
		r.cache.put(&cacheEntry{
			key:         vr.Source,
			data:        []byte(vr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	r.attachAndWarm(token, req)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// resolveVidloveDirect resolves vidlove against its JSON API. The first
// vidloveRaceServers servers are probed in parallel and the first success
// wins; the remaining servers are walked sequentially only if every raced
// probe failed. All results carry the same headers.
func (r *Resolver) resolveVidloveDirect(parent context.Context, req MediaRequest) (*directResolution, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", r.cfg.VidLoveOrigin+"/")
	headers.Set("Accept", "application/json")

	path := "/movie?id=" + url.QueryEscape(req.ID) + "&mode=json"
	if req.Type == TV {
		path = "/tv?id=" + url.QueryEscape(req.ID) +
			"&season=" + url.QueryEscape(req.Season) +
			"&episode=" + url.QueryEscape(req.Episode) + "&mode=json"
	}

	client := &http.Client{Transport: r.transport, Timeout: 10 * time.Second}
	var lastErr error

	raceCount := vidloveRaceServers
	if raceCount > len(vidloveServers) {
		raceCount = len(vidloveServers)
	}
	type serverResult struct {
		res *directResolution
		err error
	}
	// Buffered so losing goroutines can always deliver their result and
	// exit, even after we return early on a winner.
	results := make(chan serverResult, raceCount)
	for i := 0; i < raceCount; i++ {
		go func(key string) {
			res, err := r.resolveVidloveServer(ctx, client, headers, path, key)
			results <- serverResult{res: res, err: err}
		}(vidloveServers[i])
	}
	var winner *directResolution
	for i := 0; i < raceCount && winner == nil; i++ {
		sr := <-results
		if sr.res != nil {
			winner = sr.res
			break
		}
		if sr.err != nil {
			lastErr = sr.err
		}
	}
	if winner == nil {
		// Every raced server failed — walk the remaining list sequentially.
		for _, key := range vidloveServers[raceCount:] {
			if ctx.Err() != nil {
				break
			}
			res, err := r.resolveVidloveServer(ctx, client, headers, path, key)
			if err != nil {
				lastErr = err
				continue
			}
			winner = res
			break
		}
	}
	if winner != nil {
		return winner, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no vidlove server attempted")
	}
	return nil, lastErr
}

// resolveVidloveServer queries one vidlove server's API and validates its
// source payload, logging the outcome (success and failure) with the server
// key. Shared by the parallel race and the sequential fallback.
func (r *Resolver) resolveVidloveServer(ctx context.Context, client *http.Client, headers http.Header, path, key string) (*directResolution, error) {
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, vidloveAPIBase+path+"&sources="+key, nil)
	if err != nil {
		return nil, err
	}
	for _, k := range []string{"User-Agent", "Referer", "Accept"} {
		if v := headers.Get(k); v != "" {
			apiReq.Header.Set(k, v)
		}
	}
	resp, err := client.Do(apiReq)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, vidloveResponseCap))
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server %q returned status %d", key, resp.StatusCode)
	}
	var parsed vidloveAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("server %q returned invalid JSON: %w", key, err)
	}
	if parsed.Source == nil || parsed.Source.URL == nil || strings.TrimSpace(*parsed.Source.URL) == "" {
		return nil, fmt.Errorf("server %q returned no source URL", key)
	}
	res, err := r.finishVidloveSource(ctx, *parsed.Source, headers)
	if err != nil {
		log.Printf("[MediaResolver] vidlove server %q unusable: %v", key, err)
		return nil, err
	}
	log.Printf("[MediaResolver] vidlove resolved via server %q source=%s inline_master=%v qualities=%d",
		key, redactQuery(res.Source), res.MasterText != "", len(parsed.Source.Qualities))
	return res, nil
}

// finishVidloveSource validates one API source payload and turns it into a
// resolution. An inline manifest wins immediately (it is the authoritative
// master); otherwise candidate URLs are probed until one serves a playlist,
// preferring masters over single-tier media playlists.
func (r *Resolver) finishVidloveSource(ctx context.Context, src vidloveSource, baseHeaders http.Header) (*directResolution, error) {
	primary := strings.TrimSpace(*src.URL)
	u, err := url.Parse(primary)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("source URL is not a valid http(s) URL")
	}
	if r.blockedUpstreamHost(ctx, u.Hostname()) {
		return nil, fmt.Errorf("source host %q blocked", u.Hostname())
	}

	// Candidate URLs: primary first, then per-quality tiers in listed order.
	candidates := []string{primary}
	seen := map[string]bool{primary: true}
	for _, q := range src.Qualities {
		qurl := strings.TrimSpace(q.URL)
		if qurl == "" || seen[qurl] {
			continue
		}
		// Skip HEVC-only tiers: playback targets broadly compatible h264.
		if strings.EqualFold(strings.TrimSpace(q.Codec), "hevc") {
			continue
		}
		seen[qurl] = true
		candidates = append(candidates, qurl)
	}

	allowed := map[string]bool{strings.ToLower(u.Host): true}

	// An inline master manifest is authoritative — use it as-is.
	if src.Manifest != nil {
		text := strings.TrimSpace(*src.Manifest)
		if strings.HasPrefix(text, "#EXTM3U") &&
			(strings.Contains(text, "#EXT-X-STREAM-INF") || strings.Contains(text, "#EXTINF")) {
			return &directResolution{Source: primary, Headers: cloneHeader(baseHeaders), Allowed: allowed, MasterText: text, NoRevalidate: true}, nil
		}
	}

	// No usable inline manifest: probe candidates, preferring masters so a
	// multi-tier listing never collapses to whichever tier answered first.
	var fallback *directResolution
	for _, cand := range candidates {
		cu, err := url.Parse(cand)
		if err != nil || (cu.Scheme != "https" && cu.Scheme != "http") || cu.Host == "" {
			continue
		}
		if r.blockedUpstreamHost(ctx, cu.Hostname()) {
			continue
		}
		ok, master := r.probeManifestClass(ctx, cand, baseHeaders)
		if !ok {
			continue
		}
		// Vidlove playlist URLs embed single-use API tokens — they may not
		// re-serve later, so these records never revalidate either.
		res := &directResolution{Source: cand, Headers: cloneHeader(baseHeaders), Allowed: map[string]bool{strings.ToLower(cu.Host): true}, NoRevalidate: true}
		if master {
			return res, nil
		}
		if fallback == nil {
			fallback = res
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("no candidate URL served a valid HLS playlist")
}
