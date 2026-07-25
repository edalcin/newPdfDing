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
	"github.com/edalcin/newpdfding/internal/store"
)

// Server wraps the HTTP router and its dependencies.
type Server struct {
	cfg      *config.Config
	db       *sql.DB
	sessions *store.SessionStore
	throttle *security.LoginThrottle
	router   http.Handler
}

// New builds the chi router with all middleware and routes wired up.
func New(cfg *config.Config, db *sql.DB) *Server {
	s := &Server{
		cfg:      cfg,
		db:       db,
		sessions: store.NewSessionStore(db),
		throttle: security.NewLoginThrottle(cfg.TrustProxyHeaders),
	}
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
