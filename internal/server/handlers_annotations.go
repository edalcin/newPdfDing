package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

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
	Note      string `json:"note" yaml:"note"`
	Color     string `json:"color" yaml:"color"`
	Rects     string `json:"rects" yaml:"rects"`
	Data      string `json:"data" yaml:"data"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
}

func toAnnotationResponse(a store.Annotation) annotationResponse {
	return annotationResponse{
		ID: a.ID, PDFID: a.PDFID, Kind: a.Kind, Page: a.Page, Text: a.Text,
		Note: a.Note, Color: a.Color, Rects: a.Rects, Data: a.Data, CreatedAt: a.CreatedAt,
	}
}

func isValidAnnotationKind(kind string) bool {
	return kind == "comment" || kind == "highlight"
}

// legacyAnnotationColors is the closed set inherited from the four
// hand-picked colors of the old viewer.
var legacyAnnotationColors = map[string]bool{"yellow": true, "green": true, "blue": true, "pink": true}

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// isValidAnnotationColor accepts the four legacy names plus any #rrggbb hex
// — the EmbedPDF SDK works with free-form WebColor.
func isValidAnnotationColor(c string) bool {
	return legacyAnnotationColors[c] || hexColorPattern.MatchString(c)
}

// maxAnnotationDataBytes caps AnnotationTransferItem JSON — stamps carry
// base64 image bytes inside ctx, so the field needs an explicit ceiling.
const maxAnnotationDataBytes = 2 << 20

// validateRects reports whether raw is a valid rects value: '' (unanchored)
// or a JSON array of [x, y, w, h] tuples, each component in [0, 1].
func validateRects(raw string) bool {
	if raw == "" {
		return true
	}
	var rects [][]float64
	if err := json.Unmarshal([]byte(raw), &rects); err != nil {
		return false
	}
	for _, rect := range rects {
		if len(rect) != 4 {
			return false
		}
		for _, v := range rect {
			if v < 0 || v > 1 {
				return false
			}
		}
	}
	return true
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
		Kind  string `json:"kind"`
		Page  *int   `json:"page"`
		Text  string `json:"text"`
		Note  string `json:"note"`
		Color string `json:"color"`
		Rects string `json:"rects"`
		Data  string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if !isValidAnnotationKind(req.Kind) || req.Page == nil {
		writeJSONError(w, http.StatusBadRequest, "kind must be comment|highlight and page is required")
		return
	}
	if req.Color == "" {
		req.Color = "yellow"
	}
	if !isValidAnnotationColor(req.Color) {
		writeJSONError(w, http.StatusBadRequest, "invalid color")
		return
	}
	if !validateRects(req.Rects) {
		writeJSONError(w, http.StatusBadRequest, "rects inválido")
		return
	}
	if len(req.Data) > maxAnnotationDataBytes {
		writeJSONError(w, http.StatusBadRequest, "data excede 2 MiB")
		return
	}

	a, err := s.annotations.Create(pdfID, req.Kind, *req.Page, req.Text, req.Note, req.Color, req.Rects, req.Data)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toAnnotationResponse(a))
}

// handlePatchAnnotation serves PATCH /api/annotations/{id}. rects is only
// accepted to re-anchor an annotation that has none yet (legacy rows whose
// geometry the viewer just resolved) — an already-anchored annotation
// rejects a further rects update with 409 (ver 05-api.md).
func (s *Server) handlePatchAnnotation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Text  *string `json:"text"`
		Note  *string `json:"note"`
		Color *string `json:"color"`
		Data  *string `json:"data"`
		Rects *string `json:"rects"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	if req.Rects != nil {
		if !validateRects(*req.Rects) {
			writeJSONError(w, http.StatusBadRequest, "rects inválido")
			return
		}
		current, err := s.annotations.Get(id)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "annotation not found")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if current.Rects != "" {
			writeJSONError(w, http.StatusConflict, "anotação já ancorada")
			return
		}
		if err := s.annotations.UpdateAnchor(id, *req.Rects); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if req.Color != nil && !isValidAnnotationColor(*req.Color) {
		writeJSONError(w, http.StatusBadRequest, "invalid color")
		return
	}
	if req.Data != nil && len(*req.Data) > maxAnnotationDataBytes {
		writeJSONError(w, http.StatusBadRequest, "data excede 2 MiB")
		return
	}

	a, err := s.annotations.Update(id, req.Text, req.Note, req.Data, req.Color)
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
