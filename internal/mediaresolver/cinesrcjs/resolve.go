package cinesrcjs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Result is a successful direct resolution.
type Result struct {
	Source   string // HLS master playlist URL
	Provider string // server id ("nebula", ...)
	Captions []Caption
}

// Caption mirrors the decrypted captions array (not consumed by the proxy;
// subtitles come from the server-side ladder).
type Caption struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	URL      string `json:"url"`
}

// Resolver runs the cinesrc challenge chain. Create one per process and
// reuse it: the challenge scripts and PoW WASM are fetched once and cached.
type Resolver struct {
	Origin      string // default "https://cinesrc.st"
	UserAgent   string // browser UA; must look genuine
	Fingerprint Fingerprint
	Transport   http.RoundTripper
	Logf        func(format string, args ...any)
	Timeout     time.Duration // per resolve; default 45s

	mu               sync.Mutex
	assets           *assets
	pow              *powRuntime       // compiled once, shared across resolves
	pkBody           map[string]string // /api/c/pk responses (static key), keyed by URL
	canvasCalls      int
	providers        []string // ranked source-server ids from getProviderList
	providersFetched time.Time
}

type assets struct {
	prodURL string
	prod    []byte
	donut   []byte
	powWasm []byte
	fetched time.Time
}

const (
	getStreamAction       = "7e401aae5708c04984ff004de286425e0af9166da6"
	getProviderListAction = "0031badf5f0118ff0585f0c9553aa8b9c28992c568"
)

var (
	reChunkSrc = regexp.MustCompile(`/_next/static/chunks/([A-Za-z0-9_-]+)\.js`)
	reProdName = regexp.MustCompile(`/([A-Za-z0-9_-]+-prod\.js)`)
	rePowWasm  = regexp.MustCompile(`/pow-v\d+\.wasm`)
	reServerID = regexp.MustCompile(`\{"id":"([a-z0-9_-]+)","name"`)
)

// Resolve performs the full chain per source server: bootstrap → issue →
// stage2 → pk → PoW → tokens → getStream action → decrypt. The challenge
// session is single-use (the first getStream call consumes it), so every
// server attempt runs a fresh session. mediaType is "movie" or "tv";
// season/episode are empty for movies. Servers are tried in provider-rank
// order (fetched from the embed's getProviderList action) until one has the
// stream.
func (r *Resolver) Resolve(ctx context.Context, mediaType, tmdbID, season, episode string, servers []string) (*Result, error) {
	if r.Timeout == 0 {
		r.Timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	phase := func(name string, start time.Time) {
		r.logf("phase %s: %d ms", name, time.Since(start).Milliseconds())
	}

	origin := r.Origin
	if origin == "" {
		origin = "https://cinesrc.st"
	}
	ua := r.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	}

	t0 := time.Now()
	if err := r.ensureAssets(ctx, origin, ua); err != nil {
		return nil, err
	}
	phase("assets", t0)

	pagePath := "/embed/" + mediaType + "/" + tmdbID
	if season != "" && episode != "" {
		pagePath += "?s=" + url.QueryEscape(season) + "&e=" + url.QueryEscape(episode)
	}

	if len(servers) == 0 {
		servers = r.providerList(ctx, origin, ua)
	}

	var lastErr error
	for _, srv := range servers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		res, err := r.attemptServer(ctx, origin, ua, mediaType, tmdbID, season, episode, pagePath, srv)
		if err != nil {
			lastErr = err
			r.logf("cinesrcjs server %q unusable: %v", srv, err)
			continue
		}
		r.logf("phase total: %d ms", time.Since(t0).Milliseconds())
		r.logf("cinesrcjs resolved directly server=%q source=%s", res.Provider, redact(res.Source))
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no servers attempted")
	}
	return nil, fmt.Errorf("cinesrcjs: all %d servers failed: %w", len(servers), lastErr)
}

