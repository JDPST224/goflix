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
}

func New(d *Deps) http.Handler {
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
	// routes below for backwards compatibility.
	for _, provider := range []string{"cinesrc", "vixsrc", "vidking", "vidlove", "vidsrcme"} {
		moviePrefix := "/api/media/source/" + provider + "/movie/"
		tvPrefix := "/api/media/source/" + provider + "/tv/"
		mux.HandleFunc(moviePrefix, d.makeMovieSourceHandler(moviePrefix, provider))
		mux.HandleFunc(tvPrefix, d.makeTVSourceHandler(tvPrefix, provider))
	}
	mux.HandleFunc("/api/media/source/movie/", d.makeMovieSourceHandler("/api/media/source/movie/", "vixsrc"))
	mux.HandleFunc("/api/media/source/tv/", d.makeTVSourceHandler("/api/media/source/tv/", "vixsrc"))
	// Direct embed routes: allows directly resolving https://cinesrc.st/embed/movie/{id}
	// via http://<host>/embed/movie/{id} or /embed/tv/{id}.
	mux.HandleFunc("/embed/movie/", d.embedMovieDirectHandler)
	mux.HandleFunc("/embed/tv/", d.embedTVDirectHandler)
	mux.HandleFunc("/api/media/proxy/", d.mediaProxyHandler)
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
	mux.Handle("/", staticFileServer("./static"))
	return mux
}
