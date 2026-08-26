package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"goflix/internal/subtitles"
)

// subtitlesVidkingHandler resolves subtitles for the VidKing server.
// It searches OpenSubtitles using TMDB ID directly, with automatic multi-query and cross-provider fallback.
func (d Deps) subtitlesVidkingHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	season := strings.TrimSpace(r.URL.Query().Get("season"))
	episode := strings.TrimSpace(r.URL.Query().Get("episode"))

	if !ValidMediaID(id) || (mediaType != "movie" && mediaType != "tv") {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}
	if mediaType == "tv" && (!ValidMediaID(season) || !ValidMediaID(episode)) {
		writeError(w, http.StatusBadRequest, "season and episode are required for TV")
		return
	}

	// Bound the whole fallback ladder: it can issue up to six sequential
	// remote calls, which at worst stacked ~70s of client timeouts onto a
	// single request before responding.
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	// Every TV lookup stays episode-scoped end to end: accepting a show-level
	// result first can serve another episode's file, which is worse than no
	// subtitles at all. Movies have no season/episode to scope by.
	querySeason, queryEpisode := "", ""
	if mediaType == "tv" {
		querySeason, queryEpisode = season, episode
	}

	var subs []subtitles.FrontendSubtitle

	// 1. OpenSubtitles needs an IMDb ID. If we have credentials, resolve it and fetch.
	if d.Client.HasCredentials() {
		if imdbID, err := d.Client.ExternalID(mediaType, id, season, episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	// 2. If still empty, fallback to Vidlove API
	if len(subs) == 0 {
		subs = subtitles.FetchVidloveSubtitles(ctx, mediaType, id, season, episode)
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidking returned %d subtitles for id=%s", len(subs), id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVidloveHandler resolves subtitles for the Vidlove server with OpenSubtitles fallback.
//
//	GET /api/subtitles/vidlove?type=movie&id=<tmdbId>
//	GET /api/subtitles/vidlove?type=tv&id=<tmdbId>&season=<S>&episode=<E>
func (d Deps) subtitlesVidloveHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	season := strings.TrimSpace(r.URL.Query().Get("season"))
	episode := strings.TrimSpace(r.URL.Query().Get("episode"))

	if !ValidMediaID(id) || (mediaType != "movie" && mediaType != "tv") {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}
	if mediaType == "tv" && (!ValidMediaID(season) || !ValidMediaID(episode)) {
		writeError(w, http.StatusBadRequest, "season and episode are required for TV")
		return
	}

	// Bound the two-provider fallback ladder: Vidlove + OpenSubtitles each carry
	// their own 10s client timeout; without an aggregate cap a slow pair
	// could stall this request for up to 20s.
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	subs := subtitles.FetchVidloveSubtitles(ctx, mediaType, id, season, episode)
	if len(subs) == 0 && d.Client.HasCredentials() {
		// Fallback to OpenSubtitles using IMDb ID
		if imdbID, err := d.Client.ExternalID(mediaType, id, season, episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, season, episode)
		}
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidlove returned %d subtitles for id=%s", len(subs), id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVidsrcmeHandler resolves subtitles for the VidSrcMe server with OpenSubtitles fallback.
//
//	GET /api/subtitles/vidsrcme?type=movie&id=<tmdbId>
//	GET /api/subtitles/vidsrcme?type=tv&id=<tmdbId>&season=<S>&episode=<E>
func (d Deps) subtitlesVidsrcmeHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	season := strings.TrimSpace(r.URL.Query().Get("season"))
	episode := strings.TrimSpace(r.URL.Query().Get("episode"))

	if !ValidMediaID(id) || (mediaType != "movie" && mediaType != "tv") {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}
	if mediaType == "tv" && (!ValidMediaID(season) || !ValidMediaID(episode)) {
		writeError(w, http.StatusBadRequest, "season and episode are required for TV")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	subs, imdbID := subtitles.FetchVidsrcmeSubtitlesWithIMDb(ctx, mediaType, id, season, episode)
	if len(subs) == 0 {
		if imdbID == "" && d.Client.HasCredentials() {
			imdbID, _ = d.Client.ExternalID(mediaType, id, season, episode)
		}
		if imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, season, episode)
		}
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] VidSrcMe returned %d subtitles for id=%s", len(subs), id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesOpenSubtitlesDownloadHandler proxies a subtitle download from OpenSubtitles and
// converts SRT to WebVTT so the browser can use it as a native track source.
//
//	GET /api/subtitles/opensubtitles/download?url=<encoded-file-url>
func (d Deps) subtitlesOpenSubtitlesDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}

	content, status, msg := d.fetchOpenSubtitlesSubtitleVTT(r.Context(), targetURL)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeSubtitleVTT(w, content)
}

