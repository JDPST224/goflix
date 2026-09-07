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
	"sort"
	"strconv"
	"strings"
	"time"
)

// Direct vidking resolution, reverse-engineered from the embed's own app
// bundle (assets/VideoPlayer-*.js served from www.vidking.net). The embed
// never exposes a playlist URL in its HTML or network traffic until its
// scripts have decrypted one; the chain it performs is fully reproducible in
// plain Go:
//
//  1. GET db.speedracelight.com/3/{movie|tv}/{tmdbId}?append_to_response=external_ids
//     → public TMDB mirror giving the title/year/imdb id the source API wants.
//  2. GET api.speedracelight.com/seed?mediaId={tmdbId} → {seed, ttlMs}.
//  3. GET api.speedracelight.com/{server}/sources-with-title?…&enc=2&seed=…
//     → base64url(XOR(JSON, keystream)) with a four-byte "mvm1" magic prefix.
//     The keystream comes from a custom 32-bit PRNG seeded with (seed, tmdbId),
//     ported below exactly as shipped (see vkState).
//  4. The decrypted payload lists one playlist URL per quality tier (up to
//     2160p on the primary "Yoru" server). Each URL is directly fetchable —
//     no cookies, no client hints; CDNs answer plain requests with CORS *.
//
// Picking the top tier ourselves fixes the quality problem the old browser
// scrape had: the embed's hls.js starts on an arbitrary rung and the scraper
// inherited whatever it chose.
const (
	vkAPIBase = "https://api.speedracelight.com"
	vkDBBase  = "https://db.speedracelight.com/3"

	// vkResponseCap bounds API responses; payloads embed subtitle lists and
	// can be large, but are nowhere near this cap.
	vkResponseCap = 1 << 20

	// vkMagic is the plaintext prefix every decrypted payload carries.
	vkMagic = "mvm1"
)

// vidkingServer describes one upstream source provider as configured in the
// vidking player bundle. Language-forced servers sit last: their catalogs are
// region-flavored fallbacks, not what a general-purpose resolver wants first.
type vidkingServer struct {
	name     string
	endpoint string
	params   map[string]string
}

var vidkingServers = []vidkingServer{
	{name: "YORU", endpoint: "cdn/sources-with-title"},
	{name: "CYPHER", endpoint: "downloader2/sources-with-title"},
	{name: "BREACH", endpoint: "m4uhd/sources-with-title"},
	{name: "NEON", endpoint: "vsrc/sources-with-title"},
	{name: "OMEN", endpoint: "lamovie/sources-with-title"},
	{name: "RAZE", endpoint: "superflix/sources-with-title"},
	{name: "VYSE", endpoint: "hdmovie/sources-with-title"},
	{name: "KILLJOY", endpoint: "meine/sources-with-title", params: map[string]string{"language": "german"}},
	{name: "FADE", endpoint: "hdmovie/sources-with-title", params: map[string]string{"qualityFilter": "Hindi"}},
}

type vidkingMeta struct {
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	ExternalIDs  struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
}

type vidkingSource struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
	Type    string `json:"type"`
}

type vidkingPayload struct {
	Sources []vidkingSource `json:"sources"`
}

// tryVidkingDirect resolves vidking against its source API without a browser,
// registers the proxy session and starts the read-ahead warmup. It reports
// false so Resolve falls back to the browser scrape when the direct chain is
// unavailable.
func (r *Resolver) tryVidkingDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	vr, err := r.resolveVidkingDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] vidking direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] vidking direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	r.rememberResolution(req, newResolutionRecord(vr))
	if vr.MasterText != "" {
		// Admit the validated playlist under its canonical URL with a long
		// TTL: these CDN playlists are VOD manifests whose segment URLs outlive
		// the session by weeks. Warmup and the proxy fast path both serve this
		// entry from RAM instead of refetching.
		r.cache.put(&cacheEntry{
			key:         vr.Source,
			data:        []byte(vr.MasterText),
			status:      http.StatusOK,
			contentType: "application/vnd.apple.mpegurl",
			expiresAt:   time.Now().Add(cacheEntryTTL),
		})
	}
	r.attachAndWarm(token, req)
	return "/api/media/proxy/" + token + ".m3u8", true
}

