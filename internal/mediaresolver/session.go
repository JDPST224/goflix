package mediaresolver

// Proxy sessions: token-scoped upstream state shared by the
// resolver, prefetcher and proxy stream.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultSessionTTL = 4 * time.Hour

// copyBufPool reuses 128 KB scratch buffers for proxying segment responses,
// providing high throughput and minimizing context switches for 4K and 1080p streams.
var copyBufPool = &sync.Pool{
	New: func() any {
		b := make([]byte, 128*1024)
		return &b
	},
}

type proxySession struct {
	source string
	// reqKey is the resolution-cache key of the request that created this
	// session, so InvalidateResolution can drop the right record.
	reqKey    string
	headers   http.Header
	allowed   map[string]bool
	expiresAt time.Time
	// liveFetches counts in-flight player-facing upstream fetches (uncached).
	// While it is above zero the read-ahead pump holds new prefetch work so
	// playback gets the bandwidth instead of competing with the filler.
	liveFetches atomic.Int32
	// warmer drives the per-session read-ahead; nil until ensureWarmer runs.
	warmer *streamWarmer
	// subs holds external subtitle renditions registered by the frontend for
	// native-HLS engines; injected into master manifests on rewrite.
	subs []SubRendition
	// subsState tracks backend attachment progress (0 none, 1 pending, 2
	// done) so the first manifest fetch can wait briefly for the provider
	// ladder instead of serving a manifest without renditions.
	subsState atomic.Int32
	// subsDone is closed by SetSubRenditions when the subtitle ladder
	// completes (successfully or not). waitForSubs selects on it to avoid
	// busy-polling with time.Sleep.
	subsDone chan struct{}
}

// inflightFetch is one shared upstream download in progress. Joiners close
// over done and read the result once it closes.
type inflightFetch struct {
	done  chan struct{}
	entry *cacheEntry
	err   error
}

func (r *Resolver) newSession(reqKey, source string, headers http.Header, allowed map[string]bool) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	// A Resolve that was already in flight when Close ran must not re-insert sessions.
	if r.closed {
		return "", errors.New("resolver is closed")
	}
	var cancels []context.CancelFunc
	for tok, session := range r.sessions {
		if now.After(session.expiresAt) {
			if session.warmer != nil && session.warmer.cancel != nil {
				cancels = append(cancels, session.warmer.cancel)
			}
			delete(r.sessions, tok)
		}
	}
	// Enforce the session cap after evicting expired sessions so a long-running
	// server doesn't hit the cap due to accumulated stale entries.
	if len(r.sessions) >= r.maxSessions() {
		return "", fmt.Errorf("session cap (%d) reached — too many concurrent viewers", r.maxSessions())
	}
	if headers == nil {
		headers = make(http.Header)
	}
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", defaultUserAgent)
	}
	r.sessions[token] = &proxySession{
		source:    source,
		reqKey:    reqKey,
		headers:   cloneHeader(headers),
		allowed:   allowed,
		expiresAt: now.Add(r.sessionTTL()),
		subsDone:  make(chan struct{}),
	}
	// CancelFunc is non-blocking, so firing these while holding r.mu cannot
	// deadlock — it only closes the read-ahead pipelines' done channels.
	for _, c := range cancels {
		c()
	}
	return token, nil
}


func cloneHeader(in http.Header) http.Header {
	out := make(http.Header)
	for k, v := range in {
		for _, x := range v {
			out.Add(k, x)
		}
	}
	return out
}

func mergeHeaders(primary, fallback http.Header) http.Header {
	out := cloneHeader(primary)
	for _, key := range playbackHeaders {
		if out.Get(key) == "" && fallback.Get(key) != "" {
			out.Set(key, fallback.Get(key))
		}
	}
	return out
}

func mergeResponseCookies(headers http.Header, cookies []*http.Cookie) {
	values := make(map[string]string)
	for _, part := range strings.Split(headers.Get("Cookie"), ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) == 2 {
			values[strings.TrimSpace(pieces[0])] = strings.TrimSpace(pieces[1])
		}
	}
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name != "" {
			values[cookie.Name] = cookie.Value
		}
	}
	parts := make([]string, 0, len(values))
	for name, value := range values {
		parts = append(parts, name+"="+value)
	}
	if len(parts) > 0 {
		headers.Set("Cookie", strings.Join(parts, "; "))
	}
}

// ── Body cache & read-ahead prefetcher ──────────────────────────────────────
//
// Playback must not stall on the proxy itself: fully-read upstream bodies are
// kept in a byte-bounded LRU, and each session runs a read-ahead that keeps
// upcoming segments downloaded ahead of the playhead.

func cloneAllowed(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
