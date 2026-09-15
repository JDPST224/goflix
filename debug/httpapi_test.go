package debug

// Black-box tests for the server's HTTP surface: accounts, userdata sync
// merge semantics, rate limiting, security headers, image-proxy validation
// and the health envelope. Everything here goes through the real mux built
// by server.New, using only the exported API.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"goflix/internal/server"
)

// httpTestServer builds the real mux backed by temp account/userdata files:
// catalog handlers are not exercised here (they need TMDB data), everything
// auth- and userdata-related runs against the real stores.
func httpTestServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	return server.New(&server.Deps{
		StartedAt: time.Now(),
		Auth:      server.NewAuthStore(filepath.Join(dir, "users.json"), ""),
		UserData:  server.NewUserDataStore(filepath.Join(dir, "userdata.json")),
	})
}

// registerAccount creates an account through the real endpoint and returns
// the session cookie.
func registerAccount(t *testing.T, h http.Handler, username, invite string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "pass1234", "invite": invite})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("register %q returned %d: %s", username, res.Code, res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Fatalf("register did not set a session cookie")
	}
	return cookies[0]
}

func getJSON(t *testing.T, h http.Handler, path string, cookie *http.Cookie) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", path, res.Code, res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s body invalid: %v", path, err)
	}
	return body
}

func postJSON(t *testing.T, h http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

// mediaKeyOf extracts the client's media key (`type-id`) from a raw synced
// list item.
func mediaKeyOf(raw json.RawMessage) string {
	var ref struct {
		ID   any    `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil || ref.ID == nil {
		return ""
	}
	t := ref.Type
	if t == "" {
		t = "movie"
	}
	switch id := ref.ID.(type) {
	case string:
		return t + "-" + id
	case float64:
		// Mirrors the server's mediaRefKey: FormatFloat keeps integral
		// numeric IDs intact ("550"), unlike the old TrimSuffix chain.
		return t + "-" + strconv.FormatFloat(id, 'f', -1, 64)
	default:
		return ""
	}
}

// TestAuthAccountFlow covers registration, login, wrong-password rejection,
// duplicate-username rejection, status and logout end to end. The site is
// open-access: anonymous callers only hit 401 on the userdata sync endpoints.
func TestAuthAccountFlow(t *testing.T) {
	h := httpTestServer(t)

	// Open access: anonymous callers only hit 401 on the userdata sync
	// endpoints — every other endpoint (and page) works without an account.
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("anonymous health returned %d, want 200 (open access)", res.Code)
	}
	res = postJSON(t, h, "/api/userdata/sync", `{}`, nil)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous sync returned %d, want 401", res.Code)
	}

	// Register, then confirm status reports the user.
	cookie := registerAccount(t, h, "alice", "")
	status := getJSON(t, h, "/api/auth/status", cookie)
	if status["authed"] != true || status["user"] != "alice" {
		t.Fatalf("status after register: %v", status)
	}

	// Duplicate username rejected.
	res = postJSON(t, h, "/api/auth/register", `{"username":"alice","password":"pass1234"}`, nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("duplicate register returned %d, want 400", res.Code)
	}

	// Logout invalidates the session server-side.
	res = postJSON(t, h, "/api/auth/logout", `{}`, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("logout returned %d", res.Code)
	}
	status = getJSON(t, h, "/api/auth/status", cookie)
	if status["authed"] != false {
		t.Fatalf("status after logout should not be authed: %v", status)
	}

	// Login with the right credentials issues a fresh session; wrong
	// password is rejected.
	res = postJSON(t, h, "/api/auth/login", `{"username":"alice","password":"wrong"}`, nil)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password returned %d, want 401", res.Code)
	}
	res = postJSON(t, h, "/api/auth/login", `{"username":"ALICE","password":"pass1234"}`, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("login returned %d: %s", res.Code, res.Body.String())
	}
	if len(res.Result().Cookies()) == 0 {
		t.Fatalf("login did not set a session cookie")
	}
}

// TestAuthInviteRequired verifies AUTH_PASSWORD gates registration only.
func TestAuthInviteRequired(t *testing.T) {
	dir := t.TempDir()
	h := server.New(&server.Deps{
		StartedAt: time.Now(),
		Auth:      server.NewAuthStore(filepath.Join(dir, "users.json"), "letmein"),
		UserData:  server.NewUserDataStore(""),
	})

	res := postJSON(t, h, "/api/auth/register", `{"username":"bob","password":"pass1234"}`, nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("registration without invite returned %d, want 400", res.Code)
	}
	status := getJSON(t, h, "/api/auth/status", nil)
	if status["inviteRequired"] != true {
		t.Fatalf("status should report inviteRequired: %v", status)
	}
	cookie := registerAccount(t, h, "bob", "letmein")
	status = getJSON(t, h, "/api/auth/status", cookie)
	if status["authed"] != true {
		t.Fatalf("registration with invite failed: %v", status)
	}
}

// TestAuthAdminAccountManagement verifies the first account is admin and can
// list and delete accounts.
func TestAuthAdminAccountManagement(t *testing.T) {
	h := httpTestServer(t)
	admin := registerAccount(t, h, "root", "")
	other := registerAccount(t, h, "second", "")

	// Admin sees both accounts.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(admin)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"username":"second"`) {
		t.Fatalf("admin user list failed: %d %s", res.Code, res.Body.String())
	}

	// Non-admin is forbidden.
	req = httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(other)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("non-admin user list returned %d, want 403", res.Code)
	}

	// Admin deletes the second account; its sessions stop working.
	del := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+userIDOf(t, h, admin, "second"), nil)
	del.AddCookie(admin)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, del)
	if res.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", res.Code, res.Body.String())
	}
	status := getJSON(t, h, "/api/auth/status", other)
	if status["authed"] != false {
		t.Fatalf("deleted account still authed: %v", status)
	}
}

