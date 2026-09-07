package server

// Request logging: one line per request with method, path, status and
// duration. The media proxy is skipped — it logs through the resolver and a
// single 4K stream would otherwise emit hundreds of lines per minute.

import (
	"log"
	"net/http"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (l *loggingResponseWriter) WriteHeader(status int) {
	if l.status == 0 {
		l.status = status
	}
	l.ResponseWriter.WriteHeader(status)
}

func (l *loggingResponseWriter) Write(p []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	return l.ResponseWriter.Write(p)
}

// Unwrap lets http.NewResponseController see through this wrapper: the media
// proxy lifts its write deadline (long segment streams must outlive the
// server-wide WriteTimeout), and that deadline lives on the underlying
// writer. Without Unwrap the controller returns "feature not supported".
func (l *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return l.ResponseWriter
}

// logRequests wraps a handler with the access log.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lw, r)
		if lw.status == 0 {
			lw.status = http.StatusOK
		}
		if r.URL.Path == "/api/health" {
			return // polled by dashboards/monitors — noise, not signal
		}
		log.Printf("[HTTP] %s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start).Round(time.Millisecond))
	})
}
