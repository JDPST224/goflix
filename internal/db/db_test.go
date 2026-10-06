package db

// Tests for the legacy JSON store migration: users.json (accounts +
// sessions) and userdata.json (v2 per-account or v1 single-household) must
// land in SQLite and be retired to *.migrated.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func countRows(t *testing.T, d *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMigrateUsersFile(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users.json")
	write(t, users, `{
		"version": 1,
		"users": [
			{"id":"u1","username":"alice","salt":"s","hash":"h","created_at":"2026-01-01T00:00:00Z","is_admin":true},
			{"id":"u2","username":"bob","email":"","salt":"s","hash":"h","created_at":"2026-01-02T00:00:00Z"}
		],
		"sessions": {"tok-hash": {"user_id":"u1","expires_at":"2026-12-01T00:00:00Z"}}
	}`)
	d, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	Migrate(d, users, "", "", "")
	if n := countRows(t, d, "users"); n != 2 {
		t.Fatalf("users = %d, want 2", n)
	}
	if n := countRows(t, d, "sessions"); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
	// Empty emails must become NULL so the UNIQUE constraint allows many
	// account-less-email users.
	var email sql.NullString
	if err := d.QueryRow(`SELECT email FROM users WHERE id = 'u2'`).Scan(&email); err != nil || email.Valid {
		t.Fatalf("empty email should be NULL: valid=%v err=%v", email.Valid, err)
	}
	if _, err := os.Stat(users + ".migrated"); err != nil {
		t.Fatalf("users file not retired: %v", err)
	}
	// Re-running must not duplicate anything.
	Migrate(d, users+".migrated", "", "", "")
	if n := countRows(t, d, "users"); n != 2 {
		t.Fatalf("users after re-run = %d, want 2", n)
	}
}

func TestMigrateUserdataFile(t *testing.T) {
	dir := t.TempDir()
	userdata := filepath.Join(dir, "userdata.json")
	write(t, userdata, `{"version":2,"users":{"uid1":{"mylist":[],"progress":{"movie-1":{"season":1,"episode":2,"position":100.5,"duration":200.0,"at":1234}},"cw":[],"removed":{}}}}`)
	d, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	Migrate(d, "", userdata, "", "")
	if n := countRows(t, d, "userdata"); n != 1 {
		t.Fatalf("userdata = %d, want 1", n)
	}
	var raw string
	if err := d.QueryRow(`SELECT state FROM userdata WHERE user_id = 'uid1'`).Scan(&raw); err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(raw, `"movie-1"`) {
		t.Fatalf("state blob lost the progress entry: %s", raw)
	}
	if _, err := os.Stat(userdata + ".migrated"); err != nil {
		t.Fatalf("userdata file not retired: %v", err)
	}
}

func TestMigrateLegacySingleHouseholdUserdata(t *testing.T) {
	dir := t.TempDir()
	userdata := filepath.Join(dir, "userdata.json")
	write(t, userdata, `{"mylist":["x"],"progress":{},"cw":[]}`)
	d, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	Migrate(d, "", userdata, "", "")
	if n := countRows(t, d, "userdata"); n != 1 {
		t.Fatalf("userdata = %d, want 1 (legacy row)", n)
	}
	var uid string
	if err := d.QueryRow(`SELECT user_id FROM userdata`).Scan(&uid); err != nil || uid != "__legacy__" {
		t.Fatalf("legacy row user_id = %q, want __legacy__ (err=%v)", uid, err)
	}
	if _, err := os.Stat(userdata + ".migrated"); err != nil {
		t.Fatalf("userdata file not retired: %v", err)
	}
}

func TestMigrateResolutionsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolutions.json")
	write(t, path, `{
		"version": 1,
		"records": {
			"movish|tv|119051|1|3": {"source":"https://moon.example/a.m3u8","created_at":"2026-01-01T00:00:00Z","tier":"1080p"},
			"vixsrc|movie|550|-|-": {"source":"","created_at":"2026-01-01T00:00:00Z"}
		}
	}`)
	d, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	Migrate(d, "", "", path, "")
	recs, err := LoadResolutions(d)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// The empty-source record must be dropped; 2026-01-01 is past the 24h
	// TTL, so everything is expired and the table stays empty — but the
	// legacy file is still retired.
	if len(recs) != 0 {
		t.Fatalf("expired records must not be imported, got %d", len(recs))
	}
	if _, err := os.Stat(path + ".migrated"); err != nil {
		t.Fatalf("resolutions file not retired: %v", err)
	}

	// A fresh record round-trips through Save/Load.
	fresh := StoredResolution{
		Source:       "https://moon.example/b.m3u8",
		Headers:      map[string][]string{"Referer": {"https://x.example/"}},
		Allowed:      map[string]bool{"moon.example": true},
		ManifestFP:   "deadbeef",
		NoRevalidate: true,
		Tier:         "2160p",
		CreatedAt:    time.Now(),
	}
	if err := SaveResolutions(d, map[string]StoredResolution{"k": fresh}); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadResolutions(d)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := loaded["k"]
	if got.Source != fresh.Source || got.ManifestFP != "deadbeef" || !got.NoRevalidate || got.Tier != "2160p" ||
		got.Headers["Referer"][0] != "https://x.example/" || !got.Allowed["moon.example"] {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestMigrateCatalogSnapshotJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_snapshot.json")
	write(t, path, `{
		"version": 1,
		"movies": [{"id":"550","title":"Fight Club","type":"movie","categories":["Popular"]}],
		"tvshows": [{"id":"1399","title":"Game of Thrones","type":"tv","categories":[]}],
		"popular": [{"id":"700","title":"Popular One","type":"movie","categories":[]}],
		"providers": {"netflix": [{"id":"800","title":"Netflix One","type":"movie","categories":[]}]}
	}`)
	d, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := EnsureSchema(d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	Migrate(d, "", "", "", path)
	caches, err := LoadCatalogCaches(d)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, name := range []string{"movies", "tvshows", "popular", "providers:netflix"} {
		if raw, ok := caches[name]; !ok || len(raw) == 0 {
			t.Fatalf("cache %s missing after migration", name)
		}
	}
	var movies []struct {
		ID         string   `json:"id"`
		Categories []string `json:"categories"`
	}
	if err := json.Unmarshal(caches["movies"], &movies); err != nil {
		t.Fatalf("unmarshal movies: %v", err)
	}
	if len(movies) != 1 || movies[0].ID != "550" || len(movies[0].Categories) != 1 {
		t.Fatalf("movies = %+v, want one item id 550", movies)
	}
	if _, err := os.Stat(path + ".migrated"); err != nil {
		t.Fatalf("catalog file not retired: %v", err)
	}
}

// Regression: the pragma query string needs a '?' separator. The earlier
// '&'-joined form made SQLite treat the whole suffix as part of the file
// NAME, so the database landed at "app.db&_pragma=..." with WAL, the busy
// timeout and FK checks silently off.
func TestOpenFileAppliesPragmasAndCleanName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	var journal string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}
	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	// Data must land at the requested path, not at a DSN-suffixed name.
	if _, err := d.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database not at requested path: %v", err)
	}
	if _, err := os.Stat(path + legacyDSNSuffix); !os.IsNotExist(err) {
		t.Fatalf("mangled DSN-suffixed file must not be created (err=%v)", err)
	}
}

// Regression: databases written under the DSN-suffixed name by the old bug
// must be adopted on the first fixed start, not orphaned.
func TestOpenAdoptsLegacyMangledFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	mangled := path + legacyDSNSuffix

	// Recreate a database written under the mangled name, with a marker row
	// that must survive adoption.
	legacy, err := sql.Open("sqlite", "file:"+mangled)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := legacy.Exec("CREATE TABLE marker (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := legacy.Exec("INSERT INTO marker VALUES (1)"); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM marker").Scan(&n); err != nil || n != 1 {
		t.Fatalf("adopted database lost the marker row: n=%d err=%v", n, err)
	}
	if _, err := os.Stat(mangled); !os.IsNotExist(err) {
		t.Fatalf("mangled file still present after adoption (err=%v)", err)
	}
}

// When both files exist the real database wins and the mangled copy is left
// untouched rather than overwriting newer data.
func TestOpenNeverOverwritesRealDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	write(t, path, "real")
	write(t, path+legacyDSNSuffix, "mangled")
	adoptLegacyMangledFile(path)
	if b, _ := os.ReadFile(path); string(b) != "real" {
		t.Fatalf("real database overwritten: %q", b)
	}
	if b, _ := os.ReadFile(path + legacyDSNSuffix); string(b) != "mangled" {
		t.Fatalf("mangled copy should be left in place, got %q", b)
	}
}
