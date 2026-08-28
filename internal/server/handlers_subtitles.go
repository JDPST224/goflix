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

// Allowed upstream domains for subtitle downloads to prevent SSRF.
var (
	openSubtitlesAllowedDomains = []string{
		"opensubtitles.org",
		"opensubtitles.com",
		"dl.opensubtitles.org",
		"rest.opensubtitles.org",
		"osdb.link",
	}
	vidloveAllowedDomains = []string{
		"vdrk.site",
		"shows.st",
		"vidlove.cc",
		"videasy.to",
	}
	vidsrcmeAllowedDomains = []string{
		"vidapi.cloud",
		"vidsrcme.ru",
		"vidsrcme.xyz",
		"cloudorchestranova.com",
	}
	cinesrcAllowedDomains = []string{
		"subs.bright67.online",
		"bright67.online",
	}
)

type subtitleQueryParams struct {
	mediaType string
	id        string
	season    string
	episode   string
}

func parseSubtitleParams(r *http.Request) (subtitleQueryParams, string, bool) {
	p := subtitleQueryParams{
		mediaType: strings.TrimSpace(r.URL.Query().Get("type")),
		id:        strings.TrimSpace(r.URL.Query().Get("id")),
		season:    strings.TrimSpace(r.URL.Query().Get("season")),
		episode:   strings.TrimSpace(r.URL.Query().Get("episode")),
	}

	if !ValidMediaID(p.id) || (p.mediaType != "movie" && p.mediaType != "tv") {
		return p, "Invalid request parameters", false
	}
	if p.mediaType == "tv" && (!ValidMediaID(p.season) || !ValidMediaID(p.episode)) {
		return p, "season and episode are required for TV", false
	}
	return p, "", true
}

