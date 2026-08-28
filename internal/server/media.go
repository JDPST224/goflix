package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"goflix/internal/mediaresolver"
)

// makeMovieSourceHandler resolves a movie HLS source for the given provider.
// prefix is the registered route prefix used to extract the TMDB ID — it is
// NOT derived from the provider, because the legacy unprefixed vixsrc routes
// ("/api/media/source/movie/...") carry no provider segment.
func (d *Deps) makeMovieSourceHandler(prefix, provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !corsGate(w, r, "GET", false) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, prefix)
		if !ValidMediaID(id) {
			writeError(w, http.StatusBadRequest, "Invalid movie ID")
			return
		}
		source, err := d.Resolver.Resolve(r.Context(), mediaresolver.MediaRequest{
			Type:     mediaresolver.Movie,
			ID:       id,
			Provider: provider,
		})
		if err != nil {
			log.Printf("[MediaResolver] %s movie resolution failed id=%s error=%v", provider, id, err)
			writeError(w, http.StatusBadGateway, "Unable to resolve media source")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "type": "hls", "url": source})
	}
}

// makeTVSourceHandler resolves a TV episode HLS source for the given provider.
// prefix is the registered route prefix; the remainder of the path must be
// <id>/<season>/<episode>.
func (d *Deps) makeTVSourceHandler(prefix, provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !corsGate(w, r, "GET", false) {
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"), "/")
		if len(parts) != 3 || !ValidMediaID(parts[0]) || !ValidMediaID(parts[1]) || !ValidMediaID(parts[2]) {
			writeError(w, http.StatusBadRequest, "Invalid TV episode parameters")
			return
		}
		source, err := d.Resolver.Resolve(r.Context(), mediaresolver.MediaRequest{
			Type:     mediaresolver.TV,
			ID:       parts[0],
			Season:   parts[1],
			Episode:  parts[2],
			Provider: provider,
		})
		if err != nil {
			log.Printf("[MediaResolver] %s TV resolution failed id=%s season=%s episode=%s error=%v", provider, parts[0], parts[1], parts[2], err)
			writeError(w, http.StatusBadGateway, "Unable to resolve media source")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "type": "hls", "url": source})
	}
}

// mediaProxyHandler streams upstream HLS playlists and segments through the
// resolver's token-authenticated proxy.
func (d *Deps) mediaProxyHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET, HEAD", true) {
		return
	}
	// Large video segments can legitimately stream longer than the server-wide
	// WriteTimeout; clear the per-response write deadline so proxied media is
	// never truncated mid-stream (truncated segments cause player parse errors
	// such as fragParsingError).
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("[MediaResolver] could not lift proxy write deadline: %v", err)
	}

	const prefix = "/api/media/proxy/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	token = strings.TrimSuffix(token, ".m3u8")
	if token == "" {
		writeError(w, http.StatusBadRequest, "Invalid proxy token")
		return
	}
	if err := d.Resolver.Proxy(w, r, token); err != nil {
		// A player aborting a request (seek, quality switch, tab close) cancels
		// its request context — normal churn, not a failure worth logging.
		// writeError would only target an already-dead connection.
		if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
			return
		}
		log.Printf("[MediaResolver] proxy failed: %v", err)
		writeError(w, http.StatusBadGateway, "Unable to proxy media source")
	}
}

// embedMovieDirectHandler directly resolves https://cinesrc.st/embed/movie/{tmdb_id}
// when accessed via http://<host>/embed/movie/{id}.
// If requested with Accept: application/json or ?json=1, it returns the JSON payload;
// otherwise, it redirects (302) directly to the proxied HLS manifest.
func (d *Deps) embedMovieDirectHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/embed/movie/")
	if !ValidMediaID(id) {
		writeError(w, http.StatusBadRequest, "Invalid movie ID")
		return
	}
	source, err := d.Resolver.Resolve(r.Context(), mediaresolver.MediaRequest{
		Type:     mediaresolver.Movie,
		ID:       id,
		Provider: "cinesrc",
	})
	if err != nil {
		log.Printf("[MediaResolver] cinesrc embed movie resolution failed id=%s error=%v", id, err)
		writeError(w, http.StatusBadGateway, "Unable to resolve media source")
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("json") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "type": "hls", "url": source})
		return
	}
	http.Redirect(w, r, source, http.StatusFound)
}

// embedTVDirectHandler directly resolves https://cinesrc.st/embed/tv/{tmdb_id}?s={s}&e={e}
// when accessed via http://<host>/embed/tv/{id}?s={s}&e={e} or /embed/tv/{id}/{s}/{e}.
func (d *Deps) embedTVDirectHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}
	pathRemainder := strings.Trim(strings.TrimPrefix(r.URL.Path, "/embed/tv/"), "/")
	parts := strings.Split(pathRemainder, "/")
	var id, season, episode string
	if len(parts) == 3 {
		id, season, episode = parts[0], parts[1], parts[2]
	} else if len(parts) == 1 && parts[0] != "" {
		id = parts[0]
		season = r.URL.Query().Get("s")
		if season == "" {
			season = r.URL.Query().Get("season")
		}
		episode = r.URL.Query().Get("e")
		if episode == "" {
			episode = r.URL.Query().Get("episode")
		}
		if season == "" {
			season = "1"
		}
		if episode == "" {
			episode = "1"
		}
	}
	if !ValidMediaID(id) || !ValidMediaID(season) || !ValidMediaID(episode) {
		writeError(w, http.StatusBadRequest, "Invalid TV episode parameters")
		return
	}
	source, err := d.Resolver.Resolve(r.Context(), mediaresolver.MediaRequest{
		Type:     mediaresolver.TV,
		ID:       id,
		Season:   season,
		Episode:  episode,
		Provider: "cinesrc",
	})
	if err != nil {
		log.Printf("[MediaResolver] cinesrc embed TV resolution failed id=%s season=%s episode=%s error=%v", id, season, episode, err)
		writeError(w, http.StatusBadGateway, "Unable to resolve media source")
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("json") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "type": "hls", "url": source})
		return
	}
	http.Redirect(w, r, source, http.StatusFound)
}

