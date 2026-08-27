package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/store"
)

// TestAjustes_DeletePDFCleansEverything covers item 11 (ver refatoracao,
// Fase G.4): deleting a PDF that has text, an embedding, an annotation, a
// tag, a share, and a queued (never-drained) embedding job leaves no trace
// of any of them behind — no orphaned rows, no orphaned storage keys, no
// leaked job.
func TestAjustes_DeletePDFCleansEverything(t *testing.T) {
	dir := t.TempDir()
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
	// StartEmbedWorker is deliberately never called: the enqueued job below
	// must stay parked (queued, never drained) until the delete cancels it —
	// exercising the exact race a worker mid-flight would otherwise win.
	srv := New(cfg, db)

	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	// Upload with a tag, so pdf_tags gets a row too.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", "Cleanup Target")
	mw.WriteField("tags", "cleanup-tag")
	fw, _ := mw.CreateFormFile("file", "doc.pdf")
	fw.Write([]byte("%PDF-1.7\ncleanup test content"))
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/pdfs", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range client.Jar.Cookies(req.URL) {
		if c.Name == "csrf" {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", resp.StatusCode, body)
	}
	var created map[string]any
	json.Unmarshal(body, &created)
	pdfID := created["id"].(string)

	pdf, err := srv.pdfs.GetByID(pdfID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// Text (POST .../text).
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+pdfID+"/text", map[string]string{"text": "extracted body text"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set text: %d %s", resp.StatusCode, body)
	}

	// Embedding — written directly (no Gemini configured in this test).
	if err := srv.pdfs.UpsertEmbedding(pdfID, "deadbeef", []float32{0.1, 0.2, 0.3}); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	// Annotation (POST .../annotations).
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+pdfID+"/annotations", map[string]any{
		"kind": "comment", "page": 1, "text": "a note",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create annotation: %d %s", resp.StatusCode, body)
	}

	// Share (POST .../share).
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+pdfID+"/share", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create share: %d %s", resp.StatusCode, body)
	}

	// A queued embedding job that the (never-started) worker will never
	// drain on its own — only the delete's cancel() removes it.
	if err := srv.embeds.enqueue(pdfID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !srv.embeds.exists(pdfID) {
		t.Fatalf("expected job to be tracked right after enqueue")
	}

	var rowID int64
	if err := db.QueryRow(`SELECT rowid FROM pdfs WHERE id = ?`, pdfID).Scan(&rowID); err != nil {
		t.Fatalf("select rowid: %v", err)
	}

	// Delete.
	resp, body = doJSON(t, client, ts.URL, http.MethodDelete, "/api/pdfs/"+pdfID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}

	for table, query := range map[string]string{
		"pdf_text":        `SELECT count(*) FROM pdf_text WHERE pdf_id = ?`,
		"pdf_embeddings":  `SELECT count(*) FROM pdf_embeddings WHERE pdf_id = ?`,
		"pdf_annotations": `SELECT count(*) FROM pdf_annotations WHERE pdf_id = ?`,
		"pdf_tags":        `SELECT count(*) FROM pdf_tags WHERE pdf_id = ?`,
		"shares":          `SELECT count(*) FROM shares WHERE pdf_id = ?`,
	} {
		var n int
		if err := db.QueryRow(query, pdfID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("expected 0 rows in %s for deleted pdf_id=%s, got %d", table, pdfID, n)
		}
	}

	var ftsCount int
	if err := db.QueryRow(`SELECT count(*) FROM pdfs_fts WHERE rowid = ?`, rowID).Scan(&ftsCount); err != nil {
		t.Fatalf("count pdfs_fts: %v", err)
	}
	if ftsCount != 0 {
		t.Fatalf("expected 0 rows in pdfs_fts for deleted pdf rowid=%d, got %d", rowID, ftsCount)
	}

	for _, key := range []string{pdf.StorageKey, pdf.PreviewKey} {
		if key == "" {
			continue
		}
		if _, _, err := srv.files.Get(context.Background(), key); err == nil {
			t.Fatalf("expected storage key %q to be gone after delete", key)
		}
	}

	if srv.embeds.exists(pdfID) {
		t.Fatalf("expected embed job for deleted pdf_id=%s to be cancelled", pdfID)
	}
}
