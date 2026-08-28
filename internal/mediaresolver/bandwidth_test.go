package mediaresolver

// Standalone bandwidth tester for every upstream media server GoFlix knows
// about: vixsrc, all nine vidking source servers, all seven vidlove source
// servers and vidsrcme. Unlike production Resolve() — which stops at the first
// server that yields a stream — this walks the full server lists so every
// upstream can be compared side by side.
//
// Run it with:
//
//	go test -v -run TestBandwidth ./internal/mediaresolver -timeout 20m
//
// Optional environment overrides:
//
//	BW_TYPE=movie|tv  BW_ID=<tmdb id>  BW_SEASON=1  BW_EPISODE=1
//	BW_PROVIDERS=vixsrc,vidking,vidlove,vidsrcme   (comma-separated subset)

//
// For every server the harness reports:
//
//	resolved  — wall time of the direct resolution chain (API calls through
//	            a validated playlist), in milliseconds. The first vidking
//	            server additionally carries the shared metadata/seed fetch.
//	ping      — average of three TCP connects to the CDN host serving the
//	            media segments, in ms.
//	ttfb      — time to the first response byte of the first segment, in ms.
//	bandwidth — aggregated goodput of consecutive media segments downloaded
//	            sequentially for ~4 s, in Mbit/s. Single connection only —
//	            the production proxy additionally runs five parallel
//	            read-ahead connections, so real playback throughput is higher.
//
// Servers that fail to resolve produce a row carrying the error and dashes
// for the numeric columns; the test itself never fails on upstream problems.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/tabwriter"
	"time"
)

const (
	// bwSegmentTargetSeconds is how long consecutive segment downloads run
	// before the goodput figure is finalized; bwSegmentMaxCount caps the
	// segment count so fast hosts do not balloon the runtime.
	bwSegmentTargetSeconds = 4
	bwSegmentMaxCount      = 4
	// bwSegmentByteCap bounds one segment download.
	bwSegmentByteCap = 24 << 20
	// bwProbeCandidates is how many segment URLs are harvested from a media
	// playlist before downloading starts.
	bwProbeCandidates = 6
	// bwOverallBudget bounds the whole sweep; rows still pending when it
	// expires are reported as skipped.
	bwOverallBudget = 12 * time.Minute
)

var (
	bwStreamInfBandwidthRE  = regexp.MustCompile(`BANDWIDTH=(\d+)`)
	bwStreamInfResolutionRE = regexp.MustCompile(`RESOLUTION=(\d+x\d+)`)
	bwAttrURIRe             = regexp.MustCompile(`URI="([^"]+)"`)
)

// bwResult is one row of the report.
type bwResult struct {
	Server    string // "vixsrc" or "vidking/YORU"-style provider/name pair
	Tier      string // resolved quality tier ("2160p", "1920x1080", …)
	CDNHost   string
	ResolveMS int64
	PingMS    string
	TTFBMS    string
	Mbps      string
	Segments  int
	Bytes     int64
	Error     string
}

// bwTarget is one server under test with its provider-specific resolver.
type bwTarget struct {
	provider, name string
	resolve        func(ctx context.Context, req MediaRequest) (*directResolution, string, error)
}

func bwEnvOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// bwProvidersFilter parses BW_PROVIDERS; nil means "all providers".
func bwProvidersFilter() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("BW_PROVIDERS"))
	if raw == "" {
		return nil
	}
	m := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			m[p] = true
		}
	}
	return m
}

// bwRequestFromEnv builds the media request every server is tested with.
func bwRequestFromEnv() MediaRequest {
	typ := MediaType(strings.ToLower(bwEnvOr("BW_TYPE", "movie")))
	if typ != Movie && typ != TV {
		typ = Movie
	}
	return MediaRequest{
		Type:    typ,
		ID:      bwEnvOr("BW_ID", "27205"), // Inception
		Season:  bwEnvOr("BW_SEASON", "1"),
		Episode: bwEnvOr("BW_EPISODE", "1"),
	}
}

func bwServerName(provider, name string) string {
	if name == "" {
		return provider
	}
	return provider + "/" + name
}

