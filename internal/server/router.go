package server

import (
	"net/http"

	"github.com/edalcin/newpdfding/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// buildRouter wires the chi router: global middleware (security headers,
// CSRF), public auth routes, and the session-protected route group. API
// routes land per their owning etapa (ver refatoracao/ETAPAS.md).
func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(security.Headers)
	r.Use(CSRF)

	r.Get("/healthz", s.handleHealthz)

	r.Post("/api/auth/login", s.handleLogin)
	r.Get("/api/auth/session", s.handleSession)

	r.Group(func(protected chi.Router) {
		protected.Use(s.AuthRequired)

		protected.Post("/api/auth/logout", s.handleLogout)
		protected.Get("/api/pdfs", s.handleListPDFs)
	})

	return r
}

// handleHealthz reports "ok" once the database responds to a trivial query.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.QueryRow("SELECT 1").Err(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("error"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
