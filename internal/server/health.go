package server

import (
	"net/http"
	"time"
)

// healthHandler reports basic service health: cache sizes and uptime.
func (d *Deps) healthHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}

	var movies, shows, popular int
	if d.Store != nil {
		movies, shows, popular = d.Store.Counts()
	}

	resp := map[string]any{
		"status":        "ok",
		"movies":        movies,
		"tvshows":       shows,
		"popular":       popular,
		"uptimeSeconds": int(time.Since(d.StartedAt).Seconds()),
	}
	if d.Resolver != nil {
		// Resolution-cache counters (hits/misses/heals/prewarms/…), so the
		// cache's real-world value is observable.
		resp["resolutionCache"] = d.Resolver.StatsSnapshot()
	}
	// Playback policy delivered to the player (quality cap, 0 = uncapped).
	resp["maxStreamHeight"] = d.MaxStreamHeight
	writeJSON(w, http.StatusOK, resp)
}