// attemptServer runs one complete challenge session against a single source
// server. The compound token is single-use, so each server needs its own
// bootstrap + gc.
func (r *Resolver) attemptServer(ctx context.Context, origin, ua, mediaType, tmdbID, season, episode, pagePath, srv string) (*Result, error) {
	// Two independent runtimes: donut (stage2 VM) and the challenge module
	// (d6). Their gc() calls run in parallel on separate goroutines — goja
	// is single-threaded per runtime, so this halves the VM wall time.
	// Both share one cookie jar so the bootstrap session cookies reach the
	// issue endpoints whichever runtime issues them.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	console := func(s string) { r.logf("vm: %s", s) }
	pathBase := "/embed/" + mediaType + "/" + tmdbID
	rtDonut, err := newRuntime(origin, pathBase, ua, r.fingerprint(), jar, r.Transport, console)
	if err != nil {
		return nil, err
	}
	rtDonut.getPow = r.sharedPow
	rtDonut.getPK = r.pkLookup
	rtDonut.storePK = r.pkStore
	rtDonut.nextCanvas = r.nextCanvas

	rtD6, err := newRuntime(origin, pathBase, ua, r.fingerprint(), jar, r.Transport, console)
	if err != nil {
		return nil, err
	}
	rtD6.getPow = r.sharedPow
	rtD6.getPK = r.pkLookup
	rtD6.storePK = r.pkStore
	rtD6.nextCanvas = r.nextCanvas

	// capture the challenge module as it registers itself
	if _, err := rtD6.vm.RunString(`
		var __d6 = null;
		addEventListener("_cs", function(ev){
			var key = ev.detail;
			if (key && window[key]) { __d6 = window[key]; try { delete window[key]; } catch(e){} }
		});
	`); err != nil {
		return nil, fmt.Errorf("listener setup: %w", err)
	}

	t1 := time.Now()
	if _, err := rtDonut.vm.RunString("(function(){\n" + string(r.assets.donut) + "\n})()"); err != nil {
		return nil, fmt.Errorf("donut load: %w", err)
	}
	if _, err := rtD6.vm.RunString("(function(){\n" + string(r.assets.prod) + "\n})()"); err != nil {
		return nil, fmt.Errorf("challenge module load: %w", err)
	}
	r.logf("phase scriptload: %d ms", time.Since(t1).Milliseconds())
	rtDonut.drain(ctx)
	rtD6.drain(ctx)

	if d6v := rtD6.vm.GlobalObject().Get("__d6"); d6v == nil || goja.IsUndefined(d6v) {
		return nil, errors.New("challenge module did not register")
	}
	if ss2v := rtDonut.vm.GlobalObject().Get("__ss2_challenge"); ss2v == nil || goja.IsUndefined(ss2v) {
		return nil, errors.New("stage2 module did not register")
	}

	// bootstrap: x-cs-q binds the media identity
	t2 := time.Now()
	q := challengeQuery(mediaType, tmdbID, season, episode)
	boot, err := rtDonut.postJSON(ctx, "/api/c/bootstrap", map[string]string{"x-cs-q": q}, nil)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	rToken, _ := boot["r"].(string)
	pToken, _ := boot["p"].(string)
	if rToken == "" || pToken == "" {
		return nil, errors.New("bootstrap returned no tokens")
	}

	// gc() with the x-cs-* header injection the embed's fetch wrapper does
	for _, rt := range []*runtime{rtDonut, rtD6} {
		rt.mu.Lock()
		rt.injected = map[string]string{"x-cs-r": rToken, "x-cs-q": q, "x-cs-p": pToken}
		rt.mu.Unlock()
	}

	c1, c2, err := rtDonut.runGCParallel(ctx, rtD6)
	if err != nil {
		return nil, err
	}
	r.logf("phase gc (bootstrap+issue+pow+vm): %d ms", time.Since(t2).Milliseconds())

	for _, rt := range []*runtime{rtDonut, rtD6} {
		rt.mu.Lock()
		rt.injected = map[string]string{}
		rt.mu.Unlock()
	}

	compound := c1 + "::c2::" + c2 + "::c3::" + rToken

	// NOTE: the embed's action body uses "show" (not "tv") as the media type.
	sVal, eVal := "$undefined", "$undefined"
	if season != "" {
		sVal = season
	}
	if episode != "" {
		eVal = episode
	}
	actionType := mediaType
	if actionType == "tv" {
		actionType = "show"
	}
	body, err := json.Marshal([]string{tmdbID, actionType, sVal, eVal, compound, srv})
	if err != nil {
		return nil, err
	}
	text, err := rtD6.postRaw(ctx, pagePath, map[string]string{
		"accept":                 "text/x-component",
		"content-type":           "text/plain;charset=UTF-8",
		"next-action":            getStreamAction,
		"next-router-state-tree": routerStateTree(mediaType, tmdbID),
	}, body)
	if err != nil {
		return nil, err
	}
	cipher, err := extractCipher(text)
	if err != nil {
		return nil, err
	}
	result, err := rtD6.decrypt(ctx, cipher)
	if err != nil {
		return nil, err
	}
	if result.Provider == "" {
		result.Provider = srv
	}
	return result, nil
}

