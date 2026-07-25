package server

import (
	"context"
	"net/http"
)

type contextKey int

const sessionIDKey contextKey = iota

const sessionCookieName = "session"

// AuthRequired enforces a valid, non-expired session cookie. On success it
// stores the session id in the request context and slides the idle timeout
// forward (ver refatoracao/08-seguranca.md, "Expiração").
func (s *Server) AuthRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		ok, err := s.sessions.Touch(cookie.Value, s.cfg.SessionIdleMinutes)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		ctx := context.WithValue(r.Context(), sessionIDKey, cookie.Value)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
