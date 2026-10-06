package server

// Account authentication backed by SQLite: username/password accounts with
// open or invite-gated registration and server-side sessions. Browsing and
// playback are open to everyone — an account adds cross-device persistence
// (the userdata sync), not access. Anonymous visitors keep everything in
// their browser's localStorage.
//
// AUTH_PASSWORD (when set) is the invite code required to register; the
// login page shows the field only when the status endpoint reports it.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
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
	minPasswordLen = 4
	maxUsernameLen = 32
	maxPasswordLen = 128
	maxEmailLen    = 254
	// maxAvatarBytes caps the decoded profile picture. The browser downsizes
	// uploads to 256×256 before they are sent, so honest avatars land well
	// under this; the cap only guards the database from abuse.
	maxAvatarBytes = 300 * 1024
)

var avatarMediaTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// defaultAvatarSVG is the same red "G" the navbar shows for anonymous
// users (DEFAULT_AVATAR in the frontend), as raw SVG bytes.
const defaultAvatarSVG = `<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'><rect width='64' height='64' rx='12' fill='#e50914'/><text x='32' y='45' font-family='Arial, sans-serif' font-size='38' font-weight='900' fill='#ffffff' text-anchor='middle'>G</text></svg>`

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

// authStore owns accounts, sessions and the gate middleware. State lives in
// SQLite (shared *sql.DB); the mutex mirrors the old JSON-file store's
// serialization so a write never interleaves SELECTs with their UPDATEs.
type authStore struct {
	mu     sync.Mutex
	db     *sql.DB
	invite string // AUTH_PASSWORD — required to register when non-empty
}

// NewAuthStore opens (or initializes) the account store. db is the shared
// SQLite handle (see internal/db); nil means persistence off (in-memory
// store for tests). invite is the AUTH_PASSWORD registration code; empty
// means open registration.
func NewAuthStore(db *sql.DB, invite string) *authStore {
	return &authStore{db: db, invite: strings.TrimSpace(invite)}
}

// --- Credentials ---

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashToken hashes a session token for storage: the database then holds no
// directly usable credentials — a leaked database cannot hijack live
// sessions.
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

// --- Row scanning ---

const userCols = `id, username, email, avatar, salt, hash, created_at, is_admin`

func scanUser(row interface{ Scan(...any) error }) (*user, error) {
	var u user
	var email, avatar sql.NullString
	var createdAt int64
	var isAdmin int
	if err := row.Scan(&u.ID, &u.Username, &email, &avatar, &u.Salt, &u.Hash, &createdAt, &isAdmin); err != nil {
		return nil, err
	}
	u.Email = email.String
	u.Avatar = avatar.String
	u.CreatedAt = time.Unix(createdAt, 0)
	u.IsAdmin = isAdmin != 0
	return &u, nil
}

func unixNow() int64 { return time.Now().Unix() }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
	// Empty emails become SQL NULL: an empty string would collide under the
	// UNIQUE constraint (SQLite UNIQUE treats '' as a value, NULL never
	// collides).
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))
	var emailVal any // nil (SQL NULL) or the normalized address
	if emailAddr != "" {
		if !validEmail(emailAddr) {
			return nil, "Enter a valid email address"
		}
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, emailAddr).Scan(&n); err != nil {
			return nil, "Could not check email"
		}
		if n > 0 {
			return nil, "That email is already in use"
		}
		emailVal = emailAddr
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ? COLLATE NOCASE`, username).Scan(&n); err != nil {
		return nil, "Could not check username"
	}
	if n > 0 {
		return nil, "That username is taken"
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
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return nil, "Could not create account"
	}
	u.IsAdmin = n == 0
	// Clean out expired sessions while we are writing anyway.
	s.pruneSessionsLocked(time.Now())
	if _, err := s.db.Exec(`INSERT INTO users (id, username, email, avatar, salt, hash, created_at, is_admin) VALUES (?,?,?,?,?,?,?,?)`,
		u.ID, u.Username, emailVal, u.Avatar, u.Salt, u.Hash, u.CreatedAt.Unix(), boolInt(u.IsAdmin)); err != nil {
		log.Printf("[Auth] insert user failed: %v", err)
		return nil, "Could not create account"
	}
	log.Printf("[Auth] registered account %q", username)
	return u, ""
}

func (s *authStore) login(username, password string) (*session, string) {
	// Returns the session record only; the HTTP handler mints the actual
	// cookie token and stores it under its hash (minting one here too would
	// leave an orphaned session row whose token never left the server).
	s.mu.Lock()
	defer s.mu.Unlock()
	username = strings.TrimSpace(username)
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ? COLLATE NOCASE`, username)
	u, err := scanUser(row)
	if err != nil {
		return nil, "Wrong username or password"
	}
	ok, upgrade := verifyPassword(password, u.Salt, u.Hash)
	if !ok {
		return nil, "Wrong username or password"
	}
	if upgrade != "" {
		// Transparent legacy-hash → bcrypt upgrade.
		if _, err := s.db.Exec(`UPDATE users SET hash = ? WHERE id = ?`, upgrade, u.ID); err != nil {
			log.Printf("[Auth] hash upgrade failed: %v", err)
		}
	}
	sess := &session{UserID: u.ID, ExpiresAt: time.Now().Add(sessionTTL)}
	s.pruneSessionsLocked(time.Now())
	return sess, ""
}

// pruneSessionsLocked drops expired sessions. Callers hold s.mu.
func (s *authStore) pruneSessionsLocked(now time.Time) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now.Unix())
}

