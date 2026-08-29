# GoFlix

A self-hosted streaming front-end: browse a TMDB-backed catalog of movies and
TV shows, pick a provider, and stream through a built-in HLS proxy that keeps
playback smooth — with subtitles that work on desktop browsers and smart-TV
players alike.

## Features

- **Catalog** — trending and categorized rows for movies and TV, search,
  detail pages with seasons/episodes (TMDB API), poster/backdrop images served
  through a disk-cached `/api/img` proxy so browsing is LAN-fast.
- **Multi-provider sources** — VixSrc, VidKing, VidLove, CineSrc and VidSrcMe
  are resolved directly against their endpoints for fast startup; if that
  fails, a headless-Chrome scrape of the provider page takes over
  automatically.
- **Streaming proxy** — token-authenticated reverse proxy with an in-memory
  segment cache and read-ahead warming: playback starts from RAM instead of
  waiting on cold upstream fetches. Range requests supported for instant
  seeks. VOD playlists are treated as immutable and served from RAM.
- **Resolution cache** — every resolved title is remembered (with a
  fingerprint of the validated manifest), so rewatches start instantly; the
  upstream link is re-validated in the background and hot-swapped invisibly
  if it goes stale. The next TV episode is pre-resolved while you watch.
  Persists across restarts (`resolutions.json`), counters exposed on
  `/api/health`.
- **Accounts & cross-device sync** — optional accounts (registration at
  `/login`, invite-code gated via `AUTH_PASSWORD`). Browsing works without an
  account (localStorage only); signed-in users sync My List, playback
  progress, Continue Watching, audio/subtitle language preferences and
  removal tombstones across devices. First registered account is admin
  (`/account`: password change, user management, force sign-outs).
- **Player** — hls.js 1.7.0 (vendored), ABR starting at the top tier with an
  optional server-side resolution cap (`MAX_STREAM_HEIGHT`), double-tap
  seek, skip back/forward, next-episode overlay, PiP, and remembered
  audio/subtitle languages across episodes.
- **Subtitles everywhere** — search across OpenSubtitles/Vidlove with SRT→WebVTT
  conversion, and server-side embedding into the HLS master manifest so
  smart-TV native players (which ignore `<track>` elements) get selectable
  subtitle languages too.
- **Hardening** — per-IP rate limits on auth and resolution, security headers
  + CSP, bcrypt password hashes, session tokens hashed at rest, optional
  HTTPS with Secure cookies.

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
| `MAX_BROWSER_SESSIONS` | `1` | Cap on concurrent headless-Chrome sessions (fallback path only) |
| `MAX_SESSIONS` | `200` | Cap on concurrent active proxy/playback sessions |
| `BROWSER_EXECUTABLE` | auto-detect | Path to a specific Chrome/Chromium binary |
| `CACHE_MAX_MB` | `512` | RAM cap for the manifest/segment body cache |
| `MAX_STREAM_HEIGHT` | `0` (off) | Cap playback resolution (e.g. `1080`); ABR stays automatic below the cap. Multiplies concurrent capacity on a fixed pipe |
| `AUTH_PASSWORD` | — | Registration invite code; unset = open registration. Browsing never requires it |
| `USERS_FILE` | `users.json` | Accounts + sessions store (`-` = memory only) |
| `USERDATA_FILE` | `userdata.json` | Per-account synced data store (`-` = memory only) |
| `RESOLUTION_CACHE_FILE` | `resolutions.json` | Resolution cache persistence (`-` = memory only) |
| `CATALOG_SNAPSHOT_FILE` | `catalog_snapshot.json` | Catalog caches persisted across restarts (`-` = off) |
| `IMAGES_DIR` | `images` | Disk cache for the `/api/img` poster proxy (`-` = redirect to CDN) |
| `AUTH_RATE_PER_MIN` / `RESOLVE_RATE_PER_MIN` | `10` / `10` | Per-IP rate limits (login+register / media resolutions) |
| `TLS_CERT` / `TLS_KEY` | — | Serve HTTPS when both set; session cookies become `Secure` |
| `DEBUG_PPROF` | `false` | Mount Go pprof endpoints under `/debug/pprof/` |
| `VIXSRC_ORIGIN` / `VIDKING_ORIGIN` / `VIDLOVE_ORIGIN` / `VIDSRCME_ORIGIN` / `VIDSRCME_DATA_ORIGIN` / `CINESRC_ORIGIN` | provider URLs | Override media source origins |

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
debug/                      black-box test suites (manifest, resolver, server, subtitles, http-api)
internal/config/            config.conf parsing (+ env overrides), defaults
internal/catalog/           TMDB client, movie/TV mapping, cache stores
internal/server/            route table, method gates, gzip, auth/accounts, userdata
                            sync, image proxy, rate limiting, security headers
