package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edalcin/newpdfding/internal/store"
)

// TestAIChat covers the Chat do documento contract: the browser-held
// history reaches Gemini as alternating user/model turns with the
// document in the system instruction, the answer comes back as markdown
// plus sanitized HTML, and the input limits reject oversized requests
// before any model call.
func TestAIChat(t *testing.T) {
	srv, _ := testServer(t, false)
	var got struct {
		SystemInstruction struct {
			Parts []struct{ Text string } `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string                  `json:"role"`
			Parts []struct{ Text string } `json:"parts"`
		} `json:"contents"`
	}
	calls := 0
	gem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"**Sim**, na página 3.<script>x</script>"}]}}]}`))
	}))
	defer gem.Close()
	srv.gemini = store.NewGeminiClient("fake-test-key")
	srv.gemini.BaseURL = gem.URL
	srv.gemini.HTTPClient = gem.Client()
	if _, err := srv.settings.Patch(map[string]string{"ai.text_model": "models/fake"}); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	client := httpClientFor(ts)
	login(t, client, ts.URL)
	id := uploadPDF(t, client, ts.URL, "Relatório", "x")["id"].(string)
	if err := srv.pdfs.SetText(id, "conteúdo sobre orquídeas"); err != nil {
		t.Fatal(err)
	}
	path := "/api/pdfs/" + id + "/chat"

	resp, body := doJSON(t, client, ts.URL, http.MethodPost, path, map[string]any{
		"history":  []map[string]string{{"question": "Q1", "answer": "A1"}},
		"question": "Q2",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Answer     string `json:"answer"`
		AnswerHTML string `json:"answer_html"`
		Truncated  bool   `json:"truncated"`
	}
	json.Unmarshal(body, &out)
	if !strings.HasPrefix(out.Answer, "**Sim**") || !strings.Contains(out.AnswerHTML, "<strong>Sim</strong>") || strings.Contains(out.AnswerHTML, "<script") || out.Truncated {
		t.Fatalf("unexpected response: %+v", out)
	}
	roles := []string{}
	for _, c := range got.Contents {
		roles = append(roles, c.Role+":"+c.Parts[0].Text)
	}
	if strings.Join(roles, ",") != "user:Q1,model:A1,user:Q2" {
		t.Fatalf("turns sent to model = %v", roles)
	}
	if !strings.Contains(got.SystemInstruction.Parts[0].Text, "conteúdo sobre orquídeas") {
		t.Fatalf("document text missing from system instruction")
	}

	history := make([]map[string]string, chatMaxExchanges)
	for i := range history {
		history[i] = map[string]string{"question": "q", "answer": "a"}
	}
	for name, payload := range map[string]map[string]any{
		"empty question":    {"question": "  "},
		"question too long": {"question": strings.Repeat("é", chatQuestionMax+1)},
		"too many turns":    {"question": "q", "history": history},
	} {
		resp, body := doJSON(t, client, ts.URL, http.MethodPost, path, payload)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d %s", name, resp.StatusCode, body)
		}
	}
	resp, _ = doJSON(t, client, ts.URL, http.MethodPost, path, map[string]any{
		"question": "q", "history": []map[string]string{{"question": "q", "answer": strings.Repeat("a", chatMaxBodyBytes)}},
	})
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: expected 413, got %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("rejected requests must not reach the model; calls = %d", calls)
	}
}
