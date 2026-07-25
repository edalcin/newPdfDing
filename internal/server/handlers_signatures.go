package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

// pngDataURLPattern matches a base64 PNG data URL (ver 05-api.md, "POST
// /api/signatures" — data must be "a data URL PNG válida").
var pngDataURLPattern = regexp.MustCompile(`^data:image/png;base64,[A-Za-z0-9+/]+=*$`)

type signatureResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Data      string `json:"data"`
	CreatedAt string `json:"created_at"`
}

func toSignatureResponse(sig store.Signature) signatureResponse {
	return signatureResponse{ID: sig.ID, Name: sig.Name, Data: sig.Data, CreatedAt: sig.CreatedAt}
}

func (s *Server) handleListSignatures(w http.ResponseWriter, r *http.Request) {
	sigs, err := s.signatures.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]signatureResponse, len(sigs))
	for i, sig := range sigs {
		out[i] = toSignatureResponse(sig)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateSignature(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if !pngDataURLPattern.MatchString(req.Data) {
		writeJSONError(w, http.StatusBadRequest, "data must be a PNG data URL")
		return
	}

	sig, err := s.signatures.Create(req.Name, req.Data)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toSignatureResponse(sig))
}

func (s *Server) handleDeleteSignature(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.signatures.Delete(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "signature not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
