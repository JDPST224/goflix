package mediaresolver

// Body cache and playlist segment bookkeeping backing the proxy.

import (
	"compress/gzip"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// cacheEntry is one fully-read upstream body held in the body cache.
type cacheEntry struct {
	key         string
	data        []byte
	status      int
	contentType string
	expiresAt   time.Time
}

// bodyCache is a byte-bounded LRU of upstream bodies keyed by absolute URL.
// Entries are shared across sessions, so two viewers of the same title reuse
// each other's downloaded data.
type bodyCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	order    *list.List // front = most recently used; elements hold *cacheEntry
	items    map[string]*list.Element
}

func newBodyCache(maxBytes int64) *bodyCache {
	return &bodyCache{maxBytes: maxBytes, order: list.New(), items: make(map[string]*list.Element)}
}

func (c *bodyCache) get(key string) (*cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if time.Now().After(e.expiresAt) {
		c.removeLocked(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return e, true
}

// put stores a body, evicting least-recently-used entries to stay within the
// byte budget. Oversized bodies are refused rather than allowed to thrash the
// whole cache.
func (c *bodyCache) put(e *cacheEntry) {
	if int64(len(e.data)) > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[e.key]; ok {
		old := el.Value.(*cacheEntry)
		c.curBytes -= int64(len(old.data))
		el.Value = e
		c.order.MoveToFront(el)
	} else {
		c.items[e.key] = c.order.PushFront(e)
	}
	c.curBytes += int64(len(e.data))
	for c.curBytes > c.maxBytes {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.removeLocked(oldest)
	}
}

// removeLocked drops one element; callers must hold c.mu.
func (c *bodyCache) removeLocked(el *list.Element) {
	e := el.Value.(*cacheEntry)
	c.curBytes -= int64(len(e.data))
	c.order.Remove(el)
	delete(c.items, e.key)
}

// cachedFetch is a fully-read upstream body returned by fetchForCache.
type cachedFetch struct {
	data        []byte
	contentType string
	status      int
}

// fetchForCache returns an upstream body, serving it from the body cache when
// present and storing fresh downloads for the next reader. It is the shared
// fetch path for the session warmup and the segment read-ahead, so everything
// it downloads benefits every viewer of the same resource.
// fetchForCache fetches an upstream body into the RAM cache, collapsing
// concurrent requests for the same URL into one shared download. The warmer
// and the proxy's live path both route through here so a player request that
// races a read-ahead prefetch joins it instead of duplicating the transfer.
func (r *Resolver) fetchForCache(ctx context.Context, s *proxySession, rawURL string, limit int64) (*cachedFetch, error) {
	c := &inflightFetch{done: make(chan struct{})}
	v, loaded := r.inflight.LoadOrStore(rawURL, c)
	if loaded {
		owner := v.(*inflightFetch)
		timer := time.NewTimer(inflightJoinWait)
		defer timer.Stop()
		select {
		case <-owner.done:
			if owner.err != nil {
				return nil, owner.err
			}
			return &cachedFetch{
				data:        owner.entry.data,
				contentType: owner.entry.contentType,
				status:      owner.entry.status,
			}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			// The shared download is stalled (upstream accepted the stream
			// but never answered). Same escape hatch as the proxy's live
			// path: stop waiting and fetch independently instead of parking
			// this caller for the owner's entire timeout window.
		}
	}
	res, err := r.doFetchForCache(ctx, s, rawURL, limit)
	c.entry, c.err = nil, err
	if res != nil {
		c.entry = &cacheEntry{
			key:         rawURL,
			data:        res.data,
			status:      res.status,
			contentType: res.contentType,
		}
	}
	close(c.done)
	r.inflight.Delete(rawURL)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// doFetchForCache performs the actual upstream download and cache admission.
func (r *Resolver) doFetchForCache(ctx context.Context, s *proxySession, rawURL string, limit int64) (*cachedFetch, error) {
	if e, ok := r.cache.get(rawURL); ok {
		return &cachedFetch{data: e.data, contentType: e.contentType, status: e.status}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("invalid upstream URL")
	}
	if r.blockedUpstreamHost(ctx, u.Hostname()) {
		return nil, fmt.Errorf("upstream host %q blocked", u.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	headers := cloneHeader(s.headers)
	r.mu.Unlock()
	for _, k := range playbackHeaders {
		if v := headers.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	if req.Header.Get("Range") == "" && !strings.Contains(strings.ToLower(u.Path), ".m3u8") {
		req.Header.Set("Range", "bytes=0-")
	}
	req.Header.Set("Accept-Encoding", "identity")
	client := &http.Client{
		Transport: r.transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if next.URL == nil || (next.URL.Scheme != "https" && next.URL.Scheme != "http") {
				return errors.New("redirect to invalid URL")
			}
			if r.blockedUpstreamHost(next.Context(), next.URL.Hostname()) {
				return fmt.Errorf("redirect to blocked host %q", next.URL.Hostname())
			}
			return nil
		},
	}
	resp, err := doWithRetry(ctx, "Cache", func() (*http.Response, error) {
		return client.Do(req.Clone(ctx))
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	reader := io.Reader(resp.Body)
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return nil, gzErr
		}
		defer gz.Close()
		reader = gz
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("upstream body exceeds cache limit")
	}
	// Merge upstream cookies so later proxied requests keep the same playback
	// context, and admit the fetched host for the allow-list check.
	if cookies := resp.Cookies(); len(cookies) > 0 {
		r.mu.Lock()
		mergeResponseCookies(s.headers, cookies)
		r.mu.Unlock()
	}
	r.mu.Lock()
	if s.allowed != nil {
		s.allowed[strings.ToLower(u.Host)] = true
	}
	r.mu.Unlock()
	result := &cachedFetch{data: data, contentType: resp.Header.Get("Content-Type"), status: resp.StatusCode}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		ttl := cacheEntryTTL
		if strings.Contains(string(data), "#EXTM3U") {
			// Playlists default to a short TTL so live-style refreshes
			// still reach upstream. A VOD playlist (marked complete by
			// EXT-X-ENDLIST) is immutable — some providers even hand out
			// single-use playlist tokens, where any refetch stalls — so
			// VOD playlists cache for the full entry lifetime.
			ttl = playlistCacheTTL
			if strings.Contains(string(data), "#EXT-X-ENDLIST") {
				ttl = cacheEntryTTL
			}
		}
		r.cache.put(&cacheEntry{
			key:         rawURL,
			data:        data,
			status:      resp.StatusCode,
			contentType: resp.Header.Get("Content-Type"),
			expiresAt:   time.Now().Add(ttl),
		})
	}
	return result, nil
}

// registerSegments feeds a media playlist's ordered segment list into the
// session's read-ahead, admits every referenced CDN host for proxied access,
// and kicks the prefetch pump.
func (r *Resolver) registerSegments(s *proxySession, base *url.URL, playlistText string) {
	w := s.warmer
	if w == nil {
		return
	}
	urls := playlistSegmentURLs(playlistText, base)
	if len(urls) == 0 {
		return
	}
	idx := make(map[string]int, len(urls))
	for i, u := range urls {
		if _, exists := idx[u]; !exists {
			idx[u] = i
		}
	}
	discovered := make(map[string]bool, 4)
	for _, raw := range urls {
		if pu, err := url.Parse(raw); err == nil && pu.Host != "" {
			discovered[strings.ToLower(pu.Host)] = true
		}
	}
	w.mu.Lock()
	w.segments = urls
	w.index = idx
	if w.served > len(urls) {
		w.served = len(urls)
	}
	w.mu.Unlock()
	// Admit hosts under r.mu — never while holding w.mu (lock ordering).
	r.mu.Lock()
	if s.allowed != nil {
		for h := range discovered {
			s.allowed[h] = true
		}
	}
	r.mu.Unlock()
	r.pumpPrefetch(s)
}

// playlistSegmentURLs extracts the absolute URL of every media segment (and
// the fMP4 init section via EXT-X-MAP) referenced by a media playlist, in
// playback order. Byte-range playlists repeat one URL per range entry; those
// immediate repeats are collapsed.
func playlistSegmentURLs(text string, base *url.URL) []string {
	var urls []string
	last := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXT-X-MAP") {
				for _, m := range uriAttrRE.FindAllStringSubmatch(line, -1) {
					if u := resolveMediaURL(base, m[1]); u != "" && u != last {
						last = u
						urls = append(urls, u)
					}
				}
			}
			continue
		}
		if strings.Contains(line, " ") {
			continue
		}
		if u := resolveMediaURL(base, line); u != "" && u != last {
			last = u
			urls = append(urls, u)
		}
	}
	return urls
}

// noteSegmentServed advances the read-ahead cursor after a segment is handed
// to the player and kicks the next prefetch download.
func (r *Resolver) noteSegmentServed(s *proxySession, rawURL string) {
	w := s.warmer
	if w == nil {
		return
	}
	w.mu.Lock()
	if idx, ok := w.index[rawURL]; ok && idx+1 > w.served {
		w.served = idx + 1
	}
	w.mu.Unlock()
	r.pumpPrefetch(s)
}

// lastPlaylistLine returns the final non-tag, non-empty line of a playlist —
// the URI of the single variant left by forceHighestQuality.
func lastPlaylistLine(text string) string {
	var last string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		last = line
	}
	return last
}
