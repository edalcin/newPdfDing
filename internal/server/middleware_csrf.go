package server

import (
	"net/http"

	"github.com/edalcin/newpdfding/internal/security"
)

const (
	csrfCookieName = "csrf"
	csrfHeaderName = "X-CSRF-Token"
)

// CSRF implements the double-submit cookie pattern (ver
// refatoracao/08-seguranca.md, "CSRF"). A GET/HEAD/OPTIONS request seeds the
// csrf cookie if it is missing; every non-idempotent method must echo the
// cookie's value back in X-CSRF-Token.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			ensureCSRFCookie(w, r)
			next.ServeHTTP(w, r)
		default:
			cookie, err := r.Cookie(csrfCookieName)
			if err != nil {
				writeJSONError(w, http.StatusForbidden, "missing CSRF cookie")
				return
			}
			if !security.ConstantTimeEqual(cookie.Value, r.Header.Get(csrfHeaderName)) {
				writeJSONError(w, http.StatusForbidden, "CSRF token mismatch")
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(csrfCookieName); err == nil {
		return
	}
	token, err := security.NewToken(32)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // must be readable by the frontend to echo in the header
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
