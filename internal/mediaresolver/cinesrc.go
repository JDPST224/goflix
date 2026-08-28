package mediaresolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// High-speed CineSrc programmatic resolver.
// Resolves CineSrc streams directly from its embed infrastructure
// (https://cinesrc.st/embed/movie/{id} or https://cinesrc.st/embed/tv/{id}?s={s}&e={e})
// using Next.js Server Actions and WebAssembly / VM challenge solvers.

type cinesrcWorker struct {
	mu        sync.Mutex
	allocCtx  context.Context
	cancelAll context.CancelFunc
	bCtx      context.Context
	cancelB   context.CancelFunc
	createdAt time.Time
}

var (
	globalCineWorkerMu sync.Mutex
	globalCineWorker   *cinesrcWorker
)

var cineBlockedPatterns = []string{
	"*.woff*", "*.woff2*", "*.ttf*", "*.otf*", "*.eot*",
	"*.png*", "*.webp*", "*.svg*", "*.ico*", "*.jpeg*", "*.jpg*", "*.gif*", "*.avif*",
	"*.css*",
	"*init.mp4*", "*playlist_*.jpg*", "*playlist_*.png*", "*playlist_*.jpeg*", "*.ts", "*.m4s",
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
	"*google-analytics.com*",
	"*googletagmanager.com*",
	"*speculation*",
	"*beacon*",
	"*rum*",
}

func getOrInitCineWorker(cfg Config) (*cinesrcWorker, error) {
	globalCineWorkerMu.Lock()
	defer globalCineWorkerMu.Unlock()

	if globalCineWorker != nil {
		healthy := globalCineWorker.bCtx != nil && globalCineWorker.bCtx.Err() == nil &&
			time.Since(globalCineWorker.createdAt) < 2*time.Hour
		if healthy {
			return globalCineWorker, nil
		}
		// Dead (browser crashed) or expired: release the old allocator
		// contexts unconditionally before re-initializing, or the replaced
		// worker's Chrome process/CDP resources leak.
		globalCineWorker.close()
		globalCineWorker = nil
	}

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", cfg.BrowserHeadless),
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
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("disable-remote-fonts", true),
	)
	if cfg.BrowserExecutable != "" {
		opts = append(opts, chromedp.ExecPath(cfg.BrowserExecutable))
	}

	allocCtx, cancelAll := chromedp.NewExecAllocator(context.Background(), opts...)
	bCtx, cancelB := chromedp.NewContext(allocCtx)

	const initScript = `
		window.__captured_d6 = null;
		window.addEventListener('_cs', (e) => {
			const key = e.detail;
			if (key && window[key]) {
				window.__captured_d6 = window[key];
			}
		});
	`

	if err := chromedp.Run(bCtx,
		network.Enable(),
		network.SetBlockedURLs(cineBlockedPatterns),
		chromedp.ActionFunc(func(c context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(initScript).Do(c)
			return err
		}),
	); err != nil {
		cancelB()
		cancelAll()
		return nil, err
	}

	globalCineWorker = &cinesrcWorker{
		allocCtx:  allocCtx,
		cancelAll: cancelAll,
		bCtx:      bCtx,
		cancelB:   cancelB,
		createdAt: time.Now(),
	}
	return globalCineWorker, nil
}

func (w *cinesrcWorker) close() {
	if w.cancelB != nil {
		w.cancelB()
	}
	if w.cancelAll != nil {
		w.cancelAll()
	}
}

type cinesrcDirectResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Details []any  `json:"details"`
	Result  struct {
		URL []struct {
			Source string `json:"source"`
			URL    string `json:"url"`
			Hash   string `json:"hash"`
		} `json:"url"`
		// Captions mirrors the upstream payload shape but is not consumed:
		// subtitles for cinesrc come from the generic server-side ladder
		// (FetchSubRenditions), embedded as TYPE=SUBTITLES renditions.
		Captions []struct {
			ID       string `json:"id"`
			Language string `json:"language"`
			URL      string `json:"url"`
		} `json:"captions"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
	} `json:"result"`
}

// tryCinesrcDirect resolves cinesrc, registers the proxy session, and primes
// the body cache. It returns false if resolution fails so Resolve can fall back.
func (r *Resolver) tryCinesrcDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()

	cr, err := r.resolveCinesrcDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] cinesrc direct resolve unavailable: %v", err)
		return "", false
	}
	token, err := r.newSession(cr.Source, cr.Headers, cr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] cinesrc direct session failed: %v", err)
		return "", false
	}
	if cr.MasterText != "" {
		// Admit the validated master under its canonical URL with a long TTL:
		// warmup and the proxy fast path serve it from RAM instead of
		// refetching the upstream.
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

// resolveCinesrcDirect acquires a concurrency semaphore slot and executes programmatic resolution.
func (r *Resolver) resolveCinesrcDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return r.resolveCinesrcProgrammatic(ctx, req)
}

func (r *Resolver) resolveCinesrcProgrammatic(ctx context.Context, req MediaRequest) (*directResolution, error) {
	worker, werr := getOrInitCineWorker(r.cfg)
	if werr != nil || worker == nil {
		return nil, fmt.Errorf("warm worker not initialized: %w", werr)
	}

	worker.mu.Lock()
	defer worker.mu.Unlock()

	if worker.bCtx == nil || worker.bCtx.Err() != nil {
		return nil, errors.New("warm worker context closed")
	}

	origin := r.cfg.CineSrcOrigin
	if origin == "" {
		origin = "https://cinesrc.st"
	}
	origin = strings.TrimRight(origin, "/")

	bType := "movie"
	if req.Type == TV {
		bType = "tv"
	}
	sParam := strings.TrimSpace(req.Season)
	eParam := strings.TrimSpace(req.Episode)
	if sParam == "0" {
		sParam = ""
	}
	if eParam == "0" {
		eParam = ""
	}
	hasSeasonEp := bType == "tv" && sParam != "" && eParam != ""

	targetURL := origin + "/embed/" + bType + "/" + url.PathEscape(req.ID)
	if hasSeasonEp {
		targetURL += "?s=" + url.QueryEscape(sParam) + "&e=" + url.QueryEscape(eParam)
	}

	evalScript := fmt.Sprintf(`
		(async () => {
			let d6 = null;
			for (let i = 0; i < 400; i++) {
				d6 = window.__captured_d6;
				if (d6?.gc && d6?.dr && window.__ss2_challenge?.gc) break;
				await new Promise(r => setTimeout(r, 10));
			}
			if (!d6?.gc || !d6?.dr || !window.__ss2_challenge?.gc) {
				return JSON.stringify({ error: "challenge handlers unavailable" });
			}
			const ss2 = window.__ss2_challenge;

			const bType = %q;
			const rawID = %q;
			const r = %q || null;
			const s = %q || null;
			const targetURL = %q;

			let n = new TextEncoder().encode(JSON.stringify([bType, rawID, r, s]));
			let a = "";
			for (let e of n) a += String.fromCharCode(e);
			let l = btoa(a).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");

			let actType = bType === "movie" ? "movie" : "show";
			let sVal = r ? String(r) : "$undefined";
			let eVal = s ? String(s) : "$undefined";
			const servers = ["nebula", "lisbon", "surge", "spark", "storm"];

			async function getCompoundToken() {
				let bResp;
				let bData;
				for (let attempt = 0; attempt < 25; attempt++) {
					bResp = await fetch("/api/c/bootstrap", { method: "POST", headers: { "x-cs-q": l }, credentials: "include", cache: "no-store" });
					if (bResp.status === 428) {
						await new Promise(res => setTimeout(res, 50));
						continue;
					}
					if (!bResp.ok) throw Error("bootstrap " + bResp.status);
					bData = await bResp.json();
					break;
				}
				if (!bData || !bData.r) throw Error("bootstrap timeout");
				let d = bData.r;
				let h = bData.p;

				let origFetch = window.fetch;
				let boundFetch = origFetch.bind(window);
				let interceptFetch = (e, t) => {
					let i = "string" == typeof e || e instanceof URL ? String(e) : e.url;
					let r = new URL(i, window.location.href);
					if (r.origin === window.location.origin && ("/api/c/issue" === r.pathname || "/api/c/stage2/issue" === r.pathname)) {
						let i = new Headers(e instanceof Request ? e.headers : t?.headers);
						if (t?.headers) {
							new Headers(t.headers).forEach((e, t) => i.set(t, e));
						}
						i.set("x-cs-r", d);
						i.set("x-cs-q", l);
						if ("/api/c/issue" === r.pathname) {
							i.set("x-cs-p", h);
						}
						return e instanceof Request ? boundFetch(new Request(e, { ...t, headers: i })) : boundFetch(e, { ...t, headers: i });
					}
					return boundFetch(e, t);
				};
				window.fetch = interceptFetch;

				let c1, c2;
				try {
					[c1, c2] = await Promise.all([d6.gc(), ss2.gc()]);
				} finally {
					window.fetch = origFetch;
				}
				return c1 + "::c2::" + c2 + "::c3::" + d;
			}

			for (let tokenAttempt = 0; tokenAttempt < 3; tokenAttempt++) {
				let token;
				try {
					token = await getCompoundToken();
				} catch (e) {
					await new Promise(res => setTimeout(res, 80));
					continue;
				}

				let hadInvalidChallenge = false;
				for (const srv of servers) {
					try {
						const streamResp = await fetch(targetURL, {
							method: "POST",
							headers: {
								"Accept": "text/x-component",
								"Content-Type": "text/plain;charset=UTF-8",
								"next-action": "7e401aae5708c04984ff004de286425e0af9166da6",
								"next-router-state-tree": "%%5B%%22%%22%%2C%%7B%%22children%%22%%3A%%5B%%22embed%%22%%2C%%7B%%22children%%22%%3A%%5B%%5B%%22type%%22%%2C%%22" + bType + "%%22%%2C%%22d%%22%%5D%%2C%%7B%%22children%%22%%3A%%5B%%5B%%22id%%22%%2C%%22" + rawID + "%%22%%2C%%22d%%22%%5D%%2C%%7B%%22children%%22%%3A%%5B%%22__PAGE__%%22%%2C%%7B%%7D%%2Cnull%%2Cnull%%5D%%7D%%2Cnull%%2Cnull%%5D%%7D%%2Cnull%%2Cnull%%5D%%7D%%2Cnull%%2Cnull%%5D%%7D%%2Cnull%%2Cnull%%2Ctrue%%5D"
							},
							body: JSON.stringify([rawID, actType, sVal, eVal, token, srv])
						});
						const actionText = await streamResp.text();
						if (actionText.includes("e1:invalid_challenge")) {
							hadInvalidChallenge = true;
							break;
						}
						if (actionText.includes("e1:")) continue;

						let rawCipher = null;
						const matchQuote = actionText.match(/[0-9]:"([^"]+)"/);
						if (matchQuote && matchQuote[1].startsWith("r2.")) {
							rawCipher = matchQuote[1];
						} else {
							const matchFlight = actionText.match(/[0-9]:T[0-9a-fA-F]+,(r2\.[^\n\r]+)/);
							if (matchFlight) {
								let str = matchFlight[1];
								const trailIdx = str.search(/[0-9]:"|\n|\r/);
								if (trailIdx !== -1) str = str.slice(0, trailIdx);
								rawCipher = str;
							}
						}

						if (!rawCipher) continue;
						let dec = await d6.dr(rawCipher);
						if (dec && dec.url && dec.url.length > 0) {
							return JSON.stringify({ success: true, result: dec });
						}
					} catch (e) {
						// continue
					}
				}
				if (hadInvalidChallenge) {
					await new Promise(res => setTimeout(res, 80));
					continue;
				}
			}
			return JSON.stringify({ error: "all cinesrc servers failed" });
		})()
	`, bType, req.ID, sParam, eParam, targetURL)

	var resJSON string
	err := chromedp.Run(worker.bCtx,
		chromedp.ActionFunc(func(c context.Context) error {
			_, _, _, _, err := page.Navigate(targetURL).Do(c)
			return err
		}),
		chromedp.Evaluate(evalScript, &resJSON, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("eval error: %w", err)
	}

	var res cinesrcDirectResult
	if err := json.Unmarshal([]byte(resJSON), &res); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	if !res.Success || len(res.Result.URL) == 0 {
		return nil, fmt.Errorf("direct resolve failed: %s", res.Error)
	}

	hlsURL := strings.TrimSpace(res.Result.URL[0].URL)
	if hlsURL == "" {
		return nil, errors.New("empty HLS URL in direct result")
	}

	// Validate the resolved URL before it becomes a session — a junk decrypt
	// result must fall back to the browser scrape instead of producing a
	// session whose first manifest fetch fails on the player. The fetched
	// master is also seeded into the body cache (same as the other direct
	// paths), so the player's first manifest fetch is served from RAM.
	if u, err := url.Parse(hlsURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("resolved HLS URL is not a valid http(s) URL")
	} else if r.blockedUpstreamHost(ctx, u.Hostname()) {
		return nil, fmt.Errorf("resolved HLS host %q blocked", u.Hostname())
	}

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", origin+"/")
	headers.Set("Origin", origin)

	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}
	masterText, err := r.fetchManifestText(ctx, client, hlsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("cinesrc master playlist validation: %w", err)
	}
	if !strings.Contains(masterText, "#EXT-X-STREAM-INF") && !strings.Contains(masterText, "#EXTINF") {
		return nil, errors.New("cinesrc master playlist has no variants or segments")
	}

	allowed := make(map[string]bool)
	if u, err := url.Parse(hlsURL); err == nil {
		allowed[strings.ToLower(u.Host)] = true
	}
	if ref := headers.Get("Referer"); ref != "" {
		if ru, err := url.Parse(ref); err == nil && ru.Host != "" {
			allowed[strings.ToLower(ru.Host)] = true
		}
	}

	return &directResolution{
		Source:     hlsURL,
		Headers:    headers,
		Allowed:    allowed,
		MasterText: masterText,
	}, nil
}
