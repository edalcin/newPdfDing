package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/store"
)

// TestETAPA8_ConsumeWatchDir covers: dropping a PDF in CONSUME_DIR makes it
// appear in GET /api/pdfs and disappear from CONSUME_DIR after a scan cycle.
func TestETAPA8_ConsumeWatchDir(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	filesDir := filepath.Join(dir, "files")
	consumeDir := filepath.Join(dir, "consume")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		t.Fatalf("mkdir files: %v", err)
	}
	if err := os.MkdirAll(consumeDir, 0o750); err != nil {
		t.Fatalf("mkdir consume: %v", err)
	}
	cfg := &config.Config{
		DBPath: filepath.Join(dir, "db.sqlite"), AdminPassword: "correcthorse", Files: filesDir,
		ListenAddr: ":0", SessionIdleMinutes: 43200, MaxUploadMB: 200, EmbedModel: "mock-embed-model",
		ConsumeEnable: true, ConsumeDir: consumeDir, ConsumeInterval: 5, ConsumeSkipExisting: true,
		ConsumeTags: "imported auto",
	}
	srv := New(cfg, db)

	// Drop a PDF in CONSUME_DIR before the first scan cycle.
	pdfPath := filepath.Join(consumeDir, "My Report.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\nwatch-dir import test content"), 0o640); err != nil {
		t.Fatalf("write consume file: %v", err)
	}

	// Run one scan cycle synchronously (StartConsumer's real ticker would
	// make this test slow and flaky).
	srv.consumeOnce(context.Background())

	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed from CONSUME_DIR after import, stat err = %v", pdfPath, err)
	}

	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	resp, body := doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list pdfs failed: %d %s", resp.StatusCode, body)
	}
	var result struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(body, &result)
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 imported pdf, got %d: %s", len(result.Items), body)
	}
	item := result.Items[0]
	if item["name"] != "My Report" {
		t.Fatalf("expected name %q (derived from filename), got %v", "My Report", item["name"])
	}
	tags, _ := item["tags"].([]any)
	tagNames := map[string]bool{}
	for _, tg := range tags {
		tagNames[tg.(map[string]any)["name"].(string)] = true
	}
	if !tagNames["imported"] || !tagNames["auto"] {
		t.Fatalf("expected CONSUME_TAGS applied, got tags: %v", tags)
	}

	// Consuming again (empty dir) is a no-op — no duplicate, no error.
	srv.consumeOnce(context.Background())
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs", nil)
	json.Unmarshal(body, &result)
	if len(result.Items) != 1 {
		t.Fatalf("expected still 1 pdf after empty consume cycle, got %d", len(result.Items))
	}
}

// TestETAPA8_ConsumeDuplicateSkipped covers: a file whose hash already
// exists in the acervo is logged and removed, without creating a new row
// (ver 05-api.md, "Comportamento de duplicidade" — watch-dir row).
func TestETAPA8_ConsumeDuplicateSkipped(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	filesDir := filepath.Join(dir, "files")
	consumeDir := filepath.Join(dir, "consume")
	os.MkdirAll(filesDir, 0o750)
	os.MkdirAll(consumeDir, 0o750)
	cfg := &config.Config{
		DBPath: filepath.Join(dir, "db.sqlite"), AdminPassword: "correcthorse", Files: filesDir,
		ListenAddr: ":0", SessionIdleMinutes: 43200, MaxUploadMB: 200, EmbedModel: "mock-embed-model",
		ConsumeEnable: true, ConsumeDir: consumeDir, ConsumeInterval: 5, ConsumeSkipExisting: true,
	}
	srv := New(cfg, db)

	content := []byte("%PDF-1.7\nduplicate detection content")
	first := filepath.Join(consumeDir, "first.pdf")
	os.WriteFile(first, content, 0o640)
	srv.consumeOnce(context.Background())

	// Same content again under a different filename.
	second := filepath.Join(consumeDir, "second.pdf")
	os.WriteFile(second, content, 0o640)
	srv.consumeOnce(context.Background())

	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("expected duplicate %s to be removed from CONSUME_DIR", second)
	}

	pdfs, _, err := srv.pdfs.List(store.ListParams{Archived: boolPtr(false)})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pdfs) != 1 {
		t.Fatalf("expected exactly 1 pdf row (duplicate not inserted), got %d", len(pdfs))
	}
}

func boolPtr(b bool) *bool { return &b }
