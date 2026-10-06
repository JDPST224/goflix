// Package db opens the SQLite database backing GoFlix's persistence
// (accounts, sessions, userdata sync) and migrates the legacy JSON stores
// (users.json, userdata.json) into it on first run. The legacy files are
// renamed to *.migrated once imported so a subsequent start starts clean.
package db

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// legacyDSNSuffix is the exact suffix older builds appended to the database
// path without the "?" separator, which made SQLite treat it as part of the
// file name.
const legacyDSNSuffix = "&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"

// adoptLegacyMangledFile rescues databases created by the missing-"?" DSN
// bug: those builds wrote everything to a file literally named
// "<path>&_pragma=journal_mode(WAL)&...". When the intended database file
// does not exist yet but the mangled one does, rename it (and any WAL
// sidecars) into place so existing accounts, sessions and caches survive the
// fix instead of being orphaned.
func adoptLegacyMangledFile(path string) {
	mangled := path + legacyDSNSuffix
	if _, err := os.Stat(mangled); err != nil {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return // the real database already exists; never overwrite it
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Rename(mangled+suffix, path+suffix); err != nil && !os.IsNotExist(err) {
			log.Printf("[DB] could not adopt legacy database file %s: %v", mangled+suffix, err)
			return
		}
	}
	log.Printf("[DB] adopted legacy database file %s as %s", mangled, path)
}

