package server

import (
	"net/http"
	"strings"
)

// staticFileServer wraps http.FileServer with hardening for the SPA assets:
//   - Cache-Control: no-cache — browsers revalidate ETags/Last-Modified after
//     every deploy, so JS/CSS updates show up without a hard refresh;
//   - X-Content-Type-Options: nosniff — blocks MIME-sniffing attacks;
//   - directory listings (/css/, /js/) return 404 instead of exposing the
//     file tree, while "/" still serves index.html.
func staticFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		// Directory requests end in "/" — refuse them before FileServer can
		// render an index listing. (path.Clean would strip the slash, so the
		// raw URL path is checked here.)
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
