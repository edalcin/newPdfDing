// Package server builds the HTTP handler: router, middleware and handlers.
// server never imports storage's domain-specific callers; store never
// imports server (ver refatoracao/01-arquitetura.md, "Camadas e regra de
// dependência").
package server

import (
	"database/sql"
	"net/http"
)

// Server wraps the HTTP router and its dependencies.
type Server struct {
	db     *sql.DB
	router http.Handler
}

// New builds the chi router with all middleware and routes wired up.
func New(db *sql.DB) *Server {
	s := &Server{db: db}
	s.router = s.buildRouter()
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}
