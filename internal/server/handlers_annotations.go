package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

type annotationResponse struct {
	ID        string `json:"id" yaml:"id"`
	PDFID     string `json:"pdf_id" yaml:"pdf_id"`
	Kind      string `json:"kind" yaml:"kind"`
	Page      int    `json:"page" yaml:"page"`
	Text      string `json:"text" yaml:"text"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
}

func toAnnotationResponse(a store.Annotation) annotationResponse {
	return annotationResponse{
		ID: a.ID, PDFID: a.PDFID, Kind: a.Kind, Page: a.Page, Text: a.Text, CreatedAt: a.CreatedAt,
	}
}

func isValidAnnotationKind(kind string) bool {
	return kind == "comment" || kind == "highlight"
}

// handleListAnnotations serves GET /api/annotations.
func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != "" && !isValidAnnotationKind(kind) {
		writeJSONError(w, http.StatusBadRequest, "invalid kind")
		return
	}

	items, next, err := s.annotations.List(store.AnnotationListParams{
		Kind:   kind,
		PDFID:  q.Get("pdf_id"),
		Cursor: q.Get("cursor"),
	})
	if errors.Is(err, store.ErrInvalidCursor) {
		writeJSONError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]annotationResponse, len(items))
	for i, a := range items {
		out[i] = toAnnotationResponse(a)
	}
	var nextCursor any
	if next != "" {
		nextCursor = next
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": nextCursor})
}

// handleCreateAnnotation serves POST /api/pdfs/{id}/annotations.
func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	pdfID := chi.URLParam(r, "id")
	if _, err := s.pdfs.GetByID(pdfID); errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	} else if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req struct {
		Kind string `json:"kind"`
		Page *int   `json:"page"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if !isValidAnnotationKind(req.Kind) || req.Page == nil {
		writeJSONError(w, http.StatusBadRequest, "kind must be comment|highlight and page is required")
		return
	}

	a, err := s.annotations.Create(pdfID, req.Kind, *req.Page, req.Text)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toAnnotationResponse(a))
}

// handlePatchAnnotation serves PATCH /api/annotations/{id}.
func (s *Server) handlePatchAnnotation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	a, err := s.annotations.UpdateText(id, req.Text)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "annotation not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAnnotationResponse(a))
}

// handleDeleteAnnotation serves DELETE /api/annotations/{id}.
func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.annotations.Delete(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "annotation not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleExportAnnotations serves GET /api/annotations/export.
func (s *Server) handleExportAnnotations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != "" && !isValidAnnotationKind(kind) {
		writeJSONError(w, http.StatusBadRequest, "invalid kind")
		return
	}
	format := q.Get("format")
	if format != "json" && format != "yaml" {
		writeJSONError(w, http.StatusBadRequest, "format must be json or yaml")
		return
	}

	items, err := s.annotations.ListAll(kind, q.Get("pdf_id"))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]annotationResponse, len(items))
	for i, a := range items {
		out[i] = toAnnotationResponse(a)
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="annotations.json"`)
		_ = json.NewEncoder(w).Encode(out)
	case "yaml":
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Content-Disposition", `attachment; filename="annotations.yaml"`)
		_ = yaml.NewEncoder(w).Encode(out)
	}
}
