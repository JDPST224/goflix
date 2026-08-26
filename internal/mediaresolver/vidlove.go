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

// vidloveServers mirrors the player's source order. Each key is tried in turn;
// the first one that yields a validated stream wins.
var vidloveServers = []string{"warden", "cinefreak", "moviebox2", "ipcloud", "tcloud", "vidapi", "moviebox"}

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
	token, err := r.newSession(vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] vidlove direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
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

// resolveVidloveDirect walks the server list, returning the first stream that
// validates. Candidates are probed best-first: the primary URL, then any
// per-quality URLs the API lists (HEVC-only tiers are skipped).
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
	for _, key := range vidloveServers {
		if ctx.Err() != nil {
			break
		}
		apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, vidloveAPIBase+path+"&sources="+key, nil)
		if err != nil {
			lastErr = err
			continue
		}
		for _, k := range []string{"User-Agent", "Referer", "Accept"} {
			if v := headers.Get(k); v != "" {
				apiReq.Header.Set(k, v)
			}
		}
		resp, err := client.Do(apiReq)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, vidloveResponseCap))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("server %q returned status %d", key, resp.StatusCode)
			continue
		}
		var parsed vidloveAPIResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = fmt.Errorf("server %q returned invalid JSON: %w", key, err)
			continue
		}
		if parsed.Source == nil || parsed.Source.URL == nil || strings.TrimSpace(*parsed.Source.URL) == "" {
			lastErr = fmt.Errorf("server %q returned no source URL", key)
			continue
		}
		res, err := r.finishVidloveSource(ctx, *parsed.Source, headers)
		if err != nil {
			lastErr = err
			log.Printf("[MediaResolver] vidlove server %q unusable: %v", key, err)
			continue
		}
		log.Printf("[MediaResolver] vidlove resolved via server %q source=%s inline_master=%v qualities=%d",
			key, redactQuery(res.Source), res.MasterText != "", len(parsed.Source.Qualities))
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no vidlove server attempted")
	}
	return nil, lastErr
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
			return &directResolution{Source: primary, Headers: cloneHeader(baseHeaders), Allowed: allowed, MasterText: text}, nil
		}
	}

	// No usable inline manifest: probe candidates, preferring masters so a
	// multi-tier listing never collapses to whichever tier answered first.
	var fallback *directResolution
	for _, cand := range candidates {
		cu, err := url.Parse(cand)
		if err != nil || cu.Scheme != "https" && cu.Scheme != "http" || cu.Host == "" {
			continue
		}
		if r.blockedUpstreamHost(ctx, cu.Hostname()) {
			continue
		}
		ok, master := r.probeManifestClass(ctx, cand, baseHeaders)
		if !ok {
			continue
		}
		res := &directResolution{Source: cand, Headers: cloneHeader(baseHeaders), Allowed: map[string]bool{strings.ToLower(cu.Host): true}}
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
