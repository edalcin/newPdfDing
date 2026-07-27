package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestOpen_AddsDataColumnToLegacyDB proves the additive migration (1b) runs
// cleanly against a pre-existing pdf_annotations table that predates the
// `data` column, preserves existing rows, and is idempotent on a second
// Open call against the same (already migrated) file.
func TestOpen_AddsDataColumnToLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE pdf_annotations (
		id TEXT PRIMARY KEY,
		pdf_id TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('comment','highlight')),
		page INTEGER NOT NULL,
		text TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '',
		color TEXT NOT NULL DEFAULT 'yellow',
		rects TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO pdf_annotations (id, pdf_id, kind, page, text, created_at) VALUES ('a1','p1','comment',1,'hello','2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// First Open exercises the ALTER TABLE path.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db.Close()

	// Second Open on the migrated file must be a no-op, not an error.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (idempotency): %v", err)
	}
	defer db2.Close()

	rows, err := db2.Query(`PRAGMA table_info(pdf_annotations)`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "data" {
			found = true
			if notnull != 1 {
				t.Errorf("data column notnull = %d, want 1", notnull)
			}
			if s, ok := dflt.(string); !ok || s != "''" {
				t.Errorf("data column dflt_value = %v, want ''", dflt)
			}
		}
	}
	rows.Close()
	if !found {
		t.Fatal("data column missing after migration")
	}

	var count int
	if err := db2.QueryRow(`SELECT count(*) FROM pdf_annotations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1 (existing row must survive)", count)
	}

	var text, data string
	if err := db2.QueryRow(`SELECT text, data FROM pdf_annotations WHERE id = 'a1'`).Scan(&text, &data); err != nil {
		t.Fatal(err)
	}
	if text != "hello" || data != "" {
		t.Errorf("legacy row = (text=%q, data=%q), want (hello, \"\")", text, data)
	}
}
