package mediaresolver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
)

// Direct VidSrcMe resolution. Reverse-engineered from the embed player bundle
// (cloudorchestranova.com / vidsrcme.ru). The chain requires no headless browser:
//
//  1. GET https://data.vidsrcme.ru/api.php?type={movie|tv}&tmdb={id}[&season={s}&episode={e}]&stream_urls
//     → JSON with { data: { stream_urls: "base64(nonce||ciphertext)" }, vs: { w: <w>, wasm_url: "..." } }
//
//  2. The ~7KB WebAssembly module from wasm_url exports alloc(len) and decrypt(ptr, len).
//     We instantiate it using wazero in pure Go, passing the decoded ciphertext.
//     The decrypted plaintext yields a newline-separated list of candidate master HLS stream URLs.
//
//  3. For each candidate stream URL, fetch an authorization token from {cdn_origin}/generate.php
//     and append ?token={token} to the master playlist URL.
//
//  4. The master playlist is probed and validated, seeded into GoFlix's body cache,
//     and proxied through the tokenized HLS proxy with read-ahead warmup.

const (
	vidsrcmeDefaultDataOrigin = "https://data.vidsrcme.ru"
	vidsrcmeDefaultReferer    = "https://cloudorchestranova.com/"
	vidsrcmeResponseCap       = 1 << 20
	wasmBinaryCap             = 256 << 10
)

type vidsrcmeAPIResponse struct {
	StatusCode string `json:"status_code"`
	Data       struct {
		Title      string `json:"title"`
		ImdbID     string `json:"imdb_id"`
		Season     string `json:"season"`
		Episode    string `json:"episode"`
		FileName   string `json:"file_name"`
		Backdrop   string `json:"backdrop"`
		StreamURLs string `json:"stream_urls"`
	} `json:"data"`
	DefaultSubs []struct {
		Lang string `json:"lang"`
		Code string `json:"code"`
		URL  string `json:"url"`
	} `json:"default_subs"`
	VS struct {
		W       int64  `json:"w"`
		WasmURL string `json:"wasm_url"`
		Wasm    string `json:"wasm"`
	} `json:"vs"`
}

// wasmEntry wraps a compiled module with a last-used timestamp for LRU eviction.
type wasmEntry struct {
	mod      wazero.CompiledModule
	lastUsed time.Time
}

// wasmCacheMax is the maximum number of compiled WASM modules retained in memory.
// When reached the least-recently-used entry is evicted.
const wasmCacheMax = 16

// wasmModuleCache caches compiled WebAssembly modules on the Resolver by window ID.
// Access is guarded by wasmCacheMu.
var (
	wasmCacheMu sync.Mutex
	wasmCache   = make(map[int64]*wasmEntry)
)

// getOrInitWasmRuntime returns the wazero runtime from the Resolver, initialising
// it exactly once via sync.Once semantics. The runtime is stored as an interface
// so the resolver.go file does not import wazero directly.
func (r *Resolver) getOrInitWasmRuntime() wazero.Runtime {
	wasmCacheMu.Lock()
	defer wasmCacheMu.Unlock()
	if r.wasmRuntime == nil {
		rt := wazero.NewRuntime(context.Background())
		r.wasmRuntime = rt
		return rt
	}
	// Type-assert safely; wasmRuntime is always set to wazero.Runtime by this function.
	if rt, ok := r.wasmRuntime.(wazero.Runtime); ok {
		return rt
	}
	rt := wazero.NewRuntime(context.Background())
	r.wasmRuntime = rt
	return rt
}

