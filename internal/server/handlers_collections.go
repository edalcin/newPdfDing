package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

type collectionResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	CreatedAt   string `json:"created_at"`
}

func toCollectionResponse(c store.Collection) collectionResponse {
	return collectionResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		IsDefault:   c.IsDefault,
		CreatedAt:   c.CreatedAt,
	}
}

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := s.collections.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]collectionResponse, len(cols))
	for i, c := range cols {
		out[i] = toCollectionResponse(c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	c, err := s.collections.Create(req.Name, req.Description)
	if errors.Is(err, store.ErrConflict) {
		writeJSONError(w, http.StatusConflict, "collection name already in use")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toCollectionResponse(c))
}

func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	c, err := s.collections.Update(id, req.Name, req.Description)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "collection not found")
	case errors.Is(err, store.ErrConflict):
		writeJSONError(w, http.StatusConflict, "collection name already in use")
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, toCollectionResponse(c))
	}
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := s.collections.Delete(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "collection not found")
	case errors.Is(err, store.ErrConflict):
		writeJSONError(w, http.StatusConflict, "cannot delete the default collection")
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
