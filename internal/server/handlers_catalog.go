package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"goflix/internal/catalog"
)

func (d Deps) moviesHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	movies := d.Store.Movies()
	if movies == nil {
		movies = []catalog.Movie{}
	}
	json.NewEncoder(w).Encode(movies)
}

// tvShowsHandler serves the cached TV show list. Cache-Control mirrors the
// movies handler: public max-age=60 overrides jsonGate's no-cache default.
func (d Deps) tvShowsHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	shows := d.Store.TVShows()
	if shows == nil {
		shows = []catalog.Movie{}
	}
	json.NewEncoder(w).Encode(shows)
}

func (d Deps) popularHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	pop := d.Store.Popular()
	if pop == nil {
		pop = []catalog.Movie{}
	}
	json.NewEncoder(w).Encode(pop)
}

func (d Deps) homeHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	combined := d.Store.HomeView()
	if combined == nil {
		combined = []catalog.Movie{}
	}
	json.NewEncoder(w).Encode(combined)
}

// searchHandler proxies TMDB search. type=movie/tv selects a scoped search;
// empty type runs the multi search, mapping media_type per result.
func (d Deps) searchHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if !d.Client.HasCredentials() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	if query == "" {
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	encoded := url.QueryEscape(query)
	var results []catalog.Movie
	switch mediaType {
	case "tv":
		results = d.Client.List("/search/tv?query="+encoded+"&language=en-US&page=1", []string{"Search"}, "tv")
	case "movie":
		results = d.Client.List("/search/movie?query="+encoded+"&language=en-US&page=1", []string{"Search"}, "movie")
	default:
		results = d.Client.SearchMulti(encoded)
	}
	if results == nil {
		results = []catalog.Movie{}
	}
	json.NewEncoder(w).Encode(results)
}

// detailHandler passes the TMDB detail payload through with the modal's
// append_to_response set; status mapping (500/502/{}) lives in the client.
func (d Deps) detailHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if !d.Client.HasCredentials() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{}`))
		return
	}
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !ValidMediaID(id) || (mediaType != "movie" && mediaType != "tv") {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{}`))
		return
	}
	status, body := d.Client.MovieDetail(mediaType, id)
	w.WriteHeader(status)
	w.Write(body)
}

func (d Deps) episodesHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if !d.Client.HasCredentials() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{}`))
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	season := strings.TrimSpace(r.URL.Query().Get("season"))
	if !ValidMediaID(id) || !ValidMediaID(season) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{}`))
		return
	}
	status, body := d.Client.SeasonEpisodes(id, season)
	w.WriteHeader(status)
	w.Write(body)
}

// ValidMediaID reports whether s is a non-empty decimal string of at most 20
// digits — the validation shared by media-source and subtitle params.
func ValidMediaID(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