// resolveVidkingDirect walks metadata → seed → each source server, returning
// the first stream whose top-tier playlist validates. The chosen tier is the
// highest-numbered quality the server lists.
func (r *Resolver) resolveVidkingDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}

	meta, err := r.fetchVidkingMeta(ctx, client, req)
	if err != nil {
		return nil, err
	}
	year := meta.ReleaseDate
	mediaType := "movie"
	if req.Type == TV {
		mediaType = "tv"
		year = meta.FirstAirDate
	}
	year = strings.TrimSpace(year[:min(4, len(year))])

	seed, err := fetchVidkingSeed(ctx, client, req.ID)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, srv := range vidkingServers {
		if ctx.Err() != nil {
			break
		}
		payload, err := r.fetchVidkingSources(ctx, client, srv, vidkingQuery{
			MediaType: mediaType, TMDBID: req.ID, Season: req.Season, Episode: req.Episode,
			Title: meta.displayTitle(), Year: year, IMDBID: meta.ExternalIDs.IMDBID,
		}, &seed)
		if err != nil {
			lastErr = err
			continue
		}
		res, tier, err := r.finishVidkingSources(ctx, payload, srv)
		if err != nil {
			lastErr = err
			log.Printf("[MediaResolver] vidking server %q unusable: %v", srv.name, err)
			continue
		}
		log.Printf("[MediaResolver] vidking resolved directly server=%q quality=%s source=%s",
			srv.name, tier, redactQuery(res.Source))
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no vidking server attempted")
	}
	return nil, fmt.Errorf("all %d vidking servers failed: %w", len(vidkingServers), lastErr)
}

// finishVidkingSources picks the highest-quality non-DASH source from a
// decrypted payload and validates its playlist by fetching it. The tier label
// ("2160p", "auto") is returned for logging; the fetched text becomes the
// resolution's MasterText, seeding the body cache.
func (r *Resolver) finishVidkingSources(ctx context.Context, payload vidkingPayload, srv vidkingServer) (*directResolution, string, error) {
	cands := make([]vidkingSource, 0, len(payload.Sources))
	for _, s := range payload.Sources {
		u := strings.TrimSpace(s.URL)
		if u == "" {
			continue
		}
		lower := strings.ToLower(u)
		// DASH (.mpd) sources cannot be proxied by the HLS pipeline; skip them
		// the way the pipeline skips everything it cannot rewrite.
		if strings.EqualFold(strings.TrimSpace(s.Type), "dash") || strings.Contains(lower, ".mpd") {
			continue
		}
		pu, err := url.Parse(u)
		if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
			continue
		}
		if r.blockedUpstreamHost(ctx, pu.Hostname()) {
			continue
		}
		cands = append(cands, s)
	}
	if len(cands) == 0 {
		return nil, "", errors.New("payload listed no proxiable sources")
	}
	// Highest numeric tier first; unranked ("Auto") tiers act as tie-broken
	// fallbacks within the same server.
	sort.SliceStable(cands, func(i, j int) bool {
		return vkQualityRank(cands[i].Quality) > vkQualityRank(cands[j].Quality)
	})

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", r.cfg.VidKingOrigin+"/")
	var lastErr error
	for _, c := range cands {
		cu, err := url.Parse(c.URL)
		if err != nil {
			continue
		}
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req2.Header.Set("User-Agent", headers.Get("User-Agent"))
		req2.Header.Set("Referer", headers.Get("Referer"))
		resp, err := (&http.Client{Transport: r.transport}).Do(req2)
		if err != nil {
			lastErr = err
			continue
		}
		text, readErr := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
		resp.Body.Close()
		if readErr != nil && len(text) == 0 {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("tier %q returned status %d", c.Quality, resp.StatusCode)
			continue
		}
		textStr := strings.TrimSpace(string(text))
		if !strings.HasPrefix(textStr, "#EXTM3U") ||
			(!strings.Contains(textStr, "#EXT-X-STREAM-INF") && !strings.Contains(textStr, "#EXTINF")) {
			lastErr = fmt.Errorf("tier %q did not serve an HLS playlist", c.Quality)
			continue
		}
		return &directResolution{
			Source:     c.URL,
			Headers:    cloneHeader(headers),
			Allowed:    map[string]bool{strings.ToLower(cu.Host): true},
			MasterText: textStr,
		}, vkQualityLabel(c.Quality), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no tier validated")
	}
	return nil, "", lastErr
}

// vidkingQuery carries everything the sources-with-title endpoint wants.
type vidkingQuery struct {
	MediaType, TMDBID, Season, Episode string
	Title, Year, IMDBID                string
}

