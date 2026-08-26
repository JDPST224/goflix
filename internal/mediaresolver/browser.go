package mediaresolver

// Headless-browser resolution pipeline: page automation, candidate
// harvesting and HLS classification.

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (r *Resolver) resolveInBrowser(parent context.Context, target string) (string, http.Header, map[string]bool, error) {
	log.Printf("[MediaResolver] Opening browser")
	browserParent := parent
	if r.cfg.BrowserTimeout > 0 {
		var cancel context.CancelFunc
		browserParent, cancel = context.WithTimeout(parent, r.cfg.BrowserTimeout)
		defer cancel()
	}

	// Build a lean set of Chromium flags optimised for headless media scraping:
	// disable everything that isn't needed for network interception while maintaining stealth.
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", r.cfg.BrowserHeadless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-quic", true),
		chromedp.Flag("user-agent", defaultUserAgent),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.Flag("lang", "en-US,en"),
		// Reduce startup overhead and memory footprint.
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-features", "TranslateUI,BlinkGenPropertyTrees"),
		chromedp.Flag("disable-popup-blocking", true),
	)
	if r.cfg.BrowserExecutable != "" {
		opts = append(opts, chromedp.ExecPath(r.cfg.BrowserExecutable))
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(browserParent, opts...)
	defer cancelAlloc()
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()

	var mu sync.Mutex
	candidates := make([]hlsCandidate, 0, 8)
	requestHeaders := make(map[string]http.Header)
	hostHeaders := make(map[string]http.Header)

	// hlsFound is signalled (non-blocking) the moment the first HLS candidate
	// is captured, allowing the wait loop below to exit immediately instead of
	// polling every 200 ms.
	hlsFound := make(chan struct{}, 1)

	chromedp.ListenTarget(ctx, func(ev any) {
		e, ok := ev.(*network.EventResponseReceived)
		if !ok || e.Response == nil || !isPotentialHLS(e.Response.URL, e.Response.MimeType) {
			return
		}
		mu.Lock()
		h := cloneHeader(requestHeaders[e.Response.URL])
		delete(requestHeaders, e.Response.URL) // consumed; keep the map bounded
		c := hlsCandidate{url: e.Response.URL, contentType: e.Response.MimeType, status: e.Response.Status, headers: h}
		candidates = append(candidates, c)
		mu.Unlock()
		log.Printf("[MediaResolver] HLS source detected url=%s status=%d mime=%s", redactQuery(c.url), c.status, c.contentType)
		// Signal immediately — non-blocking so multiple events don't deadlock.
		select {
		case hlsFound <- struct{}{}:
		default:
		}
	})

	chromedp.ListenTarget(ctx, func(ev any) {
		e, ok := ev.(*network.EventRequestWillBeSent)
		if !ok {
			return
		}
		h := make(http.Header)
		for k, v := range e.Request.Headers {
			if value, ok := v.(string); ok {
				switch strings.ToLower(k) {
				case "user-agent", "referer", "origin", "cookie", "accept", "accept-language":
					h.Set(k, value)
				}
			}
		}
		parsed, err := url.Parse(e.Request.URL)
		if err != nil || parsed.Host == "" {
			return
		}
		host := strings.ToLower(parsed.Host)
		mu.Lock()
		// Cap the per-URL header map: long-lived pages can issue thousands of
		// requests and most are never matched to an HLS response.
		if len(requestHeaders) < 512 {
			requestHeaders[e.Request.URL] = cloneHeader(h)
		}
		// Retain the latest safe browser headers per host.
		if len(h) > 0 {
			hostHeaders[host] = cloneHeader(h)
		}
		mu.Unlock()
	})

	if strings.Contains(strings.ToLower(target), "vidking.net/embed/") || strings.Contains(strings.ToLower(target), "vidlove.cc/embed/") {
		log.Printf("[MediaResolver] Embed used only to discover its HLS manifest; frontend will receive the proxied HLS URL")
	}

	// Enable network interception synchronously before navigation so no events are missed.
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		return "", nil, nil, fmt.Errorf("network.Enable failed: %w", err)
	}

	navDone := make(chan error, 1)
	go func() {
		log.Printf("[MediaResolver] Navigating to media page (async)")
		var navErr error
		for attempt := 1; attempt <= 3; attempt++ {
			navErr = chromedp.Run(ctx, chromedp.Navigate(target))
			if navErr == nil {
				break
			}
			msg := navErr.Error()
			log.Printf("[MediaResolver] Navigation attempt %d/3 failed error=%v", attempt, navErr)
			if !strings.Contains(strings.ToLower(msg), "err_connection_reset") {
				break
			}
			select {
			case <-parent.Done():
				navDone <- parent.Err()
				return
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		navDone <- navErr
	}()

	// Click-to-play automation: some embed pages render a poster/overlay that
	// must be clicked to start the video player. A single JS evaluation sweeps
	// all known play-button selectors and clicks the first match — chromedp's
	// own Click action polls for the node until the context deadline, so the
	// first missing selector would stall the ladder for the whole browser
	// timeout and later selectors would never be tried.
	clickToPlayJS := `(function(){
		var sels = ['.vjs-big-play-button', '.jw-icon-display', '.plyr__control--overlaid',
			'button[aria-label*="play" i]', '.play-button', '.btn-play', '#play-btn',
			'[class*="play-btn"]', '[class*="playBtn"]', 'video'];
		for (var i = 0; i < sels.length; i++) {
			var el = document.querySelector(sels[i]);
			if (el) { el.click(); return sels[i]; }
		}
		return "";
	})()`

	go func() {
		// Two attempts: some pages build their player overlay later than
		// others. Candidate checks run under mu — this goroutine must not
		// receive from hlsFound or it consumes the wait loop's wake-up token.
		delay := 3 * time.Second
		for attempt := 1; attempt <= 2; attempt++ {
			select {
			case <-time.After(delay):
			case <-parent.Done():
				return
			}
			mu.Lock()
			found := chooseCandidate(candidates).url != ""
			mu.Unlock()
			if found {
				return
			}
			var clicked string
			if err := chromedp.Run(ctx, chromedp.Evaluate(clickToPlayJS, &clicked)); err != nil {
				log.Printf("[MediaResolver] Click-to-play attempt %d failed error=%v", attempt, err)
				return
			}
			if clicked != "" {
				log.Printf("[MediaResolver] Click-to-play clicked %q", clicked)
			}
			delay = 5 * time.Second
		}
	}()

	log.Printf("[MediaResolver] Waiting for HLS source")

	var firstFound time.Time
	navConsumed := false
	failedProbes := make(map[string]bool)
	validatedProbes := make(map[string]bool)
	masterCands := make(map[string]bool)

	// buildPlaybackContext merges the candidate's captured request headers
	// with the per-host fallback headers observed in the browser session,
	// and derives the set of upstream hosts playback is allowed to touch.
	buildPlaybackContext := func(c hlsCandidate) (http.Header, map[string]bool) {
		merged := cloneHeader(c.headers)
		if merged.Get("User-Agent") == "" {
			merged.Set("User-Agent", defaultUserAgent)
		}
		if u, e := url.Parse(c.url); e == nil {
			if fallback := hostHeaders[strings.ToLower(u.Host)]; fallback != nil {
				merged = mergeHeaders(merged, fallback)
			}
		}
		allowed := map[string]bool{}
		if u, e := url.Parse(c.url); e == nil {
			allowed[strings.ToLower(u.Host)] = true
		}
		if ref := merged.Get("Referer"); ref != "" {
			if ru, e := url.Parse(ref); e == nil && ru.Host != "" {
				allowed[strings.ToLower(ru.Host)] = true
			}
		}
		return merged, allowed
	}

	for {
		mu.Lock()
		best := chooseCandidate(candidates)
		var ranked []hlsCandidate
		if best.url != "" {
			ranked = rankCandidates(candidates)
		}
		mu.Unlock()

		if len(ranked) > 0 {
			if firstFound.IsZero() {
				firstFound = time.Now()
			}
			// Give the page a brief grace period to surface additional
			// candidates (master + variants), then validate them best-first.
			// This prevents a decoy/broken .m3u8 (ad interstitials, teaser
			// manifests) from being handed to the player and causing
			// fragment parse errors downstream. Canonical primary-manifest URL
			// shapes (/playlist/, master.m3u8, manifest.m3u8) are validated
			// immediately — waiting out the full grace would only add startup
			// delay when the real manifest is already on the table.
			confident := candidateScore(ranked[0]) >= confidentCandidateScore
			if navConsumed || confident || time.Since(firstFound) >= candidateGracePeriod {
				// Probe every not-yet-classified candidate; results accumulate
				// across wake-ups so each URL is probed at most once.
				for _, cand := range ranked {
					if failedProbes[cand.url] || validatedProbes[cand.url] {
						continue
					}
					merged, _ := buildPlaybackContext(cand)
					ok, master := r.probeManifestClass(browserParent, cand.url, merged)
					if !ok {
						failedProbes[cand.url] = true
						log.Printf("[MediaResolver] Candidate failed manifest validation url=%s", redactQuery(cand.url))
						continue
					}
					validatedProbes[cand.url] = true
					if master {
						masterCands[cand.url] = true
					}
				}
				// Prefer a master playlist over single-tier media playlists: a
				// media playlist pins playback to one quality tier, while a
				// master lets the player's ABR move between tiers (warmup still
				// pre-warms the top tier). Within each group the earlier rank
				// order stands.
				chosen := hlsCandidate{}
				for _, cand := range ranked {
					if validatedProbes[cand.url] && masterCands[cand.url] {
						chosen = cand
						break
					}
				}
				if chosen.url == "" {
					for _, cand := range ranked {
						if validatedProbes[cand.url] {
							chosen = cand
							break
						}
					}
				}
				if chosen.url != "" {
					merged, allowed := buildPlaybackContext(chosen)
					return chosen.url, merged, allowed, nil
				}
				if navConsumed {
					return "", nil, nil, errors.New("captured HLS sources failed manifest validation")
				}
				// Navigation is still running — keep waiting in case a valid
				// candidate appears later.
			}
		}

		graceRemaining := candidateGracePeriod
		if !firstFound.IsZero() {
			graceRemaining = candidateGracePeriod - time.Since(firstFound)
			if graceRemaining < 50*time.Millisecond {
				graceRemaining = 50 * time.Millisecond
			}
		}

		select {
		case <-parent.Done():
			return "", nil, nil, parent.Err()
		case <-browserParent.Done():
			// The browser deadline fired without a usable HLS source. Without
			// this case the loop below would tick forever, never returning and
			// holding the Resolve semaphore slot until the HTTP client gives up.
			return "", nil, nil, fmt.Errorf("resolution timed out: %w", browserParent.Err())
		case navErr := <-navDone:
			navConsumed = true
			if navErr != nil && len(ranked) == 0 {
				return "", nil, nil, fmt.Errorf("navigation failed: %w", navErr)
			}
			if navErr != nil {
				log.Printf("[MediaResolver] Navigation error but HLS already captured; continuing error=%v", navErr)
			}
		case <-hlsFound:
			// loop again to pick up the candidate immediately
		case <-time.After(graceRemaining):
			// Wake up to re-check the candidate-grace deadline.
		}
	}
}

// probeManifestClass fetches the first bytes of an HLS URL and classifies the
// playlist: ok reports that it is actually playable (#EXTM3U header plus at
// least one variant or segment entry — HTML error pages, JSON stubs and empty
// bodies fail), and master reports that it is a master playlist listing
// multiple variants. Callers prefer masters so playback starts on the highest
// tier instead of whatever single-tier media playlist answered first.
func (r *Resolver) probeManifestClass(ctx context.Context, raw string, headers http.Header) (ok bool, master bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return false, false
	}
	if r.blockedUpstreamHost(ctx, u.Hostname()) {
		return false, false
	}
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, false
	}
	for _, k := range playbackHeaders {
		if v := headers.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false, false
	}
	reader := io.Reader(resp.Body)
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return false, false
		}
		defer gz.Close()
		reader = gz
	}
	data, _ := io.ReadAll(io.LimitReader(reader, manifestProbeBytes))
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, "#EXTM3U") {
		return false, false
	}
	// A real playlist contains either variant entries (master) or media
	// segments (media playlist); only a master lists multiple quality tiers.
	isMaster := strings.Contains(text, "#EXT-X-STREAM-INF")
	return isMaster || strings.Contains(text, "#EXTINF"), isMaster
}