// sessionFor resolves a cookie token to a live session, sliding its expiry.
// Tokens are stored hashed, so the cookie value never appears on disk.
func (s *authStore) sessionFor(token string) (*session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tokHash string
	var userID string
	var expiresAt int64
	err := s.db.QueryRow(`SELECT token_hash, user_id, expires_at FROM sessions WHERE token_hash = ?`, hashToken(token)).
		Scan(&tokHash, &userID, &expiresAt)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	if now.After(time.Unix(expiresAt, 0)) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokHash)
		return nil, false
	}
	// Sliding expiry: extend immediately. SQLite writes are cheap and the
	// per-request cost stays far below one file write per request.
	_, _ = s.db.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, now.Add(sessionTTL).Unix(), tokHash)
	return &session{UserID: userID, ExpiresAt: now.Add(sessionTTL)}, true
}

func (s *authStore) logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
}

func (s *authStore) usernameByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var name string
	if err := s.db.QueryRow(`SELECT username FROM users WHERE id = ?`, id).Scan(&name); err != nil {
		return ""
	}
	return name
}

// emailByID returns the account's stored email ("" when unset or unknown).
func (s *authStore) emailByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var email sql.NullString
	if err := s.db.QueryRow(`SELECT email FROM users WHERE id = ?`, id).Scan(&email); err != nil {
		return ""
	}
	return email.String
}

// avatarByID returns the account's stored avatar data-URI ("" when unset).
func (s *authStore) avatarByID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var avatar sql.NullString
	if err := s.db.QueryRow(`SELECT avatar FROM users WHERE id = ?`, id).Scan(&avatar); err != nil {
		return ""
	}
	return avatar.String
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
	res, err := s.db.Exec(`UPDATE users SET avatar = ? WHERE id = ?`, dataURI, userID)
	if err != nil {
		return "Could not save avatar"
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "Account not found"
	}
	return ""
}

// validateAvatar checks a data-URI profile picture: supported type and a
// decoded size comfortably below the cap.
func validateAvatar(dataURI string) string {
	media, b64, ok := strings.Cut(dataURI, ";base64,")
	// Browsers send the RFC 2397 form ("data:image/jpeg;base64,..."); the
	// "data:" scheme prefix is not part of the media type.
	media, hasScheme := strings.CutPrefix(media, "data:")
	if !ok || !hasScheme || !avatarMediaTypes[media] {
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
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, userID)
	u, err := scanUser(row)
	if err != nil {
		return "Account not found"
	}
	if ok, _ := verifyPassword(password, u.Salt, u.Hash); !ok {
		return "Current password is wrong"
	}
	if u.IsAdmin {
		var admins int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
			return "Could not delete account"
		}
		if admins <= 1 {
			return "Cannot delete the last admin account"
		}
	}
	return s.deleteUserLocked(userID)
}

// deleteUserLocked removes an account row and its sessions. Callers hold
// s.mu.
func (s *authStore) deleteUserLocked(userID string) string {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		log.Printf("[Auth] session cleanup failed: %v", err)
		return "Could not delete account"
	}
	if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		log.Printf("[Auth] delete failed: %v", err)
		return "Could not delete account"
	}
	return ""
}

// changePassword verifies the current password and replaces the hash.
// Returns "" on success or a user-facing error message.
func (s *authStore) changePassword(userID, current, next string) string {
	if len(next) < minPasswordLen || len(next) > maxPasswordLen {
		return "New password must be 4-128 characters"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, userID)
	u, err := scanUser(row)
	if err != nil {
		return "Account not found"
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
	if _, err := s.db.Exec(`UPDATE users SET hash = ? WHERE id = ?`, hash, userID); err != nil {
		return "Could not change password"
	}
	return ""
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
	rows, err := s.db.Query(`SELECT ` + userCols + `, (SELECT COUNT(*) FROM sessions s WHERE s.user_id = users.id) FROM users ORDER BY created_at`)
	if err != nil {
		return []userRow{}
	}
	defer rows.Close()
	out := []userRow{}
	for rows.Next() {
		var r userRow
		var email sql.NullString
		var createdAt int64
		var isAdmin, sessions int
		var avatar sql.NullString
		if err := rows.Scan(&r.ID, &r.Username, &email, &avatar, new(string), new(string), &createdAt, &isAdmin, &sessions); err != nil {
			continue
		}
		r.Email = email.String
		r.CreatedAt = time.Unix(createdAt, 0)
		r.IsAdmin = isAdmin != 0
		r.HasAvatar = avatar.Valid && avatar.String != ""
		out = append(out, r)
	}
	return out
}

func (s *authStore) userIsAdmin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var isAdmin int
	if err := s.db.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, id).Scan(&isAdmin); err != nil {
		return false
	}
	return isAdmin != 0
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
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, targetID)
	u, err := scanUser(row)
	if err != nil {
		return "Account not found"
	}
	if u.IsAdmin {
		var admins int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
			return "Could not delete account"
		}
		if admins <= 1 {
			return "Cannot delete the last admin account"
		}
	}
	return s.deleteUserLocked(targetID)
}

// logoutAllSessions destroys every session belonging to a user; returns the
// count dropped. Used by admins to force-sign-out a device.
func (s *authStore) logoutAllSessions(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
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
	_, _ = d.Auth.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashToken(token), u.ID, time.Now().Add(sessionTTL).Unix())
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
	_, _ = d.Auth.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashToken(token), sess.UserID, sess.ExpiresAt.Unix())
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
	w.Header().Set("Content-Type", strings.TrimPrefix(media, "data:"))
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
			// Same default avatar the navbar ships with. It is plain
			// (URL-quoted) SVG, not base64, so serve it directly instead of
			// pushing it through the base64 decode path below.
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("Cache-Control", "private, max-age=300")
			_, _ = w.Write([]byte(defaultAvatarSVG))
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
		w.Header().Set("Content-Type", strings.TrimPrefix(media, "data:"))
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
