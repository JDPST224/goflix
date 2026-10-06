package catalog

import (
	"database/sql"
	"sync"
	"testing"

	"goflix/internal/db"
)

func testSnapshotDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open("")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return d
}

// persistSnapshot used to share one "<path>.tmp" file across every caller;
// the refresh loop fires all four caches in parallel, so concurrent writers
// could rename each other's half-written temp file and leave a corrupt (or
// missing) snapshot on disk. The snapshot mutex must make every concurrent
// write end with a valid, complete snapshot in the database.
func TestPersistSnapshotConcurrentWrites(t *testing.T) {
	s := NewStore(nil)
	s.SetSnapshotDB(testSnapshotDB(t))

	movies := make([]Movie, 50)
	for i := range movies {
		movies[i] = Movie{ID: itoa2(i + 1), Title: "Movie " + itoa2(i+1), Type: "movie"}
	}
	shows := make([]Movie, 30)
	for i := range shows {
		shows[i] = Movie{ID: itoa2(1000 + i), Title: "Show " + itoa2(i+1), Type: "tv"}
	}
	s.moviesMu.Lock()
	s.movies = movies
	s.moviesMu.Unlock()
	s.tvMu.Lock()
	s.tvShows = shows
	s.tvMu.Unlock()

	const writers = 8
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			s.persistSnapshot()
		}()
	}
	wg.Wait()

	var n int
	if err := s.snapshotDB.QueryRow(`SELECT COUNT(*) FROM catalog_items WHERE cache = 'movies'`).Scan(&n); err != nil {
		t.Fatalf("count movies: %v", err)
	}
	if n != len(movies) {
		t.Fatalf("movies = %d rows, want %d", n, len(movies))
	}
	if err := s.snapshotDB.QueryRow(`SELECT COUNT(*) FROM catalog_items WHERE cache = 'tvshows'`).Scan(&n); err != nil {
		t.Fatalf("count tvshows: %v", err)
	}
	if n != len(shows) {
		t.Fatalf("tvshows = %d rows, want %d", n, len(shows))
	}

	// The snapshot must also round-trip through LoadSnapshot into a fresh
	// store (the restart path this exists for).
	fresh := NewStore(nil)
	fresh.SetSnapshotDB(s.snapshotDB)
	fresh.LoadSnapshot()
	if got := len(fresh.Movies()); got != len(movies) {
		t.Errorf("LoadSnapshot restored %d movies, want %d", got, len(movies))
	}
	if got := len(fresh.TVShows()); got != len(shows) {
		t.Errorf("LoadSnapshot restored %d shows, want %d", got, len(shows))
	}
	if got := fresh.Movies()[0]; got.ID != "1" || got.Title != "Movie 1" {
		t.Errorf("first movie = %+v, want id 1 in save order", got)
	}
}

// persistSnapshot must stay a no-op when persistence is disabled or nothing
// worth persisting exists yet.
func TestPersistSnapshotSkipsEmptyCaches(t *testing.T) {
	d := testSnapshotDB(t)
	s := NewStore(nil)
	s.SetSnapshotDB(d)
	s.persistSnapshot()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM catalog_items`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("snapshot written for empty caches; want none (rows=%d err=%v)", n, err)
	}

	off := NewStore(nil) // nil snapshot DB disables persistence
	off.moviesMu.Lock()
	off.movies = []Movie{{ID: "1", Title: "x", Type: "movie"}}
	off.moviesMu.Unlock()
	off.persistSnapshot() // must not panic or write anywhere
}

// The providers carousel must round-trip too: each provider key becomes its
// own cache with order preserved.
func TestProvidersSnapshotRoundTrip(t *testing.T) {
	s := NewStore(nil)
	s.SetSnapshotDB(testSnapshotDB(t))
	s.providersMu.Lock()
	s.providers = map[string][]Movie{
		"netflix": {{ID: "10", Title: "N1", Type: "movie"}, {ID: "11", Title: "N2", Type: "tv"}},
		"prime":   {{ID: "20", Title: "P1", Type: "movie"}},
	}
	s.providersMu.Unlock()
	s.persistSnapshot()

	fresh := NewStore(nil)
	fresh.SetSnapshotDB(s.snapshotDB)
	fresh.LoadSnapshot()
	providers := fresh.Providers()
	if len(providers) != 2 {
		t.Fatalf("restored %d providers, want 2", len(providers))
	}
	got := providers["netflix"]
	if len(got) != 2 || got[0].ID != "10" || got[1].ID != "11" {
		t.Errorf("netflix = %v, want ids [10 11] in order", got)
	}
	// The provider cache key must be namespaced so item rows cannot collide
	// with the main caches.
	var n int
	if err := s.snapshotDB.QueryRow(`SELECT COUNT(*) FROM catalog_items WHERE cache LIKE 'providers:%'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("provider rows = %d (err %v), want 3", n, err)
	}
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
