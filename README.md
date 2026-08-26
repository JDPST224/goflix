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

The listen address is fixed at `:8080`. Any config value can be overridden by
an environment variable of the same name.

## Configuration

All settings live in `config.conf` (`KEY = value`, `#` comments); values are
parsed leniently — an invalid value falls back to the default.

| Key | Default | Description |
|-----|---------|-------------|
| `TMDB_API_KEY` / `TMDB_ACCESS_TOKEN` | — | TMDB credentials; Bearer token preferred when both set |
| `BROWSER_HEADLESS` | `true` | Run the fallback scraper without a visible window |
| `BROWSER_TIMEOUT` | `45s` | Per-attempt timeout for browser-based resolution |
| `MAX_BROWSER_SESSIONS` | `3` | Cap on concurrent headless-Chrome sessions |
| `BROWSER_EXECUTABLE` | auto-detect | Path to a specific Chrome/Chromium binary |
| `CACHE_MAX_MB` | `256` | RAM cap for the manifest/segment body cache |
| `VIXSRC_ORIGIN` / `VIDKING_ORIGIN` / `VIDLOVE_ORIGIN` | provider URLs | Override media source origins |

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
internal/config/            config.conf parsing (+ env overrides), defaults
internal/catalog/           TMDB client, movie/TV mapping, cache stores
internal/server/            route table, method/CORS gates, gzip, handlers
internal/subtitles/         OpenSubtitles/Vidlove search clients, SRT→WebVTT converter
internal/mediaresolver/     the media pipeline:
    resolver.go               Resolve() orchestration, sessions, config
    vidking/vidlove/vidsrcme/vixsrc.go per-provider direct resolvers
    browser.go                headless-Chrome fallback scrape
    proxy.go                  the streaming reverse-proxy endpoint
    manifest.go               manifest rewriting + subtitle rendition embedding
    session.go / cache.go / warmer.go   proxy state, body LRU, read-ahead
static/js/                  vanilla ES modules: main.js (orchestrator),
                            storage.js (My List / progress), utils.js
```

## HTTP API

- `GET /api/home|movies|tvshows|popular` — gzipped catalog JSON
- `GET /api/search?q=&type=` · `/api/detail?type=&id=` · `/api/episodes?id=&season=`
- `GET /api/media/source/<provider>/movie/<tmdbId>` and `/tv/<id>/<s>/<e>`
  (`provider` ∈ vixsrc | vidking | vidlove | vidsrcme); legacy unprefixed routes map
  to VixSrc
- `GET /api/media/proxy/<token>.m3u8?url=...` — HLS proxy (supports Range)
- `POST /api/media/subs/<token>` — register extra subtitle renditions on a
  live session; the resolver embeds its own ladder automatically for
  vidking/vidlove/vidsrcme, this tops it up
- `GET /api/subtitles/vidking|vidlove|vidsrcme?type=&id=&season=&episode=` — search
- `GET /api/subtitles/opensubtitles/download?url=` ·
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
- **Port already in use** — the listen address is hard-coded to `:8080`.

## Bandwidth Testing

GoFlix includes a standalone bandwidth tester to evaluate its upstream media servers. You can run it with the standard Go test tool:

```bash
go test -v -run TestBandwidth ./internal/mediaresolver -timeout 20m
```

Optional environment variables for customizing the test:
- `BW_PROVIDERS` — test a comma-separated subset (e.g., `vixsrc,vidking,vidlove`)
- `BW_TYPE` — test a specific media type (`movie` or `tv`)
- `BW_ID` — test a specific TMDB ID (default is 27205 for Inception)
- `BW_SEASON` and `BW_EPISODE` — test specific TV show episodes

Example testing a specific TV show episode on a subset of providers:

```bash
BW_PROVIDERS=vidking,vidlove BW_TYPE=tv BW_ID=1399 BW_SEASON=1 BW_EPISODE=1 go test -v -run TestBandwidth ./internal/mediaresolver -timeout 20m
```

> **Note:** Go caches successful test results. If your test finishes instantly with `(cached)`, add the `-count=1` flag to force a fresh run (e.g., `go test -count=1 -v ...`), or run `go clean -testcache` to clear the cache globally.

## Notes

- The frontend has no build step: edit the ES modules under `static/js/`
  directly. hls.js loads from a CDN (pinned 1.7.0), so playback clients need
  internet access to it.
- Go's standard library only, plus `chromedp/chromedp` for the fallback
  scraper.

> **Security:** if your `config.conf` holds real TMDB credentials, treat them
> like passwords — don't share the file. Rotate any key that has ever been
> exposed.
