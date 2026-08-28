package mediaresolver

// Regression tests for the proxy's stream-through cache admission: a
// partial-content (206) answer to a forwarded player Range request must
// never be admitted into the body cache under the full URL — serving it
// later as a complete 200 corrupts playback (fragParsingError). Also covers
// boundedMirror partial-write behavior and bodyCache eviction/expiry.

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testSegmentSize = 4096

// testUpstreamHost is a fake hostname used for the upstream: literal loopback
// IPs are hard-blocked by the SSRF guard (blockedUpstreamHost fails closed on
// any IP before consulting the verdict cache), so the tests address the
// httptest server by hostname and redirect the dialer to the real listener.
const testUpstreamHost = "cdn.goflix.test"

func newCacheTestResolver(dialAddr string) *Resolver {
	dialer := &net.Dialer{}
	return &Resolver{
		done:     make(chan struct{}),
		sessions: make(map[string]*proxySession),
		cache:    newBodyCache(1 << 20),
		transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, dialAddr)
			},
		},
	}
}

// newDormantSession registers a session whose warmer exists but holds no
// segment list, so ensureWarmer does not spawn the warmup chain (its own
// prefetches would race the assertions below).
func newDormantSession(r *Resolver, source, host string) string {
	token := "testsessiontoken"
	s := &proxySession{
		source:    source,
		headers:   make(http.Header),
		allowed:   map[string]bool{strings.ToLower(host): true},
		expiresAt: time.Now().Add(time.Hour),
		subsDone:  make(chan struct{}),
		warmer: &streamWarmer{
			index:    make(map[string]int),
			inflight: make(map[string]bool),
			failedAt: make(map[string]time.Time),
			ctx:      context.Background(),
		},
	}
	r.sessions[token] = s
	return token
}

func allowUpstreamHost(r *Resolver, host string) {
	r.blockCache.Store(strings.ToLower(host), dnsCacheEntry{blocked: false, expires: time.Now().Add(time.Hour)})
}

func newSegmentUpstream(t *testing.T) (*httptest.Server, []byte) {
	t.Helper()
	full := make([]byte, testSegmentSize)
	for i := range full {
		full[i] = byte(i % 251)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		if req.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 100-199/"+strconv.Itoa(len(full)))
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(full[100:200])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(full)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(full)
	}))
	t.Cleanup(srv.Close)
	return srv, full
}

func proxySegmentRequest(t *testing.T, r *Resolver, token, segURL string, rangeHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/media/proxy/"+token+".m3u8?url="+url.QueryEscape(segURL), nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	rec := httptest.NewRecorder()
	if err := r.Proxy(rec, req, token); err != nil {
		t.Fatalf("Proxy(%q) returned error: %v", rangeHeader, err)
	}
	return rec
}

func TestProxyRangeResponseNotCached(t *testing.T) {
	srv, full := newSegmentUpstream(t)
	su, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	dialAddr := net.JoinHostPort("127.0.0.1", su.Port())
	upstreamBase := "http://" + net.JoinHostPort(testUpstreamHost, su.Port())
	r := newCacheTestResolver(dialAddr)
	allowUpstreamHost(r, testUpstreamHost)
	segURL := upstreamBase + "/seg.ts"
	token := newDormantSession(r, upstreamBase+"/warm.m3u8", testUpstreamHost+":"+su.Port())

	// A player Range request must be forwarded (206) but never admitted.
	rec := proxySegmentRequest(t, r, token, segURL, "bytes=100-199")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("Proxy(range) status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != string(full[100:200]) {
		t.Fatalf("Proxy(range) body length = %d, want %d", len(got), 100)
	}
	if _, ok := r.cache.get(segURL); ok {
		t.Fatal("partial-content body was admitted into the body cache")
	}

	// Sanity: a complete 200 response IS admitted, at its full length.
	rec2 := proxySegmentRequest(t, r, token, segURL, "")
	if rec2.Code != http.StatusOK || rec2.Body.Len() != len(full) {
		t.Fatalf("Proxy(full) = %d, %d bytes; want 200, %d", rec2.Code, rec2.Body.Len(), len(full))
	}
	e, ok := r.cache.get(segURL)
	if !ok {
		t.Fatal("complete 200 segment was not admitted into the body cache")
	}
	if len(e.data) != len(full) {
		t.Fatalf("cached entry length = %d, want %d", len(e.data), len(full))
	}

	// The cached full body must serve byte-accurate ranges afterwards.
	rec3 := proxySegmentRequest(t, r, token, segURL, "bytes=100-199")
	if rec3.Code != http.StatusPartialContent || rec3.Body.String() != string(full[100:200]) {
		t.Fatalf("cached range serve = %d, %d bytes; want 206 with 100 body bytes", rec3.Code, rec3.Body.Len())
	}
}

