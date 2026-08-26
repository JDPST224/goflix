package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"goflix/internal/catalog"
	"goflix/internal/mediaresolver"
	"goflix/internal/subtitles"
)

// maxSubsRegisterBytes caps the rendition registration body.
const maxSubsRegisterBytes = 16 << 10

// validLocalSubtitlePath admits only same-origin /api/subtitles/ paths —
// never absolute URLs — so registration can't turn the wrap endpoint into an
// open redirector or the manifest into a fetch of third-party content.
func validLocalSubtitlePath(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/api/subtitles/") {
		return false
	}
	if strings.Contains(p, "..") || strings.ContainsAny(p, "\\\"'\r\n\t ") {
		return false
	}
	u, err := url.ParseRequestURI(p)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return false
	}
	return true
}

// validLocalSubtitleWrapSrc restricts the wrap endpoint's src parameter to
// the two download endpoints that actually emit WebVTT, so a wrap playlist
// cannot chain into another wrap or fetch a JSON search endpoint.
func validLocalSubtitleWrapSrc(p string) bool {
	if !validLocalSubtitlePath(p) {
		return false
	}
	u, err := url.ParseRequestURI(p)
	if err != nil {
		return false
	}
	if u.Path == "/api/subtitles/vidlove/download" {
		return true
	}
	if u.Path == "/api/subtitles/vidsrcme/download" {
		return true
	}
	if u.Path == "/api/subtitles/opensubtitles/download" {
		return true
	}
	return false
}

