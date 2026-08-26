package mediaresolver

// Read-ahead stream warmer: prefetches manifests/segments ahead of
// the player.

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"
)

// streamWarmer tracks the ordered segment list of a session's active media
// playlist and keeps up to prefetchLookahead segments ahead of the furthest
// segment handed to the player downloaded into the body cache.
type streamWarmer struct {
	mu       sync.Mutex
	segments []string       // absolute segment URLs in playlist order
	index    map[string]int // segment URL -> position in segments
	served   int            // segments[0:served] have been handed to the player
	inflight map[string]bool
	failedAt map[string]time.Time
	ctx      context.Context
	cancel   context.CancelFunc
}

func (w *streamWarmer) context() context.Context { return w.ctx }

// ensureWarmer lazily attaches the warmup/read-ahead pipeline to a session.
// Called from the resolve path right after session creation, and defensively
// from the proxy path on the first request.
func (r *Resolver) ensureWarmer(s *proxySession) {
	r.mu.Lock()
	if s.warmer != nil || r.closed {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.warmer = &streamWarmer{
		index:    make(map[string]int),
		inflight: make(map[string]bool),
		failedAt: make(map[string]time.Time),
		ctx:      ctx,
		cancel:   cancel,
	}
	w := s.warmer
	r.mu.Unlock()
	go r.warmManifestChain(w.context(), s)
}

// startWarmup kicks off the read-ahead pipeline for a freshly created session.
func (r *Resolver) startWarmup(token string) {
	r.mu.Lock()
	s := r.sessions[token]
	r.mu.Unlock()
	if s != nil {
		r.ensureWarmer(s)
	}
}

// warmManifestChain runs right after a session is created: it downloads the
// resolved manifest chain (source → highest-quality variant media playlist)
// through the body cache so the player's first manifest requests are served
// from memory, then leaves the segment read-ahead running.
func (r *Resolver) warmManifestChain(ctx context.Context, s *proxySession) {
	text, base, ok := r.warmPlaylist(ctx, s, s.source, maxManifestBytes)
	if !ok || ctx.Err() != nil {
		return
	}
	if strings.Contains(text, "#EXTINF") {
		r.registerSegments(s, base, text)
		return
	}
	// Master playlist: follow the same variant forceHighestQuality picks, so
	// the media playlist is warm by the time the player asks for it.
	variant := lastPlaylistLine(forceHighestQuality(text))
	variantURL := resolveMediaURL(base, variant)
	if variantURL == "" {
		return
	}
	vtext, vbase, ok := r.warmPlaylist(ctx, s, variantURL, maxManifestBytes)
	if ok && strings.Contains(vtext, "#EXTINF") {
		r.registerSegments(s, vbase, vtext)
	}
}

// warmPlaylist fetches one playlist through the body cache and returns its
// text and base URL for further parsing.
func (r *Resolver) warmPlaylist(ctx context.Context, s *proxySession, raw string, limit int64) (string, *url.URL, bool) {
	fetch, err := r.fetchForCache(ctx, s, raw, limit)
	if err != nil {
		return "", nil, false
	}
	base, err := url.Parse(raw)
	if err != nil {
		return "", nil, false
	}
	text := string(fetch.data)
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return "", nil, false
	}
	return text, base, true
}

// pumpPrefetch schedules the next read-ahead downloads: the earliest segments
// in the lookahead window that are neither cached, in flight, nor recently
// failed. A small bounded parallelism (two in steady state, four while the
// initial window is still filling) keeps one slow segment from serializing
// playback while remaining gentle on the CDN. Targets are always picked
// front-to-back from the served cursor, so download order follows playlist
// order even when two run at once.
func (r *Resolver) pumpPrefetch(s *proxySession) {
	w := s.warmer
	if w == nil {
		return
	}
	w.mu.Lock()
	if len(w.segments) == 0 {
		w.mu.Unlock()
		return
	}
	windowEnd := w.served + prefetchLookahead
	if windowEnd > len(w.segments) {
		windowEnd = len(w.segments)
	}
	// Player-priority throttle: while a player-facing fetch is downloading
	// uncached data upstream, read-ahead keeps running at reduced parallelism
	// rather than stopping outright. A full stop let the player outrun the
	// window — once it did, every segment arrived through a fresh live fetch,
	// and each of those exposes playback to a complete upstream round trip,
	// which WAN loss turns into visible stalls. Two prefetch downloads barely
	// dent upstream capacity (these CDNs cap per connection anyway) while
	// keeping the cache ahead of the player; full parallelism resumes when the
	// last live fetch drains (the proxy re-kicks this pump at that moment).
	maxInflight := prefetchMaxInflight
	if w.served == 0 {
		maxInflight = prefetchInitialInflight
	}
	if s.liveFetches.Load() > 0 && maxInflight > prefetchLiveMaxInflight {
		maxInflight = prefetchLiveMaxInflight
	}
	now := time.Now()
	inflightCount := 0
	var targets []string
	for i := w.served; i < windowEnd; i++ {
		raw := w.segments[i]
		if w.inflight[raw] {
			inflightCount++
			continue
		}
		if inflightCount+len(targets) >= maxInflight {
			break
		}
		if _, cached := r.cache.get(raw); cached {
			continue
		}
		// Skip URLs another goroutine (player live fetch, sibling prefetch)
		// is already downloading via the shared single-flight layer.
		if _, busy := r.inflight.Load(raw); busy {
			continue
		}
		if t, bad := w.failedAt[raw]; bad && now.Sub(t) < prefetchFailureBackoff {
			continue
		}
		w.inflight[raw] = true
		targets = append(targets, raw)
	}
	w.mu.Unlock()
	for _, target := range targets {
		go func(target string) {
			_, err := r.fetchForCache(w.context(), s, target, maxCachedSegmentBytes)
			w.mu.Lock()
			delete(w.inflight, target)
			if err != nil {
				w.failedAt[target] = time.Now()
			}
			w.mu.Unlock()
			r.pumpPrefetch(s)
		}(target)
	}
}
