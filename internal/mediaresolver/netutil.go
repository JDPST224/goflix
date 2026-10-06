package mediaresolver

// Small network utilities: bounded retries, byte-range parsing,
// DNS/IP guarding, query redaction.

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// upstreamAttempts bounds retries for playback fetches that fail before the
// body starts flowing, and retryableUpstreamStatus lists transient statuses
// worth re-requesting. Retries are safe in both fetch paths that use them:
// nothing has been forwarded to the player yet, and HLS traffic is idempotent
// GETs. Some provider CDNs occasionally accept a request on an HTTP/2
// connection and then never answer it; a retry lands on a fresh (or
// different) host and typically succeeds immediately.
const upstreamAttempts = 3

// mediaHostHeaderConcurrency caps simultaneous upstream header-phases
// (connection + request + response headers) per hostname for media and
// resolve fetches. The CDN's rate limiter counts request rate: the
// read-ahead's parallel prefetches, the player's live requests and a burst
// of resolves must not pile up into a 429 storm. Body reads happen outside
// the limiter, so a slow segment download never blocks other requests from
// starting.
const mediaHostHeaderConcurrency = 6

// Per-host cooldown (a lightweight circuit breaker). When one upstream edge
// starts answering 502/503/504, every concurrent request's independent retry
// loop keeps hitting it — a blip that recovers in seconds is stretched into a
// 15-second storm of failing retries (observed: a next-episode prewarm plus
// live playback stacked ~8 parallel request streams onto one edge). The
// throttle paces all fetches to that host: every failed attempt widens the
// cooldown (750ms, 1.5s, 3s… capped), new fetches wait it out before starting,
// and in-flight retries sleep at least as long as the current cooldown instead
// of their base 400ms cadence. The first sane answer clears it.
const (
	throttleBase = 750 * time.Millisecond
	throttleMax  = 8 * time.Second
	// maxThrottleWait caps how long a NEW fetch waits for the cooldown to
	// expire before proceeding anyway: it bounds the added latency for
	// player-facing requests during an outage — the attempt itself still
	// runs and reports its own failure.
	maxThrottleWait = 6 * time.Second
)

type hostThrottle struct {
	mu      sync.Mutex
	strikes int
	until   time.Time
}

// failed widens the cooldown after one more upstream failure and returns the
// current cooldown expiry.
func (t *hostThrottle) failed(now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.strikes++
	d := throttleBase << (t.strikes - 1)
	if d > throttleMax || d <= 0 {
		d = throttleMax
	}
	if until := now.Add(d); until.After(t.until) {
		t.until = until
	}
	// Measure against the caller's clock, not the wall clock: callers pass
	// time.Now() today, but tests (and any future caller) may use a fake
	// clock, and mixing the two returns a bogus duration.
	return t.until.Sub(now)
}

// recovered clears the cooldown after a sane upstream answer.
func (t *hostThrottle) recovered() {
	t.mu.Lock()
	t.strikes = 0
	t.until = time.Time{}
	t.mu.Unlock()
}

// remaining reports how long until the cooldown expires (0 when clear).
func (t *hostThrottle) remaining() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if d := time.Until(t.until); d > 0 {
		return d
	}
	return 0
}

// throttleFor returns the cooldown state for a host, creating it on first use.
func (r *Resolver) throttleFor(host string) *hostThrottle {
	v, _ := r.hostThrottle.LoadOrStore(strings.ToLower(host), &hostThrottle{})
	return v.(*hostThrottle)
}

// noteUpstreamOutcome feeds one fetch attempt's result into the host's
// cooldown state: retryable statuses and transport errors widen it, a clean
// answer clears it.
func (r *Resolver) noteUpstreamOutcome(host string, err error, resp *http.Response) {
	th := r.throttleFor(host)
	if err != nil || retryableUpstreamStatus(resp.StatusCode) {
		th.failed(time.Now())
		return
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		th.recovered()
	}
}

