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
	"sync"
	"time"
)

// Direct movy.sx resolution, reverse-engineered from the site's own player
// bundle. movy runs the same encrypted-sources protocol the late
// vidking/speedracelight network used — the "mvm1" XOR keystream below is
// byte-for-byte the port that shipped for vidking — but over a fresh
// operator's API:
//
//  1. GET api.themoviedb.org/3/{movie|tv}/{tmdbId}?append_to_response=external_ids
//     → TMDB giving the title/year/imdb id the source API wants.
//  2. GET api.wecollege.net/seed?mediaId={tmdbId} → {seed, ttlMs} (short-lived,
//     cached 30 s site-side).
//  3. GET api.wecollege.net/{server}/sources?title={enc}&mediaType=&year=
//     [&episodeId=&seasonId=|totalSeasons=]&imdbId=&language=&enc=2&seed=…
//     → base64url(XOR(JSON, keystream)) with a four-byte "mvm1" magic prefix.
//     The keystream is the same custom 32-bit PRNG seeded with
//     (seed, tmdbId); the JS bundle's parity-based branches are dead code
//     (e*(e+1)&1 is always 0/0), exactly as in the vidking bundle.
//  4. The decrypted payload lists one playlist URL per quality tier —
//     up to 2160p on the primary "Miami" server — plus a subtitle list.
//     Tier playlists are individually fetchable and validated; the top
//     non-DASH tier wins.
//
// Picking the top tier ourselves fixes the quality problem a browser scrape
// has: the site's hls.js starts on an arbitrary rung.
const (
	mvAPIBase = "https://api.wecollege.net"

	// mvTMDBBase serves the metadata record (title/year/imdb id) the source
	// API requires. Fetched straight from TMDB with the configured
	// credentials (Bearer token preferred).
	mvTMDBBase = "https://api.themoviedb.org/3"

	// mvResponseCap bounds API responses; payloads embed subtitle lists and
	// can be large, but are nowhere near this cap.
	mvResponseCap = 1 << 20

	// mvMagic is the plaintext prefix every decrypted payload carries.
	mvMagic = "mvm1"

	// mvSeedTTL is the default seed lifetime when the API does not report
	// ttlMs.
	mvSeedTTL = 30 * time.Second
)

// movyServer describes one upstream source provider as configured in the
// movy player bundle, in the site's own preference order. Language-forced or
// language-labelled servers sit last: their catalogs are region-flavored
// fallbacks, not what a general-purpose resolver wants first.
type movyServer struct {
	name     string
	endpoint string
	params   map[string]string
	// qualityFilter selects sources whose quality field holds a language
	// label (Austin and Delhi label language through quality).
	qualityFilter string
}

var movyServers = []movyServer{
	{name: "Miami", endpoint: "miami"},
	{name: "Boise", endpoint: "boise"},
	{name: "Houston", endpoint: "houston"},
	{name: "Phoenix", endpoint: "phoenix"},
	{name: "Atlanta", endpoint: "atlanta"},
	{name: "Portland", endpoint: "portland"},
	{name: "Austin", endpoint: "austin", qualityFilter: "English"},
	{name: "Dallas", endpoint: "dallas"},
	{name: "Tampa", endpoint: "tampa"},
	{name: "Orlando", endpoint: "orlando"},
	{name: "Munich", endpoint: "munich", params: map[string]string{"language": "german"}},
	{name: "Berlin", endpoint: "berlin"},
	{name: "Paris", endpoint: "paris"},
	{name: "Delhi", endpoint: "delhi", qualityFilter: "Hindi"},
	{name: "Cancun", endpoint: "cancun"},
}

type movyMeta struct {
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	ExternalIDs  struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
}

type movySource struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
	Type    string `json:"type"`
}

type movyPayload struct {
	Sources   []movySource `json:"sources"`
	Subtitles []struct {
		URL      string `json:"url"`
		Lang     string `json:"lang"`
		Language string `json:"language"`
	} `json:"subtitles"`
}