// subtitlesVidkingHandler resolves subtitles for the VidKing server.
// It searches OpenSubtitles using TMDB ID directly, with automatic multi-query and cross-provider fallback.
func (d *Deps) subtitlesVidkingHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	p, errMsg, ok := parseSubtitleParams(r)
	if !ok {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	querySeason, queryEpisode := "", ""
	if p.mediaType == "tv" {
		querySeason, queryEpisode = p.season, p.episode
	}

	var subs []subtitles.FrontendSubtitle

	// 1. OpenSubtitles needs an IMDb ID. If we have credentials, resolve it and fetch.
	if d.Client.HasCredentials() {
		if imdbID, err := d.Client.ExternalID(p.mediaType, p.id, p.season, p.episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	// 2. If still empty, fallback to Vidlove API
	if len(subs) == 0 {
		subs = subtitles.FetchVidloveSubtitles(ctx, p.mediaType, p.id, p.season, p.episode)
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidking returned %d subtitles for id=%s", len(subs), p.id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVidloveHandler resolves subtitles for the Vidlove server with OpenSubtitles fallback.
func (d *Deps) subtitlesVidloveHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	p, errMsg, ok := parseSubtitleParams(r)
	if !ok {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	subs := subtitles.FetchVidloveSubtitles(ctx, p.mediaType, p.id, p.season, p.episode)
	if len(subs) == 0 && d.Client.HasCredentials() {
		// Fallback to OpenSubtitles using IMDb ID (scoped to episode for TV only)
		querySeason, queryEpisode := "", ""
		if p.mediaType == "tv" {
			querySeason, queryEpisode = p.season, p.episode
		}
		if imdbID, err := d.Client.ExternalID(p.mediaType, p.id, p.season, p.episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidlove returned %d subtitles for id=%s", len(subs), p.id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVidsrcmeHandler resolves subtitles for the VidSrcMe server with OpenSubtitles fallback.
func (d *Deps) subtitlesVidsrcmeHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	p, errMsg, ok := parseSubtitleParams(r)
	if !ok {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	subs, imdbID := subtitles.FetchVidsrcmeSubtitlesWithIMDb(ctx, p.mediaType, p.id, p.season, p.episode)
	if len(subs) == 0 {
		if imdbID == "" && d.Client.HasCredentials() {
			imdbID, _ = d.Client.ExternalID(p.mediaType, p.id, p.season, p.episode)
		}
		if imdbID != "" {
			// Movies must not pass season/episode to OpenSubtitles (Bug #5 fix)
			querySeason, queryEpisode := "", ""
			if p.mediaType == "tv" {
				querySeason, queryEpisode = p.season, p.episode
			}
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] VidSrcMe returned %d subtitles for id=%s", len(subs), p.id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
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

// fetchSubtitleVTT validates targetURL against an allowed domain list (preventing SSRF),
// downloads the subtitle file, handles gzip decompression if necessary, and converts
// SRT to standard WebVTT.
func fetchSubtitleVTT(ctx context.Context, targetURL, referer string, allowedDomains []string, logProvider string) (string, int, string) {
	u, err := url.Parse(targetURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", http.StatusBadRequest, "Invalid subtitle URL"
	}

	host := strings.ToLower(u.Hostname())
	allowedHost := false
	for _, dom := range allowedDomains {
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
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("User-Agent", subtitleUpstreamUA)

	res, err := subtitles.Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] %s download error %s://%s%s: %v", logProvider, u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Subtitle download failed"
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] %s download %s://%s%s: upstream status %d", logProvider, u.Scheme, u.Host, u.Path, res.StatusCode)
		return "", http.StatusBadGateway, "Subtitle not found"
	}

	body, maxed, err := readBounded(res.Body)
	if err != nil {
		log.Printf("[Subtitles] %s download read error %s://%s%s: %v", logProvider, u.Scheme, u.Host, u.Path, err)
		return "", http.StatusBadGateway, "Failed to read subtitle content"
	}
	if maxed {
		log.Printf("[Subtitles] %s download %s://%s%s exceeded %d bytes", logProvider, u.Scheme, u.Host, u.Path, maxSubtitleBytes)
		return "", http.StatusBadGateway, "Subtitle file too large"
	}

	// Decompress gzip payload if present
	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			log.Printf("[Subtitles] %s gzip reader error: %v", logProvider, err)
			return "", http.StatusBadGateway, "Subtitle decompression failed"
		}
		unzipped, err := io.ReadAll(gr)
		gr.Close()
		if err != nil {
			log.Printf("[Subtitles] %s gzip read error: %v", logProvider, err)
			return "", http.StatusBadGateway, "Subtitle decompression failed"
		}
		body = unzipped
	}

	return subtitles.SrtToWebVTT(string(body)), http.StatusOK, ""
}

// fetchOpenSubtitlesSubtitleVTT downloads a raw OpenSubtitles subtitle file by its absolute
// upstream URL (guarded against SSRF by openSubtitlesAllowedDomains) and converts it to WebVTT.
func (d *Deps) fetchOpenSubtitlesSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	return fetchSubtitleVTT(ctx, targetURL, "", openSubtitlesAllowedDomains, "OpenSubtitles")
}

// fetchVidloveSubtitleVTT downloads a Vidlove subtitle file by its absolute upstream URL and converts it to WebVTT.
func (d *Deps) fetchVidloveSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	return fetchSubtitleVTT(ctx, targetURL, "https://player.vidlove.cc/", vidloveAllowedDomains, "Vidlove")
}

// fetchVidsrcmeSubtitleVTT downloads a raw VidSrcMe subtitle file and converts it to WebVTT.
func (d *Deps) fetchVidsrcmeSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	return fetchSubtitleVTT(ctx, targetURL, "https://cloudorchestranova.com/", vidsrcmeAllowedDomains, "VidSrcMe")
}

// fetchCinesrcSubtitleVTT downloads a raw CineSrc subtitle file and converts it to WebVTT.
func (d *Deps) fetchCinesrcSubtitleVTT(ctx context.Context, targetURL string) (string, int, string) {
	return fetchSubtitleVTT(ctx, targetURL, "https://cinesrc.st/", cinesrcAllowedDomains, "CineSrc")
}

func (d *Deps) subtitlesCinesrcHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	p, errMsg, ok := parseSubtitleParams(r)
	if !ok {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	subs := subtitles.FetchCinesrcSubtitles(ctx, p.mediaType, p.id, p.season, p.episode)
	if len(subs) == 0 && d.Client.HasCredentials() {
		querySeason, queryEpisode := "", ""
		if p.mediaType == "tv" {
			querySeason, queryEpisode = p.season, p.episode
		}
		if imdbID, err := d.Client.ExternalID(p.mediaType, p.id, p.season, p.episode); err == nil && imdbID != "" {
			subs = subtitles.FetchOpenSubtitles(ctx, imdbID, querySeason, queryEpisode)
		}
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] CineSrc returned %d subtitles for id=%s", len(subs), p.id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

func (d *Deps) subtitlesCinesrcDownloadHandler(w http.ResponseWriter, r *http.Request) {
	d.handleSubtitleDownload(w, r, d.fetchCinesrcSubtitleVTT)
}

func (d *Deps) handleSubtitleDownload(w http.ResponseWriter, r *http.Request, fetchFn func(context.Context, string) (string, int, string)) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "Invalid request parameters")
		return
	}

	content, status, msg := fetchFn(r.Context(), targetURL)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeSubtitleVTT(w, content)
}

func (d *Deps) subtitlesOpenSubtitlesDownloadHandler(w http.ResponseWriter, r *http.Request) {
	d.handleSubtitleDownload(w, r, d.fetchOpenSubtitlesSubtitleVTT)
}

func (d *Deps) subtitlesVidloveDownloadHandler(w http.ResponseWriter, r *http.Request) {
	d.handleSubtitleDownload(w, r, d.fetchVidloveSubtitleVTT)
}

func (d *Deps) subtitlesVidsrcmeDownloadHandler(w http.ResponseWriter, r *http.Request) {
	d.handleSubtitleDownload(w, r, d.fetchVidsrcmeSubtitleVTT)
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
// align WebVTT cues served as HLS segments against the media timeline.
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

// subtitlesWrapVTTHandler serves a local WebVTT endpoint's content with the
// X-TIMESTAMP-MAP header injected. Native HLS players require that mapping to
// align WebVTT cue timestamps against the MPEG-TS presentation timeline of
// the stream the track accompanies.
func (d *Deps) subtitlesWrapVTTHandler(w http.ResponseWriter, r *http.Request) {
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
	case u.Path == "/api/subtitles/cinesrc/download":
		targetURL := u.Query().Get("url")
		if targetURL == "" {
			writeError(w, http.StatusBadRequest, "Invalid subtitle source")
			return
		}
		content, status, msg = d.fetchCinesrcSubtitleVTT(r.Context(), targetURL)
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