// providerList fetches the embed's ranked source-server list (15 entries) so
// titles without a stream on the first servers can fall through to later
// ones. Cached 10 minutes; falls back to the classic top-5 on failure.
func (r *Resolver) providerList(ctx context.Context, origin, ua string) []string {
	r.mu.Lock()
	if r.providers != nil && time.Since(r.providersFetched) < 10*time.Minute {
		out := r.providers
		r.mu.Unlock()
		return out
	}
	r.mu.Unlock()

	client := &http.Client{Transport: r.Transport, Timeout: 12 * time.Second}
	body, err := json.Marshal([]string{})
	if err == nil {
		req, rerr := http.NewRequestWithContext(ctx, "POST", origin+"/embed/movie/550", bytes.NewReader(body))
		if rerr == nil {
			req.Header.Set("accept", "text/x-component")
			req.Header.Set("content-type", "text/plain;charset=UTF-8")
			req.Header.Set("next-action", getProviderListAction)
			req.Header.Set("next-router-state-tree", routerStateTree("movie", "550"))
			req.Header.Set("user-agent", ua)
			req.Header.Set("origin", origin)
			req.Header.Set("referer", origin+"/")
			if res, derr := client.Do(req); derr == nil {
				raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
				res.Body.Close()
				var ids []string
				for _, m := range reServerID.FindAllStringSubmatch(string(raw), -1) {
					ids = append(ids, m[1])
				}
				if len(ids) > 0 {
					r.mu.Lock()
					r.providers = ids
					r.providersFetched = time.Now()
					r.mu.Unlock()
					r.logf("provider list: %s", strings.Join(ids, ","))
					return ids
				}
			}
		}
	}
	return []string{"nebula", "lisbon", "surge", "spark", "storm"}
}

// pkLookup / pkStore implement the engine-wide /api/c/pk cache: the RSA
// public key is static, so only the first resolve pays that round trip.
func (r *Resolver) pkLookup(u string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.pkBody[u]
	return v, ok
}

func (r *Resolver) pkStore(u, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pkBody == nil {
		r.pkBody = map[string]string{}
	}
	if len(r.pkBody) < 4 {
		r.pkBody[u] = body
	}
}

// nextCanvas rotates the fingerprint canvas renders engine-wide so the two
// parallel runtimes never emit identical canvas hashes.
func (r *Resolver) nextCanvas() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	urls := r.fingerprint().CanvasURLs
	if len(urls) == 0 {
		return ""
	}
	u := urls[r.canvasCalls%len(urls)]
	r.canvasCalls++
	return u
}

// runGCParallel runs d6.gc() and ss2.gc() on separate runtimes in parallel,
// mirroring the embed's Promise.all while giving each its own thread.
func (rt *runtime) runGCParallel(ctx context.Context, other *runtime) (c1, c2 string, err error) {
	var (
		wg     sync.WaitGroup
		errD6  error
		errSS2 error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		c1, errD6 = rtD6GC(ctx, other)
	}()
	go func() {
		defer wg.Done()
		c2, errSS2 = rtSS2GC(ctx, rt)
	}()
	wg.Wait()
	if errD6 != nil {
		return "", "", errD6
	}
	if errSS2 != nil {
		return "", "", errSS2
	}
	return c1, c2, nil
}

func rtD6GC(ctx context.Context, rt *runtime) (string, error) {
	if _, err := rt.vm.RunString(`
		(function(){
			__gcOut = null;
			window.__d6.gc().then(function(v){ __gcOut = {ok: true, v: v}; },
			                     function(e){ __gcOut = {ok: false, err: String(e && e.message || e)}; });
		})()
	`); err != nil {
		return "", fmt.Errorf("cinesrcjs: gc dispatch: %w", err)
	}
	return gcWait(ctx, rt)
}

func rtSS2GC(ctx context.Context, rt *runtime) (string, error) {
	if _, err := rt.vm.RunString(`
		(function(){
			__gcOut = null;
			window.__ss2_challenge.gc().then(function(v){ __gcOut = {ok: true, v: v}; },
			                                   function(e){ __gcOut = {ok: false, err: String(e && e.message || e)}; });
		})()
	`); err != nil {
		return "", fmt.Errorf("cinesrcjs: gc dispatch: %w", err)
	}
	return gcWait(ctx, rt)
}