// movySeedEntry caches a media id's short-lived decryption seed.
type movySeedEntry struct {
	seed      string
	expiresAt time.Time
}

var (
	movySeedMu sync.Mutex
	movySeed   = make(map[string]movySeedEntry)
)

// tryMovyDirect resolves movy against its source API without a browser,
// registers the proxy session and starts the read-ahead warmup. It reports
// false so Resolve falls back to the browser scrape when the direct chain is
// unavailable.
func (r *Resolver) tryMovyDirect(parent context.Context, req MediaRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	vr, err := r.resolveMovyDirect(ctx, req)
	if err != nil {
		log.Printf("[MediaResolver] movy direct resolve unavailable (%v); falling back to browser scrape", err)
		return "", false
	}
	token, err := r.newSession(resolutionKey(req), vr.Source, vr.Headers, vr.Allowed)
	if err != nil {
		log.Printf("[MediaResolver] movy direct session failed (%v); falling back to browser scrape", err)
		return "", false
	}
	// Dashboard resolution: movy serves per-tier media playlists that carry
	// no RESOLUTION attribute, so the tier label is authoritative.
	r.noteStreamHeightToken(token, vr.MasterText, vr.Tier)
	r.rememberResolution(req, newResolutionRecord(vr))
	if vr.MasterText != "" {
		// Admit the validated playlist under its canonical URL with a long
		// TTL: these CDN playlists are VOD manifests whose segment URLs
		// outlive the session by weeks. Warmup and the proxy fast path both
		// serve this entry from RAM instead of refetching.
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

// resolveMovyDirect walks metadata → seed → each source server, returning
// the first stream whose top-tier playlist validates. The chosen tier is the
// highest-numbered quality the server lists.
func (r *Resolver) resolveMovyDirect(ctx context.Context, req MediaRequest) (*directResolution, error) {
	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}

	meta, err := r.fetchMovyMeta(ctx, client, req)
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

	seed, err := r.fetchMovySeed(ctx, client, req.ID)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, srv := range movyServers {
		if ctx.Err() != nil {
			break
		}
		payload, err := r.fetchMovySources(ctx, client, srv, mvQuery{
			MediaType: mediaType, TMDBID: req.ID, Season: req.Season, Episode: req.Episode,
			Title: meta.displayTitle(), Year: year, IMDBID: meta.ExternalIDs.IMDBID,
		}, &seed)
		if err != nil {
			lastErr = err
			continue
		}
		res, tier, err := r.finishMovySources(ctx, payload, srv)
		if err != nil {
			lastErr = err
			log.Printf("[MediaResolver] movy server %q unusable: %v", srv.name, err)
			continue
		}
		log.Printf("[MediaResolver] movy resolved directly server=%q quality=%s source=%s",
			srv.name, tier, redactQuery(res.Source))
		res.Tier = tier
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no movy server attempted")
	}
	return nil, fmt.Errorf("all %d movy servers failed: %w", len(movyServers), lastErr)
}

// finishMovySources picks the highest-quality non-DASH source from a
// decrypted payload and validates its playlist by fetching it. The tier
// label ("2160p", "auto") is returned for logging; the fetched text becomes
// the resolution's MasterText, seeding the body cache.
func (r *Resolver) finishMovySources(ctx context.Context, payload movyPayload, srv movyServer) (*directResolution, string, error) {
	cands := make([]movySource, 0, len(payload.Sources))
	for _, s := range payload.Sources {
		u := strings.TrimSpace(s.URL)
		if u == "" {
			continue
		}
		if srv.qualityFilter != "" &&
			!strings.EqualFold(strings.TrimSpace(s.Quality), srv.qualityFilter) {
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
		return movyQualityRank(cands[i].Quality) > movyQualityRank(cands[j].Quality)
	})

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", r.cfg.MovyOrigin+"/")
	// No client timeout: the tier playlist body can run multi-megabyte and
	// the resolve context already bounds the whole chain.
	client := &http.Client{Transport: r.transport}
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
		resp, err := r.resolveFetch(ctx, client, req2)
		if err != nil {
			lastErr = err
			continue
		}
		text, readErr := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
		resp.Body.Close()
		// A partial read (connection reset mid-body) can still start with
		// "#EXTM3U" — accepting the truncated tail would cache a corrupt
		// manifest for the full entry TTL. Any read error fails the tier.
		if readErr != nil {
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
		}, movyQualityLabel(c.Quality), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no tier validated")
	}
	return nil, "", lastErr
}

