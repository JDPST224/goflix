# GoFlix

A self-hosted streaming front-end: browse a TMDB-backed catalog of movies and
TV shows, pick a provider, and stream through a built-in HLS proxy that keeps
playback smooth — with subtitles that work on desktop browsers and smart-TV
players alike.

## Features

- **Catalog** — trending and categorized rows for movies and TV, search,
  detail pages with seasons/episodes (TMDB API).
- **Multi-provider sources** — VixSrc, VidKing and VidLove are resolved
  directly against their endpoints for fast startup; if that fails, a
  headless-Chrome scrape of the provider page takes over automatically.
- **Streaming proxy** — token-authenticated reverse proxy with an in-memory
  segment cache and read-ahead warming: playback starts from RAM instead of
  waiting on cold upstream fetches. Range requests supported for instant seeks.
- **Subtitles everywhere** — search across OpenSubtitles/Vidlove with SRT→WebVTT
  conversion, and server-side embedding into the HLS master manifest so
  smart-TV native players (which ignore `<track>` elements) get selectable
  subtitle languages too.
- **My List & continue watching** — stored locally in the browser, no
  accounts, no database.

## Requirements

- Go 1.26 or newer
- A TMDB API key or v4 Bearer Read Access Token (free at themoviedb.org)
- Chrome/Chromium installed — only needed when the fallback scraper runs

## Quick start

1. Put your TMDB credentials in `config.conf` (`TMDB_API_KEY` or
   `TMDB_ACCESS_TOKEN`).
2. Build and run:

   ```
   go build -o goflix .
   ./goflix        # Windows: goflix.exe
   ```

3. Open http://localhost:8080

The default listen address is `:8080` (configurable via `LISTEN_ADDR`). Any config value can be overridden by an environment variable of the same name.

## Configuration

All settings live in `config.conf` (`KEY = value`, `#` comments); values are
parsed leniently — an invalid value falls back to the default.

| Key | Default | Description |
|-----|---------|-------------|
| `LISTEN_ADDR` | `:8080` | TCP address the HTTP server binds to |
| `TMDB_API_KEY` / `TMDB_ACCESS_TOKEN` | — | TMDB credentials; Bearer token preferred when both set |
| `BROWSER_HEADLESS` | `true` | Run the fallback scraper without a visible window |
| `BROWSER_TIMEOUT` | `45s` | Per-attempt timeout for browser-based resolution |
| `MAX_BROWSER_SESSIONS` | `3` | Cap on concurrent headless-Chrome sessions |
| `MAX_SESSIONS` | `200` | Cap on concurrent active proxy/playback sessions |
| `BROWSER_EXECUTABLE` | auto-detect | Path to a specific Chrome/Chromium binary |
| `CACHE_MAX_MB` | `512` | RAM cap for the manifest/segment body cache |
| `VIXSRC_ORIGIN` / `VIDKING_ORIGIN` / `VIDLOVE_ORIGIN` / `VIDSRCME_ORIGIN` / `VIDSRCME_DATA_ORIGIN` | provider URLs | Override media source origins |

## Smart-TV playback

When the browser can't run hls.js (most TV browsers), the player switches to
native HLS. Native engines ignore DOM `<track>` subtitles, so GoFlix embeds
them into the master manifest instead: during resolution it fetches the
subtitle ladder server-side and rewrites the manifest with proper
`TYPE=SUBTITLES` renditions, exactly like providers that ship their own
subtitle groups. Providers that hand out a bare media playlist (no master)
get a synthesized one so there is always somewhere to declare the group.

## Layout

```
main.go                     wiring only: config → resolver → client/store → HTTP server
debug/                      unit test suites (manifest, resolver, server, subtitles)
internal/config/            config.conf parsing (+ env overrides), defaults
internal/catalog/           TMDB client, movie/TV mapping, cache stores
internal/server/            route table, method/CORS gates, gzip, handlers
internal/subtitles/         OpenSubtitles/Vidlove search clients, SRT→WebVTT converter
internal/mediaresolver/     the media pipeline:
    resolver.go               Resolve() orchestration, sessions, config
    cinesrc/vidking/vidlove/vidsrcme/vixsrc.go per-provider direct resolvers
    browser.go                headless-Chrome fallback scrape
    proxy.go                  the streaming reverse-proxy endpoint
    manifest.go               manifest rewriting + subtitle rendition embedding
    session.go / cache.go / warmer.go   proxy state, body LRU, read-ahead
    bandwidth_test.go         live upstream bandwidth benchmark
static/js/                  vanilla ES modules: main.js (orchestrator),
                            storage.js (My List / progress), utils.js
```

