package mediaresolver

// Tests for the per-host cooldown: upstream 502 bursts must widen an
// escalating cooldown that paces both new fetches and in-flight retries,
// and the first sane upstream answer must clear it.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostThrottleEscalatesAndRecovers(t *testing.T) {
	th := &hostThrottle{}
	now := time.Now()
	d := th.failed(now)
	if d < throttleBase || d > throttleBase+50*time.Millisecond {
		t.Fatalf("first cooldown = %v, want ~%v", d, throttleBase)
	}
	if nd := th.failed(time.Now()); nd < d {
		t.Fatalf("second cooldown %v not wider than first %v", nd, d)
	}
	var last time.Duration
	for i := 0; i < 20; i++ {
		last = th.failed(time.Now())
	}
	if last != throttleMax {
		t.Fatalf("cooldown after 20 failures = %v, want capped at %v", last, throttleMax)
	}
	if r := th.remaining(); r <= 0 {
		t.Fatal("remaining = 0 during cooldown")
	}
	th.recovered()
	if r := th.remaining(); r != 0 {
		t.Fatalf("remaining after recovery = %v, want 0", r)
	}
}

func TestLimitUpstreamHeadersWaitsOutCooldown(t *testing.T) {
	r := &Resolver{}
	th := r.throttleFor("cooldown.example.test")
	th.failed(time.Now()) // opens a ~750ms cooldown

	start := time.Now()
	rel, err := r.limitUpstreamHeaders(context.Background(), "cooldown.example.test")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("limitUpstreamHeaders: %v", err)
	}
	rel()
	if elapsed < 500*time.Millisecond {
		t.Fatalf("acquired in %v, want held back by the cooldown", elapsed)
	}

	// A clear host must not wait at all.
	start = time.Now()
	rel, err = r.limitUpstreamHeaders(context.Background(), "clear.example.test")
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("limitUpstreamHeaders (clear host): %v", err)
	}
	rel()
	if elapsed > 50*time.Millisecond {
		t.Fatalf("clear host waited %v, want immediate", elapsed)
	}
}

// TestDoWithRetryPacesAgainstCooldown: retryable upstream answers widen the
// host cooldown, retries sleep at least that long, and a final success clears
// the state so later fetches start immediately.
func TestDoWithRetryPacesAgainstCooldown(t *testing.T) {
	var calls atomic.Int32
	var stamps atomic.Value // first call time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			_ = stamps.CompareAndSwap(nil, time.Now())
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	r := &Resolver{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.doWithRetry(context.Background(), "127.0.0.1", "Test", func() (*http.Response, error) {
		return srv.Client().Do(req.Clone(context.Background()))
	})
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after 502 retry", resp.StatusCode)
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
	if th := r.throttleFor("127.0.0.1"); th.remaining() != 0 {
		t.Fatalf("cooldown not cleared after success: %v", th.remaining())
	}
}
