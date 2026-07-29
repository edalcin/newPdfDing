package server

import (
	"net/http"
	"strings"

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
	r.Use(security.Headers(extractScriptHashes(webRoot())))
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
		protected.Get("/api/pdfs/{id}/preview", s.handleServePreview)
		protected.Post("/api/pdfs/{id}/preview", s.handleUploadPreview)
		protected.Post("/api/pdfs/{id}/text", s.handleUploadText)
		protected.Get("/api/pdfs/{id}/download", s.handleDownloadPDF)
		protected.Put("/api/pdfs/{id}/file", s.handlePutPDFFile)
		protected.Post("/api/pdfs/{id}/embed", s.handleEmbedPDF)
		protected.Get("/api/embed/jobs", s.handleEmbedJobs)
		protected.Post("/api/pdfs/{id}/describe", s.handleAIDescribe)
		protected.Post("/api/pdfs/{id}/suggest-tags", s.handleAISuggestTags)

		protected.Get("/api/annotations", s.handleListAnnotations)
		protected.Get("/api/annotations/export", s.handleExportAnnotations)
		protected.Post("/api/pdfs/{id}/annotations", s.handleCreateAnnotation)
		protected.Patch("/api/annotations/{id}", s.handlePatchAnnotation)
		protected.Delete("/api/annotations/{id}", s.handleDeleteAnnotation)

		protected.Post("/api/pdfs/{id}/share", s.handleCreateShare)
		protected.Delete("/api/pdfs/{id}/share", s.handleDeleteShare)
		protected.Get("/api/shares", s.handleListShares)

		protected.Get("/api/settings", s.handleGetSettings)
		protected.Patch("/api/settings", s.handlePatchSettings)
		protected.Get("/api/ai/models", s.handleAIModels)

		protected.Get("/api/admin/info", s.handleAdminInfo)
		protected.Post("/api/admin/reindex", s.handleAdminReindex)
		protected.Get("/api/admin/backup", s.handleAdminBackup)
		protected.Post("/api/admin/restore", s.handleAdminRestore)
	})

	// SPA estática embutida via go:embed — fallback para index.html em
	// qualquer rota que não comece por /api (ver 06-frontend.md, "Saída de
	// build e integração com go:embed"). Registrada por último: rotas /api
	// já cadastradas acima têm prioridade no roteamento do chi.
	r.Get("/*", s.handleSPA)

	return r
}

// handleSPA serves the embedded frontend build. An unmatched /api/* path
// still gets a proper 404 JSON error, never the SPA shell.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	spaHandler(webRoot())(w, r)
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