// bwTrimErr flattens an error to one short table-safe line.
func bwTrimErr(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 70 {
		s = s[:69] + "…"
	}
	return s
}

// TestBandwidth resolves every upstream server with the production direct
// chains, then pings and downloads real segments from each. It is a report,
// not an assertion: upstream outages never fail the test.
func TestBandwidth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live bandwidth benchmark in short mode")
	}
	req := bwRequestFromEnv()
	filter := bwProvidersFilter()
	t.Logf("Probing upstream servers with %s %s (season %s, episode %s)",
		req.Type, req.ID, req.Season, req.Episode)

	headless := true
	if v := os.Getenv("BROWSER_HEADLESS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			headless = b
		}
	}
	r, err := New(Config{MaxBrowserSessions: 1, BrowserHeadless: headless})

	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), bwOverallBudget)
	defer cancel()

	// Shared vidking metadata + seed: fetched once on first use and reused by
	// every vidking server so per-server timings cover that server's own
	// chain only (the first vidking row carries the shared fetch).
	var vk struct {
		ready  bool
		client *http.Client
		meta   *vidkingMeta
		seed   string
		err    error
	}
	prepVidking := func(ctx context.Context) (*vidkingMeta, string, error) {
		if !vk.ready {
			vk.ready = true
			vk.client = &http.Client{Transport: r.transport, Timeout: 12 * time.Second}
			vk.meta, vk.err = r.fetchVidkingMeta(ctx, vk.client, req)
			if vk.err == nil {
				vk.seed, vk.err = fetchVidkingSeed(ctx, vk.client, req.ID)
			}
		}
		return vk.meta, vk.seed, vk.err
	}

	filtered := func(provider string) bool { return filter == nil || filter[provider] }
	var targets []bwTarget
	add := func(provider, name string, fn func(ctx context.Context, req MediaRequest) (*directResolution, string, error)) {
		if filtered(provider) {
			targets = append(targets, bwTarget{provider: provider, name: name, resolve: fn})
		}
	}

	add("vixsrc", "", func(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		res, err := r.resolveVixsrcDirect(cctx, req)
		return res, "", err
	})

	for _, srv := range vidkingServers {
		add("vidking", srv.name, func(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
			meta, seed, perr := prepVidking(ctx)
			if perr != nil {
				return nil, "", fmt.Errorf("shared metadata/seed: %w", perr)
			}
			year := meta.ReleaseDate
			mediaType := "movie"
			if req.Type == TV {
				mediaType = "tv"
				year = meta.FirstAirDate
			}
			if len(year) > 4 {
				year = year[:4]
			}
			cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			payload, err := r.fetchVidkingSources(cctx, vk.client, srv, vidkingQuery{
				MediaType: mediaType, TMDBID: req.ID, Season: req.Season, Episode: req.Episode,
				Title: meta.displayTitle(), Year: year, IMDBID: meta.ExternalIDs.IMDBID,
			}, &seed)
			if err != nil {
				return nil, "", err
			}
			return r.finishVidkingSources(cctx, payload, srv)
		})
	}

	for _, key := range vidloveServers {
		add("vidlove", key, func(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			headers := make(http.Header)
			headers.Set("User-Agent", defaultUserAgent)
			headers.Set("Referer", r.cfg.VidLoveOrigin+"/")
			headers.Set("Accept", "application/json")
			path := "/movie?id=" + url.QueryEscape(req.ID) + "&mode=json"
			if req.Type == TV {
				path = "/tv?id=" + url.QueryEscape(req.ID) +
					"&season=" + url.QueryEscape(req.Season) +
					"&episode=" + url.QueryEscape(req.Episode) + "&mode=json"
			}
			apiReq, err := http.NewRequestWithContext(cctx, http.MethodGet, vidloveAPIBase+path+"&sources="+key, nil)
			if err != nil {
				return nil, "", err
			}
			for _, k := range []string{"User-Agent", "Referer", "Accept"} {
				apiReq.Header.Set(k, headers.Get(k))
			}
			client := &http.Client{Transport: r.transport, Timeout: 10 * time.Second}
			resp, err := client.Do(apiReq)
			if err != nil {
				return nil, "", err
			}
			body, rerr := io.ReadAll(io.LimitReader(resp.Body, vidloveResponseCap))
			resp.Body.Close()
			if rerr != nil {
				return nil, "", rerr
			}
			if resp.StatusCode != http.StatusOK {
				return nil, "", fmt.Errorf("api returned status %d", resp.StatusCode)
			}
			var parsed vidloveAPIResponse
			if jerr := json.Unmarshal(body, &parsed); jerr != nil {
				return nil, "", fmt.Errorf("invalid JSON: %w", jerr)
			}
			if parsed.Source == nil || parsed.Source.URL == nil || strings.TrimSpace(*parsed.Source.URL) == "" {
				return nil, "", fmt.Errorf("api returned no source URL")
			}
			res, ferr := r.finishVidloveSource(cctx, *parsed.Source, headers)
			return res, "", ferr
		})
	}

	add("vidsrcme", "", func(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		res, err := r.resolveVidsrcmeDirect(cctx, req)
		return res, "", err
	})

	add("cinesrc", "", func(ctx context.Context, req MediaRequest) (*directResolution, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		res, err := r.resolveCinesrcDirect(cctx, req)
		return res, "", err
	})


	var results []bwResult
	for _, tg := range targets {
		name := bwServerName(tg.provider, tg.name)
		if ctx.Err() != nil {
			results = append(results, bwResult{Server: name, Error: "skipped: time budget exhausted"})
			continue
		}
		t.Logf("[%s] resolving…", name)
		res := r.bwMeasure(ctx, req, tg)
		if res.Error == "" {
			t.Logf("[%s] %s Mbit/s (ping %s ms, resolved in %d ms)",
				name, res.Mbps, res.PingMS, res.ResolveMS)
		} else {
			t.Logf("[%s] failed: %s", name, res.Error)
		}
		results = append(results, res)
	}
	bwPrintReport(t, results)
}

