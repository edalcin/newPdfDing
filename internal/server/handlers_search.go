package server

import (
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

// handleEmbedPDF serves POST /api/pdfs/{id}/embed — enqueues the document
// onto the background embedding worker and returns immediately (ver
// refatoracao Fase F.3; extraction and the Gemini call itself happen in
// runEmbedJob, polled via GET /api/embed/jobs).
func (s *Server) handleEmbedPDF(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeJSONError(w, http.StatusPreconditionFailed, "busca semântica desabilitada")
		return
	}

	id := chi.URLParam(r, "id")
	if _, err := s.pdfs.GetByID(id); errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	} else if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	switch err := s.embeds.enqueue(id); {
	case errors.Is(err, errAlreadyQueued):
		writeJSONError(w, http.StatusConflict, "embedding já em curso")
		return
	case errors.Is(err, errQueueFull):
		writeJSONError(w, http.StatusServiceUnavailable, "fila de embedding cheia")
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"state": string(embedQueued)})
}

// handleEmbedJobs serves GET /api/embed/jobs — the frontend polls this
// while any job is non-terminal (ver Fase F.4).
func (s *Server) handleEmbedJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.embeds.snapshot()})
}
