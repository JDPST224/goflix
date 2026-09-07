package server

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

// debugRoutes mounts Go's pprof profiling endpoints. Registration is
// opt-in (DEBUG_PPROF); the app is open-access by design, so only enable
// this on trusted networks.
func debugRoutes(mux *http.ServeMux) {
	prefix := "/debug/pprof/"
	mux.HandleFunc(prefix, guardedPprof(pprof.Index))
	mux.HandleFunc(prefix+"cmdline", guardedPprof(pprof.Cmdline))
	mux.HandleFunc(prefix+"profile", guardedPprof(pprof.Profile))
	mux.HandleFunc(prefix+"symbol", guardedPprof(pprof.Symbol))
	mux.HandleFunc(prefix+"trace", guardedPprof(pprof.Trace))
}

// guardedPprof wraps a pprof handler so pprof's own path handling (it serves
// named profiles like /debug/pprof/heap via Index) keeps working.
func guardedPprof(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// pprof.Index dispatches on the trimmed path; other handlers want the
		// prefix stripped so net/http/pprof sees what it expects.
		if !strings.HasPrefix(r.URL.Path, "/debug/pprof") {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}
