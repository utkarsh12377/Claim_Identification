package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                    string
	DatabaseURL             string
	SeedFile                string
	MediaDir                string
	PublicBaseURL           string
	RewriteMediaURLs        bool
	IncludeStructuredClaims bool
	Workers                 int
	QueueSize               int
	WorkflowTimeout         time.Duration
	ShutdownTimeout         time.Duration
	LogLevel                slog.Level
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                    env("PORT", ":8080"),
		DatabaseURL:             env("DATABASE_URL", ""),
		SeedFile:                env("SEED_FILE", "seed/product.json"),
		MediaDir:                env("MEDIA_DIR", "assets/images"),
		PublicBaseURL:           env("PUBLIC_BASE_URL", "http://localhost:8080"),
		RewriteMediaURLs:        true,
		IncludeStructuredClaims: true,
		Workers:                 4,
		QueueSize:               64,
		WorkflowTimeout:         30 * time.Second,
		ShutdownTimeout:         15 * time.Second,
		LogLevel:                slog.LevelInfo,
	}

	if !strings.Contains(cfg.Addr, ":") {
		cfg.Addr = ":" + cfg.Addr
	}

	var err error
	if cfg.RewriteMediaURLs, err = envBool("REWRITE_MEDIA_URLS", cfg.RewriteMediaURLs); err != nil {
		return Config{}, err
	}
	if cfg.IncludeStructuredClaims, err = envBool("CLAIMS_INCLUDE_STRUCTURED", cfg.IncludeStructuredClaims); err != nil {
		return Config{}, err
	}
	if cfg.Workers, err = envInt("WORKFLOW_WORKERS", cfg.Workers); err != nil {
		return Config{}, err
	}
	if cfg.QueueSize, err = envInt("WORKFLOW_QUEUE_SIZE", cfg.QueueSize); err != nil {
		return Config{}, err
	}
	if cfg.WorkflowTimeout, err = envDuration("WORKFLOW_TIMEOUT", cfg.WorkflowTimeout); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = envLogLevel("LOG_LEVEL", cfg.LogLevel); err != nil {
		return Config{}, err
	}

	if cfg.Workers < 1 {
		return Config{}, fmt.Errorf("WORKFLOW_WORKERS must be at least 1, got %d", cfg.Workers)
	}
	if cfg.QueueSize < 1 {
		return Config{}, fmt.Errorf("WORKFLOW_QUEUE_SIZE must be at least 1, got %d", cfg.QueueSize)
	}

	return cfg, nil
}

func (c Config) UsesPostgres() bool { return c.DatabaseURL != "" }

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", key, raw)
	}
	return v, nil
}

func envInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, raw)
	}
	return v, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30s, got %q", key, raw)
	}
	return v, nil
}

func envLogLevel(key string, fallback slog.Level) (slog.Level, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(raw))); err != nil {
		return 0, fmt.Errorf("%s must be one of debug, info, warn, error, got %q", key, raw)
	}
	return level, nil
}