func isPotentialHLS(raw, mime string) bool {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	// Standard HLS MIME types.
	if strings.Contains(m, "mpegurl") {
		return true
	}
	// URL-based detection: match .m3u8 anywhere in the full URL (path or query string).
	if strings.Contains(strings.ToLower(raw), "m3u8") {
		return true
	}
	// Some providers serve manifests as application/octet-stream or text/plain.
	if m == "application/octet-stream" || m == "text/plain" {
		p := strings.ToLower(u.Path)
		return strings.Contains(p, "playlist") || strings.Contains(p, "master") ||
			strings.Contains(p, "manifest") || strings.Contains(p, "index") ||
			strings.Contains(p, "stream") || strings.Contains(p, "video")
	}
	return false
}

// candidateScore ranks how likely an HLS candidate is the primary playback
// manifest. Higher is better.
func candidateScore(c hlsCandidate) int {
	lower := strings.ToLower(c.url)
	switch {
	case strings.Contains(lower, "/playlist/"):
		return 100
	case strings.Contains(lower, "master.m3u8"):
		return 95
	case strings.Contains(lower, "manifest.m3u8"):
		return 90
	case strings.Contains(lower, "master") && strings.Contains(lower, ".m3u8"):
		return 85
	case strings.Contains(lower, "index.m3u8") || strings.Contains(lower, "stream.m3u8"):
		return 80
	case strings.HasSuffix(strings.Split(lower, "?")[0], ".m3u8"):
		return 70
	case strings.Contains(lower, ".m3u8"):
		return 60
	case strings.Contains(strings.ToLower(c.contentType), "mpegurl"):
		return 50
	default:
		return 20
	}
}

// CandidateScore ranks how likely an HLS candidate URL is the primary playback manifest.
func CandidateScore(rawURL, contentType string) int {
	return candidateScore(hlsCandidate{url: rawURL, contentType: contentType})
}


func chooseCandidate(cs []hlsCandidate) hlsCandidate {
	bestScore := -1
	var best hlsCandidate
	for _, c := range cs {
		if c.status < 200 || c.status >= 400 {
			continue
		}
		if s := candidateScore(c); s > bestScore {
			bestScore = s
			best = c
		}
	}
	return best
}

// rankCandidates returns the playable candidates ordered best-first so a
// failing primary manifest can fall back to the next-best capture.
func rankCandidates(cs []hlsCandidate) []hlsCandidate {
	playable := make([]hlsCandidate, 0, len(cs))
	for _, c := range cs {
		if c.status < 200 || c.status >= 400 {
			continue
		}
		playable = append(playable, c)
	}
	// Insertion sort by descending score (stable; candidate lists are tiny).
	for i := 1; i < len(playable); i++ {
		for j := i; j > 0 && candidateScore(playable[j]) > candidateScore(playable[j-1]); j-- {
			playable[j], playable[j-1] = playable[j-1], playable[j]
		}
	}
	return playable
}
