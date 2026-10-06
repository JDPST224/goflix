package server

// Regression: a Range request for a compressible asset must not be gzipped.
// Content-Range offsets refer to the uncompressed bytes, so a gzipped 206
// body is undecodable — media players issuing byte-range probes for
// .m3u8/.vtt assets would get corrupt data.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticRangeRequestNotGzipped(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("console.log('x');\n", 20) // 360 bytes of .js
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte(body), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	h := staticFileServer(dir)

	// Control: a plain request IS gzipped when the client accepts it.
	plain := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	plain.Header.Set("Accept-Encoding", "gzip")
	plainRec := httptest.NewRecorder()
	h.ServeHTTP(plainRec, plain)
	if ce := plainRec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("plain request Content-Encoding = %q, want gzip", ce)
	}

	// A range probe must come back as a plain 206 with exact bytes.
	ranged := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	ranged.Header.Set("Accept-Encoding", "gzip")
	ranged.Header.Set("Range", "bytes=0-9")
	rangeRec := httptest.NewRecorder()
	h.ServeHTTP(rangeRec, ranged)
	if rangeRec.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", rangeRec.Code)
	}
	if ce := rangeRec.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("range response gzipped: Content-Encoding = %q", ce)
	}
	if got := rangeRec.Body.String(); got != body[:10] {
		t.Fatalf("range body = %q, want %q", got, body[:10])
	}
}
