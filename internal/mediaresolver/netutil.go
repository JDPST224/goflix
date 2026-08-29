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
	"time"
)

// upstreamAttempts bounds retries for playback fetches that fail before the
// body starts flowing, and retryableUpstreamStatus lists transient statuses
// worth re-requesting. Retries are safe in both fetch paths that use them:
// nothing has been forwarded to the player yet, and HLS traffic is idempotent
// GETs. vidlove's CDN occasionally accepts a request on an HTTP/2 connection
// and then never answers it; a retry lands on a fresh (or different) host and
// typically succeeds immediately.
const upstreamAttempts = 3

func retryableUpstreamStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// doWithRetry runs attempt() up to upstreamAttempts times. An attempt is
// retried when it errors, or when it returns one of the transient statuses —
// in which case its body is drained and closed first. The final response (or
// error) is returned; ctx cancellation aborts between attempts.
func doWithRetry(ctx context.Context, logPrefix string, attempt func() (*http.Response, error)) (*http.Response, error) {
	for n := 1; ; n++ {
		resp, err := attempt()
		if err == nil && (!retryableUpstreamStatus(resp.StatusCode) || n >= upstreamAttempts) {
			return resp, nil
		}
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
		select {
		case <-ctx.Done():
			if err == nil {
				return nil, ctx.Err()
			}
			return nil, err
		case <-time.After(time.Duration(n) * 400 * time.Millisecond):
		}
	}
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

