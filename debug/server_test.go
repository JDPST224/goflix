package debug

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goflix/internal/catalog"
	"goflix/internal/mediaresolver"
	"goflix/internal/server"
)

// registerCookie creates a test account through the real endpoint and
// returns its session cookie, so tests can exercise gated handlers.
func registerCookie(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"username":"tester","password":"pass1234"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("test account registration returned %d: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Fatalf("registration did not set a session cookie")
	}
	return cookies[0]
}

// authedDeps builds standard test deps with account stores in a temp dir.
func authedDeps(t *testing.T) *server.Deps {
	t.Helper()
	dir := t.TempDir()
	return &server.Deps{
		Auth:     server.NewAuthStore(filepath.Join(dir, "users.json"), ""),
		UserData: server.NewUserDataStore(""),
	}
}

func TestSubtitlesSSRFProtection(t *testing.T) {
	client := catalog.NewClient("", "")
	store := catalog.NewStore(client)
	resolver, err := mediaresolver.New(mediaresolver.Config{MaxBrowserSessions: 1, BrowserHeadless: true})
	if err != nil {
		t.Fatalf("mediaresolver.New error: %v", err)
	}
	defer resolver.Close()

	deps := authedDeps(t)
	deps.Resolver = resolver
	deps.Store = store
	deps.Client = client
	deps.StartedAt = time.Now()
	handler := server.New(deps)
	cookie := registerCookie(t, handler)

	blockedTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:8080/secret",
		"http://127.0.0.1:22",
		"https://evil-attacker.com/malicious.srt",
		"https://notopensubtitles.org/fake.srt",
	}

	endpoints := []string{
		"/api/subtitles/opensubtitles/download?url=",
		"/api/subtitles/vidsrcme/download?url=",
		"/api/subtitles/cinesrc/download?url=",
	}

	for _, ep := range endpoints {
		for _, target := range blockedTargets {
			req := httptest.NewRequest(http.MethodGet, ep+target, nil)
			req.AddCookie(cookie)
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
	deps := authedDeps(t)
	deps.StartedAt = time.Now().Add(-10 * time.Minute)
	handler := server.New(deps)
	cookie := registerCookie(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/health returned %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("GET /api/health body missing status ok: %s", rec.Body.String())
	}
}

func TestCineSrcRoutesValidation(t *testing.T) {
	handler := server.New(authedDeps(t))
	cookie := registerCookie(t, handler)

	for _, tc := range []struct {
		url      string
		wantCode int
	}{
		{"/api/media/source/cinesrc/movie/abc", http.StatusBadRequest},
		{"/api/media/source/cinesrc/movie/", http.StatusBadRequest},
		{"/embed/movie/abc", http.StatusBadRequest},
		{"/embed/movie/", http.StatusBadRequest},
		{"/embed/tv/abc", http.StatusBadRequest},
		{"/embed/tv/550/abc/1", http.StatusBadRequest},
		{"/api/subtitles/cinesrc?type=movie&id=abc", http.StatusBadRequest},
		{"/api/subtitles/cinesrc?type=tv&id=550", http.StatusBadRequest},
		{"/api/subtitles/cinesrc/download", http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Errorf("GET %s returned code %d, want %d", tc.url, rec.Code, tc.wantCode)
		}
	}
}


