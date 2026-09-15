package server

// White-box tests for the subtitle download pipeline internals that the
// black-box suites in debug/ cannot reach (they cannot point the
// domain-allowlisted fetch at a local listener).

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// TestDecompressSubtitleBodyBounded: a gzip stream that decompresses past
// the subtitle byte cap must be rejected instead of buffered whole — a small
// compressed payload can legally expand to gigabytes (gzip bomb).
func TestDecompressSubtitleBodyBounded(t *testing.T) {
	bomb := strings.Repeat("A", maxSubtitleBytes*4)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(bomb)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if buf.Len() > maxSubtitleBytes {
		t.Skipf("compressed fixture itself exceeds the cap (%d bytes)", buf.Len())
	}

	out, status, msg := decompressSubtitleBody(buf.Bytes(), "Test")
	if status == 0 {
		t.Fatalf("oversized decompressed body accepted (len=%d)", len(out))
	}
	if msg != "Subtitle file too large" {
		t.Errorf("msg = %q, want Subtitle file too large", msg)
	}
}

// TestDecompressSubtitleBodyOK: a normally-sized gzip payload decompresses
// unchanged.
func TestDecompressSubtitleBodyOK(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	out, status, msg := decompressSubtitleBody(buf.Bytes(), "Test")
	if status != 0 {
		t.Fatalf("decompression rejected: %d %s", status, msg)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("decompressed payload lost content: %q", out)
	}
}
