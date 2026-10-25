package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A rejected PATCH must not persist anything: an invalid color alongside rects
// used to anchor the annotation anyway, so the corrected retry got 409 and the
// note edit was lost.
func TestPatchAnnotation_InvalidColorDoesNotAnchor(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	pdfID := uploadPDF(t, client, ts.URL, "Doc", "content")["id"].(string)
	resp, body := doJSON(t, client, ts.URL, http.MethodPost, "/api/pdfs/"+pdfID+"/annotations",
		map[string]any{"kind": "comment", "page": 1})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create annotation: %d %s", resp.StatusCode, body)
	}
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rects := "[[0.1,0.1,0.2,0.2]]"
	resp, body = doJSON(t, client, ts.URL, http.MethodPatch, "/api/annotations/"+a.ID,
		map[string]any{"rects": rects, "color": "nope", "note": "n"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid color, got %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, ts.URL, http.MethodPatch, "/api/annotations/"+a.ID,
		map[string]any{"rects": rects, "color": "green", "note": "n"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on corrected retry, got %d %s", resp.StatusCode, body)
	}
	var got annotationResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Rects != rects || got.Color != "green" || got.Note != "n" {
		t.Fatalf("retry not fully applied: %+v", got)
	}
}
