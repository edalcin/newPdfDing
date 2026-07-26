package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestETAPA10_AdminInfoAndReindex covers: GET /api/admin/info reports
// accurate counts and disk usage, and POST /api/admin/reindex rebuilds the
// FTS5 index without touching embeddings (ver 05-api.md, "Admin").
func TestETAPA10_AdminInfoAndReindex(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	uploadPDF(t, client, ts.URL, "Report One", "quarterly report body")
	uploadPDF(t, client, ts.URL, "Report Two", "annual report body")

	resp, body := doJSON(t, client, ts.URL, http.MethodGet, "/api/admin/info", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/info: %d %s", resp.StatusCode, body)
	}
	var info struct {
		Version               string         `json:"version"`
		PDFsCount             int            `json:"pdfs_count"`
		TagsCount             int            `json:"tags_count"`
		CollectionsCount      int            `json:"collections_count"`
		FilesBytes            int64          `json:"files_bytes"`
		EmbeddingStatusCounts map[string]int `json:"embedding_status_counts"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, body)
	}
	if info.PDFsCount != 2 {
		t.Fatalf("expected pdfs_count=2, got %d", info.PDFsCount)
	}
	if info.CollectionsCount != 1 {
		t.Fatalf("expected collections_count=1 (default), got %d", info.CollectionsCount)
	}
	if info.FilesBytes <= 0 {
		t.Fatalf("expected files_bytes > 0, got %d", info.FilesBytes)
	}
	if info.EmbeddingStatusCounts["none"] != 2 {
		t.Fatalf("expected 2 pdfs with embedding_status none, got %v", info.EmbeddingStatusCounts)
	}

	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/admin/reindex", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/admin/reindex: %d %s", resp.StatusCode, body)
	}
	var reindexed struct {
		Reindexed bool `json:"reindexed"`
	}
	if err := json.Unmarshal(body, &reindexed); err != nil || !reindexed.Reindexed {
		t.Fatalf("expected reindexed=true, got %s", body)
	}

	// Reindexed FTS5 must still find the just-uploaded content.
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs?q=quarterly", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/pdfs?q=quarterly after reindex: %d %s", resp.StatusCode, body)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(body, &list)
	if len(list.Items) != 1 || list.Items[0]["name"] != "Report One" {
		t.Fatalf("expected reindexed FTS5 to still find Report One, got %s", body)
	}

	// GET /api/admin/info without a session -> 401.
	anon := httpClientFor(ts)
	resp, _ = doJSON(t, anon, ts.URL, http.MethodGet, "/api/admin/info", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", resp.StatusCode)
	}
}
