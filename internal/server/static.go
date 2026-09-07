package server

import (
	"compress/gzip"
	"net/http"
	"path"
	"strings"
)

// staticFileServer wraps http.FileServer with hardening and performance for
// the SPA assets:
//   - Cache-Control: no-cache for app assets — browsers revalidate
//     ETags/Last-Modified after every deploy, so JS/CSS updates show up
//     without a hard refresh; /vendor/ gets an immutable year-long policy
//     instead (reference it with a versioned query for cache busting);
//   - gzip for text assets when the client accepts it — main.js and
//     index.css dominate the first load;
//   - X-Content-Type-Options: nosniff — blocks MIME-sniffing attacks;
//   - directory listings (/css/, /js/) return 404 instead of exposing the
//     file tree, while "/" still serves index.html.
func staticFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Directory requests end in "/" — refuse them before FileServer can
		// render an index listing. (path.Clean would strip the slash, so the
		// raw URL path is checked here.)
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/vendor/") {
			// Vendored libraries are pinned by their versioned query string.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if compressibleStatic(r.URL.Path) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			gz := gzipWriterPool.Get().(*gzip.Writer)
			gz.Reset(w)
			defer func() {
				_ = gz.Close()
				gzipWriterPool.Put(gz)
			}()
			files.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// compressibleStatic reports whether a static path is a text format worth
// gzipping.
func compressibleStatic(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".css", ".html", ".svg", ".json", ".txt", ".m3u8", ".vtt":
		return true
	}
	return false
}
