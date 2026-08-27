package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goflix/internal/catalog"
	"goflix/internal/mediaresolver"
	"goflix/internal/server"
)

func TestSubtitlesSSRFProtection(t *testing.T) {
	client := catalog.NewClient("", "")
	store := catalog.NewStore(client)
	resolver, err := mediaresolver.New(mediaresolver.Config{MaxBrowserSessions: 1})
	if err != nil {
		t.Fatalf("mediaresolver.New error: %v", err)
	}
	defer resolver.Close()

	handler := server.New(&server.Deps{
		Resolver:  resolver,
		Store:     store,
		Client:    client,
		StartedAt: time.Now(),
	})

	blockedTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:8080/secret",
		"http://127.0.0.1:22",
		"https://evil-attacker.com/malicious.srt",
		"https://notopensubtitles.org/fake.srt",
	}

	endpoints := []string{
		"/api/subtitles/opensubtitles/download?url=",
		"/api/subtitles/vidlove/download?url=",
		"/api/subtitles/vidsrcme/download?url=",
	}

	for _, ep := range endpoints {
		for _, target := range blockedTargets {
			req := httptest.NewRequest(http.MethodGet, ep+target, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s%s returned status %d, want %d (SSRF blocked)", ep, target, rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), "Invalid subtitle URL") {
				t.Errorf("%s%s body = %q, expected 'Invalid subtitle URL'", ep, target, rec.Body.String())
			}
		}
	}
}

func TestHealthHandler(t *testing.T) {
	handler := server.New(&server.Deps{
		StartedAt: time.Now().Add(-10 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/health returned %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("GET /api/health body missing status ok: %s", rec.Body.String())
	}
}
