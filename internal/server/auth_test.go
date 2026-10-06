package server

// White-box tests for the auth store internals (session expiry cleanup) that
// black-box tests in debug/ cannot reach.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"goflix/internal/db"
)

func testAuthDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open("")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return d
}

func TestSessionForDropsExpiredSession(t *testing.T) {
	s := NewAuthStore(testAuthDB(t), "")
	u, msg := s.register("alice", "pass1234", "", "")
	if msg != "" {
		t.Fatalf("register failed: %s", msg)
	}
	expiredAt := time.Now().Add(-time.Minute).Unix()
	liveAt := time.Now().Add(time.Hour).Unix()
	s.mu.Lock()
	_, _ = s.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashToken("tok-expired"), u.ID, expiredAt)
	_, _ = s.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashToken("tok-live"), u.ID, liveAt)
	s.mu.Unlock()

	if _, ok := s.sessionFor("tok-expired"); ok {
		t.Fatal("expired session still authorizes")
	}
	s.mu.Lock()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("expired session not deleted from the store: %d sessions remain", n)
	}
	if _, ok := s.sessionFor("tok-live"); !ok {
		t.Fatal("live session no longer authorizes")
	}
}

// TestLoginDoesNotLeakOrphanSessions pins the fix where login() used to mint
// a session token that never reached the browser: the handler mints the real
// one, so an extra row in the database was pure bloat (and inflated the
// admin dashboard's session counts).
func TestLoginDoesNotLeakOrphanSessions(t *testing.T) {
	s := NewAuthStore(testAuthDB(t), "")
	if _, errMsg := s.register("alice", "pass1234", "", ""); errMsg != "" {
		t.Fatalf("register failed: %s", errMsg)
	}

	sess, errMsg := s.login("alice", "pass1234")
	if errMsg != "" {
		t.Fatalf("login failed: %s", errMsg)
	}

	s.mu.Lock()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("login() stored %d session row(s) without a token ever being issued", n)
	}

	// The returned record is inert until the handler stores it under a token.
	if sess == nil || sess.UserID == "" {
		t.Fatal("login returned no session record")
	}
}

// Regression: browsers POST canvas.toDataURL() output, which always carries
// the RFC 2397 "data:" scheme prefix ("data:image/jpeg;base64,...").
// validateAvatar used to compare the media type WITH the prefix against the
// allowlist, so every browser upload was rejected.
func TestValidateAvatarAcceptsBrowserDataURI(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})
	if msg := validateAvatar("data:image/jpeg;base64," + payload); msg != "" {
		t.Fatalf("browser jpeg data-URI rejected: %s", msg)
	}
	if msg := validateAvatar("data:image/png;base64," + payload); msg != "" {
		t.Fatalf("browser png data-URI rejected: %s", msg)
	}
	if msg := validateAvatar("data:image/webp;base64," + payload); msg != "" {
		t.Fatalf("browser webp data-URI rejected: %s", msg)
	}
	if msg := validateAvatar("data:image/gif;base64," + payload); msg == "" {
		t.Fatal("gif must stay rejected")
	}
	if msg := validateAvatar("image/png;base64," + payload); msg == "" {
		t.Fatal("non-data URI must stay rejected")
	}
}

// Regression: GET /api/auth/avatar for an account without a picture used to
// 500 ("Corrupt avatar") because the default-avatar data URI is URL-quoted
// SVG, not base64. It must serve the SVG directly.
func TestAvatarGetDefaultServesSVG(t *testing.T) {
	s := NewAuthStore(testAuthDB(t), "")
	u, msg := s.register("alice", "pass1234", "", "")
	if msg != "" {
		t.Fatalf("register failed: %s", msg)
	}
	dep := &Deps{Auth: s}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/avatar", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, u.ID))
	rec := httptest.NewRecorder()
	dep.authAvatarHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("default avatar status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("default avatar Content-Type = %q, want image/svg+xml", ct)
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("default avatar body is not SVG: %q", rec.Body.String())
	}
}

// Regression: after a browser upload the GET must answer with the raw bytes
// and a clean media type — not "data:image/jpeg" (an invalid Content-Type
// that browsers refuse to render).
func TestAvatarUploadThenGetRoundTrip(t *testing.T) {
	s := NewAuthStore(testAuthDB(t), "")
	u, msg := s.register("alice", "pass1234", "", "")
	if msg != "" {
		t.Fatalf("register failed: %s", msg)
	}
	dep := &Deps{Auth: s}
	raw := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG SOI marker
	uri := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)

	post := httptest.NewRequest(http.MethodPost, "/api/auth/avatar",
		strings.NewReader(`{"data":`+strconv.Quote(uri)+`}`))
	post = post.WithContext(context.WithValue(post.Context(), ctxUserID, u.ID))
	postRec := httptest.NewRecorder()
	dep.authAvatarHandler(postRec, post)
	if postRec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200: %s", postRec.Code, postRec.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/auth/avatar", nil)
	get = get.WithContext(context.WithValue(get.Context(), ctxUserID, u.ID))
	getRec := httptest.NewRecorder()
	dep.authAvatarHandler(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", ct)
	}
	if !bytes.Equal(getRec.Body.Bytes(), raw) {
		t.Fatalf("avatar bytes mismatch: %x", getRec.Body.Bytes())
	}
}
