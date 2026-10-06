package mediaresolver

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"goflix/internal/db"
)

// TestResolutionCachePersistence round-trips a record through the database:
// a second resolver over the same database must restore the record, and an
// expired record must not be restored.
func TestResolutionCachePersistence(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer database.Close()
	if err := db.EnsureSchema(database); err != nil {
		t.Fatalf("schema: %v", err)
	}

	newResolver := func() *Resolver {
		r, err := New(Config{MaxBrowserSessions: 1, DB: database})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return r
	}

	req := MediaRequest{Type: TV, ID: "119051", Season: "1", Episode: "3", Provider: "movish"}
	vr := &directResolution{
		Source:     "https://moon.example/vd/abc/index-s1080p-v1-a1.m3u8",
		Headers:    http.Header{"Referer": []string{"https://movish.to/"}},
		Allowed:    map[string]bool{"moon.example": true},
		MasterText: "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv.m3u8\n",
	}

	first := newResolver()
	defer first.Close()
	first.rememberResolution(req, newResolutionRecord(vr))
	key := resolutionKey(req)
	if _, ok := first.resolutions[key]; !ok {
		t.Fatalf("record not stored")
	}
	stored := first.resolutions[key]
	if stored.manifestFP == "" {
		t.Fatalf("fingerprint not computed")
	}

	// A second resolver over the same database restores the record.
	second := newResolver()
	defer second.Close()
	got, ok := second.resolutions[key]
	if !ok {
		t.Fatalf("record not restored from database")
	}
	if got.source != vr.Source {
		t.Errorf("source mismatch: got %q", got.source)
	}
	if got.manifestFP != stored.manifestFP {
		t.Errorf("fingerprint mismatch: got %q want %q", got.manifestFP, stored.manifestFP)
	}
	if string(got.manifest) != vr.MasterText {
		t.Errorf("manifest not restored")
	}

	// An expired record must be dropped on load.
	recs := map[string]db.StoredResolution{
		key: {
			Source:    vr.Source,
			CreatedAt: time.Now().Add(-2 * resolutionTTL),
		},
	}
	if err := db.SaveResolutions(database, recs); err != nil {
		t.Fatalf("save expired record: %v", err)
	}
	third := newResolver()
	defer third.Close()
	if _, ok := third.resolutions[key]; ok {
		t.Fatalf("expired record was restored")
	}
}

// TestResolutionCacheManifestCap verifies oversized masters keep the
// fingerprint but not the bytes, keeping record memory bounded.
func TestResolutionCacheManifestCap(t *testing.T) {
	r, err := New(Config{MaxBrowserSessions: 1, ResolutionCachePath: "-"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	big := "#EXTM3U\n" + string(make([]byte, maxCachedManifestBytes+1))
	rec := newResolutionRecord(&directResolution{
		Source:     "https://cdn.example/big.m3u8",
		Headers:    http.Header{"User-Agent": []string{defaultUserAgent}},
		Allowed:    map[string]bool{"cdn.example": true},
		MasterText: big,
	})
	if rec.manifest != nil {
		t.Fatalf("oversized manifest should not be retained")
	}
	if rec.manifestFP == "" {
		t.Fatalf("fingerprint missing for oversized manifest")
	}
}

// TestNextEpisodeNumber covers padding-preserving episode incrementing.
func TestNextEpisodeNumber(t *testing.T) {
	cases := map[string]string{
		"1": "2", "9": "10", "04": "05", "19": "20", "099": "100",
	}
	for in, want := range cases {
		got, ok := nextEpisodeNumber(in)
		if !ok || got != want {
			t.Errorf("nextEpisodeNumber(%q) = %q,%v; want %q", in, got, ok, want)
		}
	}
	if _, ok := nextEpisodeNumber("abc"); ok {
		t.Errorf("non-numeric episode should report false")
	}
}

// TestResolveSingleFlight verifies a follower joins an in-flight resolve and
// receives its result without running its own chain.
func TestResolveSingleFlight(t *testing.T) {
	r, err := New(Config{MaxBrowserSessions: 1, ResolutionCachePath: "-"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	req := MediaRequest{Type: TV, ID: "119051", Season: "1", Episode: "3", Provider: "movish"}
	fl := &resolveFlight{done: make(chan flightResult, 1)}
	fl.done <- flightResult{proxyURL: "/api/media/proxy/leader.m3u8"}
	r.flights[resolutionKey(req)] = fl

	got, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/api/media/proxy/leader.m3u8" {
		t.Errorf("follower got %q, want the leader's result", got)
	}
}