// mvQuery carries everything the sources endpoint wants.
type mvQuery struct {
	MediaType, TMDBID, Season, Episode string
	Title, Year, IMDBID                string
}

// fetchMovySources calls one server's sources endpoint and decrypts the
// payload. A 401 means the seed went stale mid-flight; it is refreshed once
// and the request retried, mirroring the embed's own recovery.
func (r *Resolver) fetchMovySources(ctx context.Context, client *http.Client, srv movyServer, q mvQuery, seed *string) (movyPayload, error) {
	var payload movyPayload
	call := func() (int, []byte, error) {
		tmdbID, _ := strconv.ParseUint(q.TMDBID, 10, 64)
		apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, mvAPIBase+"/"+srv.endpoint+"/sources?"+r.movyQueryString(srv, q, *seed, tmdbID), nil)
		if err != nil {
			return 0, nil, err
		}
		apiReq.Header.Set("User-Agent", defaultUserAgent)
		resp, err := r.resolveFetch(ctx, client, apiReq)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, mvResponseCap))
		return resp.StatusCode, body, err
	}

	status, body, err := call()
	if err != nil {
		return payload, fmt.Errorf("server %q: %w", srv.name, err)
	}
	if status == http.StatusUnauthorized {
		// Stale/rejected seed: refresh once and retry.
		fresh, serr := r.fetchMovySeed(ctx, client, q.TMDBID)
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
	clear, err := decryptMVSources(string(body), *seed, uint32(tmdbNum))
	if err != nil {
		return payload, fmt.Errorf("server %q: %w", srv.name, err)
	}
	if err := json.Unmarshal(clear, &payload); err != nil {
		return payload, fmt.Errorf("server %q decrypted invalid JSON: %w", srv.name, err)
	}
	return payload, nil
}

// movyQueryString mirrors the embed's URL construction. ofetch skips params
// with undefined values (season/episode fields on movies) and encodes the
// already-encodeURIComponent'd title a second time, so q.Title is inserted
// escaped exactly as the site sends it.
func (r *Resolver) movyQueryString(srv movyServer, q mvQuery, seed string, tmdbID uint64) string {
	qs := url.Values{}
	qs.Set("title", url.QueryEscape(q.Title))
	qs.Set("mediaType", q.MediaType)
	if q.Year != "" {
		qs.Set("year", q.Year)
	}
	if q.Season != "" {
		qs.Set("seasonId", q.Season)
		ep := strings.TrimSpace(q.Episode)
		if ep == "" {
			ep = "1"
		}
		qs.Set("episodeId", ep)
	}
	qs.Set("tmdbId", strconv.FormatUint(tmdbID, 10))
	if q.IMDBID != "" {
		qs.Set("imdbId", q.IMDBID)
	}
	qs.Set("enc", "2")
	qs.Set("seed", seed)
	for k, v := range srv.params {
		qs.Set(k, v)
	}
	return qs.Encode()
}

