package server

import (
	"encoding/json"
	"net/http"

	"github.com/edalcin/newpdfding/internal/security"
)

// handleLogin verifies ADMIN_PASSWORD and starts a session (ver
// refatoracao/05-api.md, "Auth").
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.throttle.Allow(r) {
		writeJSONError(w, http.StatusTooManyRequests, "too many failed login attempts")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	if !security.VerifyPassword(req.Password, s.cfg.AdminPassword) {
		s.throttle.RecordFailure(r)
		writeJSONError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	s.throttle.RecordSuccess(r)

	token, err := security.NewToken(32)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.sessions.Create(token); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionIdleMinutes * 60,
	})
	w.WriteHeader(http.StatusOK)
}

// handleLogout deletes the session row and expires the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if id, _ := r.Context().Value(sessionIDKey).(string); id != "" {
		_ = s.sessions.Delete(id)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusOK)
}

// handleSession reports whether the request carries a valid session, used by
// the SPA to decide whether to show the login screen.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if ok, _ := s.sessions.Touch(cookie.Value, s.cfg.SessionIdleMinutes); ok {
			authenticated = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": authenticated})
}