// subtitleUpstreamUA is the browser-like User-Agent both subtitle providers
// expect; requests without it come back empty or refused.
const subtitleUpstreamUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// maxSubtitleBytes caps how much of an upstream subtitle file gets buffered
// before conversion. Subtitle text almost never reaches two megabytes;
// anything larger is either an HTML error page or an endless stream, and
// neither is worth converting or shipping to the player.
const maxSubtitleBytes = 2 << 20

// readBounded reads r until EOF, reporting maxed=true — and no body — once
// the payload exceeds maxSubtitleBytes, instead of buffering it all.
func readBounded(r io.Reader) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxSubtitleBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > maxSubtitleBytes {
		return nil, true, nil
	}
	return body, false, nil
}

// fetchOpenSubtitlesSubtitleVTT downloads a raw OpenSubtitles subtitle file by its absolute
// upstream URL and converts it to WebVTT.
func (d Deps) fetchOpenSubtitlesSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	u, err := url.Parse(targetURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	req.Header.Set("User-Agent", subtitleUpstreamUA)

	res, err := subtitles.Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] OpenSubtitles download error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] OpenSubtitles download %s://%s%s: upstream status %d", u.Scheme, u.Host, u.Path, res.StatusCode)
		return "", http.StatusBadGateway, "Subtitle not found"
	}

	body, maxed, err := readBounded(res.Body)
	if err != nil {
		log.Printf("[Subtitles] OpenSubtitles download read error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Failed to read subtitle content"
	}
	if maxed {
		log.Printf("[Subtitles] OpenSubtitles download %s://%s%s exceeded %d bytes", u.Scheme, u.Host, u.Path, maxSubtitleBytes)
		return "", http.StatusBadGateway, "Subtitle file too large"
	}

	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			log.Printf("[Subtitles] OpenSubtitles gzip reader error: %v", err)
			return "", http.StatusBadGateway, "Subtitle decompression failed"
		}
		unzipped, err := io.ReadAll(gr)
		gr.Close()
		if err != nil {
			log.Printf("[Subtitles] OpenSubtitles gzip read error: %v", err)
			return "", http.StatusBadGateway, "Subtitle decompression failed"
		}
		body = unzipped
	}

	return subtitles.SrtToWebVTT(string(body)), http.StatusOK, ""
}

// writeSubtitleVTT serves converted WebVTT with the headers track elements
// and players expect.
func writeSubtitleVTT(w http.ResponseWriter, content string) {
	h := w.Header()
	h.Set("Content-Type", "text/vtt; charset=utf-8")
	h.Set("Cache-Control", "max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, content)
}

// injectTimestampMap adds the HLS timestamp mapping native players need to
// align WebVTT cues served as HLS segments against the media timeline. VTTs
// that already carry the header, and non-VTT payloads, pass through untouched.
func injectTimestampMap(vtt string, pts uint64) string {
	if !strings.HasPrefix(vtt, "WEBVTT") || strings.Contains(vtt, "X-TIMESTAMP-MAP") {
		return vtt
	}
	line := "X-TIMESTAMP-MAP:LOCAL:00:00:00.000,HLS:" + strconv.FormatUint(pts, 10) + "\n"
	nl := strings.IndexByte(vtt, '\n')
	if nl < 0 {
		return vtt + "\n" + line
	}
	return vtt[:nl+1] + line + vtt[nl+1:]
}

// fetchVidloveSubtitleVTT downloads a Vidlove subtitle file by its absolute
// upstream URL — the value the search results encode into the download link's
// query string — and converts it to WebVTT.
func (d Deps) fetchVidloveSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	u, err := url.Parse(targetURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	// Match the registrable domain exactly (or as a subdomain of it) on
	// Hostname(), which strips any port: a bare suffix check would admit
	// attacker-registered lookalikes such as "myvdrk.site" while wrongly
	// rejecting an explicit port like "vdrk.site:8443". Without this gate the
	// download endpoint would proxy GETs to any http(s) host.
	host := strings.ToLower(u.Hostname())
	allowedHost := false
	for _, dom := range []string{"vdrk.site", "shows.st", "vidlove.cc", "videasy.to"} {
		if host == dom || strings.HasSuffix(host, "."+dom) {
			allowedHost = true
			break
		}
	}
	if !allowedHost {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	req.Header.Set("Referer", "https://player.vidlove.cc/")
	req.Header.Set("User-Agent", subtitleUpstreamUA)

	res, err := subtitles.Client.Do(req)
	if err != nil {
		// Log scheme://host/path only — the same query redaction the proxy
		// applies, since the URL came in over the wire from the browser.
		log.Printf("[Subtitles] Vidlove download error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] Vidlove download %s://%s%s: upstream status %d", u.Scheme, u.Host, u.Path, res.StatusCode)
		return "", http.StatusBadGateway, "Subtitle not found"
	}

	body, maxed, err := readBounded(res.Body)
	if err != nil {
		log.Printf("[Subtitles] Vidlove download read error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Failed to read subtitle content"
	}
	if maxed {
		log.Printf("[Subtitles] Vidlove download %s://%s%s exceeded %d bytes", u.Scheme, u.Host, u.Path, maxSubtitleBytes)
		return "", http.StatusBadGateway, "Subtitle file too large"
	}
	return subtitles.SrtToWebVTT(string(body)), http.StatusOK, ""
}

// subtitlesVidloveDownloadHandler proxies a Vidlove subtitle download and
// converts SRT to WebVTT so the browser can use it as a native track source.
//
//	GET /api/subtitles/vidlove/download?url=<encoded-file-url>
func (d Deps) subtitlesVidloveDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}

	content, status, msg := d.fetchVidloveSubtitleVTT(r.Context(), targetURL)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeSubtitleVTT(w, content)
}

// fetchVidsrcmeSubtitleVTT downloads a raw VidSrcMe subtitle file and converts it to WebVTT.
func (d Deps) fetchVidsrcmeSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	u, err := url.Parse(targetURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	host := strings.ToLower(u.Hostname())
	allowedHost := false
	for _, dom := range []string{"vidapi.cloud", "vidsrcme.ru", "vidsrcme.xyz", "cloudorchestranova.com"} {
		if host == dom || strings.HasSuffix(host, "."+dom) {
			allowedHost = true
			break
		}
	}
	if !allowedHost {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	req.Header.Set("Referer", "https://cloudorchestranova.com/")
	req.Header.Set("User-Agent", subtitleUpstreamUA)

	res, err := subtitles.Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] VidSrcMe download error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] VidSrcMe download %s://%s%s: upstream status %d", u.Scheme, u.Host, u.Path, res.StatusCode)
		return "", http.StatusBadGateway, "Subtitle not found"
	}

	body, maxed, err := readBounded(res.Body)
	if err != nil {
		log.Printf("[Subtitles] VidSrcMe download read error %s://%s%s: %v", u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Failed to read subtitle content"
	}
	if maxed {
		log.Printf("[Subtitles] VidSrcMe download %s://%s%s exceeded %d bytes", u.Scheme, u.Host, u.Path, maxSubtitleBytes)
		return "", http.StatusBadGateway, "Subtitle file too large"
	}
	return subtitles.SrtToWebVTT(string(body)), http.StatusOK, ""
}

