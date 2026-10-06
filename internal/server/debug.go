package server

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// debugRoutes mounts Go's pprof profiling endpoints. Registration is
// opt-in (DEBUG_PPROF) and admin-only: CPU/heap profiles and goroutine dumps
// can expose credentials and live session data, so the endpoints must never
// be world-readable (the docs promise they are auth gated).
func (d *Deps) debugRoutes(mux *http.ServeMux) {
	prefix := "/debug/pprof/"
	mux.HandleFunc(prefix, d.guardedPprof(pprof.Index))
	mux.HandleFunc(prefix+"cmdline", d.guardedPprof(pprof.Cmdline))
	mux.HandleFunc(prefix+"profile", d.guardedPprof(pprof.Profile))
	mux.HandleFunc(prefix+"symbol", d.guardedPprof(pprof.Symbol))
	mux.HandleFunc(prefix+"trace", d.guardedPprof(pprof.Trace))
}

// guardedPprof wraps a pprof handler so pprof's own path handling (it serves
// named profiles like /debug/pprof/heap via Index) keeps working, and gates
// access to signed-in admins.
func (d *Deps) guardedPprof(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// pprof.Index dispatches on the trimmed path; other handlers want the
		// prefix stripped so net/http/pprof sees what it expects.
		if !strings.HasPrefix(r.URL.Path, "/debug/pprof") {
			http.NotFound(w, r)
			return
		}
		if _, ok := d.adminGuard(w, r); !ok {
			return
		}
		h(w, r)
	}
}