// Open opens (creating when missing) addr's SQLite database with the
// pragmas a mostly-read, occasionally-written single-process app wants:
// WAL journaling for concurrent readers, a 5s busy timeout and FK checks.
// An empty addr means :memory: so in-process tests and persistence-disabled
// runs still work.
func Open(addr string) (*sql.DB, error) {
	if addr == "" || addr == "-" {
		addr = "file:goflix_mem?mode=memory"
	} else {
		adoptLegacyMangledFile(addr)
		addr = "file:" + addr
	}
	// The pragma query string needs a '?' separator: without it the parameters
	// are not parsed at all and the whole suffix becomes part of the database
	// FILENAME (creating files literally named "goflix.db&_pragma=..." with
	// WAL, the busy timeout and FK checks silently off).
	sep := "?"
	if strings.Contains(addr, "?") {
		sep = "&"
	}
	d, err := sql.Open("sqlite", addr+sep+"_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	// A single connection serializes access and sidesteps SQLITE_BUSY
	// entirely; every store already funnels its writes through one mutex as
	// it did with the JSON files.
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// Migrate imports the legacy JSON stores into the SQLite database when they
// exist and the corresponding tables are empty. Pass "" for a store the
// deployment never had. It is idempotent and safe to call on every start.
func Migrate(d *sql.DB, usersJSON, userdataJSON, resolutionsJSON, catalogJSON string) {
	migrateUsers(d, usersJSON)
	migrateUserdata(d, userdataJSON)
	migrateResolutionsJSON(d, resolutionsJSON)
	migrateCatalogJSON(d, catalogJSON)
}

// --- Schema ---

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id         TEXT PRIMARY KEY,
	username   TEXT NOT NULL UNIQUE COLLATE NOCASE,
	email      TEXT UNIQUE,
	avatar     BLOB,
	salt       TEXT NOT NULL DEFAULT '',
	hash       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	is_admin   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS userdata (
	user_id   TEXT PRIMARY KEY,
	state     TEXT NOT NULL, -- the JSON blob of one account's synced state
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS resolution_records (
	key          TEXT PRIMARY KEY,
	source       TEXT NOT NULL,
	headers      TEXT, -- JSON http.Header
	allowed      TEXT, -- JSON map[string]bool
	manifest     BLOB,
	manifest_fp  TEXT NOT NULL DEFAULT '',
	no_revalidate INTEGER NOT NULL DEFAULT 0,
	tier         TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS catalog_items (
	cache     TEXT NOT NULL, -- 'movies' | 'tvshows' | 'popular' | 'providers:<key>'
	position  INTEGER NOT NULL,
	item      TEXT NOT NULL, -- the JSON of one catalog item
	PRIMARY KEY (cache, position)
);
`

// EnsureSchema creates the tables. keystore is shared so both stores call it.
func EnsureSchema(d *sql.DB) error {
	_, err := d.Exec(schema)
	return err
}

// --- users.json → users/sessions ---

type legacyUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	Avatar    string    `json:"avatar,omitempty"`
	Salt      string    `json:"salt"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	IsAdmin   bool      `json:"is_admin,omitempty"`
}

type legacySession struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type legacyUsersFile struct {
	Version  int                       `json:"version"`
	Users    []*legacyUser             `json:"users"`
	Sessions map[string]*legacySession `json:"sessions"`
}

func migrateUsers(d *sql.DB, path string) {
	if path == "" || path == "-" {
		return
	}
	if n := count(d, "users"); n > 0 {
		return // already migrated or in use
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	var f legacyUsersFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != 1 || len(f.Users) == 0 {
		log.Printf("[DB] discarding unreadable users file %s", path)
		return
	}
	tx, err := d.Begin()
	if err != nil {
		log.Printf("[DB] users migration failed: %v", err)
		return
	}
	defer tx.Rollback()
	for _, u := range f.Users {
		email := any(u.Email)
		if email == "" {
			email = nil
		}
		avatar := any(u.Avatar)
		if avatar == "" {
			avatar = nil
		}
		if _, err := tx.Exec(`INSERT INTO users (id, username, email, avatar, salt, hash, created_at, is_admin) VALUES (?,?,?,?,?,?,?,?)`,
			u.ID, u.Username, email, avatar, u.Salt, u.Hash, u.CreatedAt.Unix(), boolInt(u.IsAdmin)); err != nil {
			log.Printf("[DB] users migration failed: %v", err)
			return
		}
	}
	for tok, sess := range f.Sessions {
		if _, err := tx.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?,?,?)`,
			tok, sess.UserID, sess.ExpiresAt.Unix()); err != nil {
			log.Printf("[DB] users migration failed: %v", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[DB] users migration failed: %v", err)
		return
	}
	log.Printf("[DB] migrated %d account(s) and %d session(s) from %s",
		len(f.Users), len(f.Sessions), path)
	renameMigrated(path)
}

// --- userdata.json → userdata ---

type legacyUserdataFile struct {
	Version int `json:"version"`
	// Either the v2 {"users": {...}} form or a v1 single-household blob.
	Users map[string]json.RawMessage `json:"users"`
}

func migrateUserdata(d *sql.DB, path string) {
	if path == "" || path == "-" {
		return
	}
	if n := count(d, "userdata"); n > 0 {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	var probe legacyUserdataFile
	if err := json.Unmarshal(data, &probe); err != nil {
		log.Printf("[DB] discarding unreadable userdata file %s", path)
		return
	}
	tx, err := d.Begin()
	if err != nil {
		log.Printf("[DB] userdata migration failed: %v", err)
		return
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	if len(probe.Users) > 0 {
		for uid, raw := range probe.Users {
			if _, err := tx.Exec(`INSERT INTO userdata (user_id, state, updated_at) VALUES (?,?,?)`,
				uid, raw, now); err != nil {
				log.Printf("[DB] userdata migration failed: %v", err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[DB] userdata migration failed: %v", err)
			return
		}
		log.Printf("[DB] migrated userdata for %d account(s) from %s", len(probe.Users), path)
		renameMigrated(path)
		return
	}
	// v1 legacy single-household file: park it under the legacy key so the
	// first registered account adopts it (same behavior as before).
	var single json.RawMessage
	if json.Unmarshal(data, &single) == nil && len(single) > 0 {
		if _, err := tx.Exec(`INSERT INTO userdata (user_id, state, updated_at) VALUES ('__legacy__', ?, ?)`,
			single, now); err != nil {
			log.Printf("[DB] userdata migration failed: %v", err)
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[DB] userdata migration failed: %v", err)
			return
		}
		log.Printf("[DB] migrated pre-accounts userdata from %s", path)
		renameMigrated(path)
	}
}

// --- helpers ---

func count(d *sql.DB, table string) int {
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		return 0
	}
	return n
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// renameMigrated gets the legacy file out of the way without deleting it —
// a manual rollback stays possible.
func renameMigrated(path string) {
	if err := os.Rename(path, path+".migrated"); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[DB] could not retire %s: %v", path, err)
		}
		return
	}
	log.Printf("[DB] retired %s (renamed to %s.migrated)", path, path)
}

// --- Resolution records -----------------------------------------------------

// StoredResolution is one remembered, validated upstream resolution as it
// lives in the resolution_records table. mediaresolver maps its internal
// record type onto this; the JSON fields match the legacy resolutions.json
// shapes one-to-one.
type StoredResolution struct {
	Source       string              `json:"source"`
	Headers      map[string][]string `json:"headers,omitempty"`
	Allowed      map[string]bool     `json:"allowed,omitempty"`
	Manifest     []byte              `json:"manifest,omitempty"`
	ManifestFP   string              `json:"manifest_fp,omitempty"`
	NoRevalidate bool                `json:"no_revalidate,omitempty"`
	Tier         string              `json:"tier,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
}

// LoadResolutions reads every stored resolution record. Callers filter by
// TTL themselves (the loader must not silently drop rows the persist path
// would rewrite anyway).
func LoadResolutions(d *sql.DB) (map[string]StoredResolution, error) {
	rows, err := d.Query(`SELECT key, source, headers, allowed, manifest, manifest_fp, no_revalidate, tier, created_at FROM resolution_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]StoredResolution{}
	for rows.Next() {
		var (
			key          string
			rec          StoredResolution
			headers      sql.NullString
			allowed      sql.NullString
			manifest     []byte
			manifestFP   sql.NullString
			noRevalidate int
			tier         sql.NullString
			createdAt    int64
		)
		if err := rows.Scan(&key, &rec.Source, &headers, &allowed, &manifest, &manifestFP, &noRevalidate, &tier, &createdAt); err != nil {
			return nil, err
		}
		rec.Manifest = manifest
		rec.ManifestFP = manifestFP.String
		rec.NoRevalidate = noRevalidate != 0
		rec.Tier = tier.String
		rec.CreatedAt = time.Unix(createdAt, 0)
		if headers.Valid && headers.String != "" {
			_ = json.Unmarshal([]byte(headers.String), &rec.Headers)
		}
		if allowed.Valid && allowed.String != "" {
			_ = json.Unmarshal([]byte(allowed.String), &rec.Allowed)
		}
		out[key] = rec
	}
	return out, rows.Err()
}

// SaveResolutions replaces the whole table contents in one transaction —
// the same whole-snapshot semantics the legacy JSON file had, and cheap at
// the cache's bounded size (≤ 512 tiny rows).
func SaveResolutions(d *sql.DB, recs map[string]StoredResolution) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM resolution_records`); err != nil {
		return err
	}
	for k, rec := range recs {
		headers := any(nil)
		allowed := any(nil)
		if len(rec.Headers) > 0 {
			if b, err := json.Marshal(rec.Headers); err == nil {
				headers = string(b)
			}
		}
		if len(rec.Allowed) > 0 {
			if b, err := json.Marshal(rec.Allowed); err == nil {
				allowed = string(b)
			}
		}
		var manifest any
		if len(rec.Manifest) > 0 {
			manifest = rec.Manifest
		}
		var createdAt int64
		if !rec.CreatedAt.IsZero() {
			createdAt = rec.CreatedAt.Unix()
		}
		if _, err := tx.Exec(`INSERT INTO resolution_records (key, source, headers, allowed, manifest, manifest_fp, no_revalidate, tier, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			k, rec.Source, headers, allowed, manifest, rec.ManifestFP, boolInt(rec.NoRevalidate), rec.Tier, createdAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateResolutionsJSON imports a legacy resolutions.json. Expired records
// are dropped: every loader discards them anyway, and the TTL boundary moves
// with the wall clock, so dropping here is strictly correct.
func migrateResolutionsJSON(d *sql.DB, path string) {
	if path == "" || path == "-" {
		return
	}
	if n := count(d, "resolution_records"); n > 0 {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	var probe struct {
		Version int `json:"version"`
		Records map[string]StoredResolution
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.Version != 1 || len(probe.Records) == 0 {
		log.Printf("[DB] discarding unreadable resolution cache %s", path)
		return
	}
	now := time.Now()
	// resolutionTTL lives in mediaresolver (24h); duplicated here because db
	// cannot import mediaresolver without an import cycle.
	const resolutionTTL = 24 * time.Hour
	for k, rec := range probe.Records {
		if rec.Source == "" || rec.CreatedAt.IsZero() || now.After(rec.CreatedAt.Add(resolutionTTL)) {
			delete(probe.Records, k)
		}
	}
	if len(probe.Records) == 0 {
		renameMigrated(path)
		return
	}
	if err := SaveResolutions(d, probe.Records); err != nil {
		log.Printf("[DB] resolution cache migration failed: %v", err)
		return
	}
	log.Printf("[DB] migrated %d resolution record(s) from %s", len(probe.Records), path)
	renameMigrated(path)
}

// --- Catalog caches ---------------------------------------------------------

// SaveCatalogCaches replaces the whole catalog_items table with the given
// caches (name → JSON array of items) in one transaction. Rows are ordered
// by position so LoadCatalogCaches restores the exact row order.
func SaveCatalogCaches(d *sql.DB, caches map[string]json.RawMessage) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM catalog_items`); err != nil {
		return err
	}
	for cache, items := range caches {
		var arr []json.RawMessage
		if err := json.Unmarshal(items, &arr); err != nil {
			return err
		}
		for i, item := range arr {
			if _, err := tx.Exec(`INSERT INTO catalog_items (cache, position, item) VALUES (?,?,?)`,
				cache, i, item); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// LoadCatalogCaches reads every catalog cache back (name → JSON array),
// preserving save order.
func LoadCatalogCaches(d *sql.DB) (map[string]json.RawMessage, error) {
	rows, err := d.Query(`SELECT cache, item FROM catalog_items ORDER BY cache, position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bufs := map[string][]json.RawMessage{}
	for rows.Next() {
		var cache string
		var item []byte
		if err := rows.Scan(&cache, &item); err != nil {
			return nil, err
		}
		bufs[cache] = append(bufs[cache], json.RawMessage(item))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(bufs))
	for cache, items := range bufs {
		blob, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		out[cache] = blob
	}
	return out, nil
}

// migrateCatalogJSON imports a legacy catalog_snapshot.json: its top-level
// array fields (movies, tvshows, popular, providers.*) become catalog_items
// rows verbatim.
func migrateCatalogJSON(d *sql.DB, path string) {
	if path == "" || path == "-" {
		return
	}
	if n := count(d, "catalog_items"); n > 0 {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		log.Printf("[DB] discarding unreadable catalog snapshot %s", path)
		return
	}
	caches := map[string]json.RawMessage{}
	for _, name := range []string{"movies", "tvshows", "popular"} {
		if raw, ok := probe[name]; ok {
			caches[name] = raw
		}
	}
	if raw, ok := probe["providers"]; ok {
		var providers map[string]json.RawMessage
		if err := json.Unmarshal(raw, &providers); err == nil {
			for key, items := range providers {
				caches["providers:"+key] = items
			}
		}
	}
	if len(caches) == 0 {
		log.Printf("[DB] discarding legacy catalog snapshot %s (no caches found)", path)
		return
	}
	if err := SaveCatalogCaches(d, caches); err != nil {
		log.Printf("[DB] catalog snapshot migration failed: %v", err)
		return
	}
	log.Printf("[DB] migrated %d catalog cache(s) from %s", len(caches), path)
	renameMigrated(path)
}