// fetchMovyMeta reads the TMDB record for the fields the source API
// requires (it refuses requests without a real title).
func (r *Resolver) fetchMovyMeta(ctx context.Context, client *http.Client, req MediaRequest) (*movyMeta, error) {
	if r.cfg.TMDBAccessToken == "" && r.cfg.TMDBAPIKey == "" {
		return nil, errors.New("no TMDB credentials configured for movy metadata lookup")
	}
	kind := "movie"
	if req.Type == TV {
		kind = "tv"
	}
	endpoint := fmt.Sprintf("%s/%s/%s?append_to_response=external_ids", mvTMDBBase, kind, url.PathEscape(req.ID))
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	apiReq.Header.Set("User-Agent", defaultUserAgent)
	apiReq.Header.Set("accept", "application/json")
	if r.cfg.TMDBAccessToken != "" {
		apiReq.Header.Set("Authorization", "Bearer "+r.cfg.TMDBAccessToken)
	} else {
		q := apiReq.URL.Query()
		q.Set("api_key", r.cfg.TMDBAPIKey)
		apiReq.URL.RawQuery = q.Encode()
	}
	resp, err := r.resolveFetch(ctx, client, apiReq)
	if err != nil {
		return nil, fmt.Errorf("metadata lookup: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mvResponseCap))
	if err != nil {
		return nil, fmt.Errorf("metadata lookup: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata lookup returned status %d", resp.StatusCode)
	}
	return parseMovyMeta(body)
}

// parseMovyMeta validates and decodes a metadata record from either source.
func parseMovyMeta(body []byte) (*movyMeta, error) {
	var meta movyMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("returned invalid JSON: %w", err)
	}
	if meta.displayTitle() == "" {
		return nil, errors.New("returned no title")
	}
	return &meta, nil
}

// fetchMovySeed obtains the short-lived decryption seed for a media id. The
// site caches it for ttlMs (minus a 5 s safety margin); the resolver reuses
// the same window so consecutive servers skip the round trip.
func (r *Resolver) fetchMovySeed(ctx context.Context, client *http.Client, mediaID string) (string, error) {
	movySeedMu.Lock()
	if e, ok := movySeed[mediaID]; ok && time.Now().Before(e.expiresAt) {
		movySeedMu.Unlock()
		return e.seed, nil
	}
	movySeedMu.Unlock()

	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		mvAPIBase+"/seed?mediaId="+url.QueryEscape(mediaID), nil)
	if err != nil {
		return "", err
	}
	apiReq.Header.Set("User-Agent", defaultUserAgent)
	resp, err := r.resolveFetch(ctx, client, apiReq)
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
		Seed  string `json:"seed"`
		TtlMs int64  `json:"ttlMs"`
	}
	if json.Unmarshal(body, &doc) != nil || strings.TrimSpace(doc.Seed) == "" {
		return "", errors.New("seed request returned no seed")
	}
	ttl := mvSeedTTL
	if doc.TtlMs > 0 {
		ttl = time.Duration(doc.TtlMs) * time.Millisecond
	}
	movySeedMu.Lock()
	movySeed[mediaID] = movySeedEntry{seed: doc.Seed, expiresAt: time.Now().Add(ttl - 5*time.Second)}
	movySeedMu.Unlock()
	return doc.Seed, nil
}

