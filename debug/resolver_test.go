package debug

import (
	"net"
	"testing"

	"goflix/internal/mediaresolver"
)

func TestParseByteRange(t *testing.T) {
	cases := []struct {
		spec   string
		size   int64
		wantS  int64
		wantE  int64
		wantOk bool
	}{
		{"bytes=0-499", 1000, 0, 499, true},
		{"bytes=500-999", 1000, 500, 999, true},
		{"bytes=-500", 1000, 500, 999, true},
		{"bytes=9500-", 10000, 9500, 9999, true},
		{"bytes=0-0", 1000, 0, 0, true},
		{"bytes=500-1500", 1000, 500, 999, true}, // clamped to size-1
		{"bytes=1000-", 1000, 0, 0, false},       // start >= size
		{"bytes=-0", 1000, 0, 0, false},          // invalid suffix
		{"bytes=500-400", 1000, 0, 0, false},     // start > end
		{"bytes=0-100,200-300", 1000, 0, 0, false}, // multi-range unsupported
		{"invalid", 1000, 0, 0, false},
		{"bytes=0-100", 0, 0, 0, false}, // size <= 0
	}

	for _, tc := range cases {
		s, e, ok := mediaresolver.ParseByteRange(tc.spec, tc.size)
		if ok != tc.wantOk {
			t.Errorf("ParseByteRange(%q, %d) ok=%v, want %v", tc.spec, tc.size, ok, tc.wantOk)
		}
		if ok && (s != tc.wantS || e != tc.wantE) {
			t.Errorf("ParseByteRange(%q, %d) = (%d, %d), want (%d, %d)", tc.spec, tc.size, s, e, tc.wantS, tc.wantE)
		}
	}
}

func TestValidNumeric(t *testing.T) {
	valid := []string{"1", "12345", "0"}
	for _, s := range valid {
		if !mediaresolver.ValidNumeric(s) {
			t.Errorf("expected %q to be valid numeric", s)
		}
	}

	invalid := []string{"", "abc", "-1", "12a", " ", "123456789012345678901"}
	for _, s := range invalid {
		if mediaresolver.ValidNumeric(s) {
			t.Errorf("expected %q to be invalid numeric", s)
		}
	}
}

func TestCandidateScore(t *testing.T) {
	tests := []struct {
		url      string
		expected int
	}{
		{"https://cdn.example.com/playlist/master.m3u8", 100},
		{"https://cdn.example.com/stream/master.m3u8", 95},
		{"https://cdn.example.com/stream/manifest.m3u8", 90},
		{"https://cdn.example.com/video/master_720.m3u8", 85},
		{"https://cdn.example.com/index.m3u8", 80},
		{"https://cdn.example.com/file.m3u8?token=abc", 70},
		{"https://cdn.example.com/file_other.m3u8", 70},
	}

	for _, tc := range tests {
		score := mediaresolver.CandidateScore(tc.url, "")
		if score != tc.expected {
			t.Errorf("CandidateScore(%q) = %d, want %d", tc.url, score, tc.expected)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"::1",
		"10.0.0.1",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.169.254", // link-local / cloud metadata
		"224.0.0.1",       // multicast
		"0.0.0.0",         // unspecified
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if !mediaresolver.IsBlockedIP(ip) {
			t.Errorf("IsBlockedIP(%s) = false, want true", s)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"142.250.190.46",
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if mediaresolver.IsBlockedIP(ip) {
			t.Errorf("IsBlockedIP(%s) = true, want false", s)
		}
	}
}

func TestSanitizeManifestAttr(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`English "CC"`, "English CC"},
		{"English, Spanish", "English Spanish"},
		{"Title\nWith\r\nBreaks", "TitleWithBreaks"},
		{"Normal Label", "Normal Label"},
	}
	for _, tc := range cases {
		got := mediaresolver.SanitizeManifestAttr(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeManifestAttr(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRedactQuery(t *testing.T) {
	urlWithQuery := "https://example.com/path?secret_token=12345&foo=bar#frag"
	redacted := mediaresolver.RedactQuery(urlWithQuery)
	if redacted != "https://example.com/path" {
		t.Errorf("RedactQuery(%q) = %q, want %q", urlWithQuery, redacted, "https://example.com/path")
	}
}
