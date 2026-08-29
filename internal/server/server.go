package server

import (
	"net/http"
	"time"

	"goflix/internal/catalog"
	"goflix/internal/mediaresolver"
)

// Deps carries every collaborator the routes need. It is passed by pointer
// so all handler methods share the same state with clear reference semantics.
type Deps struct {
	Resolver  *mediaresolver.Resolver
	Store     *catalog.Store
	Client    *catalog.Client
	// StartedAt anchors the uptime reported by /api/health.
	StartedAt time.Time
	// Auth is the account store + gate; always present.
	Auth *authStore
	// UserData is the cross-device sync store; nil disables persistence but
	// the endpoints still answer with the client's own state.
	UserData *userDataStore
	// DebugProfiling mounts the pprof endpoints under /debug/pprof/ (auth
	// gated like everything else).
	DebugProfiling bool
	// AuthRatePerMin caps login/register attempts per client IP (default 10).
	AuthRatePerMin int
	// ResolveRatePerMin caps media-source resolutions per client IP
	// (default 10) — each resolve runs a costly 5-25s chain.
	ResolveRatePerMin int

	authLimiter    *ipLimiter
	resolveLimiter *ipLimiter
	// SecureCookies marks session cookies Secure (when the server runs TLS).
	SecureCookies bool
	// ImagesDir is the disk cache for the /api/img image proxy; "-" disables
	// the disk cache (images redirect to the CDN instead).
	ImagesDir string
	// MaxStreamHeight caps playback resolution via hls.js level capping
	// (0 = uncapped).
	MaxStreamHeight int
}

