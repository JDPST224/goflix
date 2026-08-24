package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"goflix/internal/subtitles"
)

// subtitlesVidkingHandler resolves subtitles for the VidKing server.
// It searches Videasy using TMDB ID directly, with automatic multi-query and cross-provider fallback.
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

	// 1. Try Videasy with TMDB ID
	subs := subtitles.FetchVideasySubtitles(ctx, id, "", "")

	// 2. If empty and TV, retry with season/episode parameters
	if len(subs) == 0 && mediaType == "tv" {
		subs = subtitles.FetchVideasySubtitles(ctx, id, season, episode)
	}

	// 3. If still empty, try IMDb ID lookup as fallback
	if len(subs) == 0 && d.Client.HasCredentials() {
		if imdbID, err := d.Client.ExternalID(mediaType, id, season, episode); err == nil && imdbID != "" {
			subs = subtitles.FetchVideasySubtitles(ctx, imdbID, "", "")
			if len(subs) == 0 && mediaType == "tv" {
				subs = subtitles.FetchVideasySubtitles(ctx, imdbID, season, episode)
			}
		}
	}

	// 4. If still empty, fallback to Vidlove API
	if len(subs) == 0 {
		subs = subtitles.FetchVidloveSubtitles(ctx, mediaType, id, season, episode)
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidking returned %d subtitles for id=%s", len(subs), id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVideasyDownloadHandler proxies a subtitle download from Videasy and
// converts SRT to WebVTT so the browser can use it as a native track source.
//
//	GET /api/subtitles/videasy/download/<subtitle-id>
func (d Deps) subtitlesVideasyDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	subtitleID := strings.TrimPrefix(r.URL.Path, "/api/subtitles/videasy/download/")
	subtitleID = strings.Trim(subtitleID, "/")
	if !subtitles.ValidSubtitleID(subtitleID) {
		writeError(w, http.StatusBadRequest, "Invalid subtitle ID")
		return
	}

	downloadURL := "https://subs.videasy.to/download?id=" + url.QueryEscape(subtitleID)
	downReq, err := http.NewRequestWithContext(r.Context(), "GET", downloadURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build download request")
		return
	}
	downReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	downRes, err := subtitles.Client.Do(downReq)
	if err != nil {
		log.Printf("[Subtitles] Videasy download failed id=%s: %v", subtitleID, err)
		writeError(w, http.StatusBadGateway, "Subtitle download failed")
		return
	}
	defer downRes.Body.Close()

	if downRes.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] Videasy download non-200 id=%s status=%d", subtitleID, downRes.StatusCode)
		writeError(w, http.StatusBadGateway, "Subtitle not found")
		return
	}

	body, maxed, err := readBounded(downRes.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to read subtitle content")
		return
	}
	if maxed {
		writeError(w, http.StatusBadGateway, "Subtitle file too large")
		return
	}

	content := subtitles.SrtToWebVTT(string(body))

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, content)
}

// subtitlesVidloveHandler resolves subtitles for the Vidlove server with Videasy fallback.
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

	subs := subtitles.FetchVidloveSubtitles(r.Context(), mediaType, id, season, episode)
	if len(subs) == 0 {
		// Fallback to Videasy
		subs = subtitles.FetchVideasySubtitles(r.Context(), id, season, episode)
	}

	if subs == nil {
		subs = []subtitles.FrontendSubtitle{}
	}
	log.Printf("[Subtitles] Vidlove returned %d subtitles for id=%s", len(subs), id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subtitles": subs})
}

// subtitlesVidloveDownloadHandler proxies a subtitle download from Vidlove
//
//	GET /api/subtitles/vidlove/download?url=<url>
func (d Deps) subtitlesVidloveDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		writeError(w, http.StatusBadRequest, "Missing url parameter")
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		writeError(w, http.StatusBadRequest, "Invalid or unauthorized url")
		return
	}
	// Match the registrable domain exactly (or as a subdomain of it) on
	// Hostname(), which strips any port: a bare suffix check would admit
	// attacker-registered lookalikes such as "myvdrk.site" while wrongly
	// rejecting an explicit port like "vdrk.site:8443".
	host := strings.ToLower(parsed.Hostname())
	allowedHost := false
	for _, d := range []string{"vdrk.site", "shows.st", "vidlove.cc", "videasy.to"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			allowedHost = true
			break
		}
	}
	if !allowedHost {
		writeError(w, http.StatusBadRequest, "Invalid or unauthorized subtitle host")
		return
	}

	downReq, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build download request")
		return
	}
	downReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	downReq.Header.Set("Referer", "https://player.vidlove.cc/")

	downRes, err := subtitles.Client.Do(downReq)
	if err != nil {
		log.Printf("[Subtitles] Vidlove download failed url=%s: %v", targetURL, err)
		writeError(w, http.StatusBadGateway, "Subtitle download failed")
		return
	}
	defer downRes.Body.Close()

	if downRes.StatusCode != http.StatusOK {
		log.Printf("[Subtitles] Vidlove download non-200 url=%s status=%d", targetURL, downRes.StatusCode)
		writeError(w, http.StatusBadGateway, "Subtitle not found")
		return
	}

	body, maxed, err := readBounded(downRes.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to read subtitle content")
		return
	}
	if maxed {
		writeError(w, http.StatusBadGateway, "Subtitle file too large")
		return
	}

	content := string(body)
	contentType := strings.ToLower(strings.TrimSpace(downRes.Header.Get("Content-Type")))

	isSRT := !strings.Contains(strings.TrimSpace(content), "WEBVTT")
	if strings.Contains(contentType, "srt") {
		isSRT = true
	}

	if isSRT {
		content = subtitles.SrtToWebVTT(content)
	}

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, content)
}

// readBounded reads up to maxSubtitleBytes+1 bytes, reporting whether the cap
// was exceeded. Both download handlers use it to keep provider responses to
// 2 MB.
func readBounded(body io.Reader) ([]byte, bool, error) {
	const maxSubtitleBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(body, maxSubtitleBytes+1))
	if err != nil {
		return nil, false, err
	}
	return data, len(data) > maxSubtitleBytes, nil
}
