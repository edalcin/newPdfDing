package server

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

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

// handleAdminReembed serves POST /api/admin/reembed: queues every PDF whose
// embedding is not current for the model in effect now — the ones never
// embedded plus the ones marked stale, which is what switching the model in
// Configurações → IA produces for the whole acervo at once. The single
// background worker then re-embeds them serially against the new model, so
// this replaces clicking "Reembedar" on each document. Progress is polled
// through the same GET /api/embed/jobs the per-document button uses.
func (s *Server) handleAdminReembed(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeJSONError(w, http.StatusPreconditionFailed, "busca semântica desabilitada")
		return
	}
	ids, err := s.pdfs.PendingEmbeddingIDs()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	go s.embeds.enqueueBulk(ids)
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(ids), "model": s.embedModelName()})
}

// handleAdminBackup serves GET /api/admin/backup: streams a consistent
// snapshot of the SQLite database as a file download. VACUUM INTO is
// SQLite's built-in online-backup primitive — it writes a coherent
// single-file (WAL-free) copy while the server keeps serving requests, no
// pausing writers required (ver 05-api.md, "Admin").
func (s *Server) handleAdminBackup(w http.ResponseWriter, r *http.Request) {
	tmp := filepath.Join(filepath.Dir(s.cfg.DBPath), fmt.Sprintf(".backup-%d.db", time.Now().UnixNano()))
	defer os.Remove(tmp)

	if _, err := s.db.Exec("VACUUM INTO ?", tmp); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "falha ao gerar backup")
		return
	}

	f, err := os.Open(tmp)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "falha ao gerar backup")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "falha ao gerar backup")
		return
	}

	filename := fmt.Sprintf("newpdfding-backup-%s.sqlite", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// requiredRestoreTables gates handleAdminRestore's validation — reject
// anything that isn't recognizably a newPdfDing database before it ever
// touches the live one.
var requiredRestoreTables = []string{"pdfs", "tags", "settings"}

// handleAdminRestore serves POST /api/admin/restore: replaces the live
// SQLite database with an uploaded file (raw body, matching the
// handlePutPDFFile convention). The upload is integrity-checked and must
// contain newPdfDing's core tables before anything touches the live
// database. Once swapped in, s.restart asks the process to shut down
// gracefully so the container's restart policy (ver compose.yaml, "restart:
// unless-stopped") brings it back up against the new file — the only way to
// hand every store and background worker a fresh *sql.DB without
// hand-rolling a hot-swap (ver 05-api.md, "Admin").
func (s *Server) handleAdminRestore(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Dir(s.cfg.DBPath)
	tmp := filepath.Join(dir, fmt.Sprintf(".restore-%d.db", time.Now().UnixNano()))

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadMB*1024*1024)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "falha ao salvar upload")
		return
	}
	n, err := io.Copy(out, r.Body)
	out.Close()
	if err != nil {
		os.Remove(tmp)
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload excede MAX_UPLOAD_MB")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "falha ao ler upload")
		return
	}
	if n == 0 {
		os.Remove(tmp)
		writeJSONError(w, http.StatusBadRequest, "arquivo vazio")
		return
	}

	if err := validateSQLiteBackup(tmp); err != nil {
		os.Remove(tmp)
		writeJSONError(w, http.StatusBadRequest, "arquivo inválido: "+err.Error())
		return
	}

	// Close the live connection and drop its WAL/SHM sidecars before the
	// swap: VACUUM INTO writes a WAL-less single file, so leftover sidecars
	// from the old database would otherwise shadow the restored data on the
	// next boot.
	_ = s.db.Close()
	os.Remove(s.cfg.DBPath + "-wal")
	os.Remove(s.cfg.DBPath + "-shm")

	if err := os.Rename(tmp, s.cfg.DBPath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "falha ao substituir banco de dados")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"restored": true, "restarting": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go s.restart()
}

// validateSQLiteBackup opens path read-only and confirms it is a coherent
// SQLite database containing newPdfDing's core tables — rejects arbitrary
// or corrupted uploads before they ever touch the live database.
func validateSQLiteBackup(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return fmt.Errorf("não foi possível abrir como SQLite: %w", err)
	}
	defer db.Close()

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("falha ao verificar integridade: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("verificação de integridade falhou: %s", result)
	}

	for _, table := range requiredRestoreTables {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			return fmt.Errorf("tabela obrigatória ausente: %s", table)
		}
	}
	return nil
}

// defaultRestart is s.restart's production implementation: it gives the
// in-flight HTTP response a moment to reach the client, then sends SIGTERM
// to this same process — reusing the graceful-shutdown path main.go already
// wires up for the OS signal, instead of a second shutdown code path.
// Overridden in tests, which must never signal the test binary itself.
func defaultRestart() {
	time.Sleep(300 * time.Millisecond)
	p, err := os.FindProcess(os.Getpid())
	if err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
}
