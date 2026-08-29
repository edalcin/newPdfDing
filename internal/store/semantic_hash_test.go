package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAttachEmbeddingStatusHashSurvivesSQLCharTruncation guards the
// invariant behind Stats()/attachEmbeddingStatus's SQL-side
// substr(CAST(body AS BLOB), 1, embedBodyChars): the SQL truncation must
// cut on the same BYTE boundary buildEmbedText cuts on. The first body
// below mixes accented Portuguese and an emoji (2-4 byte UTF-8 sequences);
// the second holds a NUL byte, which extracted PDF text really does carry
// and which makes SQLite's TEXT substr()/length() stop early while Go's
// byte truncation does not — the divergence that left a real document
// reporting "stale" forever after a successful embed.
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

	// Same shape, but with a NUL early in the text: everything past it is
	// invisible to SQLite's character-counting string functions.
	nulBody := unit + "\x00" + fullBody

	const model = "models/gemini-embedding-2"
	s := NewPDFStore(db, func() string { return model })

	for _, tc := range []struct {
		id, label, body string
	}{
		{"01911111-1111-7111-8111-111111111111", "utf8 multibyte", fullBody},
		{"01911111-1111-7111-8111-111111111112", "body with NUL", nulBody},
	} {
		pdf, err := s.Create(CreateParams{
			ID:          tc.id,
			Name:        name,
			Description: description,
			StorageKey:  "k-" + tc.id,
			SHA256:      "deadbeef" + tc.id,
			Text:        tc.body,
		})
		if err != nil {
			t.Fatalf("%s: Create: %v", tc.label, err)
		}

		// The hash a real embed call stored: computed once, from the full
		// (untruncated) body, exactly as runEmbedJob does.
		hashFromFullBody := contentHash(model, buildEmbedText(name, description, tc.body))
		if err := s.UpsertEmbedding(pdf.ID, hashFromFullBody, []float32{1, 0, 0, 0}); err != nil {
			t.Fatalf("%s: UpsertEmbedding: %v", tc.label, err)
		}

		// attachEmbeddingStatus (paginated IN (...) path, exercised via
		// GetByID) recomputes the hash from the SQL-truncated body.
		got, err := s.GetByID(pdf.ID)
		if err != nil {
			t.Fatalf("%s: GetByID: %v", tc.label, err)
		}
		if got.EmbeddingStatus != "current" {
			t.Fatalf("%s: attachEmbeddingStatus hash diverged from full-body buildEmbedText; status = %q, want \"current\"", tc.label, got.EmbeddingStatus)
		}
	}

	// Stats (streaming LEFT JOIN path) must derive the identical hashes too.
	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.EmbeddingStatusCounts["current"] != 2 || stats.EmbeddingStatusCounts["stale"] != 0 {
		t.Fatalf("Stats: hash via SQL substr diverged from full-body hash; counts = %+v, want current=2 stale=0", stats.EmbeddingStatusCounts)
	}
}
