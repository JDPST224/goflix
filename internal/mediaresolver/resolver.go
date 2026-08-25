package mediaresolver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

type MediaType string

const (
	Movie MediaType = "movie"
	TV    MediaType = "tv"
)

type MediaRequest struct {
	Type                MediaType
	ID, Season, Episode string
	Provider            string
}

type hlsCandidate struct {
	url, contentType string
	status           int64
	headers          http.Header
}

type Config struct {
	TargetOrigin, VidKingOrigin, VidLoveOrigin string
	BrowserHeadless                            bool
	BrowserTimeout, SourceResolutionTimeout    time.Duration
	MaxBrowserSessions                         int
	BrowserExecutable                          string
	// SessionTTL is the sliding lifetime of a proxy session. Every proxied
	// request extends it, so playback is not cut off mid-stream.
	// Defaults to 4 hours when <= 0.
	SessionTTL time.Duration
	// CacheMaxBytes bounds the in-memory body cache that serves repeated
	// manifests/segments and read-ahead data from RAM. Defaults to 256 MiB
	// when <= 0 (CACHE_MAX_MB).
	CacheMaxBytes int64
}

// playbackHeaders are the browser headers captured during resolution and
// replayed against the upstream so the CDN sees the same playback context.
// playbackHeaders is forwarded from the proxy session to every upstream
// fetch. Sec-Ch-Ua* client-hint headers are included because some provider
// endpoints (vixsrc /playlist/) reject requests without them.
var playbackHeaders = []string{
	"User-Agent", "Referer", "Origin", "Cookie", "Accept", "Accept-Language",
	"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform",
}

// maxManifestBytes caps upstream manifest bodies. Anything larger is rejected
// rather than silently truncated into a corrupt playlist.
const maxManifestBytes = 8 << 20

// candidateGracePeriod is how long the resolver keeps collecting HLS
// candidates after the first one is seen, so a broken/decoy manifest can be
// swapped for a better one during validation. High-confidence candidates (see
// confidentCandidateScore) skip the wait entirely.
const candidateGracePeriod = 1500 * time.Millisecond

// confidentCandidateScore is the candidateScore at or above which a captured
// HLS URL is treated as the canonical primary manifest (/playlist/,
// master.m3u8, manifest.m3u8) and validated immediately instead of waiting
// out candidateGracePeriod — waiting would only add startup delay.
const confidentCandidateScore = 85

// manifestProbeBytes caps how much of a manifest is read during validation.
const manifestProbeBytes = 64 << 10

// Playback smoothness knobs. The body cache keeps fully-read upstream bodies
// (playlists and media segments) in RAM so the player is served from memory
// instead of waiting on a cold upstream fetch, and each session runs a
// read-ahead that keeps upcoming segments downloaded before they are asked
// for — upstream jitter then never reaches the player as buffering.
const (
	// defaultCacheMaxBytes bounds the body cache; override with CACHE_MAX_MB.
	defaultCacheMaxBytes = 256 << 20
	// maxCachedSegmentBytes caps a single cached segment. Larger bodies are
	// still streamed to the player, just never retained.
	maxCachedSegmentBytes = 64 << 20
	// cacheEntryTTL is the lifetime of a cached segment; segments are immutable.
	cacheEntryTTL = 6 * time.Hour
	// playlistCacheTTL keeps cached manifests fresh enough that live-style
	// playlist refreshes still reach upstream.
	playlistCacheTTL = 5 * time.Second
	// prefetchLookahead is how many segments ahead of the furthest segment
	// handed to the player the read-ahead keeps downloaded. Eighteen segments
	// gives the pump enough candidate targets to keep five-plus downloads
	// busy even on playlists with very short segments, where ten would starve
	// the window faster than the pump could drain it.
	prefetchLookahead = 18
	// Provider CDNs reward parallel connections rather than capping total
	// bandwidth: probes showed a single sequential connection sustaining
	// 1.7-23 Mbps depending on provider while 4-6 parallel connections
	// aggregated 51-800+ Mbps. Steady state therefore keeps five read-ahead
	// downloads running alongside the player's local fetches, and the
	// initial burst runs eight to fill the first window fast. The
	// liveFetches knob keeps the player prioritized: while a player-facing
	// fetch is active, read-ahead drops to prefetchLiveMaxInflight parallel
	// downloads instead of five, and full parallelism resumes the moment the
	// last live fetch drains.
	prefetchMaxInflight     = 5
	prefetchInitialInflight = 8
	// prefetchFailureBackoff waits before re-attempting a segment that failed,
	// so one bad segment cannot spin the prefetcher into a retry storm.
	prefetchFailureBackoff = 30 * time.Second
	// prefetchLiveMaxInflight caps read-ahead parallelism while a player
	// fetch is active — the reduced player-priority mode in pumpPrefetch.
	prefetchLiveMaxInflight = 2
)

