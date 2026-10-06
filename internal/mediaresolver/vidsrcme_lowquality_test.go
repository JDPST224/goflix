package mediaresolver

// Low-quality release flagging: vidsrcme's data API reports the exact release
// each title's HLS was cut from. YIFY/YTS rips ship crushed ~128 kbps stereo
// audio that downstream tier selection cannot undo, so they are flagged and
// rerouted to a provider carrying a proper encode.

import (
	"errors"
	"testing"
)

func TestIsLowQualityRelease(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Inception (2010) [1080p]/Inception.2010.1080p.BrRip.x264.YIFY.mp4", true},
		{"Rewind.2024.1080p.WEB-DL.YTS.MX.mkv", true},
		{"The.Matrix.1999.1080p.BluRay.YIFY.mp4", true},
		{"Greek.2010.1080p.BluRay.yify.h264.mkv", true},
		{"Breaking.Bad.S01E01.Pilot.1080p.WEB.DL.DD5.1.H.264.mkv", false},
		{"Movie.2022.1080p.WEBRip.x264-RARBG.mp4", false},
		{"Show (2019) [1080p]/Show.2019.1080p.BrRip.x264.mp4", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isLowQualityRelease(tc.name); got != tc.want {
			t.Errorf("isLowQualityRelease(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestReleaseQualityErr verifies the sentinel wires through errors.Is and
// that the logged label drops the catalog folder path.
func TestReleaseQualityErr(t *testing.T) {
	shelved := "Inception (2010) [1080p]/Inception.2010.1080p.BrRip.x264.YIFY.mp4"
	err := (&Resolver{}).releaseQualityErr(shelved)
	if err == nil || !errors.Is(err, errLowQualityRelease) {
		t.Fatalf("releaseQualityErr(%q) = %v, want an errLowQualityRelease wrap", shelved, err)
	}
	if got := err.Error(); got != "low-quality release: Inception.2010.1080p.BrRip.x264.YIFY.mp4" {
		t.Fatalf("label = %q, want the bare release file name", got)
	}
	if err := (&Resolver{}).releaseQualityErr("Breaking.Bad.S01E01.Pilot.1080p.WEB.DL.DD5.1.H.264.mkv"); err != nil {
		t.Fatalf("proper WEB-DL release must not be flagged, got %v", err)
	}
}
