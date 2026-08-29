package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// TestListFilterByEmbeddingStatus exercises the embedding_status filter of
// List, including its cursor: embedding_status has no column to filter on
// (it is derived from a content hash at read time), so List scans the
// ordered sequence in batches and resumes from the cursor of the last row
// it kept. Limit=1 forces one page per match, which is what breaks if the
// resume cursor is wrong — a repeated or skipped row.
func TestListFilterByEmbeddingStatus(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const model = "models/gemini-embedding-2"
	s := NewPDFStore(db, func() string { return model })

	// Ordem "newest" é por created_at DESC, id DESC; os ids abaixo crescem,
	// então a listagem devolve pdf-5 primeiro e pdf-1 por último.
	want := map[string]string{
		"01911111-1111-7111-8111-000000000001": "current",
		"01911111-1111-7111-8111-000000000002": "none",
		"01911111-1111-7111-8111-000000000003": "stale",
		"01911111-1111-7111-8111-000000000004": "current",
		"01911111-1111-7111-8111-000000000005": "none",
	}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("01911111-1111-7111-8111-00000000000%d", i)
		body := fmt.Sprintf("corpo do documento %d", i)
		pdf, err := s.Create(CreateParams{
			ID:         id,
			Name:       fmt.Sprintf("Documento %d", i),
			StorageKey: "k" + id,
			SHA256:     "sha" + id,
			Text:       body,
		})
		if err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		switch want[id] {
		case "current":
			hash := contentHash(model, buildEmbedText(pdf.Name, pdf.Description, body))
			if err := s.UpsertEmbedding(id, hash, []float32{1, 0, 0, 0}); err != nil {
				t.Fatalf("UpsertEmbedding %s: %v", id, err)
			}
		case "stale":
			if err := s.UpsertEmbedding(id, "hash-de-um-conteudo-antigo", []float32{1, 0, 0, 0}); err != nil {
				t.Fatalf("UpsertEmbedding %s: %v", id, err)
			}
		}
	}

	for _, state := range []string{"none", "current", "stale"} {
		var expected []string
		for i := 5; i >= 1; i-- { // ordem newest: id decrescente
			id := fmt.Sprintf("01911111-1111-7111-8111-00000000000%d", i)
			if want[id] == state {
				expected = append(expected, id)
			}
		}

		// Uma página por vez (Limit=1), seguindo o cursor até o fim.
		var got []string
		cursor := ""
		for range expected {
			items, next, err := s.List(ListParams{Embedding: state, Limit: 1, Cursor: cursor})
			if err != nil {
				t.Fatalf("List %s: %v", state, err)
			}
			if len(items) != 1 {
				t.Fatalf("List %s (cursor %q): esperava 1 item, veio %d", state, cursor, len(items))
			}
			if items[0].EmbeddingStatus != state {
				t.Fatalf("List %s: item %s veio com status %q", state, items[0].ID, items[0].EmbeddingStatus)
			}
			got = append(got, items[0].ID)
			cursor = next
		}
		if fmt.Sprint(got) != fmt.Sprint(expected) {
			t.Fatalf("List %s: paginação devolveu %v, esperava %v", state, got, expected)
		}

		// A página seguinte à última só pode estar vazia — nada repetido.
		items, _, err := s.List(ListParams{Embedding: state, Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatalf("List %s (última página): %v", state, err)
		}
		if len(items) != 0 {
			t.Fatalf("List %s: depois de esgotar os %d itens ainda veio %s", state, len(expected), items[0].ID)
		}
	}

	// Sem filtro, os cinco continuam vindo.
	all, _, err := s.List(ListParams{Limit: 50})
	if err != nil {
		t.Fatalf("List sem filtro: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("List sem filtro: esperava 5 itens, veio %d", len(all))
	}
}
