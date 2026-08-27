package debug

import (
	"strings"
	"testing"

	"goflix/internal/subtitles"
)

func TestSrtToWebVTT_Basic(t *testing.T) {
	srt := `1
00:00:01,000 --> 00:00:04,000
Hello World!

2
00:00:05,500 --> 00:00:08,200
Second subtitle line.
`
	vtt := subtitles.SrtToWebVTT(srt)
	if !strings.HasPrefix(vtt, "WEBVTT") {
		t.Fatalf("expected WEBVTT header, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "00:00:01.000 --> 00:00:04.000") {
		t.Errorf("expected dot timestamp format, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "Hello World!") {
		t.Errorf("missing text in:\n%s", vtt)
	}
}

func TestSrtToWebVTT_BOMAndCRLF(t *testing.T) {
	srt := "\xef\xbb\xbf1\r\n00:00:01,234 --> 00:00:02,345\r\nWith BOM and CRLF\r\n"
	vtt := subtitles.SrtToWebVTT(srt)
	if strings.Contains(vtt, "\xef\xbb\xbf") {
		t.Errorf("expected BOM to be stripped, got:\n%s", vtt)
	}
	if strings.Contains(vtt, "\r") {
		t.Errorf("expected CRLF normalized to LF, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "00:00:01.234 --> 00:00:02.345") {
		t.Errorf("timestamp comma not converted to period:\n%s", vtt)
	}
}

func TestSrtToWebVTT_Deduplication(t *testing.T) {
	srt := `1
00:00:01,000 --> 00:00:04,000
Overlapping text

2
00:00:02,000 --> 00:00:05,000
Overlapping text
`
	vtt := subtitles.SrtToWebVTT(srt)
	count := strings.Count(vtt, "Overlapping text")
	if count != 1 {
		t.Errorf("expected duplicate overlapping cue to be dropped, found %d instances:\n%s", count, vtt)
	}
}

func TestSrtToWebVTT_AlreadyVTT(t *testing.T) {
	input := `WEBVTT

00:00:01.000 --> 00:00:04.000
Already WebVTT
`
	vtt := subtitles.SrtToWebVTT(input)
	if strings.Count(vtt, "WEBVTT") != 1 {
		t.Errorf("expected exactly one WEBVTT header, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "Already WebVTT") {
		t.Errorf("missing text in:\n%s", vtt)
	}
}

func TestParseCueTimestamp(t *testing.T) {
	cases := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"00:01:02.500", 62.5, true},
		{"00:01:02,500", 62.5, true},
		{"01:02.500", 62.5, true},
		{"", 0, false},
		{"invalid", 0, false},
	}
	for _, tc := range cases {
		got, ok := subtitles.ParseCueTimestamp(tc.input)
		if ok != tc.ok {
			t.Errorf("ParseCueTimestamp(%q) ok = %v, want %v", tc.input, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Errorf("ParseCueTimestamp(%q) = %f, want %f", tc.input, got, tc.want)
		}
	}
}
