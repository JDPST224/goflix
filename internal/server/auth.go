package server

// Account authentication: username/password accounts with open or
// invite-gated registration and server-side sessions. Browsing and playback
// are open to everyone — an account adds cross-device persistence (the
// userdata sync), not access. Anonymous visitors keep everything in their
// browser's localStorage.
//
// AUTH_PASSWORD (when set) is the invite code required to register; the
// login page shows the field only when the status endpoint reports it.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	authCookieName = "goflix_session"
	// sessionTTL is the session lifetime. It slides in memory: every
	// authenticated request pushes the expiry forward. The persisted value
	// is refreshed on login/register/logout.
	sessionTTL = 30 * 24 * time.Hour
	// minPasswordLen keeps accidental "a" passwords out without nagging a
	// household app.
	minPasswordLen  = 4
	maxUsernameLen  = 32
	maxPasswordLen  = 128
	maxEmailLen     = 254
	maxSessionsKeep = 4096
	// maxAvatarBytes caps the decoded profile picture. The browser downsizes
	// uploads to 256×256 before they are sent, so honest avatars land well
	// under this; the cap only guards the users file from abuse.
	maxAvatarBytes = 300 * 1024
)

var avatarMediaTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// user is one registered account.
type user struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"` // optional; for future verification flows
	Avatar    string    `json:"avatar,omitempty"`
	Salt      string    `json:"salt"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	// IsAdmin marks account managers: the first account registered becomes
	// admin (it can list/delete accounts and force sign-outs).
	IsAdmin bool `json:"is_admin,omitempty"`
}

// session is one logged-in browser.
type session struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type usersFile struct {
	Version  int                 `json:"version"`
	Users    []*user             `json:"users"`
	Sessions map[string]*session `json:"sessions"`
}

// authStore owns accounts, sessions and the gate middleware.
type authStore struct {
	mu     sync.Mutex
	path   string
	invite string // AUTH_PASSWORD — required to register when non-empty
	secret []byte // reserved for future signed values
	file   usersFile
}

// NewAuthStore loads (or initializes) the account store. invite is the
// AUTH_PASSWORD registration code; empty means open registration.
func NewAuthStore(path, invite string) *authStore {
	s := &authStore{
		path:   path,
		invite: strings.TrimSpace(invite),
		file:   usersFile{Version: 1, Sessions: map[string]*session{}},
	}
	if path == "" || path == "-" {
		s.path = ""
	}
	if s.path != "" {
		if data, err := os.ReadFile(s.path); err == nil {
			var f usersFile
			if err := json.Unmarshal(data, &f); err != nil || f.Version != 1 {
				log.Printf("[Auth] discarding unreadable users file %s", s.path)
			} else {
				s.file = f
				if s.file.Sessions == nil {
					s.file.Sessions = map[string]*session{}
				}
				log.Printf("[Auth] restored %d account(s) from %s", len(f.Users), s.path)
			}
		}
	}
	if len(s.file.Users) == 0 {
		s.file.Version = 1
	}
	s.persistLocked()
	return s
}

// persistLocked writes the store atomically. Callers hold s.mu.
func (s *authStore) persistLocked() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(&s.file, "", "  ")
	if err != nil {
		log.Printf("[Auth] marshal failed: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("[Auth] write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		log.Printf("[Auth] rename failed: %v", err)
	}
}

// --- Credentials ---

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashToken hashes a session token for storage: the users file then holds no
// directly usable credentials — a leaked file cannot hijack live sessions.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// hashPassword produces the stored password hash. bcrypt is deliberately
// slow and salt-embedded, the correct choice for human passwords; the old
// salted-sha256 scheme is still accepted for legacy accounts and upgraded
// transparently on their next successful login (see verifyPassword).
func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// legacyHash is the pre-bcrypt scheme (salted, iterated sha256).
func legacyHash(password, salt string) string {
	h := []byte(salt + "|" + password)
	for i := 0; i < 4096; i++ {
		sum := sha256.Sum256(h)
		h = sum[:]
	}
	return hex.EncodeToString(h)
}

// verifyPassword checks a password against a stored hash. When the stored
// hash is legacy sha256 and the password is correct, it returns a fresh
// bcrypt replacement for the caller to persist.
func verifyPassword(password, salt, stored string) (ok bool, upgrade string) {
	if strings.HasPrefix(stored, "$2") {
		err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
		return err == nil, ""
	}
	if subtle.ConstantTimeCompare([]byte(legacyHash(password, salt)), []byte(stored)) == 1 {
		if h, err := hashPassword(password); err == nil {
			return true, h
		}
		return true, ""
	}
	return false, ""
}

func validUsername(u string) bool {
	if len(u) < 3 || len(u) > maxUsernameLen {
		return false
	}
	for _, r := range u {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// validEmail does a pragmatic syntax check. Email is optional metadata for
// future verification flows, so this only rejects obvious garbage rather
// than attempting full RFC compliance.
func validEmail(e string) bool {
	if len(e) < 3 || len(e) > maxEmailLen {
		return false
	}
	if strings.ContainsAny(e, " \t\r\n") {
		return false
	}
	local, domain, ok := strings.Cut(e, "@")
	if !ok || local == "" || domain == "" {
		return false
	}
	dot := strings.Index(domain, ".")
	return dot > 0 && dot < len(domain)-1
}

// --- Registration / login ---

func (s *authStore) register(username, password, invite, emailAddr string) (*user, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invite != "" && subtle.ConstantTimeCompare([]byte(invite), []byte(s.invite)) != 1 {
		return nil, "Invalid invite code"
	}
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return nil, "Username must be 3-32 characters (letters, digits, _ or -)"
	}
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return nil, "Password must be 4-128 characters"
	}
	// Email is optional. When given, normalize it and keep addresses unique
	// so a future verification flow can rely on one account per address.
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))
	if emailAddr != "" {
		if !validEmail(emailAddr) {
			return nil, "Enter a valid email address"
		}
		for _, u := range s.file.Users {
			if u.Email == emailAddr {
				return nil, "That email is already in use"
			}
		}
	}
	for _, u := range s.file.Users {
		if strings.EqualFold(u.Username, username) {
			return nil, "That username is taken"
		}
	}
	salt := randomHex(16)
	hash, err := hashPassword(password)
	if err != nil {
		return nil, "Could not hash password"
	}
	u := &user{
		ID:        randomHex(12),
		Username:  username,
		Email:     emailAddr,
		Salt:      salt,
		Hash:      hash,
		CreatedAt: time.Now(),
	}
	// The first account manages the instance.
	if len(s.file.Users) == 0 {
		u.IsAdmin = true
	}
	// Clean out expired sessions while we are writing anyway.
	s.pruneSessionsLocked(time.Now())
	s.file.Users = append(s.file.Users, u)
	s.persistLocked()
	log.Printf("[Auth] registered account %q", username)
	return u, ""
}

func (s *authStore) login(username, password string) (*session, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	username = strings.TrimSpace(username)
	for _, u := range s.file.Users {
		if strings.EqualFold(u.Username, username) {
			ok, upgrade := verifyPassword(password, u.Salt, u.Hash)
			if ok {
				if upgrade != "" {
					// Transparent legacy-hash → bcrypt upgrade.
					u.Hash = upgrade
					s.persistLocked()
				}
				sess := &session{UserID: u.ID, ExpiresAt: time.Now().Add(sessionTTL)}
				token := randomHex(32)
				s.file.Sessions[hashToken(token)] = sess
				s.pruneSessionsLocked(time.Now())
				s.persistLocked()
				return sess, ""
			}
			break
		}
	}
	return nil, "Wrong username or password"
}

// pruneSessionsLocked drops expired sessions and, when far over budget, the
// oldest extras. Callers hold s.mu.
func (s *authStore) pruneSessionsLocked(now time.Time) {
	for tok, sess := range s.file.Sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.file.Sessions, tok)
		}
	}
}

// sessionFor resolves a cookie token to a live session, sliding its expiry.
// Tokens are stored hashed, so the cookie value never appears on disk.
func (s *authStore) sessionFor(token string) (*session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.file.Sessions[hashToken(token)]
	if !ok {
		return nil, false
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) {
		delete(s.file.Sessions, token)
		s.persistLocked()
		return nil, false
	}
	// Sliding expiry: extend in memory; persisted on the next login/
	// register/logout write to keep request-path disk writes at zero.
	sess.ExpiresAt = now.Add(sessionTTL)
	return sess, true
}

func (s *authStore) logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.file.Sessions[hashToken(token)]; ok {
		delete(s.file.Sessions, hashToken(token))
		s.persistLocked()
	}
}

func (s *authStore) usernameByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID == id {
			return u.Username
		}
	}
	return ""
}

// emailByID returns the account's stored email ("" when unset or unknown).
func (s *authStore) emailByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID == id {
			return u.Email
		}
	}
	return ""
}

// avatarByID returns the account's stored avatar data-URI ("" when unset).
func (s *authStore) avatarByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID == id {
			return u.Avatar
		}
	}
	return ""
}

// setAvatar stores (dataURI != "") or removes (dataURI == "") the account's
// profile picture. Returns "" on success or a user-facing error message.
func (s *authStore) setAvatar(userID, dataURI string) string {
	if dataURI != "" {
		if err := validateAvatar(dataURI); err != "" {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID == userID {
			u.Avatar = dataURI
			s.persistLocked()
			return ""
		}
	}
	return "Account not found"
}

// validateAvatar checks a data-URI profile picture: supported type and a
// decoded size comfortably below the cap.
func validateAvatar(dataURI string) string {
	media, b64, ok := strings.Cut(dataURI, ";base64,")
	if !ok || !avatarMediaTypes[media] {
		return "Avatar must be a PNG, JPEG or WebP image"
	}
	if n := base64.StdEncoding.DecodedLen(len(b64)); n > maxAvatarBytes {
		return "Avatar is too large (max 300 KB)"
	}
	if _, err := base64.StdEncoding.DecodeString(b64); err != nil {
		return "Avatar data is corrupted"
	}
	return ""
}

// deleteAccountSelf removes the caller's own account after a password
// re-check. The last remaining admin cannot delete themselves (the instance
// would be unmanageable). Returns "" on success or a user-facing message.
func (s *authStore) deleteAccountSelf(userID, password string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID != userID {
			continue
		}
		if ok, _ := verifyPassword(password, u.Salt, u.Hash); !ok {
			return "Current password is wrong"
		}
		if u.IsAdmin {
			admins := 0
			for _, other := range s.file.Users {
				if other.IsAdmin {
					admins++
				}
			}
			if admins <= 1 {
				return "Cannot delete the last admin account"
			}
		}
		for i, other := range s.file.Users {
			if other.ID == userID {
				s.file.Users = append(s.file.Users[:i], s.file.Users[i+1:]...)
				break
			}
		}
		for tok, sess := range s.file.Sessions {
			if sess.UserID == userID {
				delete(s.file.Sessions, tok)
			}
		}
		s.persistLocked()
		return ""
	}
	return "Account not found"
}

// changePassword verifies the current password and replaces the hash.
// Returns "" on success or a user-facing error message.
func (s *authStore) changePassword(userID, current, next string) string {
	if len(next) < minPasswordLen || len(next) > maxPasswordLen {
		return "New password must be 4-128 characters"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID != userID {
			continue
		}
		ok, upgrade := verifyPassword(current, u.Salt, u.Hash)
		if !ok {
			return "Current password is wrong"
		}
		if upgrade != "" {
			u.Hash = upgrade // persist the legacy→bcrypt upgrade first
		}
		hash, err := hashPassword(next)
		if err != nil {
			return "Could not hash password"
		}
		u.Hash = hash
		s.persistLocked()
		return ""
	}
	return "Account not found"
}

// userRow is the admin-facing account summary.
type userRow struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	IsAdmin   bool      `json:"is_admin"`
	Sessions  int       `json:"sessions"`
	HasAvatar bool      `json:"has_avatar"`
}

func (s *authStore) listUsers() []userRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]userRow, 0, len(s.file.Users))
	for _, u := range s.file.Users {
		sessions := 0
		for _, sess := range s.file.Sessions {
			if sess.UserID == u.ID {
				sessions++
			}
		}
		out = append(out, userRow{
			ID: u.ID, Username: u.Username, Email: u.Email,
			CreatedAt: u.CreatedAt, IsAdmin: u.IsAdmin, Sessions: sessions,
			HasAvatar: u.Avatar != "",
		})
	}
	return out
}

func (s *authStore) userIsAdmin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.file.Users {
		if u.ID == id {
			return u.IsAdmin
		}
	}
	return false
}

// deleteAccount removes an account (and its sessions). Rules: you cannot
// delete yourself, and you cannot delete the last remaining admin.
// Returns "" on success or a user-facing error message.
func (s *authStore) deleteAccount(requesterID, targetID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requesterID == targetID {
		return "You cannot delete your own account here"
	}
	idx := -1
	var target *user
	for i, u := range s.file.Users {
		if u.ID == targetID {
			idx, target = i, u
			break
		}
	}
	if target == nil {
		return "Account not found"
	}
	if target.IsAdmin {
		admins := 0
		for _, u := range s.file.Users {
			if u.IsAdmin {
				admins++
			}
		}
		if admins <= 1 {
			return "Cannot delete the last admin account"
		}
	}
	s.file.Users = append(s.file.Users[:idx], s.file.Users[idx+1:]...)
	for tok, sess := range s.file.Sessions {
		if sess.UserID == targetID {
			delete(s.file.Sessions, tok)
		}
	}
	s.persistLocked()
	return ""
}

// logoutAllSessions destroys every session belonging to a user; returns the
// count dropped. Used by admins to force-sign-out a device.
func (s *authStore) logoutAllSessions(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for tok, sess := range s.file.Sessions {
		if sess.UserID == userID {
			delete(s.file.Sessions, tok)
			dropped++
		}
	}
	if dropped > 0 {
		s.persistLocked()
	}
	return dropped
}

// --- Middleware + context ---

type ctxKey int

const ctxUserID ctxKey = 1

// userIDFrom returns the authenticated user's id, or "" when anonymous.
func userIDFrom(r *http.Request) string {
	v, _ := r.Context().Value(ctxUserID).(string)
	return v
}

func (s *authStore) authorize(r *http.Request) (string, bool) {
	c, err := r.Cookie(authCookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	sess, ok := s.sessionFor(c.Value)
	if !ok {
		return "", false
	}
	return sess.UserID, true
}

// middleware attaches the caller's identity when a valid session cookie is
// present, but never blocks: the site is fully usable anonymously (browsing
// and playback), and only the userdata sync endpoints require an account.
// Data persistence — not access — is what accounts provide.
func (s *authStore) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := s.authorize(r)
		if ok {
			if s.usernameByID(userID) != "" {
				r = r.WithContext(context.WithValue(r.Context(), ctxUserID, userID))
			}
			// A session pointing at a deleted account is treated as
			// anonymous — same open access, no identity.
		}
		next.ServeHTTP(w, r)
	})
}

// --- Handlers ---

// setSessionCookie issues (maxAge > 0) or clears (maxAge < 0) the session
// cookie. Secure is on when the server runs TLS.
func (d *Deps) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   d.SecureCookies,
		MaxAge:   maxAge,
	})
}

// loginPageHandler serves the standalone sign-in page. Reachable with or
// without a session.
func (d *Deps) loginPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/login" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "./static/login.html")
}

// authRegisterHandler creates an account and signs the new user in.
func (d *Deps) authRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Invite   string `json:"invite"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	u, errMsg := d.Auth.register(body.Username, body.Password, body.Invite, body.Email)
	if u == nil {
		time.Sleep(500 * time.Millisecond)
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	token := randomHex(32)
	d.Auth.mu.Lock()
	d.Auth.file.Sessions[hashToken(token)] = &session{UserID: u.ID, ExpiresAt: time.Now().Add(sessionTTL)}
	d.Auth.persistLocked()
	d.Auth.mu.Unlock()
	d.setSessionCookie(w, token, int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "user": u.Username})
}

