package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

// providersHandler serves the cached "Only on …" carousel map, keyed by
// provider (netflix, prime, max, ...). Empty map when never populated.
func (d Deps) providersHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	providers := d.Store.Providers()
	if providers == nil {
		providers = map[string][]catalog.Movie{}
	}
	json.NewEncoder(w).Encode(providers)
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

// discoverHandler serves one page of TMDB discover results for a genre,
// powering the Movies/TV Shows genre grid's infinite scroll. Response shape
// matches the other catalog endpoints: a plain array of Movies. An empty
// array means "no more pages" to the frontend.
func (d Deps) discoverHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if !d.Client.HasCredentials() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	// Page cap is TMDB's discover limit; beyond it the API just errors.
	if err != nil || page < 1 || page > 500 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	// Provider feed: one movie page and one TV page per grid page, both
	// exclusive to the provider (US region), interleaved so the grid mixes
	// types like the carousel row does. Takes precedence over genre.
	if providerKey := strings.TrimSpace(r.URL.Query().Get("provider")); providerKey != "" {
		id, label, ok := catalog.ProviderInfo(providerKey)
		if !ok {
			json.NewEncoder(w).Encode([]catalog.Movie{})
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		cats := []string{label}
		pageURL := func(mediaType string) string {
			return fmt.Sprintf("/discover/%s?with_watch_providers=%d&watch_region=US&language=en-US&page=%d&sort_by=popularity.desc", mediaType, id, page)
		}
		movies := d.Client.List(pageURL("movie"), cats, "movie")
		shows := d.Client.List(pageURL("tv"), cats, "tv")
		results := catalog.InterleaveMovies(movies, shows)
		if results == nil {
			results = []catalog.Movie{}
		}
		json.NewEncoder(w).Encode(results)
		return
	}
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	genreName := strings.TrimSpace(r.URL.Query().Get("genre"))
	if (mediaType != "movie" && mediaType != "tv") || genreName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	genreID, ok := catalog.GenreID(genreName)
	if !ok {
		// Unknown name (frontend chips always come from our own map): no
		// server pages to add, empty array ends the frontend feed.
		json.NewEncoder(w).Encode([]catalog.Movie{})
		return
	}
	// Discover pages shift slowly; a longer TTL than the catalog handlers'
	// 60s keeps scroll-back-up free without serving stale data.
	w.Header().Set("Cache-Control", "public, max-age=300")
	endpoint := fmt.Sprintf("/discover/%s?with_genres=%d&language=en-US&page=%d&sort_by=popularity.desc", mediaType, genreID, page)
	results := d.Client.List(endpoint, []string{genreName}, mediaType)
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