// byteReader is a plain io.Reader without WriteTo, so io.CopyBuffer uses the
// provided buffer instead of a single Write.
type byteReader struct {
	data []byte
	pos  int
}

func (b *byteReader) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

type failingWriter struct {
	writes int
	failAt int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.writes++
	if f.writes > f.failAt {
		return 0, errors.New("client gone")
	}
	return len(p), nil
}

func TestBoundedMirrorPartialWrite(t *testing.T) {
	// Client disconnects mid-copy: only bytes actually forwarded may be
	// buffered, so a truncated body can never be admitted as complete.
	m := &boundedMirror{dst: &failingWriter{failAt: 1}, cap: 64}
	nw, err := io.CopyBuffer(m, &byteReader{data: []byte(strings.Repeat("x", 32))}, make([]byte, 8))
	if err == nil {
		t.Fatal("expected copy error from failing writer")
	}
	if nw != 8 {
		t.Fatalf("forwarded %d bytes, want 8", nw)
	}
	if len(m.buf) != 8 {
		t.Fatalf("mirror buffered %d bytes, want 8", len(m.buf))
	}

	// Oversized bodies forward fully but the mirror stops at cap.
	m2 := &boundedMirror{dst: &failingWriter{failAt: 99}, cap: 8}
	if _, err := io.CopyBuffer(m2, &byteReader{data: []byte(strings.Repeat("y", 32))}, make([]byte, 8)); err != nil {
		t.Fatalf("unexpected copy error: %v", err)
	}
	if len(m2.buf) != 8 {
		t.Fatalf("mirror capped at %d bytes, want 8", len(m2.buf))
	}
}

func TestBodyCacheEvictionAndExpiry(t *testing.T) {
	c := newBodyCache(10)
	c.put(&cacheEntry{key: "a", data: make([]byte, 6), expiresAt: time.Now().Add(time.Hour)})
	c.put(&cacheEntry{key: "b", data: make([]byte, 6), expiresAt: time.Now().Add(time.Hour)})
	if _, ok := c.get("a"); ok {
		t.Fatal("expected LRU eviction of the oldest entry")
	}
	if _, ok := c.get("b"); !ok {
		t.Fatal("expected the newest entry to survive")
	}
	c.put(&cacheEntry{key: "c", data: make([]byte, 11), expiresAt: time.Now().Add(time.Hour)})
	if _, ok := c.get("c"); ok {
		t.Fatal("oversized entry must be refused")
	}
	c.put(&cacheEntry{key: "d", data: make([]byte, 5), expiresAt: time.Now().Add(-time.Second)})
	if _, ok := c.get("d"); ok {
		t.Fatal("expired entry must not be served")
	}
}

// TestMaybeAttachSubRenditionsProviders pins the set of providers whose
// sessions get the server-side subtitle ladder embedded into their master
// manifest. It must cover every provider whose direct path calls
// attachAndWarm (cinesrc, vidking, vidlove, vidsrcme); vixsrc warms via
// startWarmup only and must stay rendition-free.
func TestMaybeAttachSubRenditionsProviders(t *testing.T) {
	cases := map[string]bool{
		"vidking":   true,
		"vidlove":   true,
		"cinesrc":   true,
		"vidsrcme":  true,
		"vixsrc":    false,
		"":          false,
	}
	for provider, wantSubs := range cases {
		r := newCacheTestResolver("127.0.0.1:0")
		r.SubRenditionProvider = func(ctx context.Context, req MediaRequest) []SubRendition {
			return []SubRendition{{Label: "English", Language: "en", URI: "/api/subtitles/wrap.m3u8?src=x"}}
		}
		token := newDormantSession(r, "https://upstream.test/master.m3u8", "upstream.test")
		r.maybeAttachSubRenditions(token, MediaRequest{Type: Movie, ID: "123", Provider: provider})

		var got []SubRendition
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			got = r.subRenditions(token)
			if len(got) > 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if wantSubs && len(got) != 1 {
			t.Fatalf("provider %q: expected 1 embedded rendition, got %d", provider, len(got))
		}
		if !wantSubs && len(got) != 0 {
			t.Fatalf("provider %q: expected no embedded renditions, got %d", provider, len(got))
		}
	}
}
