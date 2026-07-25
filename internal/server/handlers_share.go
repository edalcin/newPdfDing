package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

type shareResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// baseURL returns cfg.BaseURL if set, else derives one from the incoming
// request (ver refatoracao/07-docker-ci-deploy.md, BASE_URL: "Não |
// derivado da requisição").
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); s.cfg.TrustProxyHeaders && proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

// handleCreateShare serves POST /api/pdfs/{id}/share.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	pdfID := chi.URLParam(r, "id")
	if _, err := s.pdfs.GetByID(pdfID); errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	} else if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	sh, err := s.shares.Create(pdfID)
	if errors.Is(err, store.ErrConflict) {
		writeJSONError(w, http.StatusConflict, "pdf already shared")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, shareResponse{
		ID:  sh.ID,
		URL: fmt.Sprintf("%s/s/%s", s.baseURL(r), sh.ID),
	})
}

// handleDeleteShare serves DELETE /api/pdfs/{id}/share.
func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	pdfID := chi.URLParam(r, "id")
	err := s.shares.DeleteByPDFID(pdfID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not shared")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListShares serves GET /api/shares.
func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	shares, err := s.shares.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	type item struct {
		ID        string `json:"id"`
		PDFID     string `json:"pdf_id"`
		PDFName   string `json:"pdf_name"`
		Views     int    `json:"views"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]item, len(shares))
	for i, sh := range shares {
		out[i] = item{ID: sh.ID, PDFID: sh.PDFID, PDFName: sh.PDFName, Views: sh.Views, CreatedAt: sh.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------
// Public routes — no session required (ver 08-seguranca.md, "Isenção" de
// CSRF para rotas de compartilhamento; todas GET, então CSRF já as ignora)
// ---------------------------------------------------------------------

// handlePublicShareView serves GET /s/{share_id}: the SPA shell in shared
// mode. Until ETAPA-9-UI-BASE embeds the built frontend, this returns a
// minimal placeholder — the route contract (404 on missing/revoked share)
// is already correct and won't change once the real shell lands.
func (s *Server) handlePublicShareView(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.shares.Get(id); errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!doctype html><title>newPdfDing — shared</title><p>Shared PDF %s — viewer arrives in ETAPA-9-UI-BASE / ETAPA-10-UI-COMPLETA.</p>`, id)
}

// handleGetSharedPDF serves GET /api/shared/{share_id}: public metadata,
// incrementing the share's view counter.
func (s *Server) handleGetSharedPDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sh, err := s.shares.Get(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "share not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pdf, err := s.pdfs.GetByID(sh.PDFID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	_ = s.shares.RecordView(id)
	writePDF(w, http.StatusOK, pdf)
}

// handleGetSharedFile serves GET /api/shared/{share_id}/file: a public,
// Range-capable stream of the shared PDF (ver 03-storage.md, "Entrega de
// arquivos ao browser").
func (s *Server) handleGetSharedFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sh, err := s.shares.Get(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "share not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	pdf, err := s.pdfs.GetByID(sh.PDFID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	f, _, err := s.files.OpenSeek(r.Context(), pdf.StorageKey)
	if err != nil {
		log.Printf("warning: shared pdf file missing on disk pdf_id=%s key=%s: %v", pdf.ID, pdf.StorageKey, err)
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	_ = s.pdfs.RecordView(pdf.ID)
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeContent(w, r, pdf.Name+".pdf", modTime(pdf.CreatedAt), f)
}
