package config_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/config"
)

// setMinimal sets only the two required variables.
// t.Setenv restores the previous value automatically, so these tests cannot run
// in parallel.
func setMinimal(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "8080")
	t.Setenv("DB_URL", "postgres://user:pass@localhost:5432/rss?sslmode=disable")
}

func TestLoadDefaults(t *testing.T) {
	setMinimal(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, 15*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, time.Minute, cfg.ScraperInterval)
	assert.Equal(t, int32(10), cfg.ScraperConcurrency)
	assert.Equal(t, int32(10), cfg.DefaultPageSize)
	assert.Equal(t, int32(100), cfg.MaxPageSize)
	assert.Equal(t, 5*time.Second, cfg.ReadHeaderTimeout)
	// The old CORS list contained "https//*", missing its colon, so the HTTPS
	// wildcard never matched anything.
	assert.Equal(t, []string{"https://*", "http://*"}, cfg.AllowedOrigins)
}

func TestLoadRequiresPortAndDBURL(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_URL", "")

	_, err := config.Load()

	require.Error(t, err)
	// Both problems are reported at once rather than one per run.
	assert.Contains(t, err.Error(), "PORT is required")
	assert.Contains(t, err.Error(), "DB_URL is required")
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantMsg string
	}{
		{name: "non-numeric port", key: "PORT", value: "http", wantMsg: "PORT must be a number"},
		{name: "bad log level", key: "LOG_LEVEL", value: "verbose", wantMsg: "LOG_LEVEL must be one of"},
		{name: "bad duration", key: "SCRAPER_INTERVAL", value: "soon", wantMsg: "SCRAPER_INTERVAL must be a duration"},
		{name: "negative duration", key: "SCRAPER_INTERVAL", value: "-1m", wantMsg: "SCRAPER_INTERVAL must be positive"},
		{name: "concurrency out of range", key: "SCRAPER_CONCURRENCY", value: "0", wantMsg: "SCRAPER_CONCURRENCY must be between"},
		{name: "concurrency not a number", key: "SCRAPER_CONCURRENCY", value: "many", wantMsg: "SCRAPER_CONCURRENCY must be a number"},
		{name: "page size out of range", key: "MAX_PAGE_SIZE", value: "99999", wantMsg: "MAX_PAGE_SIZE must be between"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimal(t)
			t.Setenv(tt.key, tt.value)

			_, err := config.Load()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// An explicitly empty value must not fall back to the allow-all default: that
// silently turns a locked-down deployment into an open one.
func TestLoadRejectsEmptyCORSOrigins(t *testing.T) {
	setMinimal(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CORS_ALLOWED_ORIGINS")
}

// A field that failed to parse is still zero, so running cross-field checks
// against it invented a second problem next to the real one.
func TestLoadDoesNotInventCrossFieldProblems(t *testing.T) {
	setMinimal(t)
	t.Setenv("DB_MAX_CONNS", "abc")
	t.Setenv("DB_MIN_CONNS", "5")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_CONNS must be a number")
	assert.NotContains(t, err.Error(), "DB_MIN_CONNS must not exceed DB_MAX_CONNS")
}

func TestLoadRejectsInconsistentBounds(t *testing.T) {
	setMinimal(t)
	t.Setenv("DEFAULT_PAGE_SIZE", "50")
	t.Setenv("MAX_PAGE_SIZE", "10")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEFAULT_PAGE_SIZE must not exceed MAX_PAGE_SIZE")
}

func TestLoadParsesOverrides(t *testing.T) {
	setMinimal(t)
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SCRAPER_INTERVAL", "30s")
	t.Setenv("SCRAPER_CONCURRENCY", "4")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com ")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
	assert.Equal(t, 30*time.Second, cfg.ScraperInterval)
	assert.Equal(t, int32(4), cfg.ScraperConcurrency)
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, cfg.AllowedOrigins)
}
