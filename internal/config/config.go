package config

import (
	"errors"
	"os"
)

// Config holds runtime configuration loaded from environment variables.
// Fields are added incrementally, one per etapa, as the code that consumes
// them lands — see refatoracao/07-docker-ci-deploy.md for the full target list.
type Config struct {
	// Required
	DBPath string

	// Optional with defaults
	ListenAddr string
	LogLevel   string
}

// Load reads configuration from environment variables. It returns an error
// if any required variable is missing.
func Load() (*Config, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		return nil, errors.New("DB_PATH is required")
	}

	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: ":8000",
		LogLevel:   "info",
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	return cfg, nil
}
