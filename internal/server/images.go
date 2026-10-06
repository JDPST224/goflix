package server

// Image proxy with a disk cache. Catalog rows point their banner/thumbnail
// URLs at this endpoint, so every device pulls posters from this server
// (LAN-fast after the first fetch) instead of hammering the TMDB CDN over
// WAN. Cached files are immutable, so responses are served with a
// year-long immutable cache policy.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// imageHostAllowlist is the only upstream host catalog image URLs may point
// at. Anything else is refused — this endpoint must not become an open
// SSRF/relay.
const imageHostAllowlist = "image.tmdb.org"

// maxImageBytes caps a single cached image; the largest TMDB originals are
// a few MB.
const maxImageBytes = 10 << 20

// imageHTTPClient fetches upstream images. A timeout bounds hung fetches.
var imageHTTPClient = &http.Client{Timeout: 15 * time.Second}

// imageHandler serves GET /api/img?u=<encoded image URL>.
func (d *Deps) imageHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "GET", false) {
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("u"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "Missing image URL")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") ||
		!strings.EqualFold(u.Host, imageHostAllowlist) || !strings.HasPrefix(u.Path, "/t/p/") {
		writeError(w, http.StatusBadRequest, "Invalid image URL")
		return
	}

	// Disk cache disabled: redirect straight through to the CDN.
	if d.ImagesDir == "" {
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}

	local := d.cachedImage(r.Context(), u.String(), u.Path)
	if local == "" {
		writeError(w, http.StatusBadGateway, "Could not fetch image")
		return
	}
	// Immutable content — caches never need to revalidate.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, local)
}

// cachedImage returns the local path of the image, downloading it on first
// use. Concurrent first requests each download and race the rename; the
// file content is identical, so last writer wins harmlessly.
func (d *Deps) cachedImage(ctx context.Context, rawURL, urlPath string) string {
	ext := strings.ToLower(filepath.Ext(urlPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".svg" && ext != ".webp" {
		ext = ".jpg"
	}
	sum := sha256.Sum256([]byte(rawURL))
	name := hex.EncodeToString(sum[:16]) + ext
	final := filepath.Join(d.ImagesDir, name)
	if info, err := os.Stat(final); err == nil && info.Size() > 0 {
		return final
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "goflix-image-cache")
	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		log.Printf("[Images] fetch failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Images] upstream status %d for %s", resp.StatusCode, urlPath)
		return ""
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		log.Printf("[Images] upstream served non-image %q", ct)
		return ""
	}

	if err := os.MkdirAll(d.ImagesDir, 0o755); err != nil {
		log.Printf("[Images] mkdir failed: %v", err)
		return ""
	}
	tmp, err := os.CreateTemp(d.ImagesDir, "dl-*"+ext)
	if err != nil {
		log.Printf("[Images] temp file failed: %v", err)
		return ""
	}
	tmpPath := tmp.Name()
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxImageBytes+1))
	closeErr := tmp.Close()
	if err != nil || closeErr != nil || n == 0 || n > maxImageBytes {
		os.Remove(tmpPath)
		return ""
	}
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		log.Printf("[Images] rename failed: %v", err)
		return ""
	}
	return final
}
