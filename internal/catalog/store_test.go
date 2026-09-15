package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// persistSnapshot used to share one "<path>.tmp" file across every caller;
// the refresh loop fires all four caches in parallel, so concurrent writers
// could rename each other's half-written temp file and leave a corrupt (or
// missing) snapshot on disk. The snapshot mutex must make every concurrent
// write end with a valid, complete file.
func TestPersistSnapshotConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_snapshot.json")
	s := NewStore(nil)
	s.SetSnapshotPath(path)

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

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot missing after concurrent writes: %v", err)
	}
	var snap catalogSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("concurrent writes produced a corrupt snapshot: %v", err)
	}
	if snap.Version != 1 {
		t.Fatalf("snapshot version = %d, want 1", snap.Version)
	}
	if len(snap.Movies) != len(movies) || len(snap.TVShows) != len(shows) {
		t.Fatalf("snapshot contents incomplete: movies=%d (want %d) tvshows=%d (want %d)",
			len(snap.Movies), len(movies), len(snap.TVShows), len(shows))
	}

	// The snapshot must also round-trip through LoadSnapshot into a fresh
	// store (the restart path this file exists for).
	fresh := NewStore(nil)
	fresh.SetSnapshotPath(path)
	fresh.LoadSnapshot()
	if got := len(fresh.Movies()); got != len(movies) {
		t.Errorf("LoadSnapshot restored %d movies, want %d", got, len(movies))
	}
	if got := len(fresh.TVShows()); got != len(shows) {
		t.Errorf("LoadSnapshot restored %d shows, want %d", got, len(shows))
	}
}

// persistSnapshot must stay a no-op when persistence is disabled or nothing
// worth persisting exists yet.
func TestPersistSnapshotSkipsEmptyCaches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_snapshot.json")
	s := NewStore(nil)
	s.SetSnapshotPath(path)
	s.persistSnapshot()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("snapshot written for empty caches; want none")
	}

	off := NewStore(nil)
	off.SetSnapshotPath("-")
	off.moviesMu.Lock()
	off.movies = []Movie{{ID: "1", Title: "x", Type: "movie"}}
	off.moviesMu.Unlock()
	off.persistSnapshot() // must not panic or write anywhere
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