func New(d *Deps) http.Handler {
	// Normalize an absent gate to an empty in-memory store so handlers can
	// rely on d.Auth being non-nil.
	if d.Auth == nil {
		d.Auth = NewAuthStore("", "")
	}
	if d.UserData == nil {
		d.UserData = NewUserDataStore("")
	}
	if d.AuthRatePerMin <= 0 {
		d.AuthRatePerMin = 10
	}
	if d.ResolveRatePerMin <= 0 {
		d.ResolveRatePerMin = 10
	}
	d.authLimiter = newIPLimiter(d.AuthRatePerMin, time.Minute)
	d.resolveLimiter = newIPLimiter(d.ResolveRatePerMin, time.Minute)
	if d.ImagesDir == "" {
		d.ImagesDir = "images"
	} else if d.ImagesDir == "-" {
		d.ImagesDir = ""
	}
	mux := http.NewServeMux()

	// Catalog JSON is large and repetitive — serve it gzipped. The media
	// proxy and subtitle streams are deliberately NOT wrapped: they stream
	// already-compressed or binary content.
	mux.HandleFunc("/api/home", GzipCatalog(d.homeHandler))
	mux.HandleFunc("/api/movies", GzipCatalog(d.moviesHandler))
	mux.HandleFunc("/api/tvshows", GzipCatalog(d.tvShowsHandler))
	mux.HandleFunc("/api/popular", GzipCatalog(d.popularHandler))
	mux.HandleFunc("/api/providers", GzipCatalog(d.providersHandler))
	mux.HandleFunc("/api/health", d.healthHandler)
	mux.HandleFunc("/api/search", d.searchHandler)
	mux.HandleFunc("/api/discover", GzipCatalog(d.discoverHandler))
	mux.HandleFunc("/api/detail", d.detailHandler)
	mux.HandleFunc("/api/episodes", d.episodesHandler)
	// Media source resolver routes. VixSrc also keeps its legacy unprefixed
	// routes below for backwards compatibility. Every resolve runs a costly
	// upstream chain, so they all sit behind the per-IP resolve limiter.
	for _, provider := range []string{"cinesrc", "vixsrc", "vidking", "vidlove", "vidsrcme"} {
		moviePrefix := "/api/media/source/" + provider + "/movie/"
		tvPrefix := "/api/media/source/" + provider + "/tv/"
		mux.HandleFunc(moviePrefix, d.resolveLimiter.guard(d.makeMovieSourceHandler(moviePrefix, provider)))
		mux.HandleFunc(tvPrefix, d.resolveLimiter.guard(d.makeTVSourceHandler(tvPrefix, provider)))
	}
	mux.HandleFunc("/api/media/source/movie/", d.resolveLimiter.guard(d.makeMovieSourceHandler("/api/media/source/movie/", "vixsrc")))
	mux.HandleFunc("/api/media/source/tv/", d.resolveLimiter.guard(d.makeTVSourceHandler("/api/media/source/tv/", "vixsrc")))
	// Direct embed routes: allows directly resolving https://cinesrc.st/embed/movie/{id}
	// via http://<host>/embed/movie/{id} or /embed/tv/{id}.
	mux.HandleFunc("/embed/movie/", d.resolveLimiter.guard(d.embedMovieDirectHandler))
	mux.HandleFunc("/embed/tv/", d.resolveLimiter.guard(d.embedTVDirectHandler))
	mux.HandleFunc("/api/media/proxy/", d.mediaProxyHandler)
	// Drop the remembered resolution behind a proxy session so the next
	// source request re-runs the full resolve chain (stale-link healing).
	mux.HandleFunc("/api/media/invalidate/", d.mediaInvalidateHandler)
	// External-subtitle rendition registration for native-HLS engines, plus
	// the single-segment wrap playlist those renditions point at.
	mux.HandleFunc("/api/media/subs/", d.subsRegisterHandler)
	mux.HandleFunc("/api/subtitles/wrap.m3u8", d.subsWrapHandler)
	mux.HandleFunc("/api/subtitles/wrap.vtt", d.subtitlesWrapVTTHandler)
	mux.HandleFunc("/api/subtitles/vidking", d.subtitlesVidkingHandler)
	mux.HandleFunc("/api/subtitles/vidlove", d.subtitlesVidloveHandler)
	mux.HandleFunc("/api/subtitles/vidsrcme", d.subtitlesVidsrcmeHandler)
	mux.HandleFunc("/api/subtitles/cinesrc", d.subtitlesCinesrcHandler)
	mux.HandleFunc("/api/subtitles/opensubtitles/download", d.subtitlesOpenSubtitlesDownloadHandler)
	mux.HandleFunc("/api/subtitles/vidlove/download", d.subtitlesVidloveDownloadHandler)
	mux.HandleFunc("/api/subtitles/vidsrcme/download", d.subtitlesVidsrcmeDownloadHandler)
	mux.HandleFunc("/api/subtitles/cinesrc/download", d.subtitlesCinesrcDownloadHandler)
	// Cross-device userdata sync (My List / progress / Continue Watching).
		mux.HandleFunc("/api/userdata", d.userdataGetHandler)
		mux.HandleFunc("/api/userdata/sync", d.userdataSyncHandler)
		mux.HandleFunc("/api/userdata/clear", d.userdataClearHandler)
	// Poster/backdrop image proxy with disk cache — catalog rows point at
	// this so clients fetch images LAN-fast instead of from the TMDB CDN.
	mux.HandleFunc("/api/img", d.imageHandler)
	// Auth gate endpoints (reachable without a session by design). Login
	// and registration are rate limited per IP to blunt brute force and
	// account spam.
	mux.HandleFunc("/api/auth/status", d.authStatusHandler)
	mux.HandleFunc("/api/auth/login", d.authLimiter.guard(d.authLoginHandler))
	mux.HandleFunc("/api/auth/register", d.authLimiter.guard(d.authRegisterHandler))
		mux.HandleFunc("/api/auth/logout", d.authLogoutHandler)
		mux.HandleFunc("/api/auth/password", d.authPasswordHandler)
		mux.HandleFunc("/api/auth/avatar", d.authAvatarHandler)
		mux.HandleFunc("/api/auth/delete", d.authDeleteHandler)
	// Account self-service page + admin account management.
		mux.HandleFunc("/account", d.accountPageHandler)
		mux.HandleFunc("/dashboard", d.dashboardPageHandler)
	mux.HandleFunc("/api/admin/users", d.adminUsersHandler)
	mux.HandleFunc("/api/admin/users/", d.adminUserHandler)
	// Login page: routed explicitly because /login has no extension for the
	// static file server to map to login.html.
	mux.HandleFunc("/login", d.loginPageHandler)
	if d.DebugProfiling {
		debugRoutes(mux)
	}
	mux.Handle("/", staticFileServer("./static"))

	// Order (outermost first): access log sees final status; security
	// headers stamp every response; the origin guard rejects cross-site
	// writes; the auth middleware attaches identity without blocking.
	var handler http.Handler = mux
	handler = d.Auth.middleware(handler)
	handler = sameOriginGuard(handler)
	handler = securityHeaders(handler)
	handler = logRequests(handler)
	return handler
}
