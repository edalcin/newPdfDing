package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/store"
	_ "modernc.org/sqlite"
)

// buildLegacyFixture creates a minimal legacy Django SQLite database plus
// media directory covering every table ImportLegacy reads: pdf_collection,
// pdf_tag, pdf_pdf, pdf_pdf_tags, pdf_pdfcomment, pdf_pdfhighlight and
// pdf_sharedpdf.
func buildLegacyFixture(t *testing.T, dir string) (dbPath, mediaDir string) {
	t.Helper()

	mediaDir = filepath.Join(dir, "legacy-media")
	if err := os.MkdirAll(filepath.Join(mediaDir, "pdf"), 0o750); err != nil {
		t.Fatalf("mkdir media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "pdf", "report.pdf"), []byte("%PDF-1.7\nlegacy import test content"), 0o640); err != nil {
		t.Fatalf("write legacy pdf: %v", err)
	}

	dbPath = filepath.Join(dir, "legacy.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	ddl := []string{
		`CREATE TABLE pdf_collection (id TEXT PRIMARY KEY, creation_date TEXT, description TEXT, name TEXT, workspace_id TEXT, default_collection INTEGER)`,
		`CREATE TABLE pdf_tag (id TEXT PRIMARY KEY, name TEXT, workspace_id TEXT)`,
		`CREATE TABLE pdf_pdf (
			id TEXT PRIMARY KEY, archived INTEGER, creation_date TEXT, collection_id TEXT,
			current_page INTEGER, description TEXT, file_directory TEXT, file TEXT,
			last_viewed_date TEXT, name TEXT, notes TEXT, number_of_pages INTEGER,
			preview TEXT, revision INTEGER, starred INTEGER, thumbnail TEXT, views INTEGER
		)`,
		`CREATE TABLE pdf_pdf_tags (id INTEGER PRIMARY KEY, pdf_id TEXT, tag_id TEXT)`,
		`CREATE TABLE pdf_pdfcomment (id TEXT PRIMARY KEY, creation_date TEXT, page INTEGER, pdf_id TEXT, text TEXT)`,
		`CREATE TABLE pdf_pdfhighlight (id TEXT PRIMARY KEY, creation_date TEXT, page INTEGER, pdf_id TEXT, text TEXT)`,
		`CREATE TABLE pdf_sharedpdf (id TEXT PRIMARY KEY, name TEXT, creation_date TEXT, password TEXT, deletion_date TEXT, views INTEGER, max_views INTEGER, pdf_id TEXT, file TEXT)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("legacy ddl %q: %v", stmt, err)
		}
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("legacy seed %q: %v", query, err)
		}
	}

	exec(`INSERT INTO pdf_collection (id, creation_date, description, name, workspace_id, default_collection) VALUES ('col-1', '2020-01-01T00:00:00Z', 'legacy default', 'Default', 'ws-1', 1)`)
	exec(`INSERT INTO pdf_tag (id, name, workspace_id) VALUES ('tag-1', 'invoices', 'ws-1')`)
	exec(`INSERT INTO pdf_pdf (
		id, archived, creation_date, collection_id, current_page, description, file_directory, file,
		last_viewed_date, name, notes, number_of_pages, preview, revision, starred, thumbnail, views
	) VALUES (
		'pdf-1', 0, '2021-06-15T10:00:00Z', 'col-1', 3, 'a report', '', 'pdf/report.pdf',
		'2021-06-16T10:00:00Z', 'My Report', 'some notes', 5, NULL, 2, 1, NULL, 4
	)`)
	exec(`INSERT INTO pdf_pdf_tags (pdf_id, tag_id) VALUES ('pdf-1', 'tag-1')`)
	exec(`INSERT INTO pdf_pdfcomment (id, creation_date, page, pdf_id, text) VALUES ('cm-1', '2021-06-15T11:00:00Z', 1, 'pdf-1', 'a comment')`)
	exec(`INSERT INTO pdf_pdfhighlight (id, creation_date, page, pdf_id, text) VALUES ('hl-1', '2021-06-15T12:00:00Z', 2, 'pdf-1', 'a highlight')`)
	exec(`INSERT INTO pdf_sharedpdf (id, name, creation_date, password, deletion_date, views, max_views, pdf_id, file) VALUES ('sh-1', 'shared report', '2021-06-17T10:00:00Z', NULL, NULL, 7, NULL, 'pdf-1', 'qr/sh-1.svg')`)

	return dbPath, mediaDir
}

// TestETAPA12_ImportLegacy covers the acceptance criterion in
// refatoracao/ETAPAS.md: importing a legacy database makes the PDF, tag and
// annotation counts match, and leaves pdf_embeddings empty (no automatic
// embedding of imported documents, ver 00-visao-geral.md decisão 5).
func TestETAPA12_ImportLegacy(t *testing.T) {
	dir := t.TempDir()
	legacyDBPath, legacyMediaDir := buildLegacyFixture(t, dir)

	db, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		t.Fatalf("mkdir files: %v", err)
	}
	cfg := &config.Config{
		DBPath: filepath.Join(dir, "db.sqlite"), AdminPassword: "correcthorse", Files: filesDir,
		ListenAddr: ":0", SessionIdleMinutes: 43200, MaxUploadMB: 200,
	}
	srv := New(cfg, db)

	report, err := srv.ImportLegacy(context.Background(), legacyDBPath, legacyMediaDir)
	if err != nil {
		t.Fatalf("ImportLegacy: %v", err)
	}

	if report.Tags != 1 {
		t.Fatalf("expected 1 tag imported, got %d", report.Tags)
	}
	if report.PDFs != 1 {
		t.Fatalf("expected 1 pdf imported, got %d", report.PDFs)
	}
	if report.Skipped != 0 {
		t.Fatalf("expected 0 skipped, got %d", report.Skipped)
	}
	if report.Annotations != 2 {
		t.Fatalf("expected 2 annotations imported (1 comment + 1 highlight), got %d", report.Annotations)
	}
	if report.Shares != 1 {
		t.Fatalf("expected 1 share imported, got %d", report.Shares)
	}

	pdfs, _, err := srv.pdfs.List(store.ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pdfs) != 1 {
		t.Fatalf("expected 1 pdf in new schema, got %d", len(pdfs))
	}
	p := pdfs[0]
	if p.Name != "My Report" || p.Views != 4 || p.CurrentPage != 3 || !p.Starred || p.Archived {
		t.Fatalf("imported pdf fields mismatch: %+v", p)
	}
	if p.EmbeddingStatus != "none" {
		t.Fatalf("expected embedding_status 'none' for an imported pdf (no automatic embedding), got %q", p.EmbeddingStatus)
	}
	if len(p.Tags) != 1 || p.Tags[0].Name != "invoices" {
		t.Fatalf("expected tag 'invoices' on imported pdf, got %+v", p.Tags)
	}

	var embeddingCount int
	if err := db.QueryRow(`SELECT count(*) FROM pdf_embeddings`).Scan(&embeddingCount); err != nil {
		t.Fatalf("count pdf_embeddings: %v", err)
	}
	if embeddingCount != 0 {
		t.Fatalf("expected 0 rows in pdf_embeddings after import, got %d", embeddingCount)
	}

	annotations, err := srv.annotations.ListAll("", p.ID)
	if err != nil {
		t.Fatalf("ListAll annotations: %v", err)
	}
	if len(annotations) != 2 {
		t.Fatalf("expected 2 annotations on imported pdf, got %d", len(annotations))
	}

	share, err := srv.shares.GetByPDFID(p.ID)
	if err != nil {
		t.Fatalf("GetByPDFID share: %v", err)
	}
	if share.ID != "sh-1" || share.Views != 7 {
		t.Fatalf("imported share fields mismatch: %+v", share)
	}

	// Re-running the import against the same target db must not duplicate
	// anything: the pdf's sha256 already exists, so it (and everything
	// scoped to it) is skipped the second time.
	report2, err := srv.ImportLegacy(context.Background(), legacyDBPath, legacyMediaDir)
	if err != nil {
		t.Fatalf("second ImportLegacy: %v", err)
	}
	if report2.PDFs != 0 || report2.Skipped != 1 {
		t.Fatalf("expected re-import to skip the duplicate pdf, got %+v", report2)
	}
}
