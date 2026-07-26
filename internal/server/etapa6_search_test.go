package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edalcin/newpdfding/internal/config"
	"github.com/edalcin/newpdfding/internal/store"
)

// mockGeminiEmbed deterministically embeds text for the test: shared
// "topic bits" make synonyms (car/automobile/vehicle) land close together
// in cosine space, proving the RRF + cosine wiring works without depending
// on the real Gemini API or its quality.
func mockGeminiEmbed(text string) []float32 {
	vec := make([]float32, 8)
	lower := strings.ToLower(text)
	if strings.Contains(lower, "car") || strings.Contains(lower, "automobile") || strings.Contains(lower, "vehicle") || strings.Contains(lower, "engine") {
		vec[0] = 1.0
	}
	if strings.Contains(lower, "recipe") || strings.Contains(lower, "cake") || strings.Contains(lower, "flour") {
		vec[1] = 1.0
	}
	if strings.Contains(lower, "mountain") || strings.Contains(lower, "hiking") || strings.Contains(lower, "trail") {
		vec[2] = 1.0
	}
	h := fnv.New32a()
	h.Write([]byte(text))
	seed := h.Sum32()
	for i := 3; i < 8; i++ {
		seed = seed*1103515245 + 12345
		vec[i] = float32(seed%1000) / 10000.0
	}
	return vec
}

func newMockGeminiServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Requests []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Requests) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		text := req.Requests[0].Content.Parts[0].Text
		vec := mockGeminiEmbed(text)
		resp := map[string]any{
			"embeddings": []map[string]any{{"values": vec}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// testServer builds a fully wired *Server against a temp SQLite DB and
// FILES dir, with its Gemini client (if withGemini) pointed at a local mock.
func testServer(t *testing.T, withGemini bool) (*Server, *httptest.Server) {
	t.Helper()
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
		ListenAddr: ":0", SessionIdleMinutes: 43200, MaxUploadMB: 200, EmbedModel: "mock-embed-model",
	}
	if withGemini {
		cfg.GeminiAPIKey = "fake-test-key"
	}

	srv := New(cfg, db)

	var mockGemini *httptest.Server
	if withGemini {
		mockGemini = newMockGeminiServer(t)
		t.Cleanup(mockGemini.Close)
		srv.gemini = store.NewGeminiClient(cfg.GeminiAPIKey, cfg.EmbedModel)
		srv.gemini.BaseURL = mockGemini.URL
		srv.gemini.HTTPClient = mockGemini.Client()
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	t.Cleanup(cancelWorker)
	srv.StartEmbedWorker(workerCtx)
	return srv, mockGemini
}

// httpClient returns an http.Client with a cookie jar wired to the TLS test
// server's trusted cert pool — this makes Secure cookies (session, csrf)
// work exactly as a real browser would, unlike a bare http.Client over
// plain HTTP.
func httpClientFor(ts *httptest.Server) *http.Client {
	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	return client
}

func doJSON(t *testing.T, client *http.Client, base, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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

func login(t *testing.T, client *http.Client, base string) {
	t.Helper()
	resp, _ := doJSON(t, client, base, http.MethodGet, "/api/auth/session", nil)
	resp.Body.Close()
	resp, body := doJSON(t, client, base, http.MethodPost, "/api/auth/login", map[string]string{"password": "correcthorse"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d %s", resp.StatusCode, body)
	}
}

func uploadPDF(t *testing.T, client *http.Client, base string, name, content string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", name)
	mw.WriteField("description", content)
	fw, _ := mw.CreateFormFile("file", "doc.pdf")
	fw.Write([]byte("%PDF-1.7\n" + name + "\n" + content))
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
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload %q failed: %d %s", name, resp.StatusCode, body)
	}
	var out map[string]any
	json.Unmarshal(body, &out)
	return out
}

// TestETAPA6_LexicalSearchAndEmbeddingStatusNone covers: 3 PDFs of distinct
// content, none embedded; GET ?q=<term from body> ranks the right one
// first via FTS5 alone; all three report embedding_status "none".
func TestETAPA6_LexicalSearchAndEmbeddingStatusNone(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	uploadPDF(t, client, ts.URL, "Doc A", "this document is about zebras and savanna wildlife")
	uploadPDF(t, client, ts.URL, "Doc B", "this document explains quantum computing basics")
	docC := uploadPDF(t, client, ts.URL, "Doc C", "unique term xylophonemelody appears only here")

	resp, body := doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs?q=xylophonemelody", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search failed: %d %s", resp.StatusCode, body)
	}
	var result struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(body, &result)
	if len(result.Items) == 0 {
		t.Fatalf("expected at least 1 result, got 0: %s", body)
	}
	if result.Items[0]["id"] != docC["id"] {
		t.Fatalf("expected Doc C first, got %v", result.Items[0]["name"])
	}
	for _, item := range result.Items {
		if item["embedding_status"] != "none" {
			t.Fatalf("expected embedding_status=none, got %v for %v", item["embedding_status"], item["name"])
		}
	}
}

// TestETAPA6_EmbedFlow covers: embed one doc -> current for it, none for
// others; PATCH description -> stale; synonym query against the embedded
// doc returns it via semantic search. Embedding is asynchronous (ver
// refatoracao Fase F): POST /embed returns 202 and the job is polled via
// GET /api/embed/jobs until it settles.
func TestETAPA6_EmbedFlow(t *testing.T) {
	srv, _ := testServer(t, true)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	docCar := uploadPDF(t, client, ts.URL, "Automobile Guide", "engine automobile maintenance manual")
	docCake := uploadPDF(t, client, ts.URL, "Baking Guide", "flour recipe cake baking instructions")
	docHike := uploadPDF(t, client, ts.URL, "Trail Guide", "mountain hiking trail safety tips")
	_ = docCake
	_ = docHike

	// Upload with extracted text so /embed has something to work with.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", "Automobile Guide 2")
	mw.WriteField("text", "engine automobile maintenance manual full body text")
	fw, _ := mw.CreateFormFile("file", "doc.pdf")
	fw.Write([]byte("%PDF-1.7\nengine automobile maintenance body unique-content-1"))
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
		t.Fatalf("upload with text failed: %d %s", resp.StatusCode, body)
	}
	var carDoc map[string]any
	json.Unmarshal(body, &carDoc)
	carID := carDoc["id"].(string)

	// Embed the car doc: 202 Accepted, no synchronous 422/200 anymore.
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, fmt.Sprintf("/api/pdfs/%s/embed", carID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("embed enqueue failed: %d %s", resp.StatusCode, body)
	}
	waitForEmbedDone(t, client, ts.URL, carID)

	// GET the embedded doc -> current.
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs/"+carID, nil)
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["embedding_status"] != "current" {
		t.Fatalf("expected current, got %v", got["embedding_status"])
	}

	// The other two docs remain "none" — proving no auto-embed happened.
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs/"+docCar["id"].(string), nil)
	json.Unmarshal(body, &got)
	if got["embedding_status"] != "none" {
		t.Fatalf("expected none for un-embedded upload, got %v", got["embedding_status"])
	}

	// PATCH description -> stale.
	resp, body = doJSON(t, client, ts.URL, http.MethodPatch, "/api/pdfs/"+carID, map[string]string{"description": "brand new description text"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch failed: %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs/"+carID, nil)
	json.Unmarshal(body, &got)
	if got["embedding_status"] != "stale" {
		t.Fatalf("expected stale after PATCH, got %v", got["embedding_status"])
	}

	// Re-embed to get back to current, then search by a synonym not in the
	// text ("car" vs "automobile") — semantic search must surface it.
	resp, body = doJSON(t, client, ts.URL, http.MethodPost, fmt.Sprintf("/api/pdfs/%s/embed", carID), nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("re-embed enqueue failed: %d %s", resp.StatusCode, body)
	}
	waitForEmbedDone(t, client, ts.URL, carID)

	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs?q=car", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("synonym search failed: %d %s", resp.StatusCode, body)
	}
	var result struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(body, &result)
	found := false
	for _, item := range result.Items {
		if item["id"] == carID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected synonym query 'car' to surface the automobile doc via semantic search, got: %s", body)
	}
}

// waitForEmbedDone polls GET /api/pdfs/{id} until embedding_status leaves
// "none"/"stale" (the async worker settled the job) or the timeout elapses.
func waitForEmbedDone(t *testing.T, client *http.Client, base, pdfID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body := doJSON(t, client, base, http.MethodGet, "/api/pdfs/"+pdfID, nil)
		var got map[string]any
		json.Unmarshal(body, &got)
		if got["embedding_status"] == "current" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("embedding job for pdf_id=%s did not complete within the deadline", pdfID)
}

// TestETAPA6_NoGeminiKey covers: without GEMINI_API_KEY, POST .../embed ->
// 412, and lexical search still works.
func TestETAPA6_NoGeminiKey(t *testing.T) {
	srv, _ := testServer(t, false)
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)

	doc := uploadPDF(t, client, ts.URL, "Doc", "searchable content here")

	resp, body := doJSON(t, client, ts.URL, http.MethodPost, fmt.Sprintf("/api/pdfs/%s/embed", doc["id"]), nil)
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 without GEMINI_API_KEY, got %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, client, ts.URL, http.MethodGet, "/api/pdfs?q=searchable", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("text search should still work: %d %s", resp.StatusCode, body)
	}
	var result struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(body, &result)
	if len(result.Items) != 1 || result.Items[0]["id"] != doc["id"] {
		t.Fatalf("expected the doc back from lexical-only search, got: %s", body)
	}
}