// authLoginHandler validates credentials and issues the session cookie.
// Failed attempts get a short delay to blunt brute-forcing. Without
// "remember" the cookie is a browser-session cookie (gone when the browser
// closes); with it, the session persists for the full sliding TTL.
func (d *Deps) authLoginHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	sess, errMsg := d.Auth.login(body.Username, body.Password)
	if sess == nil {
		time.Sleep(500 * time.Millisecond)
		log.Printf("[Auth] failed login for %q from %s", body.Username, r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, errMsg)
		return
	}
	token := randomHex(32)
	d.Auth.mu.Lock()
	d.Auth.file.Sessions[hashToken(token)] = sess
	d.Auth.persistLocked()
	d.Auth.mu.Unlock()
	maxAge := int(sessionTTL.Seconds())
	if !body.Remember {
		maxAge = 0 // no Max-Age attribute → browser-session cookie
	}
	d.setSessionCookie(w, token, maxAge)
	log.Printf("[Auth] login %q from %s", body.Username, r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "user": body.Username})
}

// authLogoutHandler destroys the session server-side and clears the cookie.
func (d *Deps) authLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	if c, err := r.Cookie(authCookieName); err == nil {
		d.Auth.logout(c.Value)
	}
	d.setSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// authPasswordHandler lets the signed-in user change their own password.
