package server

import (
	"net/http"

	"github.com/edalcin/newpdfding/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// buildRouter wires the chi router: global middleware (security headers,
// CSRF), public auth routes, and the session-protected route group. API
// routes land per their owning etapa (ver refatoracao/ETAPAS.md).
func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(security.Headers)
	r.Use(CSRF)

	r.Get("/healthz", s.handleHealthz)

	r.Post("/api/auth/login", s.handleLogin)
	r.Get("/api/auth/session", s.handleSession)

	r.Get("/s/{id}", s.handlePublicShareView)
	r.Get("/api/shared/{id}", s.handleGetSharedPDF)
	r.Get("/api/shared/{id}/file", s.handleGetSharedFile)

	r.Group(func(protected chi.Router) {
		protected.Use(s.AuthRequired)

		protected.Post("/api/auth/logout", s.handleLogout)

		protected.Get("/api/collections", s.handleListCollections)
		protected.Post("/api/collections", s.handleCreateCollection)
		protected.Patch("/api/collections/{id}", s.handleUpdateCollection)
		protected.Delete("/api/collections/{id}", s.handleDeleteCollection)

		protected.Get("/api/tags", s.handleListTags)
		protected.Patch("/api/tags/{id}", s.handleRenameTag)
		protected.Delete("/api/tags/{id}", s.handleDeleteTag)
		protected.Post("/api/tags/substitute", s.handleSubstituteTag)

		protected.Get("/api/pdfs", s.handleListPDFs)
		protected.Post("/api/pdfs", s.handleCreatePDF)
		protected.Post("/api/pdfs/bulk", s.handleBulkCreatePDFs)
		protected.Post("/api/pdfs/bulk-actions", s.handleBulkActions)
		protected.Get("/api/pdfs/{id}", s.handleGetPDF)
		protected.Patch("/api/pdfs/{id}", s.handlePatchPDF)
		protected.Delete("/api/pdfs/{id}", s.handleDeletePDF)
		protected.Get("/api/pdfs/{id}/file", s.handleServeFile)
		protected.Get("/api/pdfs/{id}/thumbnail", s.handleServeThumbnail)
		protected.Get("/api/pdfs/{id}/preview", s.handleServePreview)
		protected.Post("/api/pdfs/{id}/thumbnail", s.handleUploadThumbnail)
		protected.Get("/api/pdfs/{id}/download", s.handleDownloadPDF)
		protected.Put("/api/pdfs/{id}/file", s.handlePutPDFFile)
		protected.Post("/api/pdfs/{id}/embed", s.handleEmbedPDF)

		protected.Get("/api/annotations", s.handleListAnnotations)
		protected.Get("/api/annotations/export", s.handleExportAnnotations)
		protected.Post("/api/pdfs/{id}/annotations", s.handleCreateAnnotation)
		protected.Patch("/api/annotations/{id}", s.handlePatchAnnotation)
		protected.Delete("/api/annotations/{id}", s.handleDeleteAnnotation)

		protected.Get("/api/signatures", s.handleListSignatures)
		protected.Post("/api/signatures", s.handleCreateSignature)
		protected.Delete("/api/signatures/{id}", s.handleDeleteSignature)

		protected.Post("/api/pdfs/{id}/share", s.handleCreateShare)
		protected.Delete("/api/pdfs/{id}/share", s.handleDeleteShare)
		protected.Get("/api/shares", s.handleListShares)
	})

	return r
}

// handleHealthz reports "ok" once the database responds to a trivial query.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.QueryRow("SELECT 1").Err(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("error"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