// tryVidsrcmeDirect resolves vidsrcme without a browser, registers the proxy
// session, attaches subtitle renditions, and starts the read-ahead warmup.
func (r *Resolver) tryVidsrcmeDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()

	vr, err := r.resolveVidsrcmeDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] vidsrcme direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] vidsrcme direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	if vr.MasterText != "" {
		r.cache.put(&cacheEntry{
			key:         vr.Source,
			data:        []byte(vr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	log.Printf("[MediaResolver] vidsrcme resolved directly source=%s", redactQuery(vr.Source))
	r.attachAndWarm(token, req)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// resolveVidsrcmeDirect queries the data API, decrypts candidate stream URLs,
// retrieves CDN tokens, and returns the first validated master playlist.
func (r *Resolver) resolveVidsrcmeDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}

	dataBase := r.cfg.VidsrcmeDataOrigin
	if dataBase == "" {
		dataBase = vidsrcmeDefaultDataOrigin
	}
	dataBase = strings.TrimRight(dataBase, "/")

	apiURL := dataBase + "/api.php?type=" + string(req.Type) + "&tmdb=" + url.QueryEscape(req.ID)
	if req.Type == TV {
		apiURL += "&season=" + url.QueryEscape(req.Season) + "&episode=" + url.QueryEscape(req.Episode)
	}
	apiURL += "&stream_urls"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("User-Agent", defaultUserAgent)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Referer", vidsrcmeDefaultReferer)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stream API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, vidsrcmeResponseCap))
	if err != nil {
		return nil, fmt.Errorf("read stream API body: %w", err)
	}

	var parsed vidsrcmeAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode stream API response: %w", err)
	}

	encURLs := strings.TrimSpace(parsed.Data.StreamURLs)
	if encURLs == "" {
		return nil, errors.New("stream API returned empty stream_urls")
	}

	// Decrypt stream URLs via WASM
	candidateURLs, err := r.decryptVidsrcmeStreamURLs(ctx, client, parsed)
	if err != nil {
		return nil, fmt.Errorf("decrypt stream URLs: %w", err)
	}
	if len(candidateURLs) == 0 {
		return nil, errors.New("no candidate stream URLs decrypted")
	}

	// Try each candidate URL: fetch token from CDN and validate playlist
	var lastErr error
	for _, cand := range candidateURLs {
		cand = strings.TrimSpace(cand)
		if cand == "" {
			continue
		}
		cu, err := url.Parse(cand)
		if err != nil || (cu.Scheme != "https" && cu.Scheme != "http") || cu.Host == "" {
			continue
		}
		if r.blockedUpstreamHost(ctx, cu.Hostname()) {
			continue
		}

		origin := cu.Scheme + "://" + cu.Host

		// Fetch JWT token from {origin}/generate.php
		token, err := r.fetchVidsrcmeCDNToken(ctx, client, origin)
		if err != nil {
			lastErr = fmt.Errorf("fetch CDN token for %s: %w", origin, err)
			continue
		}

		finalURL := applyVidsrcmeToken(cand, token)

		// Probe playlist
		headers := make(http.Header)
		headers.Set("User-Agent", defaultUserAgent)
		headers.Set("Referer", vidsrcmeDefaultReferer)

		ok, _ := r.probeManifestClass(ctx, finalURL, headers)
		if !ok {
			lastErr = fmt.Errorf("playlist at %s failed probe", redactQuery(finalURL))
			continue
		}

		// Read the master manifest text
		manifestText, err := r.fetchManifestText(ctx, client, finalURL, headers)
		if err != nil {
			lastErr = fmt.Errorf("fetch manifest text: %w", err)
			continue
		}

		allowed := map[string]bool{strings.ToLower(cu.Host): true}
		return &directResolution{
			Source:     finalURL,
			Headers:    headers,
			Allowed:    allowed,
			MasterText: manifestText,
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no candidate stream URL produced a valid playlist")
}

// decryptVidsrcmeStreamURLs runs the ChaCha20 decryptor inside the WebAssembly binary.
func (r *Resolver) decryptVidsrcmeStreamURLs(ctx context.Context, client *http.Client, apiRes vidsrcmeAPIResponse) ([]string, error) {
	encBytes, err := base64.StdEncoding.DecodeString(apiRes.Data.StreamURLs)
	if err != nil {
		return nil, fmt.Errorf("base64 decode ciphertext: %w", err)
	}

	rt := r.getOrInitWasmRuntime()

	// Obtain compiled module (cached by window ID)
	compiled, err := r.getCompiledWasm(ctx, client, rt, apiRes)
	if err != nil {
		return nil, fmt.Errorf("get compiled wasm: %w", err)
	}

	// Instantiate fresh module for clean linear memory
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("instantiate wasm module: %w", err)
	}
	defer mod.Close(ctx)

	alloc := mod.ExportedFunction("alloc")
	decrypt := mod.ExportedFunction("decrypt")
	mem := mod.Memory()

	if alloc == nil || decrypt == nil || mem == nil {
		return nil, errors.New("wasm module missing required exports (alloc, decrypt, memory)")
	}

	res, err := alloc.Call(ctx, uint64(len(encBytes)))
	if err != nil {
		return nil, fmt.Errorf("wasm alloc error: %w", err)
	}
	ptr := uint32(res[0])

	if !mem.Write(ptr, encBytes) {
		return nil, errors.New("wasm failed to write ciphertext into memory")
	}

	decRes, err := decrypt.Call(ctx, uint64(ptr), uint64(len(encBytes)))
	if err != nil {
		return nil, fmt.Errorf("wasm decrypt error: %w", err)
	}
	outLen := uint32(decRes[0])

	outBytes, ok := mem.Read(ptr+12, outLen)
	if !ok {
		return nil, errors.New("wasm failed to read plaintext from memory")
	}

	lines := strings.Split(string(outBytes), "\n")
	var urls []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls, nil
}

// getCompiledWasm loads or compiles the WASM module for the given window ID.
// Evicts the least-recently-used entry when the cache exceeds wasmCacheMax.
func (r *Resolver) getCompiledWasm(ctx context.Context, client *http.Client, rt wazero.Runtime, apiRes vidsrcmeAPIResponse) (wazero.CompiledModule, error) {
	wasmCacheMu.Lock()
	defer wasmCacheMu.Unlock()

	w := apiRes.VS.W
	if w != 0 {
		if entry, ok := wasmCache[w]; ok {
			entry.lastUsed = time.Now()
			return entry.mod, nil
		}
	}

	var wasmBytes []byte
	var err error

	if apiRes.VS.Wasm != "" {
		wasmBytes, err = base64.StdEncoding.DecodeString(apiRes.VS.Wasm)
		if err != nil {
			return nil, fmt.Errorf("decode inline wasm: %w", err)
		}
	} else if apiRes.VS.WasmURL != "" {
		wasmReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiRes.VS.WasmURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create wasm request: %w", err)
		}
		wasmReq.Header.Set("User-Agent", defaultUserAgent)
		wasmReq.Header.Set("Referer", vidsrcmeDefaultReferer)

		resp, err := client.Do(wasmReq)
		if err != nil {
			return nil, fmt.Errorf("fetch wasm: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch wasm status: %d", resp.StatusCode)
		}

		wasmBytes, err = io.ReadAll(io.LimitReader(resp.Body, wasmBinaryCap))
		if err != nil {
			return nil, fmt.Errorf("read wasm binary: %w", err)
		}
	} else {
		return nil, errors.New("no wasm or wasm_url provided in response")
	}

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	if w != 0 {
		// Enforce LRU cap before inserting new entry.
		if len(wasmCache) >= wasmCacheMax {
			// Find and evict the least-recently-used entry.
			var oldestKey int64
			var oldest time.Time
			for k, e := range wasmCache {
				if oldest.IsZero() || e.lastUsed.Before(oldest) {
					oldest = e.lastUsed
					oldestKey = k
				}
			}
			delete(wasmCache, oldestKey)
		}
		wasmCache[w] = &wasmEntry{mod: compiled, lastUsed: time.Now()}
	}
	return compiled, nil
}

type cdnTokenEntry struct {
	token     string
	expiresAt time.Time
}

var (
	cdnTokenMu    sync.Mutex
	cdnTokenCache = make(map[string]cdnTokenEntry)
)

// evictExpiredCDNTokens removes all expired entries from cdnTokenCache.
// Called by the session sweeper in resolver.go so the map doesn't grow unboundedly.
func evictExpiredCDNTokens() {
	now := time.Now()
	cdnTokenMu.Lock()
	defer cdnTokenMu.Unlock()
	for origin, entry := range cdnTokenCache {
		if now.After(entry.expiresAt) {
			delete(cdnTokenCache, origin)
		}
	}
}


// fetchVidsrcmeCDNToken retrieves the JWT playback authorization token from {cdnOrigin}/generate.php,
// caching valid tokens for up to 5 minutes to avoid rate-limit (429) errors.
func (r *Resolver) fetchVidsrcmeCDNToken(ctx context.Context, client *http.Client, cdnOrigin string) (string, error) {
	cdnOrigin = strings.TrimRight(cdnOrigin, "/")

	cdnTokenMu.Lock()
	if entry, ok := cdnTokenCache[cdnOrigin]; ok && time.Now().Before(entry.expiresAt) {
		cdnTokenMu.Unlock()
		return entry.token, nil
	}
	cdnTokenMu.Unlock()

	tokenURL := cdnOrigin + "/generate.php"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Referer", vidsrcmeDefaultReferer)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		cdnTokenMu.Lock()
		entry, ok := cdnTokenCache[cdnOrigin]
		cdnTokenMu.Unlock()
		if ok && entry.token != "" {
			log.Printf("[MediaResolver] generate.php 429 rate limit hit, using cached CDN token for %s", cdnOrigin)
			return entry.token, nil
		}
		return "", fmt.Errorf("generate.php status %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("generate.php status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}

	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", errors.New("empty token response")
	}

	token := raw
	// If the response is JSON, parse out token/data
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var tokenJSON struct {
			Token  string `json:"token"`
			Data   string `json:"data"`
			Result string `json:"result"`
			String string `json:"string"`
		}
		if json.Unmarshal([]byte(raw), &tokenJSON) == nil {
			if tokenJSON.Token != "" {
				token = tokenJSON.Token
			} else if tokenJSON.Data != "" {
				token = tokenJSON.Data
			} else if tokenJSON.Result != "" {
				token = tokenJSON.Result
			} else if tokenJSON.String != "" {
				token = tokenJSON.String
			}
		}
	}

	cdnTokenMu.Lock()
	cdnTokenCache[cdnOrigin] = cdnTokenEntry{
		token:     token,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	cdnTokenMu.Unlock()

	return token, nil
}

// applyVidsrcmeToken attaches the token to the URL, replacing __TOKEN__ or appending ?token=.
func applyVidsrcmeToken(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	if strings.Contains(rawURL, "__TOKEN__") {
		return strings.ReplaceAll(rawURL, "__TOKEN__", token)
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "token=" + url.QueryEscape(token)
}

// fetchManifestText fetches the master playlist content directly.
func (r *Resolver) fetchManifestText(ctx context.Context, client *http.Client, targetURL string, headers http.Header) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return "", err
	}

	str := strings.TrimSpace(string(body))
	if !strings.HasPrefix(str, "#EXTM3U") {
		return "", errors.New("manifest does not begin with #EXTM3U")
	}
	return str, nil
}