type Resolver struct {
	cfg      Config
	sem      chan struct{}
	mu       sync.Mutex
	closed   bool
	done     chan struct{} // closed by Close() to wake the sweeper goroutine immediately
	sessions map[string]*proxySession
	// transport is shared across proxy requests for connection reuse.
	transport *http.Transport
	// blockCache memoizes per-hostname SSRF checks (hostname -> blocked).
	blockCache sync.Map
	// cache holds fully-read upstream bodies (playlists, segments) so repeat
	// requests and read-ahead hits are served from RAM.
	cache *bodyCache
	// inflight dedupes concurrent upstream fetches of the same URL across the
	// proxy's live path and the read-ahead warmer: a second requester waits
	// for the first to finish instead of double-downloading the same bytes.
	inflight sync.Map // string(absolute URL) -> *inflightFetch
	// SubRenditionProvider, when set, is called in a background goroutine
	// right after a session is created for providers that ship no embedded
	// subtitle renditions (vidking/vidlove). Whatever it returns is embedded
	// into the master manifest as TYPE=SUBTITLES entries so native-HLS
	// engines (smart TVs) see the tracks without any frontend help.
	SubRenditionProvider func(ctx context.Context, req MediaRequest) []SubRendition
}

func New(cfg Config) (*Resolver, error) {
	if cfg.MaxBrowserSessions < 1 {
		return nil, errors.New("MAX_BROWSER_SESSIONS must be greater than zero")
	}
	if cfg.TargetOrigin == "" {
		cfg.TargetOrigin = "https://vixsrc.to"
	}
	if cfg.VidKingOrigin == "" {
		cfg.VidKingOrigin = "https://www.vidking.net"
	}
	if cfg.VidLoveOrigin == "" {
		cfg.VidLoveOrigin = "https://player.vidlove.cc"
	}
	cfg.TargetOrigin = strings.TrimRight(cfg.TargetOrigin, "/")
	cfg.VidKingOrigin = strings.TrimRight(cfg.VidKingOrigin, "/")
	cfg.VidLoveOrigin = strings.TrimRight(cfg.VidLoveOrigin, "/")
	for k, v := range map[string]string{"VIXSRC_ORIGIN": cfg.TargetOrigin, "VIDKING_ORIGIN": cfg.VidKingOrigin, "VIDLOVE_ORIGIN": cfg.VidLoveOrigin} {
		u, e := url.Parse(v)
		if e != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("%s must be an HTTPS origin", k)
		}
		if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("%s must be a bare HTTPS origin (no path, query, or fragment)", k)
		}
	}
	cacheMax := cfg.CacheMaxBytes
	if cacheMax <= 0 {
		cacheMax = defaultCacheMaxBytes
	}
	r := &Resolver{
		cfg:      cfg,
		sem:      make(chan struct{}, cfg.MaxBrowserSessions),
		done:     make(chan struct{}),
		sessions: make(map[string]*proxySession),
		cache:    newBodyCache(cacheMax),
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConns:          512,
			MaxIdleConnsPerHost:   64,
			MaxConnsPerHost:       128,
			IdleConnTimeout:       120 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ForceAttemptHTTP2:     true,
			// Health-check HTTP/2 connections that go quiet with PING frames
			// and close them when the peer stops answering. Without this, one
			// dead connection stalls every stream multiplexed onto it until
			// ResponseHeaderTimeout fires ("http2: timeout awaiting response
			// headers" — seen on vidlove's CDN under load).
			HTTP2:           &http.HTTP2Config{SendPingTimeout: 10 * time.Second},
			ReadBufferSize:  128 * 1024,
			WriteBufferSize: 128 * 1024,
		},
	}
	// Periodically sweep expired proxy sessions so memory doesn't grow
	// indefinitely. The done channel lets Close() wake the goroutine instantly
	// instead of waiting up to 5 minutes for the next tick.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-r.done:
				return
			case <-ticker.C:
				now := time.Now()
				var cancels []context.CancelFunc
				r.mu.Lock()
				for tok, s := range r.sessions {
					if now.After(s.expiresAt) {
						if s.warmer != nil && s.warmer.cancel != nil {
							cancels = append(cancels, s.warmer.cancel)
						}
						delete(r.sessions, tok)
					}
				}
				r.mu.Unlock()
				for _, c := range cancels {
					c() // stop the read-ahead of every expired session
				}
				// Evict expired SSRF-verdict entries as well — hosts seen once
				// in a manifest and never revisited would otherwise accumulate
				// forever (they are only deleted lazily on access).
				r.blockCache.Range(func(key, value any) bool {
					if entry, ok := value.(dnsCacheEntry); ok && now.After(entry.expires) {
						r.blockCache.Delete(key)
					}
					return true
				})
			}
		}
	}()
	return r, nil
}

func (r *Resolver) Close() {
	var cancels []context.CancelFunc
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		for _, s := range r.sessions {
			if s.warmer != nil && s.warmer.cancel != nil {
				cancels = append(cancels, s.warmer.cancel)
			}
		}
		r.sessions = make(map[string]*proxySession)
		close(r.done) // wake the sweeper goroutine immediately
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c() // stop every session's read-ahead pipeline
	}
	r.transport.CloseIdleConnections()
}