// subtitlesVidsrcmeDownloadHandler proxies a VidSrcMe subtitle download and
// converts SRT to WebVTT so the browser can use it as a native track source.
//
//	GET /api/subtitles/vidsrcme/download?url=<encoded-file-url>
func (d Deps) subtitlesVidsrcmeDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}

	content, status, msg := d.fetchVidsrcmeSubtitleVTT(r.Context(), targetURL)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeSubtitleVTT(w, content)
}

// subtitlesWrapVTTHandler serves a local WebVTT endpoint's content with the
// X-TIMESTAMP-MAP header injected. Native HLS players require that mapping to
// align WebVTT cue timestamps against the MPEG-TS presentation timeline of
// the stream the track accompanies; a bare .vtt served without it renders out
// of sync (or not at all) once muxed into HLS playback. The default mapping
// places cue-time zero at PTS 900000 — the conventional 10-second TS base —
// so cues line up without any player-side offset math.
//
//	GET /api/subtitles/wrap.vtt?src=/api/subtitles/opensubtitles/download?url=<url>[&pts=<uint>]
func (d Deps) subtitlesWrapVTTHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", true) {
		return
	}
	src := r.URL.Query().Get("src")
	if !validLocalSubtitleWrapSrc(src) {
		writeError(w, http.StatusBadRequest, "Invalid subtitle source")
		return
	}

	pts := uint64(900000)
	if raw := r.URL.Query().Get("pts"); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || v == 0 {
			writeError(w, http.StatusBadRequest, "Invalid pts value")
			return
		}
		pts = v
	}

	u, err := url.Parse(src)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid subtitle source")
		return
	}
	var content, msg string
	var status int
	switch {
	case u.Path == "/api/subtitles/vidlove/download":
		targetURL := u.Query().Get("url")
		if targetURL == "" {
			writeError(w, http.StatusBadRequest, "Invalid subtitle source")
			return
		}
		content, status, msg = d.fetchVidloveSubtitleVTT(r.Context(), targetURL)
	case u.Path == "/api/subtitles/vidsrcme/download":
		targetURL := u.Query().Get("url")
		if targetURL == "" {
			writeError(w, http.StatusBadRequest, "Invalid subtitle source")
			return
		}
		content, status, msg = d.fetchVidsrcmeSubtitleVTT(r.Context(), targetURL)
	case u.Path == "/api/subtitles/opensubtitles/download":
		targetURL := u.Query().Get("url")
		if targetURL == "" {
			writeError(w, http.StatusBadRequest, "Invalid subtitle source")
			return
		}
		content, status, msg = d.fetchOpenSubtitlesSubtitleVTT(r.Context(), targetURL)
	default:
		writeError(w, http.StatusBadRequest, "Invalid subtitle source")
		return
	}
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeSubtitleVTT(w, injectTimestampMap(content, pts))
}
