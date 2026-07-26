package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
)

// handleGetSettings serves GET /api/settings — the full map, closed keys
// filled with defaults where unset (ver refatoracao/05-api.md,
// "Preferências").
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.settings.All()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, all)
}

// handlePatchSettings serves PATCH /api/settings.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	all, err := s.settings.Patch(updates)
	if errors.Is(err, store.ErrInvalidSetting) {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, all)
}
