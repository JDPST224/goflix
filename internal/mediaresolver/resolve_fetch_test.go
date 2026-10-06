package mediaresolver

// Regression tests for resolveFetch: provider-facing requests made during the
// resolve phase (APIs, embed pages, manifest probes) must share the per-host
// header-phase pacing with the media paths and retry transient upstream
// throttling (429/5xx) instead of surfacing it to the resolve chain.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestResolveFetchRetriesRateLimited: a 429 on the first attempt must not be
// returned to the caller — resolveFetch backs off (honoring Retry-After) and
// retries, so a transient provider throttle does not fail the resolve.
func TestResolveFetchRetriesRateLimited(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	r := &Resolver{done: make(chan struct{})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.resolveFetch(context.Background(), srv.Client(), req)
	if err != nil {
		t.Fatalf("resolveFetch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after 429 retry", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (429 then 200)", got)
	}
}

// TestResolveFetchRetriesTransient5xx: transient server errors are retried
// the same way, using the lighter non-429 backoff.
func TestResolveFetchRetriesTransient5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	r := &Resolver{done: make(chan struct{})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.resolveFetch(context.Background(), srv.Client(), req)
	if err != nil {
		t.Fatalf("resolveFetch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after 503 retry", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (503 then 200)", got)
	}
}

// TestLimitUpstreamHeadersCapsPerHost: once the per-host header-phase cap is
// reached, further acquisitions wait (and respect ctx) instead of piling a
// resolve burst onto the same provider host; other hosts stay unaffected and
// a release unblocks the host immediately.
func TestLimitUpstreamHeadersCapsPerHost(t *testing.T) {
	r := &Resolver{}
	ctx := context.Background()
	releases := make([]func(), 0, mediaHostHeaderConcurrency)
	for i := 0; i < mediaHostHeaderConcurrency; i++ {
		rel, err := r.limitUpstreamHeaders(ctx, "cdn.example.test")
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		releases = append(releases, rel)
	}

	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := r.limitUpstreamHeaders(blocked, "cdn.example.test"); err == nil {
		t.Fatal("acquisition past the per-host cap succeeded, want blocked until release")
	}

	other, err := r.limitUpstreamHeaders(ctx, "other.example.test")
	if err != nil {
		t.Fatalf("other host blocked by saturated one: %v", err)
	}
	other()

	releases[0]()
	rel, err := r.limitUpstreamHeaders(ctx, "cdn.example.test")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel()
}