// userIDOf resolves an account id from the admin user list.
func userIDOf(t *testing.T, h http.Handler, admin *http.Cookie, username string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(admin)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	var body struct {
		Users []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("user list invalid: %v", err)
	}
	for _, u := range body.Users {
		if u.Username == username {
			return u.ID
		}
	}
	t.Fatalf("account %q not in admin list", username)
	return ""
}

// TestUserdataPerUserIsolation verifies two accounts never see each other's
// synced state.
func TestUserdataPerUserIsolation(t *testing.T) {
	h := httpTestServer(t)
	alice := registerAccount(t, h, "alice", "")
	bob := registerAccount(t, h, "bob", "")

	res := postJSON(t, h, "/api/userdata/sync",
		`{"mylist":[{"id":"1","type":"movie","title":"AliceMovie"}],"progress":{},"cw":[],"removed":{}}`, alice)
	if res.Code != http.StatusOK {
		t.Fatalf("alice sync failed: %s", res.Body.String())
	}

	// Bob's view must not contain Alice's list item.
	req := httptest.NewRequest(http.MethodGet, "/api/userdata", nil)
	req.AddCookie(bob)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if strings.Contains(res.Body.String(), "AliceMovie") {
		t.Fatalf("bob sees alice's data: %s", res.Body.String())
	}

	// Alice still sees it.
	req = httptest.NewRequest(http.MethodGet, "/api/userdata", nil)
	req.AddCookie(alice)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if !strings.Contains(res.Body.String(), "AliceMovie") {
		t.Fatalf("alice lost her data: %s", res.Body.String())
	}
}

