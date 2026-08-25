package config

import (
	"log/slog"
	"testing"
	"time"
)

// Every key Load reads. Tests blank them so a developer's own environment
// cannot change the outcome; Load treats an empty value as unset.
var envKeys = []string{
	"PORT",
	"DATABASE_URL",
	"SEED_FILE",
	"MEDIA_DIR",
	"PUBLIC_BASE_URL",
	"REWRITE_MEDIA_URLS",
	"CLAIMS_INCLUDE_STRUCTURED",
	"WORKFLOW_WORKERS",
	"WORKFLOW_QUEUE_SIZE",
	"WORKFLOW_TIMEOUT",
	"SHUTDOWN_TIMEOUT",
	"LOG_LEVEL",
}

func isolateEnv(t *testing.T) {
	t.Helper()
	for _, key := range envKeys {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	isolateEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want \":8080\"", cfg.Addr)
	}
	if cfg.SeedFile != "seed/product.json" {
		t.Errorf("SeedFile = %q, want \"seed/product.json\"", cfg.SeedFile)
	}
	if cfg.MediaDir != "assets/images" {
		t.Errorf("MediaDir = %q, want \"assets/images\"", cfg.MediaDir)
	}
	if cfg.PublicBaseURL != "http://localhost:8080" {
		t.Errorf("PublicBaseURL = %q, want \"http://localhost:8080\"", cfg.PublicBaseURL)
	}
	if !cfg.RewriteMediaURLs {
		t.Error("RewriteMediaURLs = false, want true")
	}
	if !cfg.IncludeStructuredClaims {
		t.Error("IncludeStructuredClaims = false, want true")
	}
	if cfg.Workers != 4 {
		t.Errorf("Workers = %d, want 4", cfg.Workers)
	}
	if cfg.QueueSize != 64 {
		t.Errorf("QueueSize = %d, want 64", cfg.QueueSize)
	}
	if cfg.WorkflowTimeout != 30*time.Second {
		t.Errorf("WorkflowTimeout = %s, want 30s", cfg.WorkflowTimeout)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %s, want INFO", cfg.LogLevel)
	}
	if cfg.UsesPostgres() {
		t.Error("UsesPostgres() = true with no DATABASE_URL, want false")
	}
}

func TestLoadNormalisesBarePort(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want \":9090\"", cfg.Addr)
	}
}

func TestLoadKeepsExplicitHostPort(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PORT", "127.0.0.1:9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, want \"127.0.0.1:9090\"", cfg.Addr)
	}
}

func TestLoadTrimsSurroundingWhitespace(t *testing.T) {
	isolateEnv(t)
	t.Setenv("SEED_FILE", "  testdata/product.json  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.SeedFile != "testdata/product.json" {
		t.Errorf("SeedFile = %q, want the trimmed path", cfg.SeedFile)
	}
}

func TestLoadOverrides(t *testing.T) {
	isolateEnv(t)
	t.Setenv("DATABASE_URL", "postgres://claims:claims@localhost:5432/claims?sslmode=disable")
	t.Setenv("REWRITE_MEDIA_URLS", "false")
	t.Setenv("CLAIMS_INCLUDE_STRUCTURED", "0")
	t.Setenv("WORKFLOW_WORKERS", "16")
	t.Setenv("WORKFLOW_QUEUE_SIZE", "256")
	t.Setenv("WORKFLOW_TIMEOUT", "90s")
	t.Setenv("SHUTDOWN_TIMEOUT", "1m")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !cfg.UsesPostgres() {
		t.Error("UsesPostgres() = false with DATABASE_URL set, want true")
	}
	if cfg.RewriteMediaURLs {
		t.Error("RewriteMediaURLs = true, want false")
	}
	if cfg.IncludeStructuredClaims {
		t.Error("IncludeStructuredClaims = true, want false")
	}
	if cfg.Workers != 16 {
		t.Errorf("Workers = %d, want 16", cfg.Workers)
	}
	if cfg.QueueSize != 256 {
		t.Errorf("QueueSize = %d, want 256", cfg.QueueSize)
	}
	if cfg.WorkflowTimeout != 90*time.Second {
		t.Errorf("WorkflowTimeout = %s, want 1m30s", cfg.WorkflowTimeout)
	}
	if cfg.ShutdownTimeout != time.Minute {
		t.Errorf("ShutdownTimeout = %s, want 1m0s", cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %s, want DEBUG", cfg.LogLevel)
	}
}

func TestLoadRejectsMalformedValues(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"boolean", "REWRITE_MEDIA_URLS", "sometimes"},
		{"integer", "WORKFLOW_WORKERS", "many"},
		{"duration", "WORKFLOW_TIMEOUT", "30"},
		{"log level", "LOG_LEVEL", "chatty"},
		{"workers below one", "WORKFLOW_WORKERS", "0"},
		{"negative workers", "WORKFLOW_WORKERS", "-2"},
		{"queue size below one", "WORKFLOW_QUEUE_SIZE", "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q returned no error, want one", tc.key, tc.value)
			}
		})
	}
}
