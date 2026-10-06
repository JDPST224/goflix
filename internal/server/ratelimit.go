package server

// Per-IP rate limiting for the endpoints where abuse is expensive or
// dangerous: authentication (brute force / account spam) and media source
// resolution (each hit runs a 5-25s resolve chain and consumes the browser
// semaphore). Token buckets refill continuously; buckets for quiet clients
// are swept so memory stays bounded.

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// maxTrackedIPs bounds the bucket map; a sweep drops idle buckets when the
// map grows past it (shared-NAT households and scanners alike).
const maxTrackedIPs = 4096

type ipBucket struct {
	tokens float64
	last   time.Time
}

type ipLimiter struct {
	mu      sync.Mutex
	burst   int
	per     time.Duration
	buckets map[string]*ipBucket
}

func newIPLimiter(burst int, per time.Duration) *ipLimiter {
	return &ipLimiter{burst: burst, per: per, buckets: map[string]*ipBucket{}}
}

// allow reports whether the IP may proceed, consuming one token.
func (l *ipLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) >= maxTrackedIPs {
		l.sweepLocked(now)
	}
	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: float64(l.burst), last: now}
		l.buckets[ip] = b
	} else {
		b.tokens += now.Sub(b.last).Seconds() / l.per.Seconds()
		if b.tokens > float64(l.burst) {
			b.tokens = float64(l.burst)
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked drops buckets idle for longer than a few refill windows.
// Callers hold l.mu.
func (l *ipLimiter) sweepLocked(now time.Time) {
	idle := 10 * l.per
	for ip, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, ip)
		}
	}
}

// clientIP extracts the bare host from RemoteAddr. X-Forwarded-For is
// deliberately ignored: behind no trusted proxy it is trivially spoofable,
// and this limiter's job is to stop untrusted clients.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// guard wraps a handler with the limiter, answering 429 with the standard
// error envelope (and a Retry-After hint) when the bucket is empty.
func (l *ipLimiter) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "30")
			writeError(w, http.StatusTooManyRequests, "Too many requests — slow down")
			return
		}
		next(w, r)
	}
}
