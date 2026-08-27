package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration loaded from environment variables.
// Fields are added incrementally, one per etapa, as the code that consumes
// them lands — see refatoracao/07-docker-ci-deploy.md for the full target list.

// EmbedModel é o único modelo de embedding suportado. Trocá-lo invalida todo vetor gravado.
const EmbedModel = "models/gemini-embedding-2"

type Config struct {
	// Required
	DBPath        string
	AdminPassword string
	Files         string

	// Optional with defaults
	ListenAddr          string
	LogLevel            string
	SessionIdleMinutes  int
	TrustProxyHeaders   bool
	MaxUploadMB         int64
	GeminiAPIKey        string // empty disables semantic search entirely
	BaseURL             string // empty = derive from the incoming request's Host header
	ConsumeEnable       bool
	ConsumeDir          string // default <FILES>/consume, resolved after FILES is known
	ConsumeInterval     int    // minutes
	ConsumeTags         string
	ConsumeSkipExisting bool
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

	files := os.Getenv("FILES")
	if files == "" {
		return nil, errors.New("FILES is required")
	}

	cfg := &Config{
		DBPath:              dbPath,
		AdminPassword:       adminPassword,
		Files:               files,
		ListenAddr:          ":8000",
		LogLevel:            "info",
		SessionIdleMinutes:  43200,
		TrustProxyHeaders:   false,
		MaxUploadMB:         200,
		ConsumeDir:          files + string(os.PathSeparator) + "consume",
		ConsumeInterval:     5,
		ConsumeSkipExisting: true,
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

	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("MAX_UPLOAD_MB must be a positive integer, got %q", v)
		}
		cfg.MaxUploadMB = n
	}

	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPIKey = v
	}

	if v := os.Getenv("BASE_URL"); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}

	if v := os.Getenv("CONSUME_ENABLE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("CONSUME_ENABLE must be a boolean, got %q", v)
		}
		cfg.ConsumeEnable = b
	}

	if v := os.Getenv("CONSUME_DIR"); v != "" {
		cfg.ConsumeDir = v
	}

	if v := os.Getenv("CONSUME_INTERVAL_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("CONSUME_INTERVAL_MINUTES must be a positive integer, got %q", v)
		}
		cfg.ConsumeInterval = n
	}

	if v := os.Getenv("CONSUME_TAGS"); v != "" {
		cfg.ConsumeTags = v
	}

	if v := os.Getenv("CONSUME_SKIP_EXISTING"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("CONSUME_SKIP_EXISTING must be a boolean, got %q", v)
		}
		cfg.ConsumeSkipExisting = b
	}

	return cfg, nil
}
