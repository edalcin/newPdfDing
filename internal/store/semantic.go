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
	"sort"
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
func dotProduct(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := range n {
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
func (s *PDFStore) EmbedModel() string { return s.embedModel }

// BuildEmbedText and ContentHash expose the pure functions above for the
// embed handler (ver handlers_search.go).
func BuildEmbedText(name, description, body string) string {
	return buildEmbedText(name, description, body)
}
func ContentHash(embedModel, text string) string { return contentHash(embedModel, text) }

// attachEmbeddingStatus derives embedding_status for each PDF in items (ver
// 04-busca-hibrida.md, "Estado de embedding") and sets p.EmbeddingStatus.
// The hash is recomputed on every read, never cached — an accepted cost.
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
	rows, err := s.db.Query(fmt.Sprintf(`SELECT pdf_id, body FROM pdf_text WHERE pdf_id IN (%s)`, placeholders), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return err
		}
		bodies[id] = body
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
		current := contentHash(s.embedModel, buildEmbedText(p.Name, p.Description, bodies[p.ID]))
		if storedHash == current {
			p.EmbeddingStatus = "current"
		} else {
			p.EmbeddingStatus = "stale"
		}
	}
	return nil
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
// Gemini embedding API client (ver 04-busca-hibrida.md, "Chamada à API
// Gemini")
// ---------------------------------------------------------------------

// GeminiClient calls the Gemini batchEmbedContents endpoint with a single
// text per call — there is no batch embedding path in this product (ver
// "Sem worker, sem automatismo").
type GeminiClient struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// BaseURL overrides the Gemini endpoint host — used to point at a mock
	// server in tests. Defaults to the real API when empty.
	BaseURL string
}

const geminiBaseURL = "https://generativelanguage.googleapis.com"

// NewGeminiClient builds a client with a bounded request timeout. Returns
// nil if apiKey is empty — callers check for nil to short-circuit with 412.
func NewGeminiClient(apiKey, model string) *GeminiClient {
	if apiKey == "" {
		return nil
	}
	return &GeminiClient{
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    geminiBaseURL,
	}
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

// Embed returns the raw (not yet normalized) embedding vector for text.
func (c *GeminiClient) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := geminiBatchRequest{
		Requests: []geminiEmbedRequest{
			{Model: c.Model, Content: geminiEmbedContent{Parts: []geminiEmbedPart{{Text: text}}}},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	base := c.BaseURL
	if base == "" {
		base = geminiBaseURL
	}
	url := fmt.Sprintf("%s/v1beta/%s:batchEmbedContents?key=%s", base, c.Model, c.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini embed: status %d: %s", resp.StatusCode, string(body))
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