func (d *Deps) authPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Sign in first")
		return
	}
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if msg := d.Auth.changePassword(userID, body.Current, body.Next); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// adminUsersHandler lists accounts (admins only).
func (d *Deps) adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" || !d.Auth.userIsAdmin(userID) {
		writeError(w, http.StatusForbidden, "Admins only")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "users": d.Auth.listUsers()})
}

// adminUserHandler dispatches /api/admin/users/{id} (DELETE = delete
// account), /api/admin/users/{id}/logout (POST = force sign-out) and
// /api/admin/users/{id}/avatar (GET = the account's profile picture).
func (d *Deps) adminUserHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/")
	if strings.HasSuffix(rest, "/avatar") && r.Method == http.MethodGet {
		d.adminUserAvatar(w, r, strings.TrimSuffix(rest, "/avatar"))
		return
	}
	if strings.HasSuffix(rest, "/logout") && r.Method == http.MethodPost {
		d.adminUserLogout(w, r, strings.TrimSuffix(rest, "/logout"))
		return
	}
	if r.Method == http.MethodDelete {
		d.adminUserDelete(w, r, rest)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

// adminUserAvatar serves another account's profile picture (admins only).
func (d *Deps) adminUserAvatar(w http.ResponseWriter, r *http.Request, targetID string) {
	if !jsonGate(w, r) {
		return
	}
	if _, ok := d.adminGuard(w, r); !ok {
		return
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "Missing account id")
		return
	}
	dataURI := d.Auth.avatarByID(targetID)
	if dataURI == "" {
		writeError(w, http.StatusNotFound, "No avatar")
		return
	}
	media, b64, ok := strings.Cut(dataURI, ";base64,")
	if !ok {
		writeError(w, http.StatusInternalServerError, "Corrupt avatar")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Corrupt avatar")
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(raw)
}

func (d *Deps) adminGuard(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID := userIDFrom(r)
	if userID == "" || !d.Auth.userIsAdmin(userID) {
		writeError(w, http.StatusForbidden, "Admins only")
		return "", false
	}
	return userID, true
}

func (d *Deps) adminUserLogout(w http.ResponseWriter, r *http.Request, targetID string) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	if _, ok := d.adminGuard(w, r); !ok {
		return
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "Missing account id")
		return
	}
	d.Auth.logoutAllSessions(targetID)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (d *Deps) adminUserDelete(w http.ResponseWriter, r *http.Request, targetID string) {
	if !corsGate(w, r, "DELETE", false) {
		return
	}
	requesterID, ok := d.adminGuard(w, r)
	if !ok {
		return
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "Missing account id")
		return
	}
	if msg := d.Auth.deleteAccount(requesterID, targetID); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if d.UserData != nil {
		d.UserData.DeleteUserState(targetID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// accountPageHandler serves the self-service account page.
func (d *Deps) accountPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/account" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "./static/account.html")
}

// authAvatarHandler serves (GET) or updates (POST) the signed-in user's
// profile picture. GET answers with the raw image bytes, falling back to the
// default red "G" SVG the navbar uses for anonymous users. POST accepts a
// JSON data-URI; an empty string removes the picture.
func (d *Deps) authAvatarHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	if userID == "" {
		if !jsonGate(w, r) {
			return
		}
		writeError(w, http.StatusUnauthorized, "Sign in first")
		return
	}
	switch r.Method {
	case http.MethodGet:
		dataURI := d.Auth.avatarByID(userID)
		if dataURI == "" {
			// Same default avatar the navbar ships with.
			dataURI = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='12' fill='%23e50914'/%3E%3Ctext x='32' y='45' font-family='Arial, sans-serif' font-size='38' font-weight='900' fill='%23ffffff' text-anchor='middle'%3EG%3C/text%3E%3C/svg%3E"
		}
		media, b64, ok := strings.Cut(dataURI, ";base64,")
		if !ok {
			writeError(w, http.StatusInternalServerError, "Corrupt avatar")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Corrupt avatar")
			return
		}
		w.Header().Set("Content-Type", media)
		w.Header().Set("Cache-Control", "private, max-age=300")
		_, _ = w.Write(raw)
	case http.MethodPost:
		if !corsGate(w, r, "POST", false) {
			return
		}
		var body struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if msg := d.Auth.setAvatar(userID, strings.TrimSpace(body.Data)); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// authDeleteHandler closes the caller's own account after a password
// re-check: the account, its sessions and its synced userdata are removed.
func (d *Deps) authDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if !corsGate(w, r, "POST", false) {
		return
	}
	userID := userIDFrom(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Sign in first")
		return
	}
	var body struct {
		Current string `json:"current"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if msg := d.Auth.deleteAccountSelf(userID, body.Current); msg != "" {
		time.Sleep(300 * time.Millisecond)
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if d.UserData != nil {
		d.UserData.DeleteUserState(userID)
	}
	d.setSessionCookie(w, "", -1)
	log.Printf("[Auth] account %s self-deleted", userID)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// dashboardPageHandler serves the admin-only dashboard. Non-admins are
// redirected: anonymous → /login, signed-in users → the homepage.
func (d *Deps) dashboardPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dashboard" {
		http.NotFound(w, r)
		return
	}
	userID, authed := d.Auth.authorize(r)
	if !authed {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !d.Auth.userIsAdmin(userID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "./static/dashboard.html")
}

// authStatusHandler tells the frontend whether this browser is signed in,
// as whom, and whether registration needs the invite code.
func (d *Deps) authStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !jsonGate(w, r) {
		return
	}
	userID, authed := d.Auth.authorize(r)
	name := ""
	if authed {
		name = d.Auth.usernameByID(userID)
		if name == "" {
			authed = false
		}
	}
	resp := map[string]any{
		"authed":         authed,
		"user":           name,
		"isAdmin":        authed && d.Auth.userIsAdmin(userID),
		"inviteRequired": d.Auth.invite != "",
	}
	if authed {
		// Exposed for future verification flows; empty when unset.
		resp["email"] = d.Auth.emailByID(userID)
		resp["hasAvatar"] = d.Auth.avatarByID(userID) != ""
	}
	writeJSON(w, http.StatusOK, resp)
}
