package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUploadText covers POST /api/pdfs/{id}/text: a PDF uploaded without a
// "text" field starts with has_text=false; posting text sets has_text=true,
// makes the PDF findable by that text via FTS5, and 404s for an unknown id.
func TestUploadText(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	doc := uploadPDF(t, client, ts.URL, "No Text Yet", "irrelevant description")
	id := doc["id"].(string)
	if doc["has_text"] != false {
		t.Fatalf("expected has_text=false right after upload without a text field, got %v", doc["has_text"])
	}

	// Empty text is rejected.
	resp, body := doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+id+"/text", map[string]string{"text": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty text, got %d %s", resp.StatusCode, body)
	}

	// Unknown pdf id 404s.
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/does-not-exist/text", map[string]string{"text": "hello"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown pdf id, got %d %s", resp.StatusCode, body)
	}

	// Backfilling real text sets has_text and makes it findable via FTS5.
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+id+"/text", map[string]string{"text": "quokkas are marsupials native to western australia"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST .../text: %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pdf: %d %s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["has_text"] != true {
		t.Fatalf("expected has_text=true after backfill, got %v", got["has_text"])
	}

	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs?q=quokkas", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET search: %d %s", resp.StatusCode, body)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("unmarshal search: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0]["id"] != id {
		t.Fatalf("expected the backfilled pdf to be found by its extracted text, got %+v", page.Items)
	}
}