func (r *Resolver) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

func (r *Resolver) sessionTTL() time.Duration {
	if r.cfg.SessionTTL > 0 {
		return r.cfg.SessionTTL
	}
	return defaultSessionTTL
}

func (r *Resolver) Resolve(parent context.Context, req MediaRequest) (string, error) {
	if err := validateRequest(req); err != nil {
		return "", err
	}
	// Fail fast before queueing on the browser semaphore so callers of a
	// closed resolver do not block until their context expires.
	if r.isClosed() {
		return "", errors.New("resolver is closed")
	}
	// vidlove and vixsrc are resolved directly against their own endpoints
	// instead of a browser scrape — faster startup, and quality is guaranteed:
	// vidlove's embed hands its player the master manifest inline (scraping can
	// never observe it), and vixsrc's master playlist URL is reconstructible
	// from its embed page (see vixsrc.go). Both fall back to the generic
	// browser path when their direct attempt fails.
	switch strings.ToLower(strings.TrimSpace(req.Provider)) {
	case "", "vixsrc":
		if proxyURL, ok := r.tryVixsrcDirect(parent, req); ok {
			return proxyURL, nil
		}
	case "vidking":
		if proxyURL, ok := r.tryVidkingDirect(parent, req); ok {
			return proxyURL, nil
		}
	case "vidlove":
		if proxyURL, ok := r.tryVidloveDirect(parent, req); ok {
			return proxyURL, nil
		}
	}
	// A fresh browser run (and proxy session) is created for every Resolve
	// call; cached browser contexts cannot be reused safely across calls.
	// The semaphore slot is held across all retry attempts so the total number
	// of concurrent browser processes never exceeds MaxBrowserSessions.
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-parent.Done():
		return "", parent.Err()
	}
	target, err := r.targetURL(req)
	if err != nil {
		return "", err
	}
	// BrowserTimeout is the single deadline authority. SourceResolutionTimeout
	// is retained in the config for backward compatibility but is no longer
	// applied here — wrapping with it would clamp BrowserTimeout and make that
	// config field useless.
	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if parent.Err() != nil {
			return "", parent.Err()
		}
		// Brief backoff before retrying so a failing provider is not hit
		// twice in rapid succession.
		if attempt > 1 {
			select {
			case <-time.After(750 * time.Millisecond):
			case <-parent.Done():
				return "", parent.Err()
			}
		}
		log.Printf("[MediaResolver] Resolving %s (attempt %d/%d)", redactQuery(target), attempt, maxAttempts)
		source, headers, allowed, err := r.resolveInBrowser(parent, target)
		if err == nil {
			token, err := r.newSession(source, headers, allowed)
			if err != nil {
				return "", err
			}
			// Providers without embedded renditions get their subtitles
			// resolved server-side and embedded into the master manifest;
			// warm the chain so playback begins from RAM either way.
			r.attachAndWarm(token, req)
			return "/api/media/proxy/" + token + ".m3u8", nil
		}
		lastErr = err
		log.Printf("[MediaResolver] Attempt %d/%d failed: %v", attempt, maxAttempts, err)
	}
	return "", lastErr
}

func (r *Resolver) targetURL(req MediaRequest) (string, error) {
	origin := r.cfg.TargetOrigin
	if strings.EqualFold(strings.TrimSpace(req.Provider), "vidking") {
		origin = r.cfg.VidKingOrigin
	} else if strings.EqualFold(strings.TrimSpace(req.Provider), "vidlove") {
		origin = r.cfg.VidLoveOrigin
	}
	base, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	var p string
	switch req.Type {
	case Movie:
		p = "/movie/" + req.ID
	case TV:
		p = "/tv/" + req.ID + "/" + req.Season + "/" + req.Episode
	default:
		return "", errors.New("unsupported media type")
	}
	if strings.EqualFold(strings.TrimSpace(req.Provider), "vidking") || strings.EqualFold(strings.TrimSpace(req.Provider), "vidlove") {
		if req.Type == Movie {
			p = "/embed/movie/" + req.ID
		} else {
			p = "/embed/tv/" + req.ID + "/" + req.Season + "/" + req.Episode
		}
	}
	u, err := base.Parse(p)
	if err != nil {
		return "", err
	}
	if u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) {
		return "", errors.New("target escaped configured origin")
	}
	return u.String(), nil
}

func validateRequest(r MediaRequest) error {
	switch r.Type {
	case Movie:
		if !validNumeric(r.ID) {
			return errors.New("invalid movie ID")
		}
	case TV:
		if !validNumeric(r.ID) || !validNumeric(r.Season) || !validNumeric(r.Episode) {
			return errors.New("invalid TV episode parameters")
		}
	default:
		return errors.New("unsupported media type")
	}
	switch strings.ToLower(strings.TrimSpace(r.Provider)) {
	case "", "vixsrc", "vidking", "vidlove":
		return nil
	default:
		return errors.New("unsupported provider")
	}
}

func validNumeric(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
