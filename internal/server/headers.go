package server

// Security headers + CSRF origin guard, applied to every response.

import (
	"net/http"
)

// csp is the content security policy for the SPA. Notes:
//   - img data: for inline SVG placeholders, https: for the TMDB CDN (detail
//     modal and episode stills are passthrough payloads);
//   - media blob: for MSE playback, worker/child blob: for hls.js workers
//     and blob subtitle tracks;
//   - style 'unsafe-inline' for style attributes used across the UI;
//   - fonts come from Google Fonts.
const csp = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data: https:; " +
	"media-src 'self' blob:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"child-src 'self' blob:; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'"

// securityHeaders stamps the hardening headers on every response. The
// content type is not sniffed, framing is refused, referrers are stripped,
// and the CSP above constrains what a successful injection could load.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// sameOriginGuard rejects state-changing requests that carry a foreign
// Origin header — belt and braces next to the SameSite=Lax session cookie.
// Requests without an Origin (curl, server-to-server, same-origin GETs)
// pass through; browsers always send Origin on cross-site POSTs.
func sameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				// Our own host over either scheme is legitimate; anything
				// else is a cross-site form of abuse.
				if origin != "http://"+r.Host && origin != "https://"+r.Host {
					writeError(w, http.StatusForbidden, "Cross-origin request rejected")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
