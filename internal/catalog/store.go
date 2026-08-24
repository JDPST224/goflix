// store.go — the three in-memory catalog caches and their 30-minute refresh
// cycle. Refreshes are guarded by TryLock so overlapping runs skip instead of
// piling up, and an empty upstream fetch keeps the previous cache.
package catalog

import (
	"log"
	"sync"
	"time"
)

// tmdbTask is one list fetch inside a cache refresh.
type tmdbTask struct {
	endpoint   string
	categories []string
	mediaType  string
}

// Store holds the movies / TV shows / popular caches behind per-cache locks.
type Store struct {
	client *Client

	moviesMu   sync.RWMutex
	tvMu       sync.RWMutex
	popularMu  sync.RWMutex
	movies     []Movie
	tvShows    []Movie
	popular    []Movie
	moviesRefM sync.Mutex // guards concurrent refreshes (TryLock)
	tvRefM     sync.Mutex
	popularRef sync.Mutex
}

func NewStore(c *Client) *Store { return &Store{client: c} }

// Movies returns the cached movie list (nil when never populated).
func (s *Store) Movies() []Movie {
	s.moviesMu.RLock()
	defer s.moviesMu.RUnlock()
	return s.movies
}

// TVShows returns the cached TV show list.
func (s *Store) TVShows() []Movie {
	s.tvMu.RLock()
	defer s.tvMu.RUnlock()
	return s.tvShows
}

// Popular returns the cached trending/new list.
func (s *Store) Popular() []Movie {
	s.popularMu.RLock()
	defer s.popularMu.RUnlock()
	return s.popular
}

// Counts reports the three cache sizes for /api/health.
func (s *Store) Counts() (movies, tvShows, popular int) {
	s.moviesMu.RLock()
	movies = len(s.movies)
	s.moviesMu.RUnlock()
	s.tvMu.RLock()
	tvShows = len(s.tvShows)
	s.tvMu.RUnlock()
	s.popularMu.RLock()
	popular = len(s.popular)
	s.popularMu.RUnlock()
	return
}

func (s *Store) runTasks(tasks []tmdbTask) []Movie {
	results := make([][]Movie, len(tasks))
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		go func(i int, t tmdbTask) {
			defer wg.Done()
			results[i] = s.client.List(t.endpoint, t.categories, t.mediaType)
		}(i, t)
	}
	wg.Wait()
	var all []Movie
	for _, movies := range results {
		all = append(all, movies...)
	}
	return all
}

// RefreshMovies repopulates the movies cache; overlapping calls skip.
func (s *Store) RefreshMovies() {
	if !s.moviesRefM.TryLock() {
		log.Println("Movies cache refresh already in progress; skipping overlapping refresh")
		return
	}
	defer s.moviesRefM.Unlock()

	log.Println("Refreshing movies cache...")
	allMovies := s.runTasks([]tmdbTask{
		{"/trending/movie/day?language=en-US", []string{"Trending Now"}, "movie"},
		{"/movie/popular?language=en-US&page=1", []string{"Popular"}, "movie"},
		{"/movie/top_rated?language=en-US&page=1", []string{"Top Rated"}, "movie"},
		{"/movie/now_playing?language=en-US&page=1", []string{"Now Playing"}, "movie"},
		{"/movie/upcoming?language=en-US&page=1", []string{"Upcoming"}, "movie"},
		{"/discover/movie?with_genres=28&language=en-US&page=1&sort_by=popularity.desc", []string{"Action"}, "movie"},
		{"/discover/movie?with_genres=35&language=en-US&page=1&sort_by=popularity.desc", []string{"Comedy"}, "movie"},
		{"/discover/movie?with_genres=27&language=en-US&page=1&sort_by=popularity.desc", []string{"Horror"}, "movie"},
		{"/discover/movie?with_genres=878&language=en-US&page=1&sort_by=popularity.desc", []string{"Sci-Fi"}, "movie"},
		{"/discover/movie?with_genres=10749&language=en-US&page=1&sort_by=popularity.desc", []string{"Romance"}, "movie"},
		{"/discover/movie?with_genres=16&language=en-US&page=1&sort_by=popularity.desc", []string{"Animation"}, "movie"},
	})

	s.moviesMu.Lock()
	if len(allMovies) > 0 {
		s.movies = allMovies
		log.Printf("Movies cache updated: %d total\n", len(allMovies))
	} else {
		log.Println("Warning: movie fetch returned 0 movies, keeping old cache")
	}
	s.moviesMu.Unlock()
}

// RefreshTVShows repopulates the TV shows cache; overlapping calls skip.
func (s *Store) RefreshTVShows() {
	if !s.tvRefM.TryLock() {
		log.Println("TV cache refresh already in progress; skipping overlapping refresh")
		return
	}
	defer s.tvRefM.Unlock()

	log.Println("Refreshing TV shows cache...")
	allShows := s.runTasks([]tmdbTask{
		{"/trending/tv/day?language=en-US", []string{"Trending TV"}, "tv"},
		{"/tv/popular?language=en-US&page=1", []string{"Popular Shows"}, "tv"},
		{"/tv/top_rated?language=en-US&page=1", []string{"Top Rated Shows"}, "tv"},
		{"/tv/on_the_air?language=en-US&page=1", []string{"Now Airing"}, "tv"},
		{"/discover/tv?with_genres=10759&language=en-US&page=1&sort_by=popularity.desc", []string{"Action & Adventure"}, "tv"},
		{"/discover/tv?with_genres=18&language=en-US&page=1&sort_by=popularity.desc", []string{"Drama"}, "tv"},
		{"/discover/tv?with_genres=35&language=en-US&page=1&sort_by=popularity.desc", []string{"Comedy Shows"}, "tv"},
		{"/discover/tv?with_genres=9648&language=en-US&page=1&sort_by=popularity.desc", []string{"Mystery"}, "tv"},
		{"/discover/tv?with_genres=10765&language=en-US&page=1&sort_by=popularity.desc", []string{"Sci-Fi & Fantasy"}, "tv"},
		{"/discover/tv?with_genres=16&language=en-US&page=1&sort_by=popularity.desc", []string{"Anime"}, "tv"},
	})

	s.tvMu.Lock()
	if len(allShows) > 0 {
		s.tvShows = allShows
		log.Printf("TV shows cache updated: %d total\n", len(allShows))
	} else {
		log.Println("Warning: TV fetch returned 0 shows, keeping old cache")
	}
	s.tvMu.Unlock()
}

