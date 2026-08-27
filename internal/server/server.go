// Package server builds the HTTP handler: router, middleware and handlers.
// server never imports storage's domain-specific callers; store never
// imports server (ver refatoracao/01-arquitetura.md, "Camadas e regra de
// dependência").
package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/security"
	"github.com/edalcin/newpdfding/internal/storage"
	"github.com/edalcin/newpdfding/internal/store"
)

// Server wraps the HTTP router and its dependencies.
type Server struct {
	cfg         *config.Config
	db          *sql.DB
	files       *storage.LocalBackend
	sessions    *store.SessionStore
	tags        *store.TagStore
	pdfs        *store.PDFStore
	annotations *store.AnnotationStore
	shares      *store.ShareStore
	settings    *store.SettingsStore
	throttle    *security.LoginThrottle
	gemini      *store.GeminiClient // nil when GEMINI_API_KEY is unset — semantic search/embed disabled
	embeds      *embedQueue         // fila em memória do worker de embedding assíncrono (ver embedqueue.go)
	restart     func()              // triggers a process restart after handleAdminRestore swaps the DB file; overridden in tests
	router      http.Handler
}

func New(cfg *config.Config, db *sql.DB) *Server {
	s := &Server{
		cfg:         cfg,
		db:          db,
		files:       storage.NewLocalBackend(cfg.Files),
		sessions:    store.NewSessionStore(db),
		tags:        store.NewTagStore(db),
		annotations: store.NewAnnotationStore(db),
		shares:      store.NewShareStore(db),
		settings:    store.NewSettingsStore(db),
		throttle:    security.NewLoginThrottle(cfg.TrustProxyHeaders),
		gemini:      store.NewGeminiClient(cfg.GeminiAPIKey),
		embeds:      newEmbedQueue(),
		restart:     defaultRestart,
	}
	s.pdfs = store.NewPDFStore(db, func() string { return config.EmbedModel })
	s.router = s.buildRouter()
	return s
}

// StartSessionCleanup runs the hourly expired-session sweep (ver
// refatoracao/08-seguranca.md, "Limpeza") until ctx is cancelled.
func (s *Server) StartSessionCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.sessions.DeleteExpired(s.cfg.SessionIdleMinutes)
			}
		}
	}()
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}
