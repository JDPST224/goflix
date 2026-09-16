package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Quoted values must work for every key type, not just string keys —
// the numeric/bool cases parse with strconv, which rejects an untrimmed
// quoted literal and would silently keep the default.
func TestLoadQuotedNumericAndBoolValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.conf")
	body := []byte(
		"# comment line\n" +
			"BROWSER_HEADLESS = \"false\"\n" +
			"BROWSER_TIMEOUT = '90s'\n" +
			"MAX_BROWSER_SESSIONS = \"5\"\n" +
			"MAX_SESSIONS = \"250\"\n" +
			"CACHE_MAX_MB = \"256\"\n" +
			"DEBUG_PPROF = \"true\"\n" +
			"AUTH_RATE_PER_MIN = \"20\"\n" +
			"RESOLVE_RATE_PER_MIN = \"30\"\n" +
			"MAX_STREAM_HEIGHT = \"1080\"\n" +
			"LISTEN_ADDR = \":9090\"\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Resolver.BrowserHeadless {
		t.Errorf("BrowserHeadless = true, want false (quoted value not parsed)")
	}
	if cfg.Resolver.BrowserTimeout != 90*time.Second {
		t.Errorf("BrowserTimeout = %v, want 90s", cfg.Resolver.BrowserTimeout)
	}
	if cfg.Resolver.MaxBrowserSessions != 5 {
		t.Errorf("MaxBrowserSessions = %d, want 5", cfg.Resolver.MaxBrowserSessions)
	}
	if cfg.Resolver.MaxSessions != 250 {
		t.Errorf("MaxSessions = %d, want 250", cfg.Resolver.MaxSessions)
	}
	if cfg.Resolver.CacheMaxBytes != 256<<20 {
		t.Errorf("CacheMaxBytes = %d, want %d", cfg.Resolver.CacheMaxBytes, 256<<20)
	}
	if !cfg.DebugProfiling {
		t.Errorf("DebugProfiling = false, want true (quoted value not parsed)")
	}
	if cfg.AuthRatePerMin != 20 {
		t.Errorf("AuthRatePerMin = %d, want 20", cfg.AuthRatePerMin)
	}
	if cfg.ResolveRatePerMin != 30 {
		t.Errorf("ResolveRatePerMin = %d, want 30", cfg.ResolveRatePerMin)
	}
	if cfg.MaxStreamHeight != 1080 {
		t.Errorf("MaxStreamHeight = %d, want 1080", cfg.MaxStreamHeight)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want \":9090\"", cfg.ListenAddr)
	}
}

// Invalid values must keep falling back to the defaults (lenient parsing).
func TestLoadInvalidValuesKeepDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.conf")
	body := []byte(
		"MAX_SESSIONS = zero\n" +
			"BROWSER_TIMEOUT = -5s\n" +
			"MAX_STREAM_HEIGHT = -1\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Resolver.MaxSessions != 200 {
		t.Errorf("MaxSessions = %d, want default 200", cfg.Resolver.MaxSessions)
	}
	if cfg.Resolver.BrowserTimeout != 45*time.Second {
		t.Errorf("BrowserTimeout = %v, want default 45s", cfg.Resolver.BrowserTimeout)
	}
	if cfg.MaxStreamHeight != 0 {
		t.Errorf("MaxStreamHeight = %d, want default 0 (uncapped)", cfg.MaxStreamHeight)
	}
}
