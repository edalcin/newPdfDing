package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation guards the
// invariant behind Stats()/attachEmbeddingStatus's SQL-side
// substr(body, 1, embedBodyChars): SQLite's substr() counts CHARACTERS
// while buildEmbedText's subsequent truncation cuts by BYTES. Every UTF-8
// character is >= 1 byte, so embedBodyChars characters contain >=
// embedBodyChars bytes, and Go's byte-truncation of that SQL result lands
// on the identical prefix as truncating the untouched full body — same
// hash either way. The body below mixes accented Portuguese and an emoji
// (2-4 byte UTF-8 sequences) to exercise the byte/char gap; a purely-ASCII
// body would pass trivially even if the invariant were broken.
func TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	name, description := "Relatório Anual", "Contém acentuação e emoji"
	unit := "café, ação, não, São Paulo, coração 🎉🚀🐍 "
	var sb strings.Builder
	for sb.Len() < embedBodyChars*2 {
		sb.WriteString(unit)
	}
	fullBody := sb.String()

	const model = "models/gemini-embedding-2"
	s := NewPDFStore(db, func() string { return model })

	pdf, err := s.Create(CreateParams{
		ID:          "01911111-1111-7111-8111-111111111111",
		Name:        name,
		Description: description,
		StorageKey:  "k",
		SHA256:      "deadbeef",
		Text:        fullBody,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The hash a real embed call would have stored: computed once, from
	// the full (untruncated) body, exactly as handlers_ai.go's caller does.
	hashFromFullBody := contentHash(model, buildEmbedText(name, description, fullBody))
	if err := s.UpsertEmbedding(pdf.ID, hashFromFullBody, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	// attachEmbeddingStatus (paginated IN (...) path, exercised via
	// GetByID) recomputes the hash from substr(body, 1, embedBodyChars).
	got, err := s.GetByID(pdf.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EmbeddingStatus != "current" {
		t.Fatalf("attachEmbeddingStatus: hash via substr(body,1,embedBodyChars) diverged from hash via full-body buildEmbedText; status = %q, want \"current\"", got.EmbeddingStatus)
	}

	// Stats (streaming LEFT JOIN path) must derive the identical hash too.
	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.EmbeddingStatusCounts["current"] != 1 || stats.EmbeddingStatusCounts["stale"] != 0 {
		t.Fatalf("Stats: hash via SQL substr diverged from full-body hash; counts = %+v, want current=1 stale=0", stats.EmbeddingStatusCounts)
	}
}
