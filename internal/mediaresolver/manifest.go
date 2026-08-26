package mediaresolver

// HLS manifest rewriting: proxy-prefix injection and quality forcing.

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var uriAttrRE = regexp.MustCompile(`URI="([^"]+)"`)

func (r *Resolver) rewriteManifest(text string, base *url.URL, token string, req *http.Request) (string, map[string]bool) {
	discovered := make(map[string]bool)

	// Clean root-relative proxy prefix: works reliably under HTTP, HTTPS, localhost, tunnels, and custom ports.
	proxyPrefix := "/api/media/proxy/" + token + ".m3u8?url="

	proxyURL := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "/api/media/proxy/") {
			return raw
		}
		u := resolveMediaURL(base, raw)
		if u == "" {
			return raw
		}
		if pu, err := url.Parse(u); err == nil && pu.Host != "" {
			discovered[strings.ToLower(pu.Host)] = true
		}
		return proxyPrefix + url.QueryEscape(u)
	}

	// All video variants are kept so the player's own ABR can move between
	// tiers: collapsing the manifest to the top variant left no headroom, and
	// a connection that could not sustain the top bitrate had nowhere to fall
	// back to (constant buffering). Warmup still pre-warms the top tier.
	// NOTE: forceHighestQuality is deliberately NOT applied here.

	// Rewrite URI="..." attributes (EXT-X-MEDIA, EXT-X-KEY, EXT-X-MAP, etc.).
	text = uriAttrRE.ReplaceAllStringFunc(text, func(m string) string {
		parts := uriAttrRE.FindStringSubmatch(m)
		if len(parts) != 2 {
			return m
		}
		return `URI="` + proxyURL(parts[1]) + `"`
	})

	// Rewrite bare playlist/segment URI lines as well.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/api/media/proxy/") {
			continue
		}
		if strings.Contains(trimmed, " ") {
			continue
		}
		if u := resolveMediaURL(base, trimmed); u != "" {
			lines[i] = proxyURL(trimmed)
		}
	}
	out := strings.Join(lines, "\n")
	// External subtitle renditions registered on this session are injected
	// AFTER upstream URI rewriting: their URIs are served by this process and
	// must not be routed through the proxy pipeline.
	if subs := r.subRenditions(token); len(subs) > 0 {
		switch {
		case strings.Contains(out, "#EXT-X-STREAM-INF"):
			// Master playlist: declare the group and link every variant.
			out = injectSubRenditions(out, subs)
		case req.URL.Query().Get("url") == "" && !strings.Contains(out, "TYPE=SUBTITLES"):
			// Session root serving a bare media playlist (VidKing hands out
			// single-variant playlists with no master): wrap it so there is
			// somewhere to reference the subtitles group. The variant points
			// back through ?url=, which serves the media playlist normally.
			out = r.synthMediaMaster(token, subs)
		}
	}
	return out, discovered
}

// bwRE extracts the BANDWIDTH value from an EXT-X-STREAM-INF line.
var bwRE = regexp.MustCompile(`BANDWIDTH=(\d+)`)

// resRE extracts the RESOLUTION value (e.g. 3840x2160, 1920x1080) from an EXT-X-STREAM-INF line.
var resRE = regexp.MustCompile(`RESOLUTION=(\d+)x(\d+)`)

// forceHighestQuality rewrites a master HLS playlist so that only the
// highest-quality video variant (4K UHD, 1080p Full HD, or highest bitrate) is retained.
// All metadata tags independent of the chosen variant (EXT-X-MEDIA audio/subtitles,
// EXT-X-SESSION-KEY, EXT-X-SESSION-DATA, EXT-X-I-FRAME-STREAM-INF) are preserved.
func forceHighestQuality(manifest string) string {
	if !strings.Contains(manifest, "#EXT-X-STREAM-INF") {
		return manifest
	}

	// Pass 1: find the highest-quality variant by resolution (pixels), with bandwidth
	// as a tiebreaker.
	maxBandwidth := -1
	maxPixels := -1
	var bestVariantLines []string

	lines := strings.Split(manifest, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			continue
		}
		bw := 0
		if m := bwRE.FindStringSubmatch(trimmed); len(m) == 2 {
			bw, _ = strconv.Atoi(m[1])
		}
		pixels := 0
		if m := resRE.FindStringSubmatch(trimmed); len(m) == 3 {
			w, _ := strconv.Atoi(m[1])
			h, _ := strconv.Atoi(m[2])
			pixels = w * h
		}

		uriLine := ""
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			uriLine = lines[j]
			i = j
			break
		}

		// Skip variants with no URI line — emitting a dangling
		// #EXT-X-STREAM-INF would produce an unparseable manifest.
		if uriLine == "" {
			continue
		}

		// If no explicit RESOLUTION attribute was found, infer resolution from 4K/2K/1080p tags.
		if pixels == 0 {
			combined := strings.ToUpper(trimmed + " " + uriLine)
			if strings.Contains(combined, "4K") || strings.Contains(combined, "2160") || strings.Contains(combined, "UHD") {
				pixels = 3840 * 2160
			} else if strings.Contains(combined, "1440") || strings.Contains(combined, "2K") || strings.Contains(combined, "QHD") {
				pixels = 2560 * 1440
			} else if strings.Contains(combined, "1080") || strings.Contains(combined, "FHD") {
				pixels = 1920 * 1080
			} else if strings.Contains(combined, "720") || strings.Contains(combined, "HD") {
				pixels = 1280 * 720
			}
		}

		// Choose this variant if it has strictly higher pixel count (e.g. 4K > 1080p),
		// or equal pixel count with higher bandwidth.
		better := pixels > maxPixels || (pixels == maxPixels && bw > maxBandwidth)
		if better {
			maxBandwidth = bw
			maxPixels = pixels
			bestVariantLines = []string{trimmed, uriLine}
		}
	}

	// Pass 2: rebuild the manifest keeping all non-variant tags intact.
	var out []string
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if t == "" || strings.HasPrefix(t, "#") {
					continue
				}
				i = j
				break
			}
			continue
		}
		out = append(out, lines[i])
	}

	// Append the single highest quality variant.
	if len(bestVariantLines) > 0 {
		out = append(out, bestVariantLines[0])
		if bestVariantLines[1] != "" {
			out = append(out, bestVariantLines[1])
		}
	}
	return strings.Join(out, "\n")
}

func resolveMediaURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	if u.Host == "" {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.String()
}