## HTTP API

- `GET /api/home|movies|tvshows|popular` — gzipped catalog JSON
- `GET /api/search?q=&type=` · `/api/detail?type=&id=` · `/api/episodes?id=&season=`
- `GET /api/media/source/<provider>/movie/<tmdbId>` and `/tv/<id>/<s>/<e>`
  (`provider` ∈ cinesrc | vixsrc | vidking | vidlove | vidsrcme); legacy unprefixed routes map
  to VixSrc
- `GET /embed/movie/<tmdbId>` and `GET /embed/tv/<id>[?s=<s>&e=<e>]` — direct CineSrc embed resolution (redirects to stream or returns JSON)
- `GET /api/media/proxy/<token>.m3u8?url=...` — HLS proxy (supports Range)
- `POST /api/media/subs/<token>` — register extra subtitle renditions on a
  live session; the resolver embeds its own ladder automatically for
  cinesrc/vidking/vidlove/vidsrcme, this tops it up
- `GET /api/subtitles/cinesrc|vidking|vidlove|vidsrcme?type=&id=&season=&episode=` — search
- `GET /api/subtitles/cinesrc/download?url=` ·
  `GET /api/subtitles/opensubtitles/download?url=` ·
  `GET /api/subtitles/vidlove/download?url=` ·
  `GET /api/subtitles/vidsrcme/download?url=` — WebVTT download/convert
- `GET /api/subtitles/wrap.m3u8?src=...` — single-segment playlist wrapping a
  local subtitle endpoint (rendition target for native players)

## Troubleshooting

- **"No TMDB credentials" warning at startup** — set `TMDB_API_KEY` or
  `TMDB_ACCESS_TOKEN` in `config.conf`; catalog pages stay empty without it.
- **Source resolution fails** — the log shows each direct attempt falling back
  to the browser scrape; make sure Chrome is installed, or point
  `BROWSER_EXECUTABLE` at it.
- **Playback stalls on slow connections** — raise `CACHE_MAX_MB` so the
  read-ahead keeps more of the stream hot in RAM.
- **Playback buffers when the server runs on a VPS** — long or lossy routes
  between the server and viewers benefit from BBR congestion control. Enable
  it once on the VPS:

  ```bash
  echo "net.core.default_qdisc=fq" | sudo tee -a /etc/sysctl.conf
  echo "net.ipv4.tcp_congestion_control=bbr" | sudo tee -a /etc/sysctl.conf
  sudo sysctl -p
  ```

  Verify with `sysctl net.ipv4.tcp_congestion_control` — it should print
  `bbr`.
- **Port already in use** — set `LISTEN_ADDR` in `config.conf` or pass the `LISTEN_ADDR` environment variable (e.g. `LISTEN_ADDR=:8081`).

## Testing & Debugging

All unit tests are located in the `debug/` directory, live upstream bandwidth benchmarks are kept in `internal/mediaresolver/bandwidth_test.go`, and proxy cache-admission regression tests in `internal/mediaresolver/proxy_cache_test.go`.

### Running Unit Tests

Run all unit tests in the `debug` suite:

```bash
go test ./debug/... -v
```

Or run all tests across the repository:

```bash
go test ./... -v
```

Test suites included in `debug/`:
- **Manifest Tests** (`debug/manifest_test.go`): Master manifest rewriting, highest quality ordering, and subtitle rendition injection.
- **Resolver Tests** (`debug/resolver_test.go`): HTTP byte-range parsing, SSRF/IP blocking, query string redaction, and candidate scoring.
- **Server Tests** (`debug/server_test.go`): Subtitle download endpoint SSRF protection and `/api/health` status checks.
- **Subtitle Tests** (`debug/subtitles_test.go`): SRT to WebVTT conversion, UTF-8 BOM/CRLF stripping, timestamp parsing, and duplicate cue removal.

### Upstream Bandwidth Testing

GoFlix includes a built-in upstream benchmark suite in `internal/mediaresolver/bandwidth_test.go` to probe and test the actual performance of upstream streaming servers. It walks each provider's real resolution chain, pings the edge CDN, measures Time-to-First-Byte (TTFB), and downloads real video segments to measure sustained throughput (Mbps).

