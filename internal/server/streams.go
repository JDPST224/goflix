package server

// Admin dashboard stream monitoring: active playback sessions, the rolling
// bandwidth history behind the dashboard graphs, and per-stream
// pause/stop/block controls. Everything here is admins only.

import (
	"net/http"
	"strings"
)

// adminStreamsHandler lists every active proxy session (admins only).
func (d *Deps) adminStreamsHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if _, ok := d.adminGuard(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"streams": d.Resolver.StreamsSnapshot(),
		"blocked": d.Resolver.BlockedIPs(),
	})
}

// adminStreamStatsHandler reports the rolling bandwidth history (admins only).
func (d *Deps) adminStreamStatsHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	if _, ok := d.adminGuard(w, r); !ok {
		return
	}
	points, bucket, window := d.Resolver.BandwidthHistory()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"points":         points,
		"bucket_seconds": bucket,
		"window_seconds": window,
	})
}

// adminStreamActionHandler dispatches the per-stream controls:
//
//	POST /api/admin/streams/{token}/pause|resume|stop|block
//	POST /api/admin/streams/unblock?ip={ip}
func (d *Deps) adminStreamActionHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	if _, ok := d.adminGuard(w, r); !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/streams/"), "/")
	if rest == "unblock" {
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			writeError(w, http.StatusBadRequest, "Missing ip parameter")
			return
		}
		d.Resolver.UnblockIP(ip)
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "Unknown stream action")
		return
	}
	token := strings.TrimSuffix(parts[0], ".m3u8")
	var ok bool
	switch parts[1] {
	case "pause":
		ok = d.Resolver.PauseStream(token)
	case "resume":
		ok = d.Resolver.ResumeStream(token)
	case "stop":
		ok = d.Resolver.StopStream(token)
	case "block":
		var ip string
		if ip, ok = d.Resolver.BlockStream(token); ok {
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "ip": ip})
			return
		}
	default:
		writeError(w, http.StatusNotFound, "Unknown stream action")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Stream not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
