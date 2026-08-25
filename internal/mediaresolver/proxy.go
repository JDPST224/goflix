package mediaresolver

// The streaming reverse-proxy endpoint.

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proxy serves HLS manifests and media resources through the server. It rewrites
// manifest URIs so the browser never contacts the provider/CDN directly.
func (r *Resolver) Proxy(w http.ResponseWriter, req *http.Request, token string) error {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return errors.New("method not allowed")
	}
	r.mu.Lock()
	s, ok := r.sessions[token]
	if ok && time.Now().After(s.expiresAt) {
		delete(r.sessions, token)
		ok = false
	}
	if !ok {
		r.mu.Unlock()
		return errors.New("proxy session expired")
	}
	// Sliding expiration: active playback keeps the session alive.
	s.expiresAt = time.Now().Add(r.sessionTTL())
	source := s.source
	sessionHeaders := cloneHeader(s.headers)
	allowed := cloneAllowed(s.allowed)
	r.mu.Unlock()
	// Make sure the read-ahead pipeline exists even if warmup never ran.
	r.ensureWarmer(s)

	// A request with no ?url= is the session root — the player's very first
	// manifest fetch. Hold it briefly while the backend subtitle ladder is
	// still running so the renditions land in THIS response instead of the
	// player seeing a rendition-less manifest and caching that impression.
	isSessionRoot := req.URL.Query().Get("url") == ""
	if isSessionRoot && s.subsState.Load() == subsPending {
		log.Printf("[MediaResolver] holding session root briefly for subtitle attach")
		r.waitForSubs(s)
	}
	raw := req.URL.Query().Get("url")
	if raw == "" {
		raw = source
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("invalid upstream URL")
	}
	host := strings.ToLower(u.Host)
	if !allowed[host] {
		return errors.New("upstream host not allowed")
	}
	if r.blockedUpstreamHost(req.Context(), u.Hostname()) {
		return errors.New("upstream host blocked")
	}
	// Fast path: a warm copy of this manifest/segment is already in the body
	// cache — serve it from RAM without a round-trip to the upstream CDN.
	if e, ok := r.cache.get(u.String()); ok {
		r.serveFromCache(w, req, s, token, u, e)
		return nil
	}
	// Join in-flight downloads: if the read-ahead warmer (or another player
	// request) is fetching this exact URL right now, waiting for it beats
	// downloading the same bytes twice over a shared connection. Only
	// non-manifest URLs join here — playlists are tiny and re-fetched freely.
	if !strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") && req.Method == http.MethodGet && req.Header.Get("Range") == "" {
		if v, ok := r.inflight.Load(u.String()); ok {
			joiner := v.(*inflightFetch)
			timer := time.NewTimer(inflightJoinWait)
			defer timer.Stop()
			select {
			case <-joiner.done:
				if joiner.err == nil && joiner.entry != nil {
					r.serveFromCache(w, req, s, token, u, joiner.entry)
					return nil
				}
			case <-req.Context().Done():
				return req.Context().Err()
			case <-timer.C:
				// The shared download is stalled (upstream accepted the
				// stream but never answered). Stop waiting and fall through
				// to an independent fetch instead of inheriting the stall.
			}
		}
	}
	// No Client.Timeout here: it spans the entire body read, so a slow large
	// segment would be cancelled mid-copy and forwarded truncated (player
	// fragParsingError). TTFB is bounded by the transport's
	// ResponseHeaderTimeout and the copy by the request context (client
	// disconnect) — the same contract a reverse proxy relies on.
	client := &http.Client{
		Transport: r.transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			nu := next.URL
			if nu == nil || (nu.Scheme != "https" && nu.Scheme != "http") || nu.Host == "" {
				return errors.New("proxy redirect to invalid URL")
			}
			if r.blockedUpstreamHost(next.Context(), nu.Hostname()) {
				// Fail closed: returning an error (rather than
				// ErrUseLastResponse) prevents the upstream 3xx response —
				// and its Location header pointing at the blocked host —
				// from being forwarded to the browser.
				return fmt.Errorf("proxy redirect to blocked host %q", strings.ToLower(nu.Host))
			}
			nhost := strings.ToLower(nu.Host)
			if !allowed[nhost] {
				r.mu.Lock()
				if current := r.sessions[token]; current != nil {
					current.allowed[nhost] = true
				}
				r.mu.Unlock()
				allowed[nhost] = true
			}
			for _, k := range playbackHeaders {
				if v := sessionHeaders.Get(k); v != "" {
					next.Header.Set(k, v)
				}
			}
			return nil
		},
	}
	upstream, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), nil)
	if err != nil {
		return err
	}
	for _, k := range playbackHeaders {
		if v := sessionHeaders.Get(k); v != "" {
			upstream.Header.Set(k, v)
		}
	}
	if upstream.Header.Get("User-Agent") == "" {
		upstream.Header.Set("User-Agent", defaultUserAgent)
	}
	for _, k := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := req.Header.Get(k); v != "" {
			upstream.Header.Set(k, v)
		}
	}
	upstream.Header.Set("Accept-Encoding", "identity")
	resp, err := doWithRetry(req.Context(), "Proxy", func() (*http.Response, error) {
		// Clone per attempt: client.Do mutates the request (redirect hops,
		// header overrides) so a reused request would leak state across tries.
		return client.Do(upstream.Clone(req.Context()))
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Player-priority accounting: from here until the response has been fully
	// forwarded, this request is consuming live upstream bandwidth. The
	// read-ahead pump sees the counter and pauses new prefetch work.
	s.liveFetches.Add(1)
	defer func() {
		// When the last live fetch drains, re-kick the read-ahead pump so any
		// window work the player-priority gate was holding resumes immediately.
		// Without this the lookahead can sit idle until the next player request
		// happens to arrive.
		if s.liveFetches.Add(-1) == 0 {
			r.pumpPrefetch(s)
		}
	}()
	if cookies := resp.Cookies(); len(cookies) > 0 {
		r.mu.Lock()
		if current := r.sessions[token]; current != nil {
			mergeResponseCookies(current.headers, cookies)
		}
		r.mu.Unlock()
	}
	log.Printf("[MediaResolver] Proxy upstream status=%d host=%s path=%s", resp.StatusCode, strings.ToLower(u.Host), u.Path)
	w.Header().Set("Cache-Control", "no-store")
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	// Sniff the first bytes of plain 200 responses: some providers serve HLS
	// manifests as application/octet-stream under URLs without a .m3u8
	// extension, and streaming such a playlist through the segment branch
	// leaves its URIs unrewritten (hls.js then resolves them against the
	// proxy URL and playback fails). Byte ranges (206) are skipped so a range
	// starting inside playlist text can't be misclassified as a manifest.
	var head []byte
	if resp.StatusCode == http.StatusOK && req.Method != http.MethodHead {
		buf := make([]byte, 512)
		n, _ := io.ReadFull(resp.Body, buf)
		head = buf[:n]
	}
	bodyReader := io.MultiReader(bytes.NewReader(head), resp.Body)

	isManifest := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "mpegurl") ||
		strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") ||
		(len(head) > 0 && strings.HasPrefix(strings.TrimSpace(string(head)), "#EXTM3U"))
	if !isManifest {
		// Segments are immutable — let the browser/player cache them instead
		// of re-downloading on every replay or seek-back.
		w.Header().Set("Cache-Control", "private, max-age=21600")
		r.noteSegmentServed(s, u.String())
		w.WriteHeader(resp.StatusCode)
		if req.Method != http.MethodHead {
			// Stream-through caching: forward to the player while mirroring
			// a private copy, capped at the per-entry cache limit so an
			// oversized body streams through without inflating memory. A
			// clean full-body pass-through is admitted into the LRU.
			mirror := &boundedMirror{dst: w, cap: maxCachedSegmentBytes}
			bufp := copyBufPool.Get().(*[]byte)
			nw, copyErr := io.CopyBuffer(mirror, bodyReader, *bufp)
			copyBufPool.Put(bufp)
			if copyErr == nil && nw > 0 && nw == int64(len(mirror.buf)) {
				r.cache.put(&cacheEntry{
					key:         u.String(),
					data:        mirror.buf,
					status:      http.StatusOK,
					contentType: resp.Header.Get("Content-Type"),
					expiresAt:   time.Now().Add(cacheEntryTTL),
				})
			}
		}
		return nil
	}
	reader := bodyReader
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return fmt.Errorf("upstream returned an invalid gzip manifest: %w", gzErr)
		}
		defer gz.Close()
		reader = gz
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxManifestBytes {
		return errors.New("upstream HLS manifest exceeds size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(data)
		return nil
	}
	if req.Method == http.MethodHead {
		// HEAD responses carry no body, so the #EXTM3U validation below would
		// always reject them as non-HLS. The upstream status was 2xx — forward
		// it with manifest headers and no body.
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		return nil
	}
	text := string(data)
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		// Upstream served an HTML/JSON error page for a .m3u8 URL. Forwarding
		// it would make the player fail with fragParsingError, so reject it.
		log.Printf("[MediaResolver] upstream returned a non-HLS payload for a manifest URL host=%s path=%s content_encoding=%q bytes=%d", strings.ToLower(u.Host), u.Path, resp.Header.Get("Content-Encoding"), len(data))
		return errors.New("upstream returned a non-HLS payload for a manifest URL")
	}
	// Feed the read-ahead so the segments this playlist references are hot in
	// the cache before the player asks for them.
	if strings.Contains(text, "#EXTINF") {
		r.registerSegments(s, u, text)
	}
	rewritten, discovered := r.rewriteManifest(text, u, token, req)
	if isSessionRoot {
		log.Printf("[MediaResolver] session root served: subtitle renditions=%d bytes=%d", len(r.subRenditions(token)), len(rewritten))
	}
	if len(discovered) > 0 {
		r.mu.Lock()
		if current := r.sessions[token]; current != nil {
			for h := range discovered {
				current.allowed[h] = true
			}
		}
		r.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Del("Content-Length")
	w.Header().Del("Content-Range")
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = io.WriteString(w, rewritten)
	}
	return nil
}

