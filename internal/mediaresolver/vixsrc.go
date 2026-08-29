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
	"strings"
	"time"
)

// Direct vixsrc resolution, reverse-engineered with cmd/vixprobe (headless
// Chrome capture + Go replay). The chain needs no browser:
//
//  1. GET /api/movie/{id} (or /api/tv/{id}/{s}/{e}) → {"src": "/embed/{pid}?token=…&expires=…"}
//
//  2. GET the embed page; its HTML contains
//     window.masterPlaylist = { params: {token, expires, asn}, url: "https://vixsrc.to/playlist/{pid}" }
//
//  3. GET {url}?token=…&expires=…[&asn=…]&h=1&lang=en with a browser-like
//     header set (UA + sec-ch-ua client hints + Referer: the embed URL) — this
//     serves the master playlist to plain HTTP clients. No cookies and no
//     special TLS are involved.
//
//     IMPORTANT: do not append b=1. The endpoint answers 403 to any playlist
//     request carrying it — the site's own player never sends it (verified by
//     capturing the live player's request and replaying both variants: b=1 →
//     403, without → 200). It looks like a deliberate bot tripwire.
//
// The resulting session forwards those same headers on every upstream fetch so
// variant playlists, audio/subtitle renditions and CDN segments all pass the
// endpoint's checks.


// vixsrcMasterBlockCap bounds how much HTML after the window.masterPlaylist
// marker is searched for the params/url literals.
const vixsrcMasterBlockCap = 800

// vixsrcEmbedHTMLCap bounds how much of the embed page is read.
const vixsrcEmbedHTMLCap = 1 << 19

var (
	// vixsrcParamRE matches single- or double-quoted key-value pairs inside the
	// window.masterPlaylist block.
	vixsrcParamRE = regexp.MustCompile(`(?i)['"]?(\w+)['"]?\s*:\s*['"]([^'"]*)['"]`)
	// vixsrcURLRE matches the url: '…' or "…" literal of window.masterPlaylist.
	vixsrcURLRE = regexp.MustCompile(`(?i)['"]?url['"]?\s*:\s*['"]([^'"]+)['"]`)
)

func applyVixsrcClientHints(h http.Header) {
	h.Set("Sec-Ch-Ua", `"Not(A:Brand";v="99", "Google Chrome";v="133", "Chromium";v="133"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"Windows"`)
}

func applyVixsrcRequestHeaders(h http.Header, referer, origin string, isHTML bool) {
	h.Set("User-Agent", defaultUserAgent)
	applyVixsrcClientHints(h)
	h.Set("Accept-Language", "en-US,en;q=0.9")
	if isHTML {
		h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		h.Set("Sec-Fetch-Dest", "iframe")
		h.Set("Sec-Fetch-Mode", "navigate")
		h.Set("Sec-Fetch-Site", "same-origin")
		h.Set("Upgrade-Insecure-Requests", "1")
	} else {
		h.Set("Accept", "*/*")
		h.Set("Sec-Fetch-Dest", "empty")
		h.Set("Sec-Fetch-Mode", "cors")
		h.Set("Sec-Fetch-Site", "same-origin")
	}
	if referer != "" {
		h.Set("Referer", referer)
	}
	if origin != "" {
		h.Set("Origin", origin)
	}
}

