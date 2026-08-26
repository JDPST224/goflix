// Package catalog implements the TMDB-backed movie/TV catalog: the HTTP
// client, TMDB→Movie mapping, and the three in-memory caches behind
// /api/home, /api/movies, /api/tvshows and /api/popular.
package catalog

import "fmt"

// Movie is the catalog item shape the frontend consumes.
type Movie struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Banner      string   `json:"banner"`
	Thumbnail   string   `json:"thumbnail"`
	Categories  []string `json:"categories"`
	Type        string   `json:"type"` // "movie" or "tv"
	Rating      float64  `json:"rating"`
	Year        string   `json:"year"`
	Genres      []string `json:"genres"`
}

// TMDBResponse is the list envelope of TMDB list/search endpoints.
type TMDBResponse struct {
	Results       []TMDBMovie `json:"results"`
	StatusMessage string      `json:"status_message"`
}

// TMDBMovie is the subset of a TMDB result entry the app consumes.
type TMDBMovie struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Name          string  `json:"name"` // For TV shows
	Overview      string  `json:"overview"`
	BackdropPath  string  `json:"backdrop_path"`
	PosterPath    string  `json:"poster_path"`
	VoteAverage   float64 `json:"vote_average"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	GenreIDs      []int   `json:"genre_ids"`
	MediaType     string  `json:"media_type,omitempty"` // set by /search/multi
}

// tmdbGenres maps TMDB genre IDs to display names (movie and TV sets merged).
var tmdbGenres = map[int]string{
	28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama", 10751: "Family", 14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music", 9648: "Mystery", 10749: "Romance", 878: "Science Fiction", 10770: "TV Movie", 53: "Thriller", 10752: "War", 37: "Western",
	10759: "Action & Adventure", 10762: "Kids", 10763: "News", 10764: "Reality", 10765: "Sci-Fi & Fantasy", 10766: "Soap", 10767: "Talk", 10768: "War & Politics",
}

// genreIDByName reverses tmdbGenres so the discover endpoint can resolve the
// same display names the frontend's genre chips are built from. Names are
// unique across the merged movie+TV sets, so one map serves both.
var genreIDByName = func() map[string]int {
	m := make(map[string]int, len(tmdbGenres))
	for id, name := range tmdbGenres {
		m[name] = id
	}
	return m
}()

// GenreID reverse-maps a display genre name to its TMDB genre ID.
func GenreID(name string) (int, bool) {
	id, ok := genreIDByName[name]
	return id, ok
}

// MapTMDBMovie converts a TMDB result into a catalog Movie, returning nil for
// entries without both a title and a poster. Title falls back
// Title → OriginalTitle → Name; the banner prefers the original backdrop and
// otherwise uses the w1280 poster; the thumbnail is always w500.
func MapTMDBMovie(m TMDBMovie, defaultType string, categories []string) *Movie {
	title := m.Title
	if title == "" {
		title = m.OriginalTitle
	}
	if title == "" {
		title = m.Name
	}

	if title == "" || m.PosterPath == "" {
		return nil
	}

	banner := ""
	if m.BackdropPath != "" {
		banner = tmdbImageBase + "/original" + m.BackdropPath
	} else {
		banner = tmdbImageBase + "/w1280" + m.PosterPath
	}

	thumbnail := tmdbImageBase + "/w500" + m.PosterPath

	var genres []string
	for _, gid := range m.GenreIDs {
		if name, ok := tmdbGenres[gid]; ok {
			genres = append(genres, name)
		}
	}

	year := ""
	if m.ReleaseDate != "" && len(m.ReleaseDate) >= 4 {
		year = m.ReleaseDate[:4]
	} else if m.FirstAirDate != "" && len(m.FirstAirDate) >= 4 {
		year = m.FirstAirDate[:4]
	}

	mType := defaultType
	if m.MediaType != "" {
		mType = m.MediaType
	}

	return &Movie{
		ID:          fmt.Sprintf("%d", m.ID),
		Title:       title,
		Description: m.Overview,
		Banner:      banner,
		Thumbnail:   thumbnail,
		Categories:  categories,
		Type:        mType,
		Rating:      m.VoteAverage,
		Year:        year,
		Genres:      genres,
	}
}
