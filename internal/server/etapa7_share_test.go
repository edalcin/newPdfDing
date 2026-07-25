package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestETAPA7_ShareFlow covers: POST .../share returns a URL; GET
// /api/shared/{share} without any cookie returns 200 and increments views;
// after DELETE the share, GET /api/shared/{share} returns 404.
func TestETAPA7_ShareFlow(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	doc := uploadPDF(t, client, ts.URL, "Shared Doc", "content for sharing")
	pdfID := doc["id"].(string)

	resp, body := doJSON(t, client, ts.URL, http.MethodPost, fmt.Sprintf("/api/pdfs/%s/share", pdfID), nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create share failed: %d %s", resp.StatusCode, body)
	}
	var share struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &share); err != nil {
		t.Fatalf("decode share response: %v", err)
	}
	if share.URL == "" {
		t.Fatalf("expected non-empty url in share response, got: %s", body)
	}
	t.Logf("share url: %s", share.URL)

	// Sharing again -> 409 (already shared).
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, fmt.Sprintf("/api/pdfs/%s/share", pdfID), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 re-sharing, got %d %s", resp.StatusCode, body)
	}

	// Public access WITHOUT any cookie (bare client, no jar reuse of the
	// authenticated session) — proves the route is genuinely unauthenticated.
	publicClient := ts.Client()
	pubResp, err := publicClient.Get(ts.URL + "/api/shared/" + share.ID)
	if err != nil {
		t.Fatalf("public get: %v", err)
	}
	pubBody := make([]byte, 4096)
	n, _ := pubResp.Body.Read(pubBody)
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for public share view, got %d %s", pubResp.StatusCode, pubBody[:n])
	}

	// views incremented to 1 after the one public GET above.
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/shares", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list shares failed: %d %s", resp.StatusCode, body)
	}
	var shares []map[string]any
	json.Unmarshal(body, &shares)
	found := false
	for _, sh := range shares {
		if sh["id"] == share.ID {
			found = true
			if views, ok := sh["views"].(float64); !ok || views != 1 {
				t.Fatalf("expected views=1 after one public GET, got %v", sh["views"])
			}
		}
	}
	if !found {
		t.Fatalf("expected share %s in /api/shares list, got: %s", share.ID, body)
	}

	// Public file download also works unauthenticated.
	pubResp, err = publicClient.Get(ts.URL + "/api/shared/" + share.ID + "/file")
	if err != nil {
		t.Fatalf("public file get: %v", err)
	}
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for public file, got %d", pubResp.StatusCode)
	}

	// Revoke the share.
	resp, body = doJSON(t, client, ts.URL, http.MethodDelete, fmt.Sprintf("/api/pdfs/%s/share", pdfID), nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete share failed: %d %s", resp.StatusCode, body)
	}

	// After DELETE, the public share endpoint returns 404.
	pubResp, err = publicClient.Get(ts.URL + "/api/shared/" + share.ID)
	if err != nil {
		t.Fatalf("public get after delete: %v", err)
	}
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after share revoked, got %d", pubResp.StatusCode)
	}

	// /s/{id} shell route also 404s once revoked.
	pubResp, err = publicClient.Get(ts.URL + "/s/" + share.ID)
	if err != nil {
		t.Fatalf("public shell get after delete: %v", err)
	}
	pubResp.Body.Close()
	if pubResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /s/{id} after revoke, got %d", pubResp.StatusCode)
	}

	// Deleting again -> 404 (nothing to revoke).
	resp, body = doJSON(t, client, ts.URL, http.MethodDelete, fmt.Sprintf("/api/pdfs/%s/share", pdfID), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 deleting already-revoked share, got %d %s", resp.StatusCode, body)
	}
}