// tryVixsrcDirect resolves vixsrc without the browser, registers the proxy
// session and starts the read-ahead warmup. It reports false when the direct
// path cannot produce a stream so Resolve can fall back to the browser scrape.
func (r *Resolver) tryVixsrcDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	vr, err := r.resolveVixsrcDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] vixsrc direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] vixsrc direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	r.rememberResolution(req, newResolutionRecord(vr))
	if vr.MasterText != "" {
		// Admit the freshly fetched master under its canonical URL with a long
		// TTL: it is a VOD manifest whose tokenized rendition URLs outlive the
		// session by weeks. Warmup walks it from RAM immediately.
		r.cache.put(&cacheEntry{
			key:         vr.Source,
			data:        []byte(vr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	log.Printf("[MediaResolver] vixsrc resolved directly source=%s", redactQuery(vr.Source))
	r.startWarmup(token)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// resolveVixsrcDirect walks api → embed → masterPlaylist params and fetches
// the master playlist directly.
func (r *Resolver) resolveVixsrcDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}
	// Use the configured origin (cfg.TargetOrigin) so VIXSRC_ORIGIN overrides work.
	origin := r.cfg.TargetOrigin
	get := func(raw, referer string, limit int64, isHTML bool) ([]byte, int, error) {
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, 0, err
		}
		applyVixsrcRequestHeaders(req2.Header, referer, origin, isHTML)
		resp, err := client.Do(req2)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
		if err != nil {
			return nil, resp.StatusCode, err
		}
		return body, resp.StatusCode, nil
	}

	apiPath := "/api/movie/" + url.PathEscape(req.ID)
	pageRef := origin + "/movie/" + url.PathEscape(req.ID)
	if req.Type == TV {
		apiPath = "/api/tv/" + url.PathEscape(req.ID) + "/" + url.PathEscape(req.Season) + "/" + url.PathEscape(req.Episode)
		pageRef = origin + "/tv/" + url.PathEscape(req.ID) + "/" + url.PathEscape(req.Season) + "/" + url.PathEscape(req.Episode)
	}

	// Step 1: stream API returns the tokenized embed path.
	srcBody, status, err := get(origin+apiPath, origin+"/", 64<<10, false)
	if err != nil {
		return nil, fmt.Errorf("stream API: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("stream API returned status %d", status)
	}
	var srcPayload struct {
		Src string `json:"src"`
	}
	if json.Unmarshal(srcBody, &srcPayload) != nil || strings.TrimSpace(srcPayload.Src) == "" {
		return nil, errors.New("stream API returned no embed src")
	}
	embedURL := origin + srcPayload.Src

	// Step 2: embed page carries the masterPlaylist params.
	html, status, err := get(embedURL, pageRef, vixsrcEmbedHTMLCap, true)
	if err != nil {
		return nil, fmt.Errorf("embed page: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("embed page returned status %d", status)
	}
	res, err := parseVixsrcEmbed(string(html), embedURL, origin)
	if err != nil {
		return nil, err
	}
	mu, err := url.Parse(res.Source)
	if err != nil || r.blockedUpstreamHost(ctx, mu.Hostname()) {
		return nil, fmt.Errorf("master host blocked: %v", err)
	}

	// Step 3: fetch the master playlist with the embed URL as Referer.
	masterHeaders := make(http.Header)
	applyVixsrcRequestHeaders(masterHeaders, embedURL, origin, false)
	text, status, err := get(res.Source, embedURL, maxManifestBytes, false)
	if err != nil {
		return nil, fmt.Errorf("master playlist: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("master playlist returned status %d", status)
	}
	textStr := strings.TrimSpace(string(text))
	if !strings.HasPrefix(textStr, "#EXTM3U") ||
		(!strings.Contains(textStr, "#EXT-X-STREAM-INF") && !strings.Contains(textStr, "#EXTINF")) {
		return nil, errors.New("master playlist is not a valid HLS payload")
	}
	res.MasterText = textStr
	res.Headers = masterHeaders
	return res, nil
}

// parseVixsrcEmbed extracts the master playlist URL from an embed page's
// window.masterPlaylist block and appends the required query parameters.
// origin is the configured VixSrc origin used to validate the parsed URL host.
func parseVixsrcEmbed(html, embedURL, origin string) (*directResolution, error) {
	i := strings.Index(html, "window.masterPlaylist")
	if i < 0 {
		return nil, errors.New("embed page has no masterPlaylist block")
	}
	block := html[i:]
	if len(block) > vixsrcMasterBlockCap {
		block = block[:vixsrcMasterBlockCap]
	}
	params := map[string]string{}
	for _, m := range vixsrcParamRE.FindAllStringSubmatch(block, -1) {
		params[m[1]] = m[2]
	}
	mURL := vixsrcURLRE.FindStringSubmatch(block)
	token, expires := params["token"], params["expires"]
	if mURL == nil || token == "" || expires == "" {
		return nil, errors.New("masterPlaylist block missing url/token/expires")
	}
	u, err := url.Parse(mURL[1])
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("masterPlaylist url %q invalid", redactQuery(mURL[1]))
	}
	// Validate the master playlist host against the configured origin and known
	// VixSrc domains so a page served from an alternate domain can't redirect us elsewhere.
	originHost := ""
	if ou, oe := url.Parse(origin); oe == nil {
		originHost = strings.ToLower(ou.Host)
	}
	hostLower := strings.ToLower(u.Host)
	isAllowedHost := false
	if originHost != "" && (hostLower == originHost || strings.HasSuffix(hostLower, "."+originHost)) {
		isAllowedHost = true
	}
	for _, d := range []string{"vixsrc.to", "vixcloud.co", "vix-content.net"} {
		if hostLower == d || strings.HasSuffix(hostLower, "."+d) {
			isAllowedHost = true
			break
		}
	}
	if !isAllowedHost {
		return nil, fmt.Errorf("masterPlaylist host %q outside configured origin %q", u.Host, originHost)
	}
	q := u.Query()
	q.Set("token", token)
	q.Set("expires", expires)
	if asn := params["asn"]; asn != "" {
		q.Set("asn", asn)
	}
	// Keep existing query parameters (like b=1 when present in mURL) and append required ones.
	q.Set("h", "1")
	q.Set("lang", "en")
	u.RawQuery = q.Encode()

	allowed := map[string]bool{
		hostLower:         true,
		"vixsrc.to":       true,
		"vixcloud.co":     true,
		"vix-content.net": true,
	}
	if originHost != "" {
		allowed[originHost] = true
	}
	return &directResolution{
		Source:  u.String(),
		Allowed: allowed,
	}, nil
}
