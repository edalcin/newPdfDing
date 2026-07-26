package server

import (
	"net/http"

	"github.com/edalcin/newpdfding/internal/store"
)

type adminInfoResponse struct {
	PDFsCount             int            `json:"pdfs_count"`
	TagsCount             int            `json:"tags_count"`
	FilesBytes            int64          `json:"files_bytes"`
	EmbeddingStatusCounts map[string]int `json:"embedding_status_counts"`
}

// handleAdminInfo serves GET /api/admin/info (ver 05-api.md, "Admin").
func (s *Server) handleAdminInfo(w http.ResponseWriter, r *http.Request) {
	pdfStats, err := s.pdfs.Stats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tagsCount, err := s.tags.Count()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	filesBytes, err := s.files.TotalBytes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, adminInfoResponse{
		PDFsCount:             pdfStats.Total,
		TagsCount:             tagsCount,
		FilesBytes:            filesBytes,
		EmbeddingStatusCounts: pdfStats.EmbeddingStatusCounts,
	})
}

// handleAdminReindex serves POST /api/admin/reindex: rebuilds pdfs_fts from
// scratch (delete-all + INSERT ... SELECT). It never touches embeddings —
// embedding is always sourced from an explicit per-document click (ver
// 05-api.md, "Admin"; 04-busca-hibrida.md).
func (s *Server) handleAdminReindex(w http.ResponseWriter, r *http.Request) {
	if err := store.RebuildFTS(s.db); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reindexed": true})
}