// RefreshPopular repopulates the popular/new cache; overlapping calls skip.
func (s *Store) RefreshPopular() {
	if !s.popularRef.TryLock() {
		log.Println("Popular cache refresh already in progress; skipping overlapping refresh")
		return
	}
	defer s.popularRef.Unlock()

	log.Println("Refreshing popular/new cache...")
	all := s.runTasks([]tmdbTask{
		{"/trending/movie/week?language=en-US", []string{"Trending Movies"}, "movie"},
		{"/trending/tv/week?language=en-US", []string{"Trending Shows"}, "tv"},
		{"/movie/now_playing?language=en-US&page=1", []string{"New in Cinemas"}, "movie"},
		{"/tv/on_the_air?language=en-US&page=1", []string{"New Episodes"}, "tv"},
	})

	s.popularMu.Lock()
	if len(all) > 0 {
		s.popular = all
		log.Printf("Popular cache updated: %d total\n", len(all))
	} else {
		log.Println("Warning: popular fetch returned 0 items, keeping old cache")
	}
	s.popularMu.Unlock()
}

// homeRowOrder is the exact row interleave of /api/home: for each entry the
// category tag and which cache it draws from.
var homeRowOrder = []struct {
	cat string
	src string
}{
	{"Trending Movies", "movie"},
	{"Trending TV", "tv"},
	{"Popular Movies", "movie"},
	{"Popular Shows", "tv"},
	{"Top Rated Movies", "movie"},
	{"Top Rated Shows", "tv"},
	{"Now Playing", "movie"},
	{"Now Airing", "tv"},
	{"Upcoming", "movie"},
	{"Action", "movie"},
	{"Action & Adventure", "tv"},
	{"Comedy", "movie"},
	{"Comedy Shows", "tv"},
	{"Horror", "movie"},
	{"Sci-Fi", "movie"},
	{"Sci-Fi & Fantasy", "tv"},
	{"Drama", "tv"},
	{"Mystery", "tv"},
	{"Romance", "movie"},
	{"Animation", "movie"},
	{"Anime", "tv"},
}

// HomeView builds the /api/home payload: items interleaved in the fixed row
// order above, deduplicated by type-ID, each relabeled with its home row.
func (s *Store) HomeView() []Movie {
	movies := s.Movies()
	shows := s.TVShows()

	// Collect movies by category
	moviesBycat := map[string][]Movie{}
	for _, m := range movies {
		for _, c := range m.Categories {
			moviesBycat[c] = append(moviesBycat[c], m)
		}
	}
	tvBycat := map[string][]Movie{}
	for _, m := range shows {
		for _, c := range m.Categories {
			tvBycat[c] = append(tvBycat[c], m)
		}
	}

	// Cache tags do not always match the Home row labels.
	if items, ok := moviesBycat["Trending Now"]; ok {
		moviesBycat["Trending Movies"] = append(moviesBycat["Trending Movies"], items...)
	}
	if items, ok := moviesBycat["Popular"]; ok {
		moviesBycat["Popular Movies"] = append(moviesBycat["Popular Movies"], items...)
	}
	if items, ok := moviesBycat["Top Rated"]; ok {
		moviesBycat["Top Rated Movies"] = append(moviesBycat["Top Rated Movies"], items...)
	}

	var combined []Movie
	seen := map[string]bool{}
	for _, o := range homeRowOrder {
		var src map[string][]Movie
		if o.src == "movie" {
			src = moviesBycat
		} else {
			src = tvBycat
		}
		for _, m := range src[o.cat] {
			key := m.Type + "-" + m.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			// Re-label with the home category name
			m.Categories = []string{o.cat}
			combined = append(combined, m)
		}
	}
	return combined
}

// StartRefreshLoop performs the initial parallel fetch and then refreshes all
// three caches every 30 minutes. Call only when credentials are configured.
func (s *Store) StartRefreshLoop() {
	done := make(chan struct{}, 3)
	go func() { s.RefreshMovies(); done <- struct{}{} }()
	go func() { s.RefreshTVShows(); done <- struct{}{} }()
	go func() { s.RefreshPopular(); done <- struct{}{} }()
	<-done
	<-done
	<-done

	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			go s.RefreshMovies()
			go s.RefreshTVShows()
			go s.RefreshPopular()
		}
	}()
}

// Seed installs cache contents directly — used by tests to exercise the
// handlers without hitting TMDB.
func (s *Store) Seed(movies, tvShows, popular []Movie) {
	s.moviesMu.Lock()
	s.movies = movies
	s.moviesMu.Unlock()
	s.tvMu.Lock()
	s.tvShows = tvShows
	s.tvMu.Unlock()
	s.popularMu.Lock()
	s.popular = popular
	s.popularMu.Unlock()
}
