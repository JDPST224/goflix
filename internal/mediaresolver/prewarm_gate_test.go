package mediaresolver

import (
	"context"
	"testing"
)

// TestNextPrewarmTarget covers the TMDB-gated next-episode selection: the
// next episode in season, the season rollover, and the series end.
func TestNextPrewarmTarget(t *testing.T) {
	// Simulated TMDB knowledge: which (id, season, episode) tuples exist.
	exists := map[string]bool{
		"60625:9:10": true,
		"60625:10:1": true,
		"1396:1:1":   true,
		// 1396 has no season 2 in this fixture → series end after s1
	}
	gate := func(id, season, episode string) bool {
		return exists[id+":"+season+":"+episode]
	}
	mk := func(r *Resolver) *Resolver { return r }

	tests := []struct {
		name       string
		req        MediaRequest
		wantTarget MediaRequest
		wantOK     bool
	}{
		{
			name:       "next episode in same season",
			req:        MediaRequest{Type: TV, ID: "60625", Season: "9", Episode: "9", Provider: "cinesrc"},
			wantTarget: MediaRequest{Type: TV, ID: "60625", Season: "9", Episode: "10", Provider: "cinesrc"},
			wantOK:     true,
		},
		{
			name:       "season rollover to next season premiere",
			req:        MediaRequest{Type: TV, ID: "60625", Season: "9", Episode: "10", Provider: "cinesrc"},
			wantTarget: MediaRequest{Type: TV, ID: "60625", Season: "10", Episode: "1", Provider: "cinesrc"},
			wantOK:     true,
		},
		{
			name:   "series end: no next season",
			req:    MediaRequest{Type: TV, ID: "1396", Season: "1", Episode: "1", Provider: "cinesrc"},
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := mk(&Resolver{})
			r.HasEpisodeProvider = func(ctx context.Context, id, season, episode string) bool {
				return gate(id, season, episode)
			}
			got, ok := r.nextPrewarmTarget(context.Background(), tc.req)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got != tc.wantTarget {
				t.Fatalf("target = %+v, want %+v", got, tc.wantTarget)
			}
		})
	}
}

// TestNextPrewarmTargetNoGate verifies the prewarm still attempts when no
// episode provider is configured (deployments without TMDB credentials).
func TestNextPrewarmTargetNoGate(t *testing.T) {
	r := &Resolver{}
	got, ok := r.nextPrewarmTarget(context.Background(), MediaRequest{Type: TV, ID: "1396", Season: "1", Episode: "1"})
	if !ok {
		t.Fatal("expected attempt when HasEpisodeProvider is unset")
	}
	if got.Episode != "2" {
		t.Fatalf("episode = %q, want 2", got.Episode)
	}
}

// TestNextPrewarmTargetNonNumericEpisode: unparseable episode → no target.
func TestNextPrewarmTargetNonNumericEpisode(t *testing.T) {
	r := &Resolver{}
	r.HasEpisodeProvider = func(ctx context.Context, id, season, episode string) bool { return true }
	if _, ok := r.nextPrewarmTarget(context.Background(), MediaRequest{Type: TV, ID: "1", Season: "1", Episode: "pilot"}); ok {
		t.Fatal("non-numeric episode must not prewarm")
	}
}
