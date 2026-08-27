package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// uploadPDFWithText uploads a PDF along with its extracted text, so the
// embedding worker has a body to embed (ver TestETAPA6_EmbedFlow).
func uploadPDFWithText(t *testing.T, client *http.Client, base, name, text string) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", name)
	mw.WriteField("text", text)
	fw, _ := mw.CreateFormFile("file", "doc.pdf")
	fw.Write([]byte("%PDF-1.7\n" + name + "\n" + text))
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, base+"/api/pdfs", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range client.Jar.Cookies(req.URL) {
		if c.Name == "csrf" {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload %q: %v", name, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload %q: %d %s", name, resp.StatusCode, body)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	return out["id"].(string)
}

func embeddingStatus(t *testing.T, client *http.Client, base, id string) string {
	t.Helper()
	resp, body := doJSON(t, client, base, http.MethodGet, "/api/pdfs/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/pdfs/%s: %d %s", id, resp.StatusCode, body)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	return fmt.Sprint(got["embedding_status"])
}

func reembedAll(t *testing.T, client *http.Client, base string) int {
	t.Helper()
	resp, body := doJSON(t, client, base, http.MethodPost, "/api/admin/reembed", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/admin/reembed: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Queued int    `json:"queued"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, body)
	}
	return out.Queued
}

// TestReembedAfterModelSwitch covers the whole guarantee behind switching the
// embedding model in Configurações → IA: every already-embedded document
// becomes stale, POST /api/admin/reembed queues exactly those documents plus
// the never-embedded ones, and after the worker drains the queue every
// document is current again under the new model.
func TestReembedAfterModelSwitch(t *testing.T) {
	srv, _ := testServer(t, true)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	a := uploadPDFWithText(t, client, ts.URL, "Doc A", "engine automobile maintenance manual body")
	b := uploadPDFWithText(t, client, ts.URL, "Doc B", "flour recipe cake baking instructions body")

	// Both start as "none" and get queued by the bulk action.
	if queued := reembedAll(t, client, ts.URL); queued != 2 {
		t.Fatalf("expected 2 queued for never-embedded docs, got %d", queued)
	}
	waitForEmbedDone(t, client, ts.URL, a)
	waitForEmbedDone(t, client, ts.URL, b)
	for _, id := range []string{a, b} {
		if st := embeddingStatus(t, client, ts.URL, id); st != "current" {
			t.Fatalf("expected current after bulk embed, got %q", st)
		}
	}

	// Switching the model invalidates every stored vector: content_hash is
	// keyed on the model name.
	resp, body := doJSON(t, client, ts.URL, http.MethodPatch, "/api/settings",
		map[string]string{"ai.embed_model": "models/gemini-embedding-next"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/settings: %d %s", resp.StatusCode, body)
	}
	for _, id := range []string{a, b} {
		if st := embeddingStatus(t, client, ts.URL, id); st != "stale" {
			t.Fatalf("expected stale after model switch, got %q", st)
		}
	}

	// And the bulk action re-embeds all of them under the new model.
	if queued := reembedAll(t, client, ts.URL); queued != 2 {
		t.Fatalf("expected 2 queued after model switch, got %d", queued)
	}
	waitForEmbedDone(t, client, ts.URL, a)
	waitForEmbedDone(t, client, ts.URL, b)
	for _, id := range []string{a, b} {
		if st := embeddingStatus(t, client, ts.URL, id); st != "current" {
			t.Fatalf("expected current after re-embed, got %q", st)
		}
	}

	// Nothing left to do -> nothing queued.
	if queued := reembedAll(t, client, ts.URL); queued != 0 {
		t.Fatalf("expected 0 queued when everything is current, got %d", queued)
	}
}

// TestReembedWithoutGeminiKey covers the 412 gate: no API key, no bulk
// re-embedding.
func TestReembedWithoutGeminiKey(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	resp, body := doJSON(t, client, ts.URL, http.MethodPost, "/api/admin/reembed", nil)
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 without GEMINI_API_KEY, got %d %s", resp.StatusCode, body)
	}
}
