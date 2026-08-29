package mediaresolver

// Embedded-JS cinesrc resolution: runs the site's own challenge scripts in a
// goja runtime with Go-native crypto/fetch/worker shims and the PoW wasm via
// wazero (see internal/mediaresolver/cinesrcjs). No browser is involved, so
// this path is fast and does not consume a browser session slot. When the
// embedded engine fails — script names moved, challenge scheme changed — the
// browser-based worker in cinesrc.go remains the fallback.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"goflix/internal/mediaresolver/cinesrcjs"
)

var (
	cinesrcJSMu       sync.Mutex
	cinesrcJSResolver *cinesrcjs.Resolver
	cinesrcJSSem      = make(chan struct{}, 2) // bound concurrent challenge sessions per process
)

// cinesrcJSEngine lazily creates the shared embedded engine. The engine
// caches the challenge scripts and the compiled PoW wasm across resolves.
func (r *Resolver) cinesrcJSEngine() *cinesrcjs.Resolver {
	cinesrcJSMu.Lock()
	defer cinesrcJSMu.Unlock()
	if cinesrcJSResolver == nil {
		cinesrcJSResolver = &cinesrcjs.Resolver{
			Origin:      r.cfg.CineSrcOrigin,
			UserAgent:   defaultUserAgent,
			Transport:   r.transport,
			Fingerprint: cinesrcjs.DefaultFingerprint(),
			Logf: func(format string, args ...any) {
				log.Printf("[MediaResolver] cinesrc embedded: "+format, args...)
			},
		}
	}
	return cinesrcJSResolver
}

// cinesrcJSResolve runs the embedded engine and validates the resulting
// master playlist exactly like the browser path does.
func (r *Resolver) cinesrcJSResolve(ctx context.Context, req MediaRequest) (*directResolution, error) {
	mediaType := "movie"
	if req.Type == TV {
		mediaType = "tv"
	}
	// Bound concurrent challenge sessions: rapid parallel challenges from
	// one IP look bot-like to the upstream, and the browser worker this
	// replaces was effectively serialized too.
	select {
	case cinesrcJSSem <- struct{}{}:
		defer func() { <-cinesrcJSSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	res, err := r.cinesrcJSEngine().Resolve(ctx, mediaType, req.ID, strings.TrimSpace(req.Season), strings.TrimSpace(req.Episode), nil)
	if err != nil {
		return nil, err
	}

	origin := r.cfg.CineSrcOrigin
	if origin == "" {
		origin = "https://cinesrc.st"
	}
	origin = strings.TrimRight(origin, "/")

	u, err := url.Parse(res.Source)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("embedded engine returned an invalid URL")
	}
	if r.blockedUpstreamHost(ctx, u.Hostname()) {
		return nil, fmt.Errorf("embedded engine HLS host %q blocked", u.Hostname())
	}

	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	headers.Set("Referer", origin+"/")
	headers.Set("Origin", origin)

	client := &http.Client{Transport: r.transport, Timeout: 12 * time.Second}
	masterText, err := r.fetchManifestText(ctx, client, res.Source, headers)
	if err != nil {
		return nil, fmt.Errorf("embedded engine master playlist validation: %w", err)
	}
	if !strings.Contains(masterText, "#EXT-X-STREAM-INF") && !strings.Contains(masterText, "#EXTINF") {
		return nil, errors.New("embedded engine master playlist has no variants or segments")
	}

	allowed := map[string]bool{strings.ToLower(u.Host): true}
	if ru, err := url.Parse(origin + "/"); err == nil && ru.Host != "" {
		allowed[strings.ToLower(ru.Host)] = true
	}

	return &directResolution{
		Source:     res.Source,
		Headers:    headers,
		Allowed:    allowed,
		MasterText: masterText,
	}, nil
}
