package server

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/edalcin/newpdfding/internal/store"
)

// doRaw issues a request with a raw binary body (used for the restore
// upload, mirroring the PUT-file convention — ver handlePutPDFFile).
func doRaw(t *testing.T, client *http.Client, base, method, path string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for _, c := range client.Jar.Cookies(req.URL) {
		if c.Name == "csrf" {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody
}

// TestETAPA13_AdminBackupAndRestore covers: GET /api/admin/backup streams a
// coherent SQLite snapshot (VACUUM INTO) containing the uploaded PDF's
// metadata; POST /api/admin/restore rejects a non-SQLite upload and an
// empty-schema SQLite file, then accepts a real backup, swaps the live DB
// file in place, and triggers a restart (captured via the injectable
// s.restart hook instead of actually signalling the test process) (ver
// 05-api.md, "Admin").
func TestETAPA13_AdminBackupAndRestore(t *testing.T) {
	srv, _ := testServer(t, false)
	restarted := make(chan struct{}, 1)
	srv.restart = func() { restarted <- struct{}{} }

	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	doc := uploadPDF(t, client, ts.URL, "Backup Me", "content that must survive a restore")

	// --- Backup ---
	resp, backupBytes := doRaw(t, client, ts.URL, http.MethodGet, "/api/admin/backup", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/backup: %d %s", resp.StatusCode, backupBytes)
	}
	if ct := resp.Header.Get("Content-Disposition"); ct == "" {
		t.Fatalf("expected Content-Disposition on backup response, got none")
	}
	if len(backupBytes) == 0 {
		t.Fatalf("expected non-empty backup body")
	}

	// Independently open the downloaded bytes as SQLite and confirm the
	// uploaded PDF's row is really in there (proves VACUUM INTO produced a
	// coherent, queryable snapshot, not just bytes).
	tmpBackup := filepath.Join(t.TempDir(), "downloaded.db")
	if err := os.WriteFile(tmpBackup, backupBytes, 0o600); err != nil {
		t.Fatalf("write downloaded backup: %v", err)
	}
	verifyDB, err := sql.Open("sqlite", "file:"+tmpBackup+"?mode=ro")
	if err != nil {
		t.Fatalf("open downloaded backup: %v", err)
	}
	var name string
	if err := verifyDB.QueryRow("SELECT name FROM pdfs WHERE id = ?", doc["id"]).Scan(&name); err != nil {
		t.Fatalf("uploaded pdf missing from backup: %v", err)
	}
	verifyDB.Close()
	if name != "Backup Me" {
		t.Fatalf("expected name %q in backup, got %q", "Backup Me", name)
	}

	// --- Restore: reject garbage ---
	resp, body := doRaw(t, client, ts.URL, http.MethodPost, "/api/admin/restore", []byte("not a sqlite file"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for garbage upload, got %d %s", resp.StatusCode, body)
	}

	// --- Restore: reject a valid-but-foreign SQLite file (no newPdfDing tables) ---
	foreignPath := filepath.Join(t.TempDir(), "foreign.db")
	foreignDB, err := sql.Open("sqlite", foreignPath)
	if err != nil {
		t.Fatalf("open foreign db: %v", err)
	}
	if _, err := foreignDB.Exec("CREATE TABLE unrelated (id INTEGER)"); err != nil {
		t.Fatalf("create unrelated table: %v", err)
	}
	foreignDB.Close()
	foreignBytes, err := os.ReadFile(foreignPath)
	if err != nil {
		t.Fatalf("read foreign db: %v", err)
	}
	resp, body = doRaw(t, client, ts.URL, http.MethodPost, "/api/admin/restore", foreignBytes)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for foreign schema, got %d %s", resp.StatusCode, body)
	}

	// --- Restore: accept the real backup ---
	resp, body = doRaw(t, client, ts.URL, http.MethodPost, "/api/admin/restore", backupBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/admin/restore: %d %s", resp.StatusCode, body)
	}

	select {
	case <-restarted:
	default:
		t.Fatalf("expected s.restart to be invoked after a successful restore")
	}

	// The live DB file on disk must now be the restored snapshot: reopen it
	// directly and confirm the PDF row is present.
	reopened, err := store.Open(srv.cfg.DBPath)
	if err != nil {
		t.Fatalf("reopen restored db: %v", err)
	}
	defer reopened.Close()
	if err := reopened.QueryRow("SELECT name FROM pdfs WHERE id = ?", doc["id"]).Scan(&name); err != nil {
		t.Fatalf("restored db missing uploaded pdf: %v", err)
	}
	if name != "Backup Me" {
		t.Fatalf("expected restored name %q, got %q", "Backup Me", name)
	}

	// POST /api/admin/restore without a session -> 401.
	anon := httpClientFor(ts)
	resp, _ = doRaw(t, anon, ts.URL, http.MethodPost, "/api/admin/restore", backupBytes)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 (missing CSRF cookie) without session, got %d", resp.StatusCode)
	}
}
