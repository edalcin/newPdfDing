package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.tags.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tags == nil {
		tags = []store.TagWithCount{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	t, err := s.tags.Rename(id, req.Name)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "tag not found")
	case errors.Is(err, store.ErrConflict):
		writeJSONError(w, http.StatusConflict, "tag name already in use")
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := s.tags.Delete(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "tag not found")
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleSubstituteTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromID string `json:"from_id"`
		ToID   string `json:"to_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if req.FromID == "" || req.ToID == "" || req.FromID == req.ToID {
		writeJSONError(w, http.StatusBadRequest, "from_id and to_id must be distinct, non-empty ids")
		return
	}

	err := s.tags.Substitute(req.FromID, req.ToID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "tag not found")
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"merged": true})
	}
}