// bwFetchPlaylist GETs raw with the resolution's headers replayed and returns
// the body text. Headers must be replayed because most CDNs reject segment
// and manifest requests without the exact UA/Referer pair used to obtain them.
func bwFetchPlaylist(ctx context.Context, client *http.Client, headers http.Header, raw string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
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
		return "", fmt.Errorf("playlist fetch returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// bwTCPPingAvg dials the CDN host three times and returns the average TCP
// connect time in milliseconds. Three samples smooth out single-packet
// spikes; "-" (instead of an error) keeps the report row table-aligned when
// the host refuses connections entirely.
func bwTCPPingAvg(ctx context.Context, u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	addr := net.JoinHostPort(u.Hostname(), port)
	dialer := &net.Dialer{Timeout: 3 * time.Second}

	var total time.Duration
	var ok int
	for i := 0; i < 3; i++ {
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			continue
		}
		conn.Close()
		total += time.Since(start)
		ok++
	}
	if ok == 0 {
		return "-"
	}
	return strconv.FormatInt((total / time.Duration(ok)).Milliseconds(), 10)
}

// bwBestVariant picks the highest-bandwidth #EXT-X-STREAM-INF entry from
// master playlist text — the variant a bandwidth test should download, since
// it is what a fast client would actually be served. Ties keep the first
// entry so results are deterministic for identical upstream playlists.
func bwBestVariant(master string) (uri string, bandwidth int64, resolution string, ok bool) {
	lines := strings.Split(master, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			continue
		}
		bw := int64(0)
		if m := bwStreamInfBandwidthRE.FindStringSubmatch(line); m != nil {
			bw, _ = strconv.ParseInt(m[1], 10, 64)
		}
		res := ""
		if m := bwStreamInfResolutionRE.FindStringSubmatch(line); m != nil {
			res = m[1]
		}
		// The variant URI is the next non-empty, non-'#' line after the
		// STREAM-INF tag; advancing i past it keeps later tags paired with
		// their own URIs.
		next := ""
		for j := i + 1; j < len(lines); j++ {
			cand := strings.TrimSpace(lines[j])
			if cand == "" || cand[0] == '#' {
				continue
			}
			next, i = cand, j
			break
		}
		if next == "" {
			continue // attribute line with no URI following: not a candidate
		}
		if !ok || bw > bandwidth {
			uri, bandwidth, resolution, ok = next, bw, res, true
		}
	}
	return uri, bandwidth, resolution, ok
}