func (m *movyMeta) displayTitle() string {
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

// movyQualityRank extracts the numeric tier from quality labels like "2160p",
// "1080p"; unranked labels ("Auto", "Auto HLS", "Auto - Vidara", language
// labels) rank 0.
func movyQualityRank(q string) int {
	q = strings.TrimSpace(strings.ToLower(q))
	q = strings.TrimSuffix(q, "p")
	n, err := strconv.Atoi(q)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// movyQualityLabel normalizes a raw tier label ("Auto HLS" → "auto") for logs
// and the dashboard height fallback.
func movyQualityLabel(q string) string {
	if movyQualityRank(q) > 0 {
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

// mvRoundConstants is Hl from the bundle — the SHA-256 round constants
// shipped in the decoder, used verbatim during state initialization's dead
// parity branch and as slot pre-fill (also dead).
var mvRoundConstants = [16]uint32{
	1116352408, 1899447441, 3049323471, 3921009573,
	961987163, 1508970993, 2453635748, 2870763221,
	3624381080, 310598401, 607225278, 1426881987,
	1925078388, 2162078206, 2614888103, 3248222580,
}

// mvFinalize is the MurmurHash3 32-bit finalizer (`o` in the bundle).
func mvFinalize(x uint32) uint32 {
	x ^= x >> 16
	x *= 2246822507
	x ^= x >> 13
	x *= 3266489909
	x ^= x >> 16
	return x
}

// mvRotl is `u` — rotate-left with JavaScript's shift-count masking.
func mvRotl(l, o uint32) uint32 {
	o &= 31
	if o == 0 {
		return l
	}
	return l<<o | l>>(32-o)
}

// mvHashSeed is the FNV-1a accumulation over the seed's code units
// (ASCII-safe because Math.imul multiplies 32-bit anyway).
func mvHashSeed(seed string) uint32 {
	h := uint32(2166136261)
	for _, r := range seed {
		h = (h ^ uint32(r)) * 16777619
	}
	return mvFinalize(h)
}

// mvState is the PRNG state ({S, acc} in the bundle). Slots start sparse:
// only indices touched during initialization participate in the `d in n`
// presence check.
type mvState struct {
	s       [61]uint32
	present [61]bool
	acc     uint32
}

// newMVState is the seed-state function. The bundle's odd-length and
// parity branches are dead code (their guards test e*(e+1)&1, always even
// for consecutive integers) and are omitted.
func newMVState(seed string, tmdbID uint32) *mvState {
	st := &mvState{}
	i := mvFinalize(mvHashSeed(seed) ^ mvFinalize(tmdbID^0x9e3779b9))
	for r := 0; r < 8; r++ {
		n := i % 61
		i = mvRotl(i+0x9e3779b9, uint32(7+r))
		st.s[n] = i ^ mvFinalize(i)
		st.present[n] = true
		i = mvFinalize(i + n)
	}
	st.acc = mvFinalize(0xa5a5a5a5 ^ i)
	_ = mvRoundConstants
	return st
}

// mvNext is the bundle's per-word keystream generator, counter from 0.
func (st *mvState) mvNext(counter uint32) uint32 {
	r := st.acc % 61
	var u uint32
	if st.present[r] {
		u = st.s[r]
	}
	d := 0x9e3779b9 * (counter + 1)
	o := u ^ d
	g := st.acc ^ o
	if st.present[r] {
		// The (t|s&i) term with e=-1: (l^o) | (l&o).
		g |= st.acc & o
	}
	g = mvRotl(g+st.acc, r&31) ^ mvRotl(st.acc, (r*7)&31)
	v := mvFinalize(g + 0x9e3779b9)
	st.s[r] = v
	st.present[r] = true
	st.acc = v
	return v
}

// mvKeystream is the byte stream (xf): words emitted little-endian, counter
// incrementing per word.
func mvKeystream(seed string, tmdbID uint32, n int) []byte {
	st := newMVState(seed, tmdbID)
	out := make([]byte, n)
	var counter uint32
	for i := 0; i < n; {
		w := st.mvNext(counter)
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

// decryptMVSources decodes and XOR-decrypts a sources payload, verifying the
// magic prefix (`i` in the bundle).
func decryptMVSources(encoded, seed string, tmdbID uint32) ([]byte, error) {
	std := strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimSpace(encoded))
	if m := len(std) % 4; m != 0 {
		std += strings.Repeat("=", 4-m)
	}
	data, err := base64.StdEncoding.DecodeString(std)
	if err != nil {
		return nil, fmt.Errorf("payload is not base64: %w", err)
	}
	ks := mvKeystream(seed, tmdbID, len(data))
	for i := range data {
		data[i] ^= ks[i]
	}
	if len(data) < len(mvMagic) || string(data[:len(mvMagic)]) != mvMagic {
		return nil, errors.New("decrypt failed: bad seed or tampered payload")
	}
	return data[len(mvMagic):], nil
}