// boundedMirror forwards every write to dst while mirroring into an internal
// buffer up to cap bytes; past the cap it keeps forwarding but stops
// mirroring, so oversized bodies stream through untruncated and unbuffered.
type boundedMirror struct {
	dst io.Writer
	buf []byte
	cap int
}

func (m *boundedMirror) Write(p []byte) (int, error) {
	n, err := m.dst.Write(p)
	if err != nil {
		return n, err
	}
	if len(m.buf) < m.cap {
		take := len(p)
		if room := m.cap - len(m.buf); take > room {
			take = room
		}
		m.buf = append(m.buf, p[:take]...)
	}
	return n, nil
}

// serveFromCache writes a cached body to the player without touching the
// upstream: manifests run through the usual rewrite pipeline, segments are
// served with byte-range support so seeks stay instant.
func (r *Resolver) serveFromCache(w http.ResponseWriter, req *http.Request, s *proxySession, token string, u *url.URL, e *cacheEntry) {
	isManifest := strings.Contains(strings.ToLower(e.contentType), "mpegurl") ||
		strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") ||
		strings.HasPrefix(strings.TrimSpace(string(e.data)), "#EXTM3U")
	if isManifest {
		text := string(e.data)
		if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
			http.Error(w, "cached manifest invalid", http.StatusBadGateway)
			return
		}
		if strings.Contains(text, "#EXTINF") {
			r.registerSegments(s, u, text)
		}
		rewritten, discovered := r.rewriteManifest(text, u, token, req)
		if req.URL.Query().Get("url") == "" {
			log.Printf("[MediaResolver] session root served (cache): subtitle renditions=%d bytes=%d", len(r.subRenditions(token)), len(rewritten))
		}
		r.mu.Lock()
		for h := range discovered {
			s.allowed[h] = true
		}
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("Content-Length")
		w.Header().Del("Content-Range")
		w.WriteHeader(http.StatusOK)
		if req.Method != http.MethodHead {
			_, _ = io.WriteString(w, rewritten)
		}
		return
	}
	// Segment (or init section): immutable — cache-friendly headers.
	w.Header().Set("Content-Type", e.contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=21600")
	size := int64(len(e.data))
	start, end := int64(0), size-1
	status := http.StatusOK
	if spec := req.Header.Get("Range"); spec != "" {
		if rs, re, ok := parseByteRange(spec, size); ok {
			start, end = rs, re
			status = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		}
	}
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(status)
	if req.Method != http.MethodHead {
		_, _ = w.Write(e.data[start : end+1])
	}
	r.noteSegmentServed(s, u.String())
}
