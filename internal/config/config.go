// Package config loads GoFlix's configuration from config.conf with
// environment-variable overrides. Values are parsed leniently: an invalid
// value leaves the default in place rather than failing startup.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"goflix/internal/mediaresolver"
)

// Config is the fully-resolved application configuration. The resolver's own
// settings live in Resolver (mediaresolver.Config) so they can be handed to
// mediaresolver.New unchanged.
type Config struct {
	Resolver mediaresolver.Config
	// TMDB credentials. A Bearer Read Access Token is preferred over the v3
	// API key when both are present.
	TMDBAccessToken string
	TMDBAPIKey      string
}

// Load parses path (typically "config.conf") on top of the built-in defaults,
// then applies environment overrides. It returns the defaults alongside an
// error when the file cannot be opened or read — callers decide whether that
// is fatal (main fatals; a missing config file has always been fatal).
func Load(path string) (Config, error) {
	cfg := Config{
		Resolver: mediaresolver.Config{
			TargetOrigin:            "https://vixsrc.to",
			VidKingOrigin:           "https://www.vidking.net",
			VidLoveOrigin:           "https://player.vidlove.cc",
			VidsrcmeOrigin:          "https://vidsrcme.ru",
			VidsrcmeDataOrigin:      "https://data.vidsrcme.ru",
			BrowserHeadless:         true,
			BrowserTimeout:          45 * time.Second,
			SourceResolutionTimeout: 20 * time.Second,
			MaxBrowserSessions:      3,
			CacheMaxBytes:           256 << 20,
		},
	}

	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "TMDB_ACCESS_TOKEN":
			cfg.TMDBAccessToken = cleanConfigValue(val)
		case "TMDB_API_KEY":
			cfg.TMDBAPIKey = cleanConfigValue(val)
		case "BROWSER_HEADLESS":
			if v, err := strconv.ParseBool(val); err == nil {
				cfg.Resolver.BrowserHeadless = v
			}
		case "BROWSER_TIMEOUT":
			if v, err := time.ParseDuration(val); err == nil && v > 0 {
				cfg.Resolver.BrowserTimeout = v
			}
		case "SOURCE_RESOLUTION_TIMEOUT":
			// Parsed but currently unused by the resolver; kept so existing
			// configs keep loading without warnings.
			if v, err := time.ParseDuration(val); err == nil && v > 0 {
				cfg.Resolver.SourceResolutionTimeout = v
			}
		case "MAX_BROWSER_SESSIONS":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.Resolver.MaxBrowserSessions = v
			}
		case "BROWSER_EXECUTABLE":
			cfg.Resolver.BrowserExecutable = val
		case "CACHE_MAX_MB":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.Resolver.CacheMaxBytes = int64(v) << 20
			}
		case "VIXSRC_ORIGIN":
			cfg.Resolver.TargetOrigin = cleanConfigValue(val)
		case "VIDKING_ORIGIN":
			cfg.Resolver.VidKingOrigin = cleanConfigValue(val)
		case "VIDLOVE_ORIGIN":
			cfg.Resolver.VidLoveOrigin = cleanConfigValue(val)
		case "VIDSRCME_ORIGIN":
			cfg.Resolver.VidsrcmeOrigin = cleanConfigValue(val)
		case "VIDSRCME_DATA_ORIGIN":
			cfg.Resolver.VidsrcmeDataOrigin = cleanConfigValue(val)
		}
	}

	if err := scanner.Err(); err != nil {
		return cfg, err
	}

	applyEnvOverrides(&cfg)

	return cfg, nil
}

// applyEnvOverrides applies explicit runtime overrides on top of file values;
// only variables that are actually set take effect.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BROWSER_HEADLESS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Resolver.BrowserHeadless = b
		}
	}
	if v := os.Getenv("BROWSER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Resolver.BrowserTimeout = d
		}
	}
	if v := os.Getenv("SOURCE_RESOLUTION_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Resolver.SourceResolutionTimeout = d
		}
	}
	if v := os.Getenv("MAX_BROWSER_SESSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Resolver.MaxBrowserSessions = n
		}
	}
	if v := os.Getenv("BROWSER_EXECUTABLE"); v != "" {
		cfg.Resolver.BrowserExecutable = v
	}
	if v := os.Getenv("CACHE_MAX_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Resolver.CacheMaxBytes = int64(n) << 20
		}
	}
	if v := os.Getenv("VIXSRC_ORIGIN"); v != "" {
		cfg.Resolver.TargetOrigin = v
	}
	if v := os.Getenv("VIDKING_ORIGIN"); v != "" {
		cfg.Resolver.VidKingOrigin = v
	}
	if v := os.Getenv("VIDLOVE_ORIGIN"); v != "" {
		cfg.Resolver.VidLoveOrigin = v
	}
	if v := os.Getenv("VIDSRCME_ORIGIN"); v != "" {
		cfg.Resolver.VidsrcmeOrigin = v
	}
	if v := os.Getenv("VIDSRCME_DATA_ORIGIN"); v != "" {
		cfg.Resolver.VidsrcmeDataOrigin = v
	}
	if v := os.Getenv("TMDB_ACCESS_TOKEN"); strings.TrimSpace(v) != "" {
		cfg.TMDBAccessToken = cleanConfigValue(v)
	}
	if v := os.Getenv("TMDB_API_KEY"); strings.TrimSpace(v) != "" {
		cfg.TMDBAPIKey = cleanConfigValue(v)
	}
}

// cleanConfigValue trims whitespace and optional single/double quotes around
// configuration values, making both KEY=value and KEY="value" work.
func cleanConfigValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') ||
			(v[0] == '\'' && v[len(v)-1] == '\'') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}
	return v
}