// joinPlaylistURL resolves ref against the playlist URL so the relative
// variant and segment paths HLS manifests are full of become absolute
// request targets pointing back at the host that served the manifest.
func joinPlaylistURL(base *url.URL, ref string) (real *url.URL, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("playlist reference is empty")
	}
	real, err = base.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("resolve %q against %s: %w", ref, base.Host, err)
	}
	return real, nil
}

// bwSegmentURLs harvests up to want playable URLs from media-playlist text.
// The #EXT-X-MAP init segment counts too: a player downloads it before any
// media segment, so goodput measured without it would understate TTFB.
func bwSegmentURLs(playlistURL string, text string, want int) []string {
	base, perr := url.Parse(playlistURL)
	if perr != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if len(out) == want {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ref := line
		if line[0] == '#' {
			// Only #EXT-X-MAP carries a downloadable body; every other tag
			// (#EXT-X-KEY metadata aside) is pure playlist decoration.
			if !strings.HasPrefix(line, "#EXT-X-MAP:") {
				continue
			}
			m := bwAttrURIRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			ref = m[1]
		}
		u, jerr := joinPlaylistURL(base, ref)
		if jerr != nil {
			continue
		}
		out = append(out, u.String())
	}
	return out
}

// bwDownloadSegments downloads segments sequentially and aggregates goodput.
// Single connection only — the production proxy additionally fans out five
// read-ahead connections, so this number is a conservative floor for real
// playback throughput. A partial copy still counts its bytes: half a segment
// is honest data the connection did deliver.
func bwDownloadSegments(ctx context.Context, client *http.Client, segs []string, headers http.Header) (count int, totalBytes int64, ttfb time.Duration, elapsed time.Duration, err error) {
	start := time.Now()
	for i := 0; i < len(segs); i++ {
		if i >= bwSegmentMaxCount || (i > 0 && time.Since(start) >= bwSegmentTargetSeconds*time.Second) {
			break
		}
		if ctx.Err() != nil {
			break
		}
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, segs[i], nil)
		if rerr != nil {
			if i == 0 {
				err = rerr
			}
			break
		}
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
		if req.Header.Get("Range") == "" {
			req.Header.Set("Range", "bytes=0-")
		}
		reqStart := time.Now()
		resp, derr := client.Do(req)
		if derr != nil {
			if i == 0 {
				err = derr
			}
			break
		}
		if ttfb == 0 {
			ttfb = time.Since(reqStart) // time to first response headers
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if i == 0 {
				err = fmt.Errorf("segment returned status %d", resp.StatusCode)
			}
			break
		}
		n, cerr := io.Copy(io.Discard, io.LimitReader(resp.Body, bwSegmentByteCap))
		resp.Body.Close()
		totalBytes += n
		count++
		if cerr != nil {
			break
		}
	}
	elapsed = time.Since(start)
	return count, totalBytes, ttfb, elapsed, err
}

