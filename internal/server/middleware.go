// Package server wires every HTTP route of GoFlix: catalog JSON, media
// source resolution, the HLS proxy, subtitle endpoints and the SPA assets.
package server

import (
	"compress/gzip"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

// writeJSON emits a JSON body with the media-style content type. No CORS
// headers: the app is same-origin by design, and an open CORS policy on an
// open-access server is pure attack surface.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("[Server] json encode error: %v", err)
	}
}


// writeError emits the {success:false,error:<msg>} envelope.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

// jsonGate applies the catalog-handler preamble: standard JSON response
// headers and method validation. It writes the response itself for OPTIONS
// preflights (204) and unsupported methods (405 envelope + Allow), returning
// false so the caller stops. Cache-Control is intentionally NOT set here —
// each handler owns its own caching policy.
func jsonGate(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return false
	case http.MethodGet, http.MethodHead:
		return true
	default:
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return false
	}
}

// corsGate implements the lighter preamble shared by media-source, proxy and
// subtitle handlers: method validation only. Cross-origin clients are not
// supported — the app and its media are same-origin.
func corsGate(w http.ResponseWriter, r *http.Request, allowMethods string, exposeHeaders bool) bool {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	for _, m := range strings.Split(allowMethods, ", ") {
		if r.Method == m {
			return true
		}
	}
	w.Header().Set("Allow", allowMethods+", OPTIONS")
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	return false
}

// gzipWriterPool recycles gzip writers for compressed API responses.
var gzipWriterPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		return gz
	},
}

// gzipResponseWriter transparently gzips everything written through it.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.wroteHeader {
		return
	}
	header := g.Header()
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	g.wroteHeader = true
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	return g.gz.Write(p)
}

func (g *gzipResponseWriter) Flush() {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	_ = g.gz.Flush()
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap keeps http.NewResponseController working through the gzip wrapper,
// mirroring loggingResponseWriter — anything wrapped inside the chain that
// touches deadlines or flush controls needs the escape hatch.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

// GzipCatalog compresses catalog JSON responses for clients advertising gzip,
// always marking Vary: Accept-Encoding so caches never hand a compressed
// response to a client that cannot decode it.
func GzipCatalog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			_ = gz.Close()
			gzipWriterPool.Put(gz)
		}()
		next(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	}
}