#### Quick Test (All Providers)

Run the full benchmark across all configured servers:

* **Linux / macOS (Bash):**
  ```bash
  go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 20m
  ```

* **Windows (PowerShell):**
  ```powershell
  go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 20m
  ```

* **Windows (CMD):**
  ```cmd
  go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 20m
  ```

#### Configuration Environment Variables

You can customize the probe target and test scope using environment variables:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `BW_PROVIDERS` | *all* | Comma-separated list of providers to test: `cinesrc`, `vixsrc`, `vidking`, `vidlove`, `vidsrcme` |
| `BW_TYPE` | `movie` | Media type: `movie` or `tv` |
| `BW_ID` | `27205` | TMDB ID (e.g. `550` for *Fight Club*, `1396` for *Breaking Bad*, `27205` for *Inception*) |
| `BW_SEASON` | `1` | TV show season number (when `BW_TYPE=tv`) |
| `BW_EPISODE` | `1` | TV show episode number (when `BW_TYPE=tv`) |

#### Usage Examples

**1. Test a Single Provider (e.g. VixSrc):**

* **Linux / macOS:**
  ```bash
  BW_PROVIDERS=vixsrc BW_TYPE=movie BW_ID=550 go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 3m
  ```
* **Windows (PowerShell):**
  ```powershell
  $env:BW_PROVIDERS="vixsrc"; $env:BW_TYPE="movie"; $env:BW_ID="550"; go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 3m
  ```

**2. Test a TV Show Episode (e.g. Breaking Bad S01E01 on VidKing and VidSrcMe):**

* **Linux / macOS:**
  ```bash
  BW_PROVIDERS=vidking,vidsrcme BW_TYPE=tv BW_ID=1396 BW_SEASON=1 BW_EPISODE=1 go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 5m
  ```
* **Windows (PowerShell):**
  ```powershell
  $env:BW_PROVIDERS="vidking,vidsrcme"; $env:BW_TYPE="tv"; $env:BW_ID="1396"; $env:BW_SEASON="1"; $env:BW_EPISODE="1"; go test -v -count=1 -run TestBandwidth ./internal/mediaresolver -timeout 5m
  ```

#### Understanding the Benchmark Report

At the end of the test, GoFlix prints a tabular performance summary:

```text
=== Upstream server bandwidth report ===
SERVER           TIER       CDN HOST            RESOLVED ms  PING ms  TTFB ms  BANDWIDTH Mbps  SEGS  MB
vidking/YORU     1080p      moon.peakstorm.top  11160        67       784      44.9            4     13.65
vidsrcme         1920x800   comityofcognomen.site 2133       55       98       42.3            4     4.78
vidlove/vidapi   1920x800   a2.shows.st         404          168      400      23.7            4     4.78
vixsrc           1920x1080  vixsrc.to           1529         152      928      4.4             2     2.18

Fastest upstream: vidking/YORU at 44.9 Mbit/s (single sequential connection)
```

* **SERVER**: The provider and specific sub-server probed (e.g. VidKing sub-servers like `YORU`, `BREACH`, etc.).
* **TIER**: The stream resolution chosen (`2160p`, `1080p`, `1920x1080`, `auto`).
* **CDN HOST**: The edge CDN host serving the media segments.
* **RESOLVED ms**: Time in milliseconds to complete direct API resolution and extract the manifest.
* **PING ms**: Network TCP handshake RTT to the media CDN edge node.
* **TTFB ms**: Time to first byte when requesting segment chunks.
* **BANDWIDTH Mbps**: Sustained download speed over a single sequential connection. Note that GoFlix runs 5–8 parallel prefetch workers, so total available pipeline bandwidth is typically 4x–5x higher than this single-stream metric.
* **SEGS / MB**: Number of real media chunks downloaded and total data transferred.

> [!TIP]
> Go caches successful test results by default. Always include `-count=1` to ensure a live, real-time benchmark run, or run `go clean -testcache` to reset the test cache.



## Notes

- The frontend has no build step: edit the ES modules under `static/js/`
  directly. hls.js loads from a CDN (pinned 1.7.0), so playback clients need
  internet access to it.
- Go's standard library only, plus `chromedp/chromedp` for the fallback
  scraper.

> **Security:** if your `config.conf` holds real TMDB credentials, treat them
> like passwords — don't share the file. Rotate any key that has ever been
> exposed.