// bwMeasure resolves one upstream server through its production chain, then
// takes ping/throughput readings against whatever host actually serves the
// media. Every failure lands in the row's Error column: a dead upstream is a
// data point, never a test failure.
func (r *Resolver) bwMeasure(ctx context.Context, req MediaRequest, tg bwTarget) bwResult {
	res := bwResult{Server: bwServerName(tg.provider, tg.name)}
	start := time.Now()

	dres, tier, err := tg.resolve(ctx, req)
	res.ResolveMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = bwTrimErr(err)
		return res
	}
	res.Tier = tier

	base, perr := url.Parse(dres.Source)
	if perr != nil || base.Host == "" {
		res.Error = "resolution returned an unusable URL"
		return res
	}

	client := &http.Client{Transport: r.transport, Timeout: 20 * time.Second}

	mediaURL, mediaText := dres.Source, dres.MasterText
	if mediaText == "" {
		text, ferr := bwFetchPlaylist(ctx, client, dres.Headers, mediaURL)
		if ferr != nil {
			res.Error = bwTrimErr(ferr)
			return res
		}
		mediaText = text
	}

	// A master playlist needs one more hop: pick the top-bandwidth variant,
	// since that is what a fast client would actually be served.
	if strings.Contains(mediaText, "#EXT-X-STREAM-INF") {
		vURI, _, vRes, vOK := bwBestVariant(mediaText)
		if !vOK {
			res.Error = "master playlist lists no parsable variant"
			return res
		}
		if res.Tier == "" && vRes != "" {
			res.Tier = vRes // vixsrc reports no tier itself; fall back to RESOLUTION
		}
		vAbs, jerr := joinPlaylistURL(base, vURI)
		if jerr != nil {
			res.Error = "bad variant URL"
			return res
		}
		variantURL := vAbs.String()
		text, ferr := bwFetchPlaylist(ctx, client, dres.Headers, variantURL)
		if ferr != nil {
			res.Error = bwTrimErr(ferr)
			return res
		}
		mediaURL, mediaText = variantURL, text
	} else if res.Tier == "" {
		res.Tier = "auto"
	}

	// Segments usually come off a different CDN host than the manifest; ping
	// whichever host will actually carry the bytes.
	pingTarget := base
	if mu, merr := url.Parse(mediaURL); merr == nil && mu.Hostname() != "" {
		pingTarget = mu
	}
	res.CDNHost = pingTarget.Hostname()
	res.PingMS = bwTCPPingAvg(ctx, pingTarget)

	segs := bwSegmentURLs(mediaURL, mediaText, bwProbeCandidates)
	if len(segs) == 0 {
		res.Error = "media playlist lists no segments"
		return res
	}

	count, total, ttfb, elapsed, derr := bwDownloadSegments(ctx, client, segs, dres.Headers)
	res.Segments = count
	res.Bytes = total
	if ttfb > 0 {
		res.TTFBMS = strconv.FormatInt(ttfb.Milliseconds(), 10)
	}
	if total > 0 && elapsed > 0 {
		res.Mbps = strconv.FormatFloat(float64(total)*8/elapsed.Seconds()/1e6, 'f', 1, 64)
	}
	if derr != nil && total == 0 {
		res.Error = bwTrimErr(derr)
	}
	return res
}

// bwPrintReport renders the aligned comparison table and a one-line summary
// all through t.Log so a single `-v` run captures everything worth keeping
// about the sweep. Errors are consolidated at the bottom.
func bwPrintReport(t *testing.T, results []bwResult) {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}

	t.Log("")
	t.Log("=== Upstream server bandwidth report ===")

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVER\tTIER\tCDN HOST\tRESOLVED ms\tPING ms\tTTFB ms\tBANDWIDTH Mbps\tSEGS\tMB")
	for _, r := range results {
		mb := "-"
		if r.Bytes != 0 {
			mb = fmt.Sprintf("%.2f", float64(r.Bytes)/(1<<20))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\t%s\n",
			r.Server, dash(r.Tier), dash(r.CDNHost), r.ResolveMS,
			dash(r.PingMS), dash(r.TTFBMS), dash(r.Mbps), r.Segments, mb)
	}
	tw.Flush()
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		t.Log(line)
	}

	fastServer, fastMbps, fastMbpsStr := "", 0.0, ""
	var errs []string
	for _, r := range results {
		if v, perr := strconv.ParseFloat(r.Mbps, 64); perr == nil && v > fastMbps {
			fastServer, fastMbps, fastMbpsStr = r.Server, v, r.Mbps
		}
		if r.Error != "" {
			errs = append(errs, fmt.Sprintf("%s: %s", r.Server, r.Error))
		}
	}
	
	t.Log("")
	if fastServer != "" {
		t.Logf("Fastest upstream: %s at %s Mbit/s (single sequential connection)",
			fastServer, fastMbpsStr)
	} else {
		t.Log("No server produced a bandwidth reading.")
	}

	if len(errs) > 0 {
		t.Log("")
		t.Log("errors:")
		for _, e := range errs {
			t.Log(e)
		}
	}
}
