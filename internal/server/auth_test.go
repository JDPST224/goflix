package server

// White-box tests for the auth store internals (session expiry cleanup) that
// black-box tests in debug/ cannot reach.

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionForDropsExpiredSession(t *testing.T) {
	s := NewAuthStore(filepath.Join(t.TempDir(), "users.json"), "")
	s.mu.Lock()
	s.file.Users = append(s.file.Users, &user{ID: "u1", Username: "alice"})
	expired := &session{UserID: "u1", ExpiresAt: time.Now().Add(-time.Minute)}
	live := &session{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}
	s.file.Sessions[hashToken("tok-expired")] = expired
	s.file.Sessions[hashToken("tok-live")] = live
	s.mu.Unlock()

	if _, ok := s.sessionFor("tok-expired"); ok {
		t.Fatal("expired session still authorizes")
	}
	s.mu.Lock()
	n := len(s.file.Sessions)
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("expired session not deleted from the store: %d sessions remain", n)
	}
	if _, ok := s.sessionFor("tok-live"); !ok {
		t.Fatal("live session no longer authorizes")
	}
}
