// store.go — the three in-memory catalog caches and their 30-minute refresh
// cycle. Refreshes are guarded by TryLock so overlapping runs skip instead of
// piling up, and an empty upstream fetch keeps the previous cache.
package catalog

import (
	"context"
	"fmt"
	"log"
	"sort"
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

	providersMu  sync.RWMutex
	providers    map[string][]Movie
	providersRef sync.Mutex // guards concurrent refreshes (TryLock)
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

// Providers returns the cached provider-keyed carousel map (nil when never
// populated).
func (s *Store) Providers() map[string][]Movie {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	return s.providers
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
		{"/discover/movie?with_genres=53&language=en-US&page=1&sort_by=popularity.desc", []string{"Thriller"}, "movie"},
		{"/discover/movie?with_genres=80&language=en-US&page=1&sort_by=popularity.desc", []string{"Crime"}, "movie"},
		{"/discover/movie?with_genres=18&language=en-US&page=1&sort_by=popularity.desc", []string{"Drama"}, "movie"},
		{"/discover/movie?with_genres=14&language=en-US&page=1&sort_by=popularity.desc", []string{"Fantasy"}, "movie"},
		{"/discover/movie?with_genres=12&language=en-US&page=1&sort_by=popularity.desc", []string{"Adventure"}, "movie"},
		{"/discover/movie?with_genres=10751&language=en-US&page=1&sort_by=popularity.desc", []string{"Family"}, "movie"},
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
		{"/discover/tv?with_genres=80&language=en-US&page=1&sort_by=popularity.desc", []string{"Crime Shows"}, "tv"},
		{"/discover/tv?with_genres=10751&language=en-US&page=1&sort_by=popularity.desc", []string{"Family Shows"}, "tv"},
		{"/discover/tv?with_genres=99&language=en-US&page=1&sort_by=popularity.desc", []string{"Documentary"}, "tv"},
		{"/discover/tv?with_genres=10764&language=en-US&page=1&sort_by=popularity.desc", []string{"Reality"}, "tv"},
		{"/discover/tv?with_genres=10762&language=en-US&page=1&sort_by=popularity.desc", []string{"Kids"}, "tv"},
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

// providerTable maps our carousel keys to TMDB watch-provider IDs (US
// region). IDs verified against /watch/providers/movie?watch_region=US.
var providerTable = []struct {
	key   string
	label string
	id    int
}{
	{"netflix", "Netflix", 8},
	{"prime", "Prime Video", 9},
	{"max", "Max", 1899},
	{"disney", "Disney+", 337},
	{"apple", "Apple TV+", 350},
	{"paramount", "Paramount+", 531},
	{"hulu", "Hulu", 15},
}

// ProviderInfo resolves a provider key (netflix, prime, ...) to its TMDB
// watch-provider ID (US region) and display label, for the paginated
// /api/discover provider feed.
func ProviderInfo(key string) (id int, label string, ok bool) {
	for _, p := range providerTable {
		if p.key == key {
			return p.id, p.label, true
		}
	}
	return 0, "", false
}

// RefreshProviders repopulates the per-provider "Only on …" carousel cache;
// overlapping calls skip.
func (s *Store) RefreshProviders() {
	if !s.providersRef.TryLock() {
		log.Println("Providers cache refresh already in progress; skipping overlapping refresh")
		return
	}
	defer s.providersRef.Unlock()

	log.Println("Refreshing providers cache...")
	type result struct {
		key   string
		items []Movie
	}
	results := make([]result, len(providerTable))
	var wg sync.WaitGroup
	for i, p := range providerTable {
		wg.Add(1)
		go func(i int, p struct {
			key   string
			label string
			id    int
		}) {
			defer wg.Done()
			idStr := fmt.Sprintf("%d", p.id)
			movieItems := s.client.List("/discover/movie?with_watch_providers="+idStr+"&watch_region=US&language=en-US&page=1&sort_by=popularity.desc", []string{p.label}, "movie")
			tvItems := s.client.List("/discover/tv?with_watch_providers="+idStr+"&watch_region=US&language=en-US&page=1&sort_by=popularity.desc", []string{p.label}, "tv")
			merged := InterleaveMovies(movieItems, tvItems)
			if len(merged) > 20 {
				merged = merged[:20]
			}
			results[i] = result{key: p.key, items: merged}
		}(i, p)
	}
	wg.Wait()

	next := map[string][]Movie{}
	for _, r := range results {
		if len(r.items) > 0 {
			next[r.key] = r.items
		}
	}

	s.providersMu.Lock()
	if len(next) > 0 {
		s.providers = next
		log.Printf("Providers cache updated: %d providers\n", len(next))
	} else {
		log.Println("Warning: providers fetch returned 0 providers, keeping old cache")
	}
	s.providersMu.Unlock()
}

// homeRowOrder is the exact row interleave of /api/home: for each entry the
// category tag and which cache it draws from.
var homeRowOrder = []struct {
	cat string
	src string
}{
	{"Trending Movies", "movie"},
	{"Trending TV", "tv"},
	{"Trending This Week", "popular"}, // special-cased: merges two popular tags, interleaved
	{"Popular Movies", "movie"},
	{"Popular Shows", "tv"},
	{"Top Rated Movies", "movie"}, // special-cased: sorted by Rating desc
	{"Top Rated Shows", "tv"},     // special-cased: sorted by Rating desc
	{"Upcoming", "movie"},
	{"New in Cinemas", "popular"},
	{"New Episodes", "popular"},
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
	popularBycat := map[string][]Movie{}
	for _, m := range s.Popular() {
		for _, c := range m.Categories {
			popularBycat[c] = append(popularBycat[c], m)
		}
	}

	// Cache tags do not always match the Home row labels. Copy the source
	// slices before appending to avoid mutating shared backing arrays (A13).
	if items, ok := moviesBycat["Trending Now"]; ok {
		existing := append([]Movie(nil), moviesBycat["Trending Movies"]...)
		moviesBycat["Trending Movies"] = append(existing, items...)
	}
	if items, ok := moviesBycat["Popular"]; ok {
		existing := append([]Movie(nil), moviesBycat["Popular Movies"]...)
		moviesBycat["Popular Movies"] = append(existing, items...)
	}
	if items, ok := moviesBycat["Top Rated"]; ok {
		existing := append([]Movie(nil), moviesBycat["Top Rated Movies"]...)
		moviesBycat["Top Rated Movies"] = append(existing, items...)
	}

	var combined []Movie
	seen := map[string]bool{}
	for _, o := range homeRowOrder {
		var rowItems []Movie
		switch {
		case o.cat == "Trending This Week":
			rowItems = InterleaveMovies(popularBycat["Trending Movies"], popularBycat["Trending Shows"])
		case o.src == "popular":
			rowItems = popularBycat[o.cat]
		case o.src == "movie":
			rowItems = moviesBycat[o.cat]
		default:
			rowItems = tvBycat[o.cat]
		}
		if o.cat == "Top Rated Movies" || o.cat == "Top Rated Shows" {
			rowItems = append([]Movie(nil), rowItems...)
			sort.SliceStable(rowItems, func(i, j int) bool { return rowItems[i].Rating > rowItems[j].Rating })
		}
		count := 0
		for _, m := range rowItems {
			if count >= 20 {
				break
			}
			key := m.Type + "-" + m.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			// Re-label with the home category name
			m.Categories = []string{o.cat}
			combined = append(combined, m)
			count++
		}
	}
	return combined
}

// InterleaveMovies alternates items from a and b (a first), for rows that
// mix two source lists (e.g. "Trending This Week" merges movie + TV pools,
// and the provider discover feed merges one page of each).
func InterleaveMovies(a, b []Movie) []Movie {
	var out []Movie
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			out = append(out, a[i])
		}
		if i < len(b) {
			out = append(out, b[i])
		}
	}
	return out
}

// StartRefreshLoop fires the initial parallel fetch asynchronously (so the
// HTTP server can start accepting requests immediately) and then refreshes all
// caches every 30 minutes. ctx controls the goroutine lifetime: cancel it (or
// let SIGTERM close it) to stop the background loop cleanly.
// Call only when credentials are configured.
func (s *Store) StartRefreshLoop(ctx context.Context) {
	// Kick off the initial population in the background — callers that need
	// fresh data before the first tick can call WaitReady.
	go func() {
		s.RefreshMovies()
		s.RefreshTVShows()
		s.RefreshPopular()
		s.RefreshProviders()
	}()

	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				go s.RefreshMovies()
				go s.RefreshTVShows()
				go s.RefreshPopular()
				go s.RefreshProviders()
			}
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
