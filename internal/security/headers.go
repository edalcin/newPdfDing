package security

import (
	"fmt"
	"net/http"
)

// cspTemplate is the exact Content-Security-Policy literal from
// refatoracao/08-seguranca.md — every relaxation is justified there. %s is
// filled with the sha256 hash(es) of SvelteKit's own required inline
// bootstrap script (adapter-static cannot use a nonce: pages are static,
// not rendered per-request — ver internal/server/csp.go), so script-src
// stays exactly 'self' 'wasm-unsafe-eval' plus that one build-specific hash.
// worker-src 'self' blob: exists only so the EmbedPDF PDFium engine can run
// its PDFium/encoder workers, which it instantiates from `blob:` URLs
// (ver refatoracao/11-desempenho-viewer.md, causa C1) — without workers,
// PDFium rasterizes on the main thread and freezes the tab on large PDFs.
// This directive does NOT relax script-src: a worker created from blob:
// runs in an isolated context with no DOM access, so it is strictly
// narrower than allowing blob: in script-src would be.
const cspTemplate = "default-src 'self'; img-src 'self' data: blob:; script-src 'self' 'wasm-unsafe-eval'%s; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'self'; object-src 'none'; base-uri 'none'"

// Headers returns middleware that sets the fixed security headers on every
// response. None of them is configurable by environment variable — they
// are product policy (ver refatoracao/08-seguranca.md, "Headers de
// segurança"). extraScriptHashes is appended verbatim to script-src (each
// entry already single-quoted, e.g. "'sha256-...'"); pass "" when none.
func Headers(extraScriptHashes string) func(http.Handler) http.Handler {
	suffix := ""
	if extraScriptHashes != "" {
		suffix = " " + extraScriptHashes
	}
	csp := fmt.Sprintf(cspTemplate, suffix)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			h.Set("Content-Security-Policy", csp)
			next.ServeHTTP(w, r)
		})
	}
}
