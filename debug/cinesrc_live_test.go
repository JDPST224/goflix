package debug

// Live end-to-end tests for the cinesrc embedded-JS engine. All are skipped
// unless CINESRC_LIVE=1, since they depend on an external service and
// perform real proof-of-work:
//
//	CINESRC_LIVE=1 go test -v -run TestLiveCinesrc ./debug -timeout 5m

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"goflix/internal/mediaresolver"
	"goflix/internal/mediaresolver/cinesrcjs"
)

func liveEnabled(t *testing.T) bool {
	t.Helper()
	if os.Getenv("CINESRC_LIVE") == "1" {
		return true
	}
	t.Skip("CINESRC_LIVE not set")
	return false
}

func liveEngine(t *testing.T) *cinesrcjs.Resolver {
	return &cinesrcjs.Resolver{
		Logf: func(f string, a ...any) { t.Logf(f, a...) },
	}
}

// TestLiveCinesrcEngineMovie: embedded engine, movie 550.
func TestLiveCinesrcEngineMovie(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	res, err := liveEngine(t).Resolve(context.Background(), "movie", "550", "", "", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Source == "" {
		t.Fatal("empty source")
	}
	t.Logf("provider=%s source=%s", res.Provider, res.Source)
}

// TestLiveCinesrcEngineTV: TV challenges bind season/episode. A non-first
// episode is included on purpose: the module binds the response to the
// location's season/episode, and a query-less location used to make every
// episode but the default S1E1 fail with resp_media_mismatch.
func TestLiveCinesrcEngineTV(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	cases := []struct {
		name     string
		id, s, e string
	}{
		{"s1e1", "1396", "1", "1"},
		{"s1e2", "1396", "1", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := liveEngine(t).Resolve(context.Background(), "tv", tc.id, tc.s, tc.e, nil)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			t.Logf("provider=%s source=%s", res.Provider, res.Source)
		})
	}
}

// TestLiveCinesrcEngineTVHighSeason: two-digit season/episode values.
func TestLiveCinesrcEngineTVHighSeason(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	res, err := liveEngine(t).Resolve(context.Background(), "tv", "60625", "9", "10", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("provider=%s source=%s", res.Provider, res.Source)
}

// TestLiveCinesrcEngineMultiServer: a title whose stream is not on the first
// provider — the engine must fall through the ranked provider list with a
// fresh challenge session per server.
func TestLiveCinesrcEngineMultiServer(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	res, err := liveEngine(t).Resolve(context.Background(), "movie", "315635", "", "", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("provider=%s source=%s", res.Provider, res.Source)
}

// TestLiveCinesrcBridge: the integrated mediaresolver path — embedded engine
// first, master playlist validation, proxy session minting — through the
// public API.
func TestLiveCinesrcBridge(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	r, err := mediaresolver.New(mediaresolver.Config{MaxBrowserSessions: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	proxyURL, err := r.Resolve(ctx, mediaresolver.MediaRequest{Type: mediaresolver.Movie, ID: "550", Provider: "cinesrc"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(proxyURL, "/api/media/proxy/") {
		t.Fatalf("unexpected proxy url %q", proxyURL)
	}
	t.Logf("proxy url=%s", proxyURL)
}