internal/subtitles/         OpenSubtitles/Vidlove search clients, SRT→WebVTT converter
internal/mediaresolver/     the media pipeline:
    resolver.go               Resolve() orchestration, sessions, config
    resolution_cache.go       instant-rewatch cache: validate/heal, prewarm, persist
    cinesrc/vidking/vidlove/vidsrcme/vixsrc.go per-provider direct resolvers
    browser.go                headless-Chrome fallback scrape
    proxy.go                  the streaming reverse-proxy endpoint
    manifest.go               manifest rewriting + subtitle rendition embedding
    session.go / cache.go / warmer.go   proxy state, body LRU, read-ahead
    bandwidth_test.go         live upstream bandwidth benchmark
static/js/                  vanilla ES modules: main.js (orchestrator),
                            storage.js (My List / progress / sync), utils.js
static/account.html         account self-service page (password, admin users)
static/login.html + js/     sign-in / registration page
```

## HTTP API

- `GET /api/home|movies|tvshows|popular` — gzipped catalog JSON
- `GET /api/search?q=&type=` · `/api/detail?type=&id=` · `/api/episodes?id=&season=`
- `GET /api/img?u=<tmdb-image-url>` — disk-cached image proxy (host-allowlisted)
- `GET /api/media/source/<provider>/movie/<tmdbId>` and `/tv/<id>/<s>/<e>`
  (`provider` ∈ cinesrc | vixsrc | vidking | vidlove | vidsrcme); legacy unprefixed routes map
  to VixSrc
- `GET /embed/movie/<tmdbId>` and `GET /embed/tv/<id>[?s=<s>&e=<e>]` — direct CineSrc embed resolution (redirects to stream or returns JSON)
- `GET /api/media/proxy/<token>.m3u8?url=...` — HLS proxy (supports Range)
- `POST /api/media/invalidate/<token>` — drop the remembered resolution behind a session (stale-link healing)
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
- `GET /api/auth/status` · `POST /api/auth/register|login|logout|password` — accounts
- `GET /api/userdata` · `POST /api/userdata/sync` — per-account data sync
  (My List, progress, Continue Watching, A/V preferences, removal tombstones)
- `GET /api/admin/users` · `DELETE /api/admin/users/<id>` ·
  `POST /api/admin/users/<id>/logout` — admin account management
- `GET /api/health` — uptime, catalog counts, resolution-cache counters,
  playback policy

## Accounts & data model

- **Anonymous visitors** can browse and play everything; their My List /
  progress / Continue Watching live only in that browser's localStorage.
- **Signed-in users** sync across devices. The merge rules: progress entries
  keep the newest timestamp, Continue Watching is newest-first (max 20),
  My List is a union, removals are tombstoned so one device's delete is not
  resurrected by another device's older copy, and A/V preferences are
  last-writer-wins.
- The **first registered account is admin** and manages accounts at
  `/account` (change password, list users, force sign-out, delete).
- State files (all safe to delete while the server is stopped):
  `users.json` (accounts/sessions), `userdata.json` (synced data),
  `resolutions.json` (resolution cache), `catalog_snapshot.json` (catalog),
  `images/` (poster cache).

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

Black-box test suites live in the `debug/` directory — they exercise the real
mux and exported API only. White-box tests that reach into `mediaresolver`
internals (unexported fields and helpers) must live beside the package they
test, per Go's testing rules: `internal/mediaresolver/bandwidth_test.go`
(live benchmark), `proxy_cache_test.go` (cache admission + read-ahead
regressions) and `resolution_cache_test.go` (cache persistence round-trips).

### Running Unit Tests

Run all black-box suites in `debug/`:

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
- **HTTP API Tests** (`debug/httpapi_test.go`): account registration/login/logout, invite codes, admin account management, per-user userdata isolation and merge semantics (progress newest-wins, My List union, removal tombstones), per-IP rate limits, security headers, `/api/img` relay protection, and the health envelope.
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
  directly. hls.js 1.7.0 is vendored at `static/vendor/hls.min.js` (served
  with an immutable cache policy), so playback works without third-party CDNs.
- Dependencies: Go standard library, `chromedp/chromedp` (fallback scraper),
  `golang.org/x/crypto` (bcrypt password hashing).

> **Security:** if your `config.conf` holds real TMDB credentials, treat them
> like passwords — don't share the file. Rotate any key that has ever been
> exposed.