// TestUserdataSyncMerge covers the merge semantics: progress newest-wins,
// My List union, tombstoned removals stay removed, re-adds outrank tombstones.
func TestUserdataSyncMerge(t *testing.T) {
	h := httpTestServer(t)
	cookie := registerAccount(t, h, "carol", "")

	post := func(body string) *httptest.ResponseRecorder {
		res := postJSON(t, h, "/api/userdata/sync", body, cookie)
		if res.Code != http.StatusOK {
			t.Fatalf("sync returned %d: %s", res.Code, res.Body.String())
		}
		return res
	}

	post(`{"mylist":[{"id":"1","type":"movie","title":"A"}],
	       "progress":{"movie-1":{"season":1,"episode":1,"position":30,"at":100}},
	       "cw":[{"id":"1","type":"movie","at":100}],"removed":{}}`)

	// Second device, same account: older progress must lose; new list item
	// joins the union.
	res := post(`{"mylist":[{"id":"1","type":"movie","title":"A"},{"id":"2","type":"tv","title":"B"}],
	       "progress":{"movie-1":{"season":1,"episode":1,"position":10,"at":50}},
	       "cw":[{"id":"2","type":"tv","at":120}],"removed":{"movie-9":200}}`)

	var merged struct {
		MyList   []json.RawMessage         `json:"mylist"`
		Progress map[string]struct {
			Position float64 `json:"position"`
		} `json:"progress"`
		CW      []json.RawMessage `json:"cw"`
		Removed map[string]int64  `json:"removed"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &merged); err != nil {
		t.Fatalf("merge response invalid: %v", err)
	}
	if len(merged.MyList) != 2 {
		t.Errorf("my list union expected 2 items, got %d", len(merged.MyList))
	}
	if p := merged.Progress["movie-1"]; p.Position != 30 {
		t.Errorf("older progress should lose, got %+v", merged.Progress["movie-1"])
	}
	if len(merged.CW) != 2 {
		t.Errorf("continue watching expected 2 items, got %d", len(merged.CW))
	}
	if first := mediaKeyOf(merged.CW[0]); first != "tv-2" {
		t.Errorf("newest CW entry should be first, got %q", first)
	}
	if merged.Removed["movie-9"] != 200 {
		t.Errorf("tombstone union lost movie-9")
	}

	// A tombstone newer than an entry's timestamp drops it…
	res = post(`{"mylist":[],"progress":{},"cw":[],"removed":{"tv-2":300}}`)
	if err := json.Unmarshal(res.Body.Bytes(), &merged); err != nil {
		t.Fatalf("merge response invalid: %v", err)
	}
	for _, raw := range merged.CW {
		if mediaKeyOf(raw) == "tv-2" {
			t.Errorf("tombstoned CW entry survived: %s", raw)
		}
	}
	// …but a re-add with a newer timestamp survives the same tombstone.
	res = post(`{"mylist":[],"progress":{},"cw":[{"id":"2","type":"tv","at":400}],"removed":{}}`)
	if err := json.Unmarshal(res.Body.Bytes(), &merged); err != nil {
		t.Fatalf("merge response invalid: %v", err)
	}
	found := false
	for _, raw := range merged.CW {
		if mediaKeyOf(raw) == "tv-2" {
			found = true
		}
	}
	if !found {
		t.Errorf("re-added CW entry was killed by its own tombstone")
	}
}

// TestRateLimits verifies the per-IP limiters: auth bursts return 429 with
// the standard envelope and Retry-After, and resolve routes are capped too.
func TestRateLimits(t *testing.T) {
	dir := t.TempDir()
	h := server.New(&server.Deps{
		StartedAt:      time.Now(),
		Auth:           server.NewAuthStore(filepath.Join(dir, "users.json"), ""),
		UserData:       server.NewUserDataStore(""),
		AuthRatePerMin: 2,
	})
	if res := postJSON(t, h, "/api/auth/register", `{"username":"user1","password":"pass1234"}`, nil); res.Code != http.StatusOK {
		t.Fatalf("first register returned %d, want 200", res.Code)
	}
	if res := postJSON(t, h, "/api/auth/register", `{"username":"user2","password":"pass1234"}`, nil); res.Code != http.StatusOK {
		t.Fatalf("second register returned %d, want 200", res.Code)
	}
	res := postJSON(t, h, "/api/auth/register", `{"username":"user3","password":"pass1234"}`, nil)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("third register returned %d, want 429", res.Code)
	}
	if !strings.Contains(res.Body.String(), "Too many requests") || res.Header().Get("Retry-After") == "" {
		t.Errorf("429 missing envelope or Retry-After: %s", res.Body.String())
	}

	// Resolve limiter (invalid ID on purpose so no upstream resolve runs).
	dir2 := t.TempDir()
	h2 := server.New(&server.Deps{
		StartedAt:         time.Now(),
		Auth:              server.NewAuthStore(filepath.Join(dir2, "users.json"), ""),
		UserData:          server.NewUserDataStore(""),
		ResolveRatePerMin: 1,
	})
	res = httptest.NewRecorder()
	h2.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/media/source/vidking/movie/abc", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("first resolve returned %d, want 400", res.Code)
	}
	res = httptest.NewRecorder()
	h2.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/media/source/vidking/movie/abc", nil))
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("second resolve returned %d, want 429", res.Code)
	}
}

// TestSecurityHeaders verifies the hardening headers land on responses.
func TestSecurityHeaders(t *testing.T) {
	h := httpTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, want := range checks {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("Content-Security-Policy missing")
	}
}

// TestImageProxyValidation verifies the /api/img endpoint only serves
// TMDB image URLs — it must never become an open relay.
func TestImageProxyValidation(t *testing.T) {
	h := httpTestServer(t)
	blocked := []string{
		"/api/img?u=http://169.254.169.254/latest/meta-data/",
		"/api/img?u=https://evil.example.com/t/p/w500/abc.jpg",
		"/api/img?u=https://image.tmdb.org/evil/path.jpg",
		"/api/img",
	}
	for _, u := range blocked {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, u, nil))
		if res.Code != http.StatusBadRequest {
			t.Errorf("GET %s returned %d, want 400", u, res.Code)
		}
	}
}

// TestSameOriginGuard verifies cross-site POSTs with a foreign Origin are
// rejected while same-origin requests pass.
func TestSameOriginGuard(t *testing.T) {
	h := httpTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example.com")
	req.Host = "goflix.lan"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("foreign origin POST returned %d, want 403", res.Code)
	}

	// Same-origin POST passes the guard (reaches the handler's own 400).
	req = httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://goflix.lan")
	req.Host = "goflix.lan"
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code == http.StatusForbidden {
		t.Fatalf("same-origin POST was rejected by the origin guard")
	}
}

// TestHealthShape verifies the health envelope.
func TestHealthShape(t *testing.T) {
	h := httpTestServer(t)
	cookie := registerAccount(t, h, "dave", "")
	body := getJSON(t, h, "/api/health", cookie)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if _, ok := body["uptimeSeconds"]; !ok {
		t.Errorf("uptimeSeconds missing")
	}
}

// TestLoginPageServed verifies /login resolves to the sign-in page instead of
// falling through the static file server (which cannot map an extensionless
// URL to login.html).
func TestLoginPageServed(t *testing.T) {
	// The static root is relative to the repo root; tests run in the package dir.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Join(cwd, "..")); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(cwd)

	h := httpTestServer(t)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/login", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("/login returned %d, want 200", res.Code)
	}
	if !strings.Contains(res.Body.String(), "Sign in") {
		t.Errorf("/login body does not look like the login page")
	}
}

// TestUserdataSyncNumericMediaID pins the mediaRefKey numeric-ID rule: a
// client sending JSON numbers as ids must get keys with the exact integer
// value — "movie-550", never the corrupted "movie-55" the old TrimSuffix
// chain produced (which collided movie 550 with movie 55).
func TestUserdataSyncNumericMediaID(t *testing.T) {
	h := httpTestServer(t)
	cookie := registerAccount(t, h, "dana", "")

	body := `{"mylist":[{"id":550,"type":"movie","title":"Numeric"}],"progress":{},"cw":[],"removed":{}}`
	res := postJSON(t, h, "/api/userdata/sync", body, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("sync returned %d: %s", res.Code, res.Body.String())
	}

	var merged struct {
		MyList []json.RawMessage `json:"mylist"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &merged); err != nil {
		t.Fatalf("merge response invalid: %v", err)
	}
	if len(merged.MyList) != 1 {
		t.Fatalf("expected 1 list item, got %d", len(merged.MyList))
	}
	if k := mediaKeyOf(merged.MyList[0]); k != "movie-550" {
		t.Errorf("numeric media key = %q, want movie-550", k)
	}

	// A second sync of the same item must dedupe (same key), not duplicate.
	res = postJSON(t, h, "/api/userdata/sync", body, cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("second sync returned %d: %s", res.Code, res.Body.String())
	}
	merged.MyList = nil
	if err := json.Unmarshal(res.Body.Bytes(), &merged); err != nil {
		t.Fatalf("second merge response invalid: %v", err)
	}
	if len(merged.MyList) != 1 {
		t.Errorf("identical numeric-ID item was not deduped: %d items", len(merged.MyList))
	}
}
