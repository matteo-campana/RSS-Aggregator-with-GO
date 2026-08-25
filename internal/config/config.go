// Package config loads and validates the application configuration from the
// environment. It reads os.Getenv only, so tests can drive it with t.Setenv;
// loading a .env file is the composition root's job.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully validated configuration of the service.
type Config struct {
	Port            string
	DatabaseURL     string
	AllowedOrigins  []string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	// HTTP server timeouts. Without these the server is vulnerable to
	// slow-client attacks (gosec G112).
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration

	// Connection pool sizing.
	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration

	// Scraper behaviour.
	ScraperConcurrency    int32
	ScraperInterval       time.Duration
	ScraperRequestTimeout time.Duration
	ScraperUserAgent      string
	// ScraperAllowPrivateAddresses lets the scraper reach loopback, private and
	// link-local addresses. Off by default: feed URLs come from API clients, so
	// enabling it turns POST /v1/feeds into a server-side request forgery
	// primitive against the machine's own network.
	ScraperAllowPrivateAddresses bool
	// ScraperHTTPTimeout bounds a single HTTP fetch. It must stay below
	// ScraperRequestTimeout, which is the budget for the whole feed: fetch plus
	// every post insert.
	ScraperHTTPTimeout time.Duration

	// Pagination bounds for GET /v1/posts.
	DefaultPageSize int32
	MaxPageSize     int32
}

// Load reads the configuration from the environment, applying defaults and
// collecting every validation problem instead of failing on the first one.
func Load() (Config, error) {
	var problems []string

	cfg := Config{
		Port:        os.Getenv("PORT"),
		DatabaseURL: os.Getenv("DB_URL"),
	}

	if cfg.Port == "" {
		problems = append(problems, "PORT is required")
	} else if _, err := strconv.Atoi(cfg.Port); err != nil {
		problems = append(problems, fmt.Sprintf("PORT must be a number, got %q", cfg.Port))
	}

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DB_URL is required")
	}

	collect := func(err error) {
		if err != nil {
			problems = append(problems, err.Error())
		}
	}

	var err error

	cfg.AllowedOrigins = splitAndTrim(envOr("CORS_ALLOWED_ORIGINS", "https://*,http://*"))
	if len(cfg.AllowedOrigins) == 0 {
		problems = append(problems, "CORS_ALLOWED_ORIGINS must list at least one origin")
	}

	cfg.LogLevel, err = parseLogLevel(envOr("LOG_LEVEL", "info"))
	collect(err)

	cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", 15*time.Second)
	collect(err)
	cfg.ReadHeaderTimeout, err = durationEnv("READ_HEADER_TIMEOUT", 5*time.Second)
	collect(err)
	cfg.ReadTimeout, err = durationEnv("READ_TIMEOUT", 15*time.Second)
	collect(err)
	cfg.WriteTimeout, err = durationEnv("WRITE_TIMEOUT", 15*time.Second)
	collect(err)
	cfg.IdleTimeout, err = durationEnv("IDLE_TIMEOUT", 60*time.Second)
	collect(err)

	cfg.DBMaxConns, err = int32Env("DB_MAX_CONNS", 10, 1, 1000)
	collect(err)
	cfg.DBMinConns, err = int32Env("DB_MIN_CONNS", 0, 0, 1000)
	collect(err)
	cfg.DBMaxConnLifetime, err = durationEnv("DB_MAX_CONN_LIFETIME", time.Hour)
	collect(err)

	cfg.ScraperConcurrency, err = int32Env("SCRAPER_CONCURRENCY", 10, 1, 1000)
	collect(err)
	cfg.ScraperInterval, err = durationEnv("SCRAPER_INTERVAL", time.Minute)
	collect(err)
	cfg.ScraperRequestTimeout, err = durationEnv("SCRAPER_REQUEST_TIMEOUT", 30*time.Second)
	collect(err)
	cfg.ScraperUserAgent = envOr("SCRAPER_USER_AGENT", "rss-aggregator/1.0 (+https://github.com/matteo-campana/rss-aggregator)")
	cfg.ScraperAllowPrivateAddresses, err = boolEnv("SCRAPER_ALLOW_PRIVATE_ADDRESSES", false)
	collect(err)

	// Default to two thirds of the feed budget, leaving the remainder for the
	// inserts that follow the fetch.
	cfg.ScraperHTTPTimeout, err = durationEnv("SCRAPER_HTTP_TIMEOUT", cfg.ScraperRequestTimeout*2/3)
	collect(err)
	if cfg.ScraperHTTPTimeout >= cfg.ScraperRequestTimeout && cfg.ScraperRequestTimeout > 0 {
		problems = append(problems,
			"SCRAPER_HTTP_TIMEOUT must be shorter than SCRAPER_REQUEST_TIMEOUT, which also has to cover storing the fetched items")
	}

	cfg.DefaultPageSize, err = int32Env("DEFAULT_PAGE_SIZE", 10, 1, 1000)
	collect(err)
	cfg.MaxPageSize, err = int32Env("MAX_PAGE_SIZE", 100, 1, 1000)
	collect(err)

	if cfg.DBMinConns > cfg.DBMaxConns {
		problems = append(problems, "DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}
	if cfg.DefaultPageSize > cfg.MaxPageSize {
		problems = append(problems, "DEFAULT_PAGE_SIZE must not exceed MAX_PAGE_SIZE")
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|error, got %q", s)
	}
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30s or 1m, got %q", key, raw)
	}
	if d <= 0 {
		return 0, errors.New(key + " must be positive")
	}
	return d, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean such as true or false, got %q", key, raw)
	}
	return v, nil
}

func int32Env(key string, fallback, minValue, maxValue int32) (int32, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", key, raw)
	}
	// Explicit bounds keep the conversion below provably in range.
	if n < int64(minValue) || n > int64(maxValue) {
		return 0, fmt.Errorf("%s must be between %d and %d, got %d", key, minValue, maxValue, n)
	}
	return int32(n), nil
}
