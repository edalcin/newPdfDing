package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config holds runtime configuration loaded from environment variables.
// Fields are added incrementally, one per etapa, as the code that consumes
// them lands — see refatoracao/07-docker-ci-deploy.md for the full target list.
type Config struct {
	// Required
	DBPath        string
	AdminPassword string

	// Optional with defaults
	ListenAddr         string
	LogLevel           string
	SessionIdleMinutes int
	TrustProxyHeaders  bool
}

// Load reads configuration from environment variables. It returns an error
// if any required variable is missing or any value is invalid.
func Load() (*Config, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		return nil, errors.New("DB_PATH is required")
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		return nil, errors.New("ADMIN_PASSWORD is required")
	}

	cfg := &Config{
		DBPath:             dbPath,
		AdminPassword:      adminPassword,
		ListenAddr:         ":8000",
		LogLevel:           "info",
		SessionIdleMinutes: 43200,
		TrustProxyHeaders:  false,
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("SESSION_IDLE_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("SESSION_IDLE_MINUTES must be a positive integer, got %q", v)
		}
		cfg.SessionIdleMinutes = n
	}

	if v := os.Getenv("TRUST_PROXY_HEADERS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("TRUST_PROXY_HEADERS must be a boolean, got %q", v)
		}
		cfg.TrustProxyHeaders = b
	}

	return cfg, nil
}
