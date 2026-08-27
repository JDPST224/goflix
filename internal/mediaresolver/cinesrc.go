package mediaresolver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Direct CineSrc resolution. Resolves CineSrc streams from its embed page
// (https://cinesrc.st/embed/movie/{id} or https://cinesrc.st/embed/tv/{id}?s={s}&e={e}).
//
// The embed uses Cloudflare Turnstile, encrypted challenges (via /donut.js and
// /130626-prod.js), and Next.js server actions. We resolve it via a high-performance,
// resource-filtered headless browser instance that caches bytecode/WASM on disk,
// suppresses non-essential assets (fonts, images, ad trackers), intercepts the
// master playlist immediately upon appearance, and verifies it in a single pass.

// tryCinesrcDirect resolves cinesrc, registers the proxy session, and primes
// the body cache. It returns false if resolution fails so Resolve can fall back.
func (r *Resolver) tryCinesrcDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()

	cr, err := r.resolveCinesrcDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] cinesrc direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(cr.Source, cr.Headers, cr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] cinesrc direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	if cr.MasterText != "" {
		r.cache.put(&cacheEntry{
			key:         cr.Source,
			data:        []byte(cr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	log.Printf("[MediaResolver] cinesrc resolved directly source=%s", redactQuery(cr.Source))
	r.attachAndWarm(token, req)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// resolveCinesrcDirect acquires a concurrency semaphore slot and runs the optimized fast-path.
func (r *Resolver) resolveCinesrcDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	target, err := r.cinesrcTargetURL(req)
	if err != nil {
		return nil, err
	}

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return r.resolveCinesrcFast(ctx, target)
}

// resolveCinesrcFast boots an optimized browser session with a persistent disk cache
// profile, blocks unneeded media/font/ad requests, and captures the master playlist.
func (r *Resolver) resolveCinesrcFast(ctx context.Context, target string) (*directResolution, error) {
	profileDir := filepath.Join(os.TempDir(), "goflix_cinesrc_profile")
	_ = os.MkdirAll(profileDir, 0700)

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", r.cfg.BrowserHeadless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-quic", true),
		chromedp.Flag("user-agent", defaultUserAgent),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("window-size", "1280,720"),
		chromedp.Flag("lang", "en-US,en"),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-features", "TranslateUI,BlinkGenPropertyTrees"),
		chromedp.UserDataDir(profileDir),
	)
	if r.cfg.BrowserExecutable != "" {
		opts = append(opts, chromedp.ExecPath(r.cfg.BrowserExecutable))
	}

	alloc, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	bCtx, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()

	var mu sync.Mutex
	var masterURL string
	capturedHeaders := make(http.Header)
	hlsFound := make(chan struct{}, 1)

	chromedp.ListenTarget(bCtx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if strings.Contains(e.Request.URL, "master.m3u8") {
				mu.Lock()
				for k, v := range e.Request.Headers {
					if s, ok := v.(string); ok {
						switch strings.ToLower(k) {
						case "user-agent", "referer", "origin", "cookie", "accept", "accept-language":
							capturedHeaders.Set(k, s)
						}
					}
				}
				mu.Unlock()
			}
		case *network.EventResponseReceived:
			u := e.Response.URL
			if strings.Contains(u, "master.m3u8") && e.Response.Status >= 200 && e.Response.Status < 400 {
				mu.Lock()
				masterURL = u
				mu.Unlock()
				select {
				case hlsFound <- struct{}{}:
				default:
				}
			}
		}
	})

	// Block heavy resources and tracking scripts that CineSrc does not need for stream resolution.
	blockedPatterns := []string{
		"*.woff", "*.woff2", "*.ttf", "*.otf", "*.eot",
		"*.png", "*.webp", "*.svg", "*.ico",
		"*image.tmdb.org*",
		"*api.themoviedb.org*",
		"*cineflix.st*",
		"*cloudflareinsights.com*",
		"*llvpn.com*",
		"*rtmark.net*",
		"*luugy.com*",
		"*adexchangerapid.com*",
		"*usrpubtrk.com*",
		"*gstatic.com*",
	}

	if err := chromedp.Run(bCtx,
		network.Enable(),
		network.SetBlockedURLs(blockedPatterns),
	); err != nil {
		return nil, fmt.Errorf("network initialization: %w", err)
	}

	go func() {
		_ = chromedp.Run(bCtx, chromedp.Navigate(target))
	}()

	select {
	case <-hlsFound:
	case <-ctx.Done():
		return nil, fmt.Errorf("cinesrc resolution timeout: %w", ctx.Err())
	}

	mu.Lock()
	mURL := masterURL
	headers := capturedHeaders.Clone()
	mu.Unlock()

	if mURL == "" {
		return nil, errors.New("no HLS source found")
	}

	allowed := make(map[string]bool)
	if u, err := url.Parse(mURL); err == nil {
		allowed[strings.ToLower(u.Host)] = true
	}
	if ref := headers.Get("Referer"); ref != "" {
		if ru, err := url.Parse(ref); err == nil && ru.Host != "" {
			allowed[strings.ToLower(ru.Host)] = true
		}
	}

	// Single probe to validate and capture MasterText for cache priming in one step.
	client := &http.Client{Transport: r.transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mURL, nil)
	if err != nil {
		return &directResolution{Source: mURL, Headers: headers, Allowed: allowed}, nil
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
		return &directResolution{Source: mURL, Headers: headers, Allowed: allowed}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
		if rerr == nil && len(body) > 0 {
			textStr := strings.TrimSpace(string(body))
			if strings.HasPrefix(textStr, "#EXTM3U") {
				return &directResolution{
					Source:     mURL,
					Headers:    headers,
					Allowed:    allowed,
					MasterText: textStr,
				}, nil
			}
		}
	}

	return &directResolution{
		Source:  mURL,
		Headers: headers,
		Allowed: allowed,
	}, nil
}

func (r *Resolver) cinesrcTargetURL(req MediaRequest) (string, error) {
	origin := r.cfg.CineSrcOrigin
	if origin == "" {
		origin = "https://cinesrc.st"
	}
	origin = strings.TrimRight(origin, "/")
	if req.Type == Movie {
		return origin + "/embed/movie/" + url.PathEscape(req.ID), nil
	}
	return fmt.Sprintf("%s/embed/tv/%s?s=%s&e=%s",
		origin, url.PathEscape(req.ID), url.QueryEscape(req.Season), url.QueryEscape(req.Episode)), nil
}
