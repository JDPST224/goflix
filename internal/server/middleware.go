// Package server wires every HTTP route of GoFlix: catalog JSON, media
// source resolution, the HLS proxy, subtitle endpoints and the SPA assets.
package server

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// writeJSON emits a JSON body with the media-style content type and CORS star.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeError emits the {success:false,error:<msg>} envelope.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

// jsonGate applies the catalog-handler preamble: standard JSON/CORS response
// headers and method validation. It writes the response itself for OPTIONS
// preflights (204) and unsupported methods (405 envelope + Allow), returning
// false so the caller stops.
func jsonGate(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
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
// subtitle handlers. allowMethods is the literal Allow-header value (and the
// accepted set); exposeHeaders adds Access-Control-Expose-Headers (the proxy
// needs Content-Length/Range visible to the player).
func corsGate(w http.ResponseWriter, r *http.Request, allowMethods string, exposeHeaders bool) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	// Use the caller-supplied allowMethods in the CORS header so browser
	// preflights for POST (subtitle registration) are not rejected.
	h.Set("Access-Control-Allow-Methods", allowMethods+", OPTIONS")
	h.Set("Access-Control-Allow-Headers", "*")
	if exposeHeaders {
		h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
	}
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
