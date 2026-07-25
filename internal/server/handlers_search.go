package server

import (
	"errors"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

// handleEmbedPDF serves POST /api/pdfs/{id}/embed — the single, synchronous
// on-demand embedding path (ver refatoracao/04-busca-hibrida.md, "Sem
// worker, sem automatismo"; refatoracao/05-api.md, "Embedding sob demanda").
func (s *Server) handleEmbedPDF(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeJSONError(w, http.StatusPreconditionFailed, "busca semântica desabilitada")
		return
	}

	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	body, err := s.pdfs.GetText(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if body == "" {
		writeJSONError(w, http.StatusUnprocessableEntity, "documento sem texto extraído")
		return
	}

	text := store.BuildEmbedText(pdf.Name, pdf.Description, body)
	hash := store.ContentHash(s.pdfs.EmbedModel(), text)

	if info, has, err := s.pdfs.GetEmbeddingInfo(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	} else if has && info.ContentHash == hash {
		writeJSONError(w, http.StatusConflict, "embedding já está atualizado")
		return
	}

	if !s.embedMu.TryLock() {
		writeJSONError(w, http.StatusConflict, "outro embedding em curso")
		return
	}
	defer s.embedMu.Unlock()

	vec, err := s.gemini.Embed(r.Context(), text)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "falha ao chamar a API de embeddings")
		return
	}

	if err := s.pdfs.UpsertEmbedding(id, hash, vec); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"embedding_status": "current", "dimensions": len(vec)})
}
