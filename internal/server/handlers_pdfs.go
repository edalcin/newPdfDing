package server

import "net/http"

// handleListPDFs is a placeholder until ETAPA-4-DOMINIO-PDF delivers the
// real PDF domain and cursor pagination (ver refatoracao/ETAPAS.md). It
// exists now only so the auth middleware has a protected route to guard.
func (s *Server) handleListPDFs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "next_cursor": nil})
}
