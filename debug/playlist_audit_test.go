package debug

// Temporary live audit probe: pin down why cinesrc.st truncates large
// variant playlists mid-read. Run with:
//
//	CINESRC_LIVE=1 go test -v -run TestLiveCinesrcPlaylistAudit ./debug -timeout 8m

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type auditClientCfg struct {
	name     string
	h1       bool
	gzipAE   bool
	keepConn bool // reuse pooled connections between fetches
}

func newAuditClient(cfg auditClientCfg) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   8,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DisableKeepAlives:     !cfg.keepConn,
	}
	if cfg.h1 {
		// leave ForceAttemptHTTP2 unset → HTTP/1.1 only
	} else {
		tr.ForceAttemptHTTP2 = true
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func fetchAndMeasure(t *testing.T, client *http.Client, rawURL string, gzipAE bool, label string) (int64, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Logf("%s: bad url: %v", label, err)
		return 0, ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://cinesrc.st/")
	req.Header.Set("Origin", "https://cinesrc.st")
	if gzipAE {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	} else {
		req.Header.Set("Accept-Encoding", "identity")
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("%s: fetch error after %v: %v", label, time.Since(start), err)
		return 0, ""
	}
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, gzErr := newGzipReader(resp.Body)
		if gzErr != nil {
			t.Logf("%s: bad gzip: %v", label, gzErr)
			return 0, ""
		}
		defer gz.Close()
		reader = gz
	}
	n, rerr := io.Copy(io.Discard, reader)
	proto := ""
	if resp.Proto != "" {
		proto = resp.Proto
	}
	t.Logf("%s: proto=%s status=%d cl=%s ce=%q bytes=%d elapsed=%s readErr=%v",
		label, proto, resp.StatusCode, resp.Header.Get("Content-Length"),
		resp.Header.Get("Content-Encoding"), n, time.Since(start).Round(time.Millisecond), rerr)
	return n, ""
}

func TestLiveCinesrcPlaylistAudit(t *testing.T) {
	if !liveEnabled(t) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	res, err := liveEngine(t).Resolve(ctx, "movie", "324857", "", "", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Logf("resolved provider=%s source=%s", res.Provider, res.Source)

	// Fetch the master once and enumerate its variants.
	client := newAuditClient(auditClientCfg{name: "h1-identity", h1: true})
	masterURL := res.Source
	req, _ := http.NewRequest(http.MethodGet, masterURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://cinesrc.st/")
	req.Header.Set("Origin", "https://cinesrc.st")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("master fetch: %v", err)
	}
	mb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	master := string(mb)
	t.Logf("master: status=%d cl=%s ce=%q bytes=%d", resp.StatusCode, resp.Header.Get("Content-Length"), resp.Header.Get("Content-Encoding"), len(master))

	base, _ := url.Parse(masterURL)
	var variants []string
	for _, line := range strings.Split(master, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if u, err := url.Parse(line); err == nil {
			variants = append(variants, base.ResolveReference(u).String())
		}
	}
	t.Logf("master has %d variant(s)", len(variants))
	if len(variants) == 0 {
		t.Fatal("no variants")
	}

	// Audit each variant (they share the same edge behaviour).
	target := variants[0]
	t.Logf("auditing variant: %s", target)

	// Config 1: h1, identity, fresh connection per attempt — 4 attempts.
	cfg := auditClientCfg{name: "h1-identity-fresh", h1: true}
	c1 := newAuditClient(cfg)
	for i := 1; i <= 4; i++ {
		fetchAndMeasure(t, c1, target, false, fmt.Sprintf("h1-fresh try%d", i))
	}

	// Config 2: h1, gzip accept, fresh connection per attempt.
	c2 := newAuditClient(auditClientCfg{name: "h1-gzip-fresh", h1: true})
	for i := 1; i <= 3; i++ {
		fetchAndMeasure(t, c2, target, true, fmt.Sprintf("h1-gzip try%d", i))
	}

	// Config 3: h2, identity, fresh connection per attempt.
	c3 := newAuditClient(auditClientCfg{name: "h2-identity-fresh", h1: false})
	for i := 1; i <= 3; i++ {
		fetchAndMeasure(t, c3, target, false, fmt.Sprintf("h2-fresh try%d", i))
	}

	// Config 4: h1, identity, REUSED pooled connection — tests the
	// "poisoned keep-alive connection" hypothesis: attempt 2+ on the same
	// connection where attempt 1 was truncated.
	c4 := newAuditClient(auditClientCfg{name: "h1-identity-keepalive", h1: true, keepConn: true})
	for i := 1; i <= 4; i++ {
		fetchAndMeasure(t, c4, target, false, fmt.Sprintf("h1-keepalive try%d", i))
	}

	// Config 5: CONCURRENT fetches — production runs the player's variant
	// request, subtitle playlists and the warmer's prefetches at once.
	// Sequential attempts above almost never fail; concurrency is the
	// remaining suspect for the mid-read truncation.
	c5 := newAuditClient(auditClientCfg{name: "h2-concurrent", h1: false, keepConn: true})
	const burst = 4
	var wg sync.WaitGroup
	for round := 1; round <= 3; round++ {
		wg.Add(burst)
		for j := 0; j < burst; j++ {
			go func(round, j int) {
				defer wg.Done()
				fetchAndMeasure(t, c5, target, false, fmt.Sprintf("h2-concurrent r%d-f%d", round, j))
			}(round, j)
		}
		wg.Wait()
		time.Sleep(2 * time.Second)
	}

	// Config 6: h1-only transport, concurrent — isolates protocol from
	// concurrency.
	c6 := newAuditClient(auditClientCfg{name: "h1-concurrent", h1: true, keepConn: true})
	for round := 1; round <= 2; round++ {
		wg.Add(burst)
		for j := 0; j < burst; j++ {
			go func(round, j int) {
				defer wg.Done()
				fetchAndMeasure(t, c6, target, false, fmt.Sprintf("h1-concurrent r%d-f%d", round, j))
			}(round, j)
		}
		wg.Wait()
		time.Sleep(2 * time.Second)
	}
}

func newGzipReader(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) }