// fetchVidkingSources calls one server's sources-with-title endpoint and
// decrypts the payload. A 401 means the seed went stale mid-flight; it is
// refreshed once and the request retried, mirroring the embed's own recovery.
func (r *Resolver) fetchVidkingSources(ctx context.Context, client *http.Client, srv vidkingServer, q vidkingQuery, seed *string) (vidkingPayload, error) {
	var payload vidkingPayload
	call := func() (int, []byte, error) {
		tmdbID, _ := strconv.ParseUint(q.TMDBID, 10, 64)
		apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, vkAPIBase+"/"+srv.endpoint+"?"+r.vidkingQueryString(srv, q, *seed, tmdbID), nil)
		if err != nil {
			return 0, nil, err
		}
		apiReq.Header.Set("User-Agent", defaultUserAgent)
		resp, err := client.Do(apiReq)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, vkResponseCap))
		return resp.StatusCode, body, err
	}

	status, body, err := call()
	if err != nil {
		return payload, fmt.Errorf("server %q: %w", srv.name, err)
	}
	if status == http.StatusUnauthorized {
		// Stale/rejected seed: refresh once and retry.
		fresh, serr := fetchVidkingSeed(ctx, client, q.TMDBID)
		if serr != nil {
			return payload, fmt.Errorf("server %q seed refresh: %w", srv.name, serr)
		}
		*seed = fresh
		status, body, err = call()
		if err != nil {
			return payload, fmt.Errorf("server %q retry: %w", srv.name, err)
		}
	}
	if status != http.StatusOK {
		return payload, fmt.Errorf("server %q returned status %d", srv.name, status)
	}
	tmdbNum, _ := strconv.ParseUint(q.TMDBID, 10, 64)
	clear, err := decryptVKSources(string(body), *seed, uint32(tmdbNum))
	if err != nil {
		return payload, fmt.Errorf("server %q: %w", srv.name, err)
	}
	if err := json.Unmarshal(clear, &payload); err != nil {
		return payload, fmt.Errorf("server %q decrypted invalid JSON: %w", srv.name, err)
	}
	return payload, nil
}

// vidkingQueryString mirrors the embed's URL construction parameter-for-
// parameter (order included, since some gateways sign loosely).
func (r *Resolver) vidkingQueryString(srv vidkingServer, q vidkingQuery, seed string, tmdbID uint64) string {
	qs := url.Values{}
	qs.Set("title", q.Title)
	qs.Set("mediaType", q.MediaType)
	qs.Set("year", q.Year)
	qs.Set("episodeId", orDefault(q.Episode, "1"))
	qs.Set("seasonId", orDefault(q.Season, "1"))
	qs.Set("tmdbId", strconv.FormatUint(tmdbID, 10))
	qs.Set("imdbId", q.IMDBID)
	qs.Set("enc", "2")
	qs.Set("seed", seed)
	qs.Set("_t", strconv.FormatInt(time.Now().UnixMilli(), 10))
	for k, v := range srv.params {
		qs.Set(k, v)
	}
	return qs.Encode()
}

