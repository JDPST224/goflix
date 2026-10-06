package debug

import (
	"net/url"
	"strings"
	"testing"

	"goflix/internal/mediaresolver"
)

func TestForceHighestQuality(t *testing.T) {
	manifest := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
360p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080
1080p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720
720p.m3u8
`
	reordered := mediaresolver.ForceHighestQuality(manifest)
	lines := strings.Split(strings.TrimSpace(reordered), "\n")

	var firstVariantURI string
	for i, l := range lines {
		if strings.HasPrefix(l, "#EXT-X-STREAM-INF") && i+1 < len(lines) {
			firstVariantURI = lines[i+1]
			break
		}
	}
	if firstVariantURI != "1080p.m3u8" {
		t.Errorf("expected top variant to be 1080p.m3u8, got %q in:\n%s", firstVariantURI, reordered)
	}
}

func TestInjectSubRenditions(t *testing.T) {
	master := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=2000000
stream.m3u8
`
	subs := []mediaresolver.SubRendition{
		{Label: "English", Language: "en", URI: "/api/subtitles/wrap.m3u8?token=xyz&lang=en"},
		{Label: "Spanish", Language: "es", URI: "/api/subtitles/wrap.m3u8?token=xyz&lang=es"},
	}

	injected := mediaresolver.InjectSubRenditions(master, subs)

	if !strings.Contains(injected, `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="goflix-ext",NAME="English",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO,LANGUAGE="en",URI="/api/subtitles/wrap.m3u8?token=xyz&lang=en"`) {
		t.Errorf("missing English subtitle rendition line in:\n%s", injected)
	}
	if !strings.Contains(injected, `SUBTITLES="goflix-ext"`) {
		t.Errorf("variant should reference goflix-ext subtitles group in:\n%s", injected)
	}
}

func TestRewriteManifest_RewritesRelativeURIs(t *testing.T) {
	baseURL, _ := url.Parse("https://cdn.example.com/hls/master.m3u8")
	token := "testtoken123"

	master := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=2000000
720p/index.m3u8
`
	r := &mediaresolver.Resolver{}
	rewritten, discovered := r.RewriteManifest(master, baseURL, token, nil)
	if !strings.Contains(rewritten, "/api/media/proxy/testtoken123.m3u8?url=") {
		t.Errorf("relative variant playlist not proxied via /api/media/proxy:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "https%3A%2F%2Fcdn.example.com%2Fhls%2F720p%2Findex.m3u8") {
		t.Errorf("variant URI was not correctly resolved against base URL:\n%s", rewritten)
	}
	if !discovered["cdn.example.com"] {
		t.Errorf("expected cdn.example.com to be added to discovered upstream hosts, got: %v", discovered)
	}
}
