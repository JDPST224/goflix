package mediaresolver

import (
	"path/filepath"
	"testing"
)

// Several provider CDNs store media playlists whose URIs name the tier
// (init-s2160p-v1-a1.mp4). These tests pin the exact parsing that feeds the
// dashboard's resolution column.
func TestHeightFromQualityToken(t *testing.T) {
	tierPlaylist := "#EXTM3U\n" +
		"#EXT-X-TARGETDURATION:6\n" +
		"#EXT-X-MAP:URI=\"https://darkgate.top/vd/QzFj/init-s2160p-v1-a1.mp4\"\n" +
		"#EXTINF:6.006,\n" +
		"https://darkgate.top/vd/QzFj/seg-1-v1-a1.mp4\n"
	if h := heightFromQualityToken(tierPlaylist); h != 2160 {
		t.Fatalf("tier playlist: got %d, want 2160", h)
	}
	src := "https://moon.peakstorm.top/vd/xx/index-s1080p-v1-a1.m3u8"
	if h := heightFromQualityToken(src); h != 1080 {
		t.Fatalf("source URL: got %d, want 1080", h)
	}
	// Bitrates and codec strings must never match.
	for _, bad := range []string{
		"https://cdn.example/chunklist_b2450000.m3u8",
		"codecs=avc1.640028,mp4a.40.2",
		"#EXT-X-TARGETDURATION:6",
		"https://cdn.example/master.m3u8",
	} {
		if h := heightFromQualityToken(bad); h != 0 {
			t.Fatalf("%q: got %d, want 0", bad, h)
		}
	}
}

func TestHighestVariantHeight(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720\n" +
		"720.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=8000000,RESOLUTION=3840x2160\n" +
		"2160.m3u8\n"
	if h := highestVariantHeight(master); h != 2160 {
		t.Fatalf("got %d, want 2160", h)
	}
	// Tag inference when RESOLUTION is omitted.
	if h := highestVariantHeight(master + "#EXT-X-STREAM-INF:BANDWIDTH=999\nUHD\nuhd.m3u8\n"); h != 2160 {
		t.Fatalf("inference: got %d, want 2160", h)
	}
	if h := highestVariantHeight("#EXTM3U\n#EXTINF:6,\nseg.ts\n"); h != 0 {
		t.Fatalf("media playlist: got %d, want 0", h)
	}
}

func TestNoteStreamHeightToken(t *testing.T) {
	r := &Resolver{sessions: make(map[string]*proxySession)}
	token, err := r.newSession("movish|movie|1|-|-",
		"https://moon.peakstorm.top/vd/x/index-s2160p-v1-a1.m3u8",
		nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// No tier, no usable manifest text — the source URL must carry it.
	r.noteStreamHeightToken(token, "", "")
	if h := int(r.sessions[token].maxHeight.Load()); h != 2160 {
		t.Fatalf("got %d, want 2160", h)
	}
	// Already stamped: a later call must not overwrite.
	r.noteStreamHeightToken(token, "#EXT-X-STREAM-INF:RESOLUTION=640x360\n360.m3u8\n", "")
	if h := int(r.sessions[token].maxHeight.Load()); h != 2160 {
		t.Fatalf("overwrite: got %d, want 2160", h)
	}

	// A master with RESOLUTION stores width too, so cinemascope encodes
	// (1920x800) can still be labeled 1080p by the dashboard.
	token2, err := r.newSession("vidsrcme|movie|2|-|-", "https://cdn.example/master.m3u8", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.noteStreamHeightToken(token2, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1920x800\nv.m3u8\n", "")
	if w := int(r.sessions[token2].maxWidth.Load()); w != 1920 {
		t.Fatalf("width: got %d, want 1920", w)
	}
	if h := int(r.sessions[token2].maxHeight.Load()); h != 800 {
		t.Fatalf("height: got %d, want 800", h)
	}
}

// TestViewerBandwidthHint pins the split the player's instant-start tier
// policy relies on: SessionBandwidthHint combines the server↔CDN sample with
// the viewer's served-throughput history, while ViewerBandwidthHint must
// exclude the server-side number so a viewer with no history reports 0 (and
// the player opens conservatively instead of trusting the server's uplink).
func TestViewerBandwidthHint(t *testing.T) {
	r := &Resolver{
		sessions:  make(map[string]*proxySession),
		bwBuckets: map[int64]*bwBucket{},
	}
	s := &proxySession{}
	s.bwHint.Store(12345) // 123.45 Mbps measured server↔CDN
	r.sessions["tok"] = s

	if got := r.ViewerBandwidthHint("10.0.0.9"); got != 0 {
		t.Fatalf("ViewerBandwidthHint without viewer traffic = %v, want 0", got)
	}
	if got := r.SessionBandwidthHint("tok", "10.0.0.9"); got != 123.45 {
		t.Fatalf("SessionBandwidthHint = %v, want the CDN rate 123.45", got)
	}

	// 5 MB served to the viewer inside the window → the viewer is the
	// bottleneck and both hints must reflect it.
	r.streamTouch(s, "10.0.0.9")
	r.recordTraffic(s, 5<<20)
	want := float64(5<<20) * 8 / (viewerBwWindowBuckets * bwBucketSeconds * 1e6)
	if got := r.ViewerBandwidthHint("10.0.0.9"); got != want {
		t.Fatalf("ViewerBandwidthHint = %v, want %v", got, want)
	}
	if got := r.SessionBandwidthHint("tok", "10.0.0.9"); got != want {
		t.Fatalf("SessionBandwidthHint = %v, want the viewer bottleneck %v", got, want)
	}
}

// TestBlockListPersistence pins the restart-survival contract: a block
// written by one resolver instance is restored by the next, and unblocking
// clears it from disk.
func TestBlockListPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked.json")
	r1, err := New(Config{MaxBrowserSessions: 1, BlockListPath: path})
	if err != nil {
		t.Fatal(err)
	}
	r1.BlockIP("203.0.113.9")
	r1.BlockIP("203.0.113.7")
	if got := r1.BlockedIPs(); len(got) != 2 {
		t.Fatalf("blocked: got %v, want 2 entries", got)
	}
	r1.UnblockIP("203.0.113.7")

	r2, err := New(Config{MaxBrowserSessions: 1, BlockListPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	got := r2.BlockedIPs()
	if len(got) != 1 || got[0] != "203.0.113.9" {
		t.Fatalf("restored: got %v, want [203.0.113.9]", got)
	}
	if !r2.ipBlocked("203.0.113.9") {
		t.Fatal("restored address is not enforced")
	}
	// Unblock everything: the file must disappear.
	r2.UnblockIP("203.0.113.9")
	if len(r2.BlockedIPs()) != 0 {
		t.Fatal("expected an empty block list")
	}
}
