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

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"movies":        movies,
		"tvshows":       shows,
		"popular":       popular,
		"uptimeSeconds": int(time.Since(d.StartedAt).Seconds()),
	})
}
