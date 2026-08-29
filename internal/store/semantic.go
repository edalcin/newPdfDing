package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// embedBodyChars caps how much of the extracted PDF text feeds the
// embedding call — larger than pkd's equivalent because the body here is a
// whole extracted PDF, not a short note (ver 04-busca-hibrida.md,
// "Embeddings sob demanda").
const embedBodyChars = 2000

// buildEmbedText assembles the fixed input to the embedding API.
func buildEmbedText(name, description, body string) string {
	if len(body) > embedBodyChars {
		body = body[:embedBodyChars]
	}
	return name + "\n" + description + "\n" + body
}

// contentHash is the fingerprint stored alongside the vector and recomputed
// on every read to derive embedding_status (ver 04-busca-hibrida.md, "Hash
// de conteúdo").
func contentHash(embedModel, text string) string {
	sum := sha256.Sum256([]byte(embedModel + "\x00" + text))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------
// Vector encode/decode + cosine (ver 04-busca-hibrida.md, "Armazenamento
// vetorial")
// ---------------------------------------------------------------------

// encodeEmbedding packs float32 values little-endian into a BLOB. The
// caller must normalizeL2 first — normalization happens at write time so
// query-time cosine is a plain dot product.
func encodeEmbedding(vec []float32) []byte {
	buf := make([]byte, 4*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeEmbedding(blob []byte) []float32 {
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec
}

// normalizeL2 returns vec scaled to unit length. A zero vector is returned
// unchanged (avoids a division by zero for a degenerate embedding).
func normalizeL2(vec []float32) []float32 {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	if sumSq == 0 {
		return vec
	}
	norm := float32(math.Sqrt(sumSq))
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = v / norm
	}
	return out
}

// dotProduct is the cosine similarity of two already-L2-normalized vectors.
// Vectors of different lengths come from different embedding models (or a
// different outputDimensionality) and are not comparable at all: a partial
// dot product over the common prefix is a meaningless score that can still
// clear semanticFloor, so it is reported as 0 instead. Those rows are
// stale by content_hash and disappear once re-embedded.
func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// ---------------------------------------------------------------------
// pdf_embeddings access
// ---------------------------------------------------------------------

// EmbeddingInfo is the persisted state of a PDF's vector, without the
// vector itself (used for the embedding_status derivation).
type EmbeddingInfo struct {
	ContentHash string
	CreatedAt   string
}

// GetEmbeddingInfo returns the stored content_hash/created_at for pdfID, or
// ok=false if the PDF has never been embedded.
func (s *PDFStore) GetEmbeddingInfo(pdfID string) (EmbeddingInfo, bool, error) {
	var info EmbeddingInfo
	err := s.db.QueryRow(`SELECT content_hash, created_at FROM pdf_embeddings WHERE pdf_id = ?`, pdfID).
		Scan(&info.ContentHash, &info.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EmbeddingInfo{}, false, nil
	}
	if err != nil {
		return EmbeddingInfo{}, false, err
	}
	return info, true, nil
}

// GetText returns the extracted text body for a PDF (pdf_text.body), or ""
// if none was recorded.
func (s *PDFStore) GetText(pdfID string) (string, error) {
	var body string
	err := s.db.QueryRow(`SELECT body FROM pdf_text WHERE pdf_id = ?`, pdfID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// UpsertEmbedding L2-normalizes vec and stores it with contentHash — the
// only code path in the product that writes to pdf_embeddings (ver
// 04-busca-hibrida.md, "Sem worker, sem automatismo").
func (s *PDFStore) UpsertEmbedding(pdfID, contentHashHex string, vec []float32) error {
	blob := encodeEmbedding(normalizeL2(vec))
	now := time.Now().UTC().Format(timeFormat)
	_, err := s.db.Exec(`
		INSERT INTO pdf_embeddings (pdf_id, content_hash, embedding, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(pdf_id) DO UPDATE SET content_hash = excluded.content_hash, embedding = excluded.embedding, created_at = excluded.created_at`,
		pdfID, contentHashHex, blob, now,
	)
	return err
}

// EmbedModel returns the configured embedding model, used by handlers to
// build the same text hashed at write time.
func (s *PDFStore) EmbedModel() string { return s.embedModel() }

// BuildEmbedText and ContentHash expose the pure functions above for the
// embed handler (ver handlers_search.go).
func BuildEmbedText(name, description, body string) string {
	return buildEmbedText(name, description, body)
}
func ContentHash(embedModel, text string) string { return contentHash(embedModel, text) }

// attachEmbeddingStatus derives embedding_status for each PDF in items (ver
// 04-busca-hibrida.md, "Estado de embedding") and sets p.EmbeddingStatus.
// The hash is recomputed on every read, never cached — an accepted cost.
// Only the first embedBodyChars BYTES of the body travel from SQLite:
// substr() over CAST(body AS BLOB) counts bytes, matching buildEmbedText's
// byte truncation exactly. Counting characters instead would diverge on any
// body holding a NUL, because SQLite's TEXT substr()/length() stop at the
// first NUL while Go's do not — the vector would be written from 2000 bytes
// and verified against a shorter prefix, leaving the PDF forever "stale".
// A handful of ids at a time (paginated listing) makes the IN (...) clause
// legitimate here.
func (s *PDFStore) attachEmbeddingStatus(items []PDF) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	for i, p := range items {
		ids[i] = p.ID
	}
	placeholders, args := idPlaceholders(ids)

	bodies := make(map[string]string, len(ids))
	bodyArgs := append([]any{embedBodyChars}, args...)
	rows, err := s.db.Query(fmt.Sprintf(`SELECT pdf_id, substr(CAST(body AS BLOB), 1, ?) FROM pdf_text WHERE pdf_id IN (%s)`, placeholders), bodyArgs...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return err
		}
		bodies[id] = string(body)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	hashes := make(map[string]string, len(ids))
	rows, err = s.db.Query(fmt.Sprintf(`SELECT pdf_id, content_hash FROM pdf_embeddings WHERE pdf_id IN (%s)`, placeholders), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			rows.Close()
			return err
		}
		hashes[id] = hash
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for i := range items {
		p := &items[i]
		storedHash, ok := hashes[p.ID]
		if !ok {
			p.EmbeddingStatus = "none"
			continue
		}
		current := contentHash(s.embedModel(), buildEmbedText(p.Name, p.Description, bodies[p.ID]))
		if storedHash == current {
			p.EmbeddingStatus = "current"
		} else {
			p.EmbeddingStatus = "stale"
		}
	}
	return nil
}

// PDFStats is the aggregate view GET /api/admin/info reports (ver
// 05-api.md, "Admin").
type PDFStats struct {
	Total                 int
	EmbeddingStatusCounts map[string]int // keys "none"|"current"|"stale"
}

// Stats computes the total PDF count and the embedding_status breakdown
// across the whole acervo with one streaming query — LEFT JOIN instead of
// the IN (...) clause attachEmbeddingStatus uses, because here every row
// in the table is in scope, never just a page of ~25. Only a running count
// is kept: no body, vector or even a []PDF for the whole acervo is ever
// held in memory at once (ver docs/proximosPassos.md, OOM em ~160 PDFs).
func (s *PDFStore) Stats() (PDFStats, error) {
	rows, err := s.db.Query(`
		SELECT p.name, p.description, substr(CAST(t.body AS BLOB), 1, ?) AS body, e.content_hash
		FROM pdfs p
		LEFT JOIN pdf_text t ON t.pdf_id = p.id
		LEFT JOIN pdf_embeddings e ON e.pdf_id = p.id`, embedBodyChars)
	if err != nil {
		return PDFStats{}, err
	}
	defer rows.Close()

	model := s.embedModel()
	counts := map[string]int{"none": 0, "current": 0, "stale": 0}
	total := 0
	for rows.Next() {
		var name, description string
		var body []byte
		var storedHash sql.NullString
		if err := rows.Scan(&name, &description, &body, &storedHash); err != nil {
			return PDFStats{}, err
		}
		total++
		if !storedHash.Valid {
			counts["none"]++
			continue
		}
		if storedHash.String == contentHash(model, buildEmbedText(name, description, string(body))) {
			counts["current"]++
		} else {
			counts["stale"]++
		}
	}
	if err := rows.Err(); err != nil {
		return PDFStats{}, err
	}
	return PDFStats{Total: total, EmbeddingStatusCounts: counts}, nil
}

// ---------------------------------------------------------------------
// Semantic candidate search (ver 04-busca-hibrida.md, "Piso semântico e
// top-k", "Custo")
// ---------------------------------------------------------------------

const (
	semanticFloor = 0.30
	semanticTopK  = 100
)

// SemanticSearch returns up to semanticTopK pdf ids whose stored embedding
// has cosine similarity >= semanticFloor against queryVec, best first. It
// is a full in-memory scan — no index, no ANN (ver "Custo": acceptable up
// to ~20 000 PDFs).
func SemanticSearch(db *sql.DB, queryVec []float32) ([]string, error) {
	if len(queryVec) == 0 {
		return nil, nil
	}
	q := normalizeL2(queryVec)

	rows, err := db.Query(`SELECT pdf_id, embedding FROM pdf_embeddings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		id    string
		score float64
	}
	var candidates []scored
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		sim := dotProduct(q, decodeEmbedding(blob))
		if sim >= semanticFloor {
			candidates = append(candidates, scored{id, sim})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].id < candidates[j].id
	})
	if len(candidates) > semanticTopK {
		candidates = candidates[:semanticTopK]
	}

	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.id
	}
	return out, nil
}

// ---------------------------------------------------------------------
// Gemini API client (embeddings, listagem de modelos e geração de texto)
// ---------------------------------------------------------------------

// GeminiClient calls the Gemini API (embeddings, model listing, text
// generation) with a single text per embed call — there is no batch
// embedding path in this product (ver "Sem worker, sem automatismo").
type GeminiClient struct {
	APIKey     string
	HTTPClient *http.Client
	// BaseURL overrides the Gemini endpoint host — used to point at a mock
	// server in tests. Defaults to the real API when empty.
	BaseURL string
}

const geminiBaseURL = "https://generativelanguage.googleapis.com"

// NewGeminiClient builds a client with a bounded request timeout. Returns
// nil if apiKey is empty — callers check for nil to short-circuit with 412.
func NewGeminiClient(apiKey string) *GeminiClient {
	if apiKey == "" {
		return nil
	}
	return &GeminiClient{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		BaseURL:    geminiBaseURL,
	}
}

// do issues one authenticated Gemini request. The API key travels in the
// x-goog-api-key header, never in the URL: a transport failure surfaces as
// *url.Error, whose message embeds the URL and would leak the key into logs.
func (c *GeminiClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	base := c.BaseURL
	if base == "" {
		base = geminiBaseURL
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini %s: status %d: %s", path, resp.StatusCode, string(out))
	}
	return out, nil
}

type geminiEmbedPart struct {
	Text string `json:"text"`
}
type geminiEmbedContent struct {
	Parts []geminiEmbedPart `json:"parts"`
}
type geminiEmbedRequest struct {
	Model   string             `json:"model"`
	Content geminiEmbedContent `json:"content"`
}
type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}
type geminiEmbedding struct {
	Values []float32 `json:"values"`
}
type geminiBatchResponse struct {
	Embeddings []geminiEmbedding `json:"embeddings"`
}

// Embed returns the raw (not yet normalized) embedding vector for text,
// using the caller-supplied model (ver config.EmbedModel).
func (c *GeminiClient) Embed(ctx context.Context, model, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	reqBody := geminiBatchRequest{
		Requests: []geminiEmbedRequest{
			{Model: model, Content: geminiEmbedContent{Parts: []geminiEmbedPart{{Text: text}}}},
		},
	}
	body, err := c.do(ctx, http.MethodPost, "/v1beta/"+model+":batchEmbedContents", reqBody)
	if err != nil {
		return nil, err
	}
	var out geminiBatchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("gemini embed: decode response: %w", err)
	}
	if len(out.Embeddings) == 0 {
		return nil, errors.New("gemini embed: empty response")
	}
	return out.Embeddings[0].Values, nil
}

// GeminiModel is one models.list entry reduced to what the settings UI needs.
type GeminiModel struct {
	Name        string `json:"name"`         // "models/gemini-2.5-flash"
	DisplayName string `json:"display_name"` // "Gemini 2.5 Flash"
}

// textModelDenySubstrings drops models that advertise generateContent but
// cannot return the plain prose the description/tag features need (áudio,
// imagem, vídeo, embeddings, answering-only).
var textModelDenySubstrings = []string{"-tts", "-image", "imagen", "veo", "aqa", "embedding"}

type geminiListModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

// isDeniedTextModel reports whether name matches any entry in
// textModelDenySubstrings.
func isDeniedTextModel(name string) bool {
	for _, sub := range textModelDenySubstrings {
		if strings.Contains(name, sub) {
			return true
		}
	}
	return false
}

// ListModels returns every model the API key can see that supports
// generateContent and isn't in the text deny list acima, ordenado por
// Name. O modelo de embedding não é mais escolhido pelo usuário (ver
// config.EmbedModel).
func (c *GeminiClient) ListModels(ctx context.Context) (text []GeminiModel, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	text = []GeminiModel{}

	path := "/v1beta/models?pageSize=1000"
	for range 5 {
		body, doErr := c.do(ctx, http.MethodGet, path, nil)
		if doErr != nil {
			return nil, doErr
		}
		var out geminiListModelsResponse
		if unmarshalErr := json.Unmarshal(body, &out); unmarshalErr != nil {
			return nil, fmt.Errorf("gemini list models: decode response: %w", unmarshalErr)
		}
		for _, m := range out.Models {
			supportsText := false
			for _, method := range m.SupportedGenerationMethods {
				if method == "generateContent" {
					supportsText = true
					break
				}
			}
			if !supportsText || isDeniedTextModel(m.Name) {
				continue
			}
			displayName := m.DisplayName
			if displayName == "" {
				displayName = m.Name
			}
			text = append(text, GeminiModel{Name: m.Name, DisplayName: displayName})
		}
		if out.NextPageToken == "" {
			break
		}
		path = "/v1beta/models?pageSize=1000&pageToken=" + url.QueryEscape(out.NextPageToken)
	}

	sort.Slice(text, func(i, j int) bool { return text[i].Name < text[j].Name })
	return text, nil
}

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}
type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}
type geminiGenerateRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}
type geminiGenerateResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
}

// GenerateText runs model:generateContent with one system instruction and
// one user prompt, returning the concatenated text of the first candidate.
// Não envia thinkingConfig nem responseMimeType: a API rejeita ambos em
// modelos que não os suportam, e o modelo aqui é escolha livre do usuário.
func (c *GeminiClient) GenerateText(ctx context.Context, model, system, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	reqBody := geminiGenerateRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: system}}},
		Contents:          []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig:  geminiGenerationConfig{Temperature: 0.2, MaxOutputTokens: 2048},
	}
	body, err := c.do(ctx, http.MethodPost, "/v1beta/"+model+":generateContent", reqBody)
	if err != nil {
		return "", err
	}
	var out geminiGenerateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("gemini generate: decode response: %w", err)
	}
	if len(out.Candidates) == 0 {
		return "", errors.New("gemini generate: empty response")
	}
	var sb strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	text := sb.String()
	if text == "" {
		return "", fmt.Errorf("gemini generate: empty response (finishReason=%s)", out.Candidates[0].FinishReason)
	}
	return text, nil
}