// subsRegisterHandler stores external subtitle renditions on a live proxy
// session so native-HLS engines (smart TVs) get them injected into the
// master manifest as TYPE=SUBTITLES renditions. The backend's own ladder
// (FetchSubRenditions) runs during Resolve; this endpoint only tops up
// whatever it missed.
//
//	POST /api/media/subs/<token>
//	body: {"subtitles":[{"label":"English","language":"en","url":"/api/subtitles/..."}]}
func (d *Deps) subsRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/media/subs/"), "/")
	if token == "" || strings.Contains(token, "/") {
		writeError(w, http.StatusBadRequest, "Invalid proxy token")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSubsRegisterBytes+1))
	if err != nil || len(body) > maxSubsRegisterBytes {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	var payload struct {
		Subtitles []struct {
			Label    string `json:"label"`
			Language string `json:"language"`
			URL      string `json:"url"`
		} `json:"subtitles"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Rendition URIs are root-relative, exactly like the provider manifests
	// TV players already accept — no scheme/host guessing at all.
	renditions := make([]mediaresolver.SubRendition, 0, len(payload.Subtitles))
	for _, s := range payload.Subtitles {
		src := strings.TrimSpace(s.URL)
		if !validLocalSubtitlePath(src) {
			continue
		}
		renditions = append(renditions, mediaresolver.SubRendition{
			Label:    mediaresolver.SanitizeManifestAttr(s.Label),
			Language: mediaresolver.SanitizeManifestAttr(s.Language),
			URI:      "/api/subtitles/wrap.m3u8?src=" + url.QueryEscape(src),
		})
	}

	if err := d.Resolver.SetSubRenditions(token, renditions); err != nil {
		writeError(w, http.StatusNotFound, "Proxy session expired")
		return
	}
	short := token
	if len(short) > 8 {
		short = short[:8]
	}
	log.Printf("[MediaResolver] registered %d subtitle renditions on session %s...", len(renditions), short)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "registered": len(renditions)})
}

// subsWrapHandler serves a single-segment HLS media playlist wrapping a local
// WebVTT endpoint, so external subtitles can be referenced as proper
// TYPE=SUBTITLES renditions by native players (which require a media
// playlist, not a bare .vtt, as the rendition URI).
//
//	GET /api/subtitles/wrap.m3u8?src=/api/subtitles/opensubtitles/download?url=<url>
func (d *Deps) subsWrapHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", true) {
		return
	}
	src := strings.TrimSpace(r.URL.Query().Get("src"))
	if !validLocalSubtitleWrapSrc(src) {
		writeError(w, http.StatusBadRequest, "Invalid subtitle source")
		return
	}

	// Byte-for-byte the vocabulary of the provider child playlists that TV
	// players demonstrably accept (single segment, ALLOW-CACHE, no
	// PLAYLIST-TYPE). The segment URI stays root-relative and resolves
	// against the wrap playlist's own origin — the same trick the provider
	// manifests use — and points at the wrap.vtt endpoint rather than a raw
	// download path, so the served WebVTT carries the X-TIMESTAMP-MAP header
	// native HLS players need to sync cues against the TS timeline. A pts on
	// this request is forwarded so the mapping stays tunable end to end.
	segment := "/api/subtitles/wrap.vtt?src=" + url.QueryEscape(src)
	if pts := strings.TrimSpace(r.URL.Query().Get("pts")); pts != "" {
		segment += "&pts=" + url.QueryEscape(pts)
	}
	body := "#EXTM3U\n" +
		"#EXT-X-VERSION:3\n" +
		"#EXT-X-MEDIA-SEQUENCE:0\n" +
		"#EXT-X-ALLOW-CACHE:YES\n" +
		"#EXT-X-TARGETDURATION:7200\n" +
		"#EXTINF:7200.000,\n" +
		segment + "\n" +
		"#EXT-X-ENDLIST\n"
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// renditionsFromSubtitles maps provider search results to manifest-ready
// renditions. Parallel uploads sharing a language+label are kept as numbered
// versions ("English", "English (2)") rather than collapsed: they usually come
// from different releases and sync differently against our stream. Only an
// identical URL is a true duplicate and gets dropped.
func renditionsFromSubtitles(subs []subtitles.FrontendSubtitle) []mediaresolver.SubRendition {
	out := make([]mediaresolver.SubRendition, 0, len(subs))
	seenLabel := make(map[string]int, len(subs))
	seenURL := make(map[string]bool, len(subs))
	for _, s := range subs {
		src := strings.TrimSpace(s.URL)
		if !validLocalSubtitlePath(src) || seenURL[src] {
			continue
		}
		seenURL[src] = true
		key := strings.ToLower(s.Language) + "__" + strings.ToLower(s.Label)
		label := SanitizeLabel(s.Label)
		n := seenLabel[key]
		if n > 0 {
			label = fmt.Sprintf("%s (%d)", label, n+1)
		}
		seenLabel[key] = n + 1
		out = append(out, mediaresolver.SubRendition{
			Label:    label,
			Language: mediaresolver.SanitizeManifestAttr(s.Language),
			URI:      "/api/subtitles/wrap.m3u8?src=" + url.QueryEscape(src),
		})
	}
	return out
}

// SanitizeLabel returns a manifest-safe rendition label, falling back to a
// neutral name when the provider gave nothing usable.
func SanitizeLabel(raw string) string {
	label := mediaresolver.SanitizeManifestAttr(raw)
	if label == "" {
		label = "Subtitle"
	}
	return label
}

// FetchSubRenditions is the server-side subtitle ladder wired into the media
// resolver: it runs during Resolve() so VidKing/VidLove sessions get their
// external subtitles embedded into the master manifest WITHOUT any frontend
// registration — smart TVs never run our JS.
func FetchSubRenditions(ctx context.Context, client *catalog.Client, req mediaresolver.MediaRequest) []mediaresolver.SubRendition {
	mediaType := string(req.Type)
	id := strings.TrimSpace(req.ID)
	if id == "" || !ValidMediaID(id) {
		return nil
	}
	if req.Type != mediaresolver.Movie && req.Type != mediaresolver.TV {
		return nil
	}
	if req.Type == mediaresolver.TV &&
		(!ValidMediaID(req.Season) || !ValidMediaID(req.Episode)) {
		return nil
	}

	// Every TV lookup stays episode-scoped end to end: accepting a show-level
	// result first can serve another episode's file, which is worse than no
	// subtitles at all. Movies have no season/episode to scope by.
	querySeason, queryEpisode := "", ""
	if req.Type == mediaresolver.TV {
		querySeason, queryEpisode = req.Season, req.Episode
	}

	var subs []subtitles.FrontendSubtitle

	if req.Provider == "vidsrcme" {
		subs = subtitles.FetchVidsrcmeSubtitles(ctx, mediaType, id, req.Season, req.Episode)
	} else if req.Provider == "vidlove" {
		subs = subtitles.FetchVidloveSubtitles(ctx, mediaType, id, req.Season, req.Episode)
	}

	if len(subs) == 0 && client.HasCredentials() {
		if imdbID, err := client.ExternalID(mediaType, id, req.Season, req.Episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	if len(subs) == 0 && req.Provider != "vidlove" && req.Provider != "vidsrcme" {
		subs = subtitles.FetchVidloveSubtitles(ctx, mediaType, id, req.Season, req.Episode)
	}
	return renditionsFromSubtitles(subs)
}
