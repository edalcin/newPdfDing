package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// buildRouter wires the chi router. Security headers, CSRF, rate limiting
// and auth middleware land in ETAPA-2-AUTH; API routes land per their owning
// etapa (ver refatoracao/ETAPAS.md).
func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealthz)

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