// retryableUpstreamStatus lists transient statuses worth re-requesting.
func retryableUpstreamStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// limitUpstreamHeaders acquires one header-phase slot for the host, bounded
// by ctx. The returned release func must be called once response headers are
// in hand (or the attempt failed).
func (r *Resolver) limitUpstreamHeaders(ctx context.Context, host string) (func(), error) {
	host = strings.ToLower(host)
	// Cooldown pacing: while this host's edge is answering 502 bursts, new
	// fetches wait out (part of) the cooldown instead of joining the stampede.
	// Waiting happens before the semaphore slot so the pacing never consumes
	// another host's or the player's concurrency budget.
	if v, ok := r.hostThrottle.Load(host); ok {
		d := v.(*hostThrottle).remaining()
		if d > maxThrottleWait {
			d = maxThrottleWait
		}
		if d > 0 {
			timer := time.NewTimer(d)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
	v, _ := r.hostSem.LoadOrStore(host, make(chan struct{}, mediaHostHeaderConcurrency))
	sem := v.(chan struct{})
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// doWithRetry runs attempt() up to upstreamAttempts times. An attempt is
// retried when it errors, or when it returns one of the transient statuses —
// in which case its body is drained and closed first. The final response (or
// error) is returned; ctx cancellation aborts between attempts. Every failed
// attempt widens the host's cooldown and every retry sleeps at least as long
// as that cooldown — independent request loops must not hammer a failing
// edge into a longer outage.
func (r *Resolver) doWithRetry(ctx context.Context, host, logPrefix string, attempt func() (*http.Response, error)) (*http.Response, error) {
	th := r.throttleFor(host)
	for n := 1; ; n++ {
		resp, err := attempt()
		if err == nil && (!retryableUpstreamStatus(resp.StatusCode) || n >= upstreamAttempts) {
			r.noteUpstreamOutcome(host, err, resp)
			return resp, nil
		}
		th.failed(time.Now())
		if err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			// The caller is gone (player disconnect, healer done) — its
			// context is dead, so retrying would only log noise and burn
			// time against a dead request. Transport-level timeouts (e.g.
			// "http2: timeout awaiting response headers") are different:
			// the context is still alive and a retry on a fresh host
			// typically succeeds.
			return nil, err
		}
		if err != nil && n >= upstreamAttempts {
			return nil, err
		}
		log.Printf("[MediaResolver] %s upstream attempt %d/%d failed (%v), retrying",
			logPrefix, n, upstreamAttempts, errStringOrStatus(err, resp))
		// Sleep at least the base backoff AND the host's current cooldown:
		// pacing the retry against the circuit beats a fixed 400ms cadence
		// when the edge is answering 502 in bursts.
		delay := retryDelay(n, resp)
		if cd := th.remaining(); cd > delay {
			delay = cd
		}
		select {
		case <-ctx.Done():
			if err == nil {
				return nil, ctx.Err()
			}
			return nil, err
		case <-time.After(delay):
		}
	}
}

// resolveFetch performs one provider-facing request outside the media proxy
// paths: provider APIs, embed pages, manifest probes and background
// revalidation. It applies the same per-host header-phase pacing as media
// fetches — a resolve burst must not outpace playback traffic against the
// same host — and retries transient statuses (429/5xx) through doWithRetry,
// honoring Retry-After on rate-limited responses. Only idempotent requests
// should use it; the caller owns the returned response body.
func (r *Resolver) resolveFetch(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	release, err := r.limitUpstreamHeaders(ctx, req.URL.Hostname())
	if err != nil {
		return nil, err
	}
	defer release()
	return r.doWithRetry(ctx, req.URL.Hostname(), "Resolve", func() (*http.Response, error) {
		return client.Do(req.Clone(ctx))
	})
}

// retryDelay computes the wait before the next attempt. Rate-limited
// responses (429) need far more than the base 400ms cadence — retrying that
// fast just burns attempts against a limit that is still engaged — so they
// wait longer, preferring the upstream's Retry-After when present.
func retryDelay(attempt int, resp *http.Response) time.Duration {
	base := time.Duration(attempt) * 400 * time.Millisecond
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return base
	}
	delay := time.Duration(attempt) * 2 * time.Second
	if ra := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ra > delay {
		delay = ra
	}
	if delay > 15*time.Second {
		delay = 15 * time.Second
	}
	return delay
}

// parseRetryAfter parses a Retry-After header (seconds or HTTP-date); a
// missing or malformed header yields zero.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func errStringOrStatus(err error, resp *http.Response) string {
	if err != nil {
		return err.Error()
	}
	return "status " + strconv.Itoa(resp.StatusCode)
}

// ParseByteRange parses a single-range "bytes=" header against a body of the
// given size. Multi-range and unsatisfiable specs report ok=false, in which
// case the caller serves the full body.
func ParseByteRange(spec string, size int64) (start, end int64, ok bool) {
	spec = strings.TrimSpace(spec)
	if !strings.HasPrefix(spec, "bytes=") || size <= 0 {
		return 0, 0, false
	}
	part := strings.TrimSpace(strings.TrimPrefix(spec, "bytes="))
	if strings.Contains(part, ",") {
		return 0, 0, false // multi-range: serve the full body instead
	}
	dash := strings.Index(part, "-")
	if dash < 0 {
		return 0, 0, false
	}
	first, last := strings.TrimSpace(part[:dash]), strings.TrimSpace(part[dash+1:])
	switch {
	case first == "":
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	case last == "":
		s, err := strconv.ParseInt(first, 10, 64)
		if err != nil || s < 0 || s >= size {
			return 0, 0, false
		}
		return s, size - 1, true
	default:
		s, err1 := strconv.ParseInt(first, 10, 64)
		e, err2 := strconv.ParseInt(last, 10, 64)
		if err1 != nil || err2 != nil || s < 0 || s > e || s >= size {
			return 0, 0, false
		}
		if e >= size {
			e = size - 1
		}
		return s, e, true
	}
}

var parseByteRange = ParseByteRange

func IsBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1]&0xc0 == 0x40 ||
			ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) ||
			ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255
	}
	return false
}

var isBlockedIP = IsBlockedIP

// dnsCacheEntry memoizes the SSRF verdict for one hostname. Both allowed and
// blocked results are cached briefly: manifest/segment playback resolves the
// same CDN hostnames on every request, and re-resolving each time adds
// latency and DNS load. The TTL bounds staleness so DNS changes are picked up.
type dnsCacheEntry struct {
	blocked bool
	expires time.Time
}

const dnsCacheTTL = 5 * time.Minute

// blockedUpstreamHost reports whether a host must not be fetched upstream.
func (r *Resolver) blockedUpstreamHost(ctx context.Context, host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.Trim(host, "[]")
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isBlockedIP(ip)
	}
	now := time.Now()
	if v, ok := r.blockCache.Load(host); ok {
		entry := v.(dnsCacheEntry)
		if now.Before(entry.expires) {
			return entry.blocked
		}
		r.blockCache.Delete(host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return true // fail closed
	}
	blocked := false
	for _, ip := range ips {
		if isBlockedIP(ip) {
			blocked = true
			break
		}
	}
	r.blockCache.Store(host, dnsCacheEntry{blocked: blocked, expires: now.Add(dnsCacheTTL)})
	return blocked
}

// RedactQuery strips query strings from URLs before they are written to logs.
func RedactQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable URL)"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

var redactQuery = RedactQuery