func gcWait(ctx context.Context, rt *runtime) (string, error) {
	for i := 0; i < 2400; i++ {
		rt.drain(ctx)
		out := rt.vm.GlobalObject().Get("__gcOut")
		if out != nil && !goja.IsUndefined(out) && !goja.IsNull(out) {
			o := out.ToObject(rt.vm)
			if o.Get("ok").ToBoolean() {
				return o.Get("v").String(), nil
			}
			return "", fmt.Errorf("cinesrcjs: challenge failed: %s", o.Get("err").String())
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("cinesrcjs: gc timeout: %w", ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	return "", errors.New("cinesrcjs: gc timeout")
}

// decrypt calls d6.dr() on the r2 cipher (HKDF-derived key + AES-GCM).
func (rt *runtime) decrypt(ctx context.Context, cipher string) (*Result, error) {
	enc, err := json.Marshal(cipher)
	if err != nil {
		return nil, err
	}
	script := `
		(function(){
			__drOut = null;
			window.__d6.dr(` + string(enc) + `).then(function(v){ __drOut = {ok: true, v: v}; },
			                                          function(e){ __drOut = {ok: false, err: String(e && e.message || e)}; });
		})()
	`
	if _, err := rt.vm.RunString(script); err != nil {
		return nil, err
	}
	for i := 0; i < 800; i++ {
		rt.drain(ctx)
		out := rt.vm.GlobalObject().Get("__drOut")
		if out != nil && !goja.IsUndefined(out) && !goja.IsNull(out) {
			o := out.ToObject(rt.vm)
			if !o.Get("ok").ToBoolean() {
				return nil, fmt.Errorf("dr failed: %s", o.Get("err").String())
			}
			raw, err := json.Marshal(o.Get("v").Export())
			if err != nil {
				return nil, err
			}
			var dr struct {
				URL []struct {
					Source string `json:"source"`
					URL    string `json:"url"`
				} `json:"url"`
				Captions []Caption `json:"captions"`
				Provider string    `json:"provider"`
			}
			if err := json.Unmarshal(raw, &dr); err != nil {
				return nil, err
			}
			if len(dr.URL) == 0 || strings.TrimSpace(dr.URL[0].URL) == "" {
				return nil, errors.New("dr returned no urls")
			}
			return &Result{
				Source:   strings.TrimSpace(dr.URL[0].URL),
				Provider: dr.Provider,
				Captions: dr.Captions,
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return nil, errors.New("dr timeout")
}

// ensureAssets fetches and caches donut.js, the versioned *-prod.js module
// and the PoW wasm. The module names live in the embed's app chunk.
func (r *Resolver) ensureAssets(ctx context.Context, origin, ua string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assets != nil && time.Since(r.assets.fetched) < time.Hour {
		return nil
	}

	client := &http.Client{Transport: r.Transport, Timeout: 20 * time.Second}
	get := func(path string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", origin+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("user-agent", ua)
		req.Header.Set("referer", origin+"/")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: status %d", path, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	}

	// locate the app chunk that names the challenge scripts. Chunks are
	// fetched concurrently: the prod-bearing chunk is the largest and often
	// late in the list, so a sequential scan costs seconds.
	html, err := get("/embed/movie/550")
	if err != nil {
		return fmt.Errorf("cinesrcjs: embed page: %w", err)
	}
	var names []string
	for _, m := range reChunkSrc.FindAllStringSubmatch(string(html), -1) {
		names = append(names, m[1])
	}
	var (
		scanMu   sync.Mutex
		prodName string
		powName  string
		wgScan   sync.WaitGroup
		scanOne  = func(name string) {
			defer wgScan.Done()
			chunk, err := get("/_next/static/chunks/" + name + ".js")
			if err != nil {
				return
			}
			scanMu.Lock()
			defer scanMu.Unlock()
			if prodName == "" {
				if pm := reProdName.FindString(string(chunk)); pm != "" {
					prodName = pm
				}
			}
			if powName == "" {
				if wm := rePowWasm.FindString(string(chunk)); wm != "" {
					powName = wm
				}
			}
		}
	)
	for _, name := range names {
		scanMu.Lock()
		done := prodName != "" && powName != ""
		scanMu.Unlock()
		if done {
			break
		}
		wgScan.Add(1)
		go scanOne(name)
	}
	wgScan.Wait()
	if prodName == "" {
		return errors.New("cinesrcjs: could not locate challenge module script")
	}
	if powName == "" {
		powName = "/pow-v3.wasm"
	}
	// Fetch the three assets concurrently.
	var (
		prod, donut, wasm []byte
		pErr, dErr, wErr  error
		wgFetch           sync.WaitGroup
	)
	wgFetch.Add(3)
	go func() { defer wgFetch.Done(); prod, pErr = get(prodName) }()
	go func() { defer wgFetch.Done(); donut, dErr = get("/donut.js") }()
	go func() { defer wgFetch.Done(); wasm, wErr = get(powName) }()
	wgFetch.Wait()
	if pErr != nil {
		return fmt.Errorf("cinesrcjs: %s: %w", prodName, pErr)
	}
	if dErr != nil {
		return fmt.Errorf("cinesrcjs: /donut.js: %w", dErr)
	}
	if wErr != nil {
		return fmt.Errorf("cinesrcjs: %s: %w", powName, wErr)
	}

	r.assets = &assets{prodURL: prodName, prod: prod, donut: donut, powWasm: wasm, fetched: time.Now()}
	// Warm the wasm through a dummy solve so the first real RPC (which has a
	// short client-side timeout) completes within milliseconds.
	p, perr := newPowRuntime(wasm)
	if perr != nil {
		return fmt.Errorf("cinesrcjs: pow wasm: %w", perr)
	}
	if _, err := p.solve(dummyChallenge()); err != nil {
		return fmt.Errorf("cinesrcjs: pow warmup: %w", err)
	}
	r.pow = p
	return nil
}

// dummyChallenge is a well-formed CSP3 blob for warmup only (never sent).
func dummyChallenge() []byte {
	b := make([]byte, 32)
	copy(b, "CSP3")
	return b
}

// sharedPow lazily compiles the PoW wasm once per Resolver; the compiled
// module stays warm so worker RPCs answer within the script's timeout.
func (r *Resolver) sharedPow() (*powRuntime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assets == nil {
		return nil, errors.New("assets not loaded")
	}
	if r.pow == nil {
		p, err := newPowRuntime(r.assets.powWasm)
		if err != nil {
			return nil, err
		}
		r.pow = p
	}
	return r.pow, nil
}

// challengeQuery builds the x-cs-q header: base64url(JSON([type,id,s,e])),
// with JS nulls for absent season/episode.
func challengeQuery(mediaType, tmdbID, season, episode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `["%s","%s",`, mediaType, tmdbID)
	if season != "" {
		fmt.Fprintf(&b, `%q`, season)
	} else {
		b.WriteString("null")
	}
	b.WriteString(",")
	if episode != "" {
		fmt.Fprintf(&b, `%q`, episode)
	} else {
		b.WriteString("null")
	}
	b.WriteString("]")
	return base64.RawURLEncoding.EncodeToString([]byte(b.String()))
}

// routerStateTree mirrors the embed's next-router-state-tree header.
func routerStateTree(mediaType, tmdbID string) string {
	v := fmt.Sprintf(`["",{"children":["embed",{"children":[["type","%s","d"],{"children":[["id","%s","d"],{"children":["__PAGE__",{},null,null]}]},null,null]},null,null]},null,null,null,true]`, mediaType, tmdbID)
	return url.QueryEscape(v)
}

// extractCipher pulls the r2.<...> payload out of a server-action response.
// Next.js emits small payloads as quoted flight rows (1:"r2.…") but large
// ones as length-prefixed text rows (2:T<hex>,r2.…) — the earlier naive
// quote-scan truncated big payloads (e.g. episodes with many subtitles) and
// the truncated base64 failed to decode in dr().
func extractCipher(text string) (string, error) {
	if strings.Contains(text, "e1:invalid_challenge") {
		return "", errors.New("invalid_challenge")
	}
	if i := strings.Index(text, "e1:"); i >= 0 {
		rest := text[i+3:]
		if len(rest) > 80 {
			rest = rest[:80]
		}
		return "", errors.New(strings.TrimSpace(rest))
	}
	i := strings.Index(text, "r2.")
	if i < 0 {
		return "", errors.New("no r2 cipher in response")
	}

	// Length-prefixed flight text row: `<id>:T<hexLen>,r2.…`
	if row := strings.LastIndex(text[:i], ":T"); row >= 0 {
		if comma := strings.Index(text[row:i], ","); comma > 2 {
			hexLen := text[row+2 : row+comma]
			if n, err := strconv.ParseInt(hexLen, 16, 64); err == nil && n > 0 && i+int(n) <= len(text) {
				return text[i : i+int(n)], nil
			}
		}
	}

	// Quoted row: value runs to the closing quote / end of line.
	j := i
	for j < len(text) && text[j] != '"' && text[j] != '\n' && text[j] != '\\' {
		j++
	}
	return text[i:j], nil
}

func (r *Resolver) fingerprint() Fingerprint {
	if len(r.Fingerprint.CanvasURLs) > 0 {
		return r.Fingerprint
	}
	return DefaultFingerprint()
}

func (r *Resolver) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

func redact(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}