// fetchVidkingMeta reads the public TMDB mirror for the fields the source API
// requires (it refuses requests without a real title).
func (r *Resolver) fetchVidkingMeta(ctx context.Context, client *http.Client, req MediaRequest) (*vidkingMeta, error) {
	kind := "movie"
	if req.Type == TV {
		kind = "tv"
	}
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/%s/%s?append_to_response=external_ids", vkDBBase, kind, url.PathEscape(req.ID)), nil)
	if err != nil {
		return nil, err
	}
	apiReq.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(apiReq)
	if err != nil {
		return nil, fmt.Errorf("metadata lookup: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, vkResponseCap))
	if err != nil {
		return nil, fmt.Errorf("metadata lookup: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata lookup returned status %d", resp.StatusCode)
	}
	var meta vidkingMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("metadata lookup returned invalid JSON: %w", err)
	}
	if meta.displayTitle() == "" {
		return nil, errors.New("metadata lookup returned no title")
	}
	return &meta, nil
}

// fetchVidkingSeed obtains the short-lived decryption seed for a media id.
func fetchVidkingSeed(ctx context.Context, client *http.Client, mediaID string) (string, error) {
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		vkAPIBase+"/seed?mediaId="+url.QueryEscape(mediaID), nil)
	if err != nil {
		return "", err
	}
	apiReq.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(apiReq)
	if err != nil {
		return "", fmt.Errorf("seed request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("seed request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seed request returned status %d", resp.StatusCode)
	}
	var doc struct {
		Seed string `json:"seed"`
	}
	if json.Unmarshal(body, &doc) != nil || strings.TrimSpace(doc.Seed) == "" {
		return "", errors.New("seed request returned no seed")
	}
	return doc.Seed, nil
}

func (m *vidkingMeta) displayTitle() string {
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

// orDefault returns fallback when s is empty.
func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// vkQualityRank extracts the numeric tier from quality labels like "2160p",
// "1080p"; unranked labels ("Auto", "Auto HLS", "Vimeos") rank 0.
func vkQualityRank(q string) int {
	q = strings.TrimSpace(strings.ToLower(q))
	if !strings.HasSuffix(q, "p") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(q, "p"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// vkQualityLabel normalizes a raw tier label ("Auto HLS" → "auto") for logs.
func vkQualityLabel(q string) string {
	if vkQualityRank(q) > 0 {
		return strings.TrimSpace(strings.ToLower(q))
	}
	return "auto"
}

// --- Decryption -------------------------------------------------------------
//
// The payload is base64url(XOR(JSON bytes, keystream)) prefixed with "mvm1".
// The keystream generator below ports the player's obfuscated PRNG verbatim;
// every constant and rotation comes straight from the shipped bundle. All
// arithmetic is uint32, matching JavaScript's Math.imul/>>> semantics natively.

const (
	vkJ = 61         // state slot count
	vkM = 2654435769 // golden-ratio multiplier used throughout
	vkS = 8          // init mixing rounds
)

// vkRoundConstants is Hl from the bundle — the first sixteen SHA-256 round
// constants, used verbatim during state initialization.
var vkRoundConstants = [16]uint32{
	1116352408, 1899447441, 3049323471, 3921009573,
	961987163, 1508970993, 2453635748, 2870763221,
	3624381080, 310598401, 607225278, 1426881987,
	1925078388, 2162078206, 2614888103, 3248222580,
}

// vkFinalize is the MurmurHash3 32-bit finalizer (`ci` in the bundle).
func vkFinalize(x uint32) uint32 {
	x ^= x >> 16
	x *= 2246822507
	x ^= x >> 13
	x *= 3266489909
	x ^= x >> 16
	return x
}

// vkRotl is `ps` — rotate-left with JavaScript's shift-count masking.
func vkRotl(l, o uint32) uint32 {
	o &= 31
	if o == 0 {
		return l
	}
	return l<<o | l>>(32-o)
}

// vkHashSeed is `vf` — FNV-1a over the seed's UTF-16 code units (ASCII-safe),
// finalized.
func vkHashSeed(seed string) uint32 {
	h := uint32(2166136261)
	for _, r := range seed {
		h = (h ^ uint32(r)) * 16777619
	}
	return vkFinalize(h)
}

// vkState is the PRNG state ({S, acc} in the bundle). Slots start sparse:
// only indices touched during initialization participate in the `in` check.
type vkState struct {
	s       [vkJ]uint32
	present [vkJ]bool
	acc     uint32
}

// newVKState is `Rf`. The bundle's odd-length branch is dead code (its guard
// tests n*(n+1)&1 === 1, impossible for consecutive integers) and is omitted.
func newVKState(seed string, tmdbID uint32) *vkState {
	st := &vkState{}
	i := vkFinalize(vkHashSeed(seed) ^ vkFinalize(tmdbID^vkM))
	for r := 0; r < vkS; r++ {
		n := i % vkJ
		i = vkRotl(i+vkM, uint32(7+(r&7)))
		st.s[n] = i ^ vkFinalize(i)
		st.present[n] = true
		i = vkFinalize(i + n)
	}
	st.acc = vkFinalize(i ^ 2779096485)
	return st
}

// next is `Cf` — one 32-bit word of keystream per call, counter from 0.
func (st *vkState) next(counter uint32) uint32 {
	r := st.acc % vkJ
	var u uint32
	if st.present[r] {
		u = st.s[r]
	}
	d := vkM * (counter + 1)
	o := u ^ d
	g := st.acc ^ o
	if st.present[r] {
		// Nf with e = -1: (l^o) | (l&o).
		g |= st.acc & o
	}
	g = vkRotl(g+st.acc, r&31) ^ vkRotl(st.acc, (r*7)&31)
	v := vkFinalize(g + vkM)
	st.s[r] = v
	st.present[r] = true
	st.acc = v
	return v
}

// vkKeystream is `xf` — words emitted little-endian into a length-n buffer.
func vkKeystream(seed string, tmdbID uint32, n int) []byte {
	st := newVKState(seed, tmdbID)
	out := make([]byte, n)
	var counter uint32
	for i := 0; i < n; {
		w := st.next(counter)
		counter++
		for _, shift := range []uint{0, 8, 16, 24} {
			if i >= n {
				break
			}
			out[i] = byte(w >> shift)
			i++
		}
	}
	return out
}

// decryptVKSources decodes and XOR-decrypts a sources payload, verifying the
// magic prefix (`Pf`).
func decryptVKSources(encoded, seed string, tmdbID uint32) ([]byte, error) {
	std := strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimSpace(encoded))
	if m := len(std) % 4; m != 0 {
		std += strings.Repeat("=", 4-m)
	}
	data, err := base64.StdEncoding.DecodeString(std)
	if err != nil {
		return nil, fmt.Errorf("payload is not base64: %w", err)
	}
	ks := vkKeystream(seed, tmdbID, len(data))
	for i := range data {
		data[i] ^= ks[i]
	}
	if len(data) < len(vkMagic) || string(data[:len(vkMagic)]) != vkMagic {
		return nil, errors.New("decrypt failed: bad seed or tampered payload")
	}
	return data[len(vkMagic):], nil
}
