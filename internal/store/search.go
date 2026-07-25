package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------
// FTS5 index maintenance (ver refatoracao/04-busca-hibrida.md, "Índice
// léxico (FTS5)")
// ---------------------------------------------------------------------

// ftsRow holds the full set of FTS5-indexed values for one PDF, keyed by
// its SQLite rowid — pdfs_fts is contentless, so removing a stale entry
// requires the exact old values, not just the rowid.
type ftsRow struct {
	RowID       int64
	Name        string
	Description string
	Notes       string
	Body        string
	Tags        string
}

// loadFTSRow reads the current indexable state for pdfID: the pdfs columns,
// the extracted text body (pdf_text), and the space-joined tag names.
func loadFTSRow(x execer, pdfID string) (ftsRow, bool, error) {
	var row ftsRow
	err := x.QueryRow(`
		SELECT p.rowid, p.name, p.description, p.notes,
		       COALESCE(pt.body, ''),
		       COALESCE((
		           SELECT group_concat(t.name, ' ')
		           FROM pdf_tags ptag JOIN tags t ON t.id = ptag.tag_id
		           WHERE ptag.pdf_id = p.id
		       ), '')
		FROM pdfs p
		LEFT JOIN pdf_text pt ON pt.pdf_id = p.id
		WHERE p.id = ?`, pdfID,
	).Scan(&row.RowID, &row.Name, &row.Description, &row.Notes, &row.Body, &row.Tags)
	if errors.Is(err, sql.ErrNoRows) {
		return ftsRow{}, false, nil
	}
	if err != nil {
		return ftsRow{}, false, err
	}
	return row, true, nil
}

// deleteFTSRow issues the FTS5 'delete' command for a contentless table,
// which requires the row's OLD values (ver 04-busca-hibrida.md,
// "Reindexação").
func deleteFTSRow(x execer, old ftsRow) error {
	_, err := x.Exec(
		`INSERT INTO pdfs_fts(pdfs_fts, rowid, name, description, notes, body, tags) VALUES ('delete', ?, ?, ?, ?, ?, ?)`,
		old.RowID, old.Name, old.Description, old.Notes, old.Body, old.Tags,
	)
	return err
}

// insertFTSRow inserts the current values into the FTS index.
func insertFTSRow(x execer, row ftsRow) error {
	_, err := x.Exec(
		`INSERT INTO pdfs_fts(rowid, name, description, notes, body, tags) VALUES (?, ?, ?, ?, ?, ?)`,
		row.RowID, row.Name, row.Description, row.Notes, row.Body, row.Tags,
	)
	return err
}

// reindexPDF replaces pdfID's FTS entry with its current state: delete the
// entry described by old (its pre-mutation values), then — unless the PDF
// was just deleted — insert its fresh state. Must run inside the same
// transaction as the write that changed the indexed content.
func reindexPDF(x execer, pdfID string, old ftsRow, deleted bool) error {
	if err := deleteFTSRow(x, old); err != nil {
		return err
	}
	if deleted {
		return nil
	}
	fresh, ok, err := loadFTSRow(x, pdfID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return insertFTSRow(x, fresh)
}

// RebuildFTS fully rebuilds pdfs_fts from the current state of pdfs,
// pdf_text and pdf_tags — run once at boot (ver 04-busca-hibrida.md,
// "Rebuild total no boot"; 01-arquitetura.md, "Mecanismo de migração").
func RebuildFTS(db *sql.DB) error {
	if _, err := db.Exec(`INSERT INTO pdfs_fts(pdfs_fts) VALUES ('delete-all')`); err != nil {
		return fmt.Errorf("store.RebuildFTS delete-all: %w", err)
	}
	_, err := db.Exec(`
		INSERT INTO pdfs_fts(rowid, name, description, notes, body, tags)
		SELECT p.rowid, p.name, p.description, p.notes,
		       COALESCE(pt.body, ''),
		       COALESCE(tl.tags, '')
		FROM pdfs p
		LEFT JOIN pdf_text pt ON pt.pdf_id = p.id
		LEFT JOIN (
		  SELECT pdf_tags.pdf_id, group_concat(tags.name, ' ') AS tags
		  FROM pdf_tags JOIN tags ON tags.id = pdf_tags.tag_id
		  GROUP BY pdf_tags.pdf_id
		) tl ON tl.pdf_id = p.id`)
	if err != nil {
		return fmt.Errorf("store.RebuildFTS insert: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------
// Lexical search (ver 04-busca-hibrida.md, "Consulta léxica")
// ---------------------------------------------------------------------

const lexicalCandidateLimit = 100

// SearchLexical returns up to 100 pdf ids ranked by FTS5 relevance (best
// first). The query is wrapped in double quotes so FTS5 syntax characters
// in free-form user input never break the query. If FTS5 returns zero rows
// or a syntax error, it falls back to a plain LIKE scan on name/description.
func SearchLexical(db *sql.DB, query string) ([]string, error) {
	matchQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`

	ids, err := ftsQuery(db, matchQuery)
	if err == nil && len(ids) > 0 {
		return ids, nil
	}

	like := "%" + query + "%"
	rows, err := db.Query(
		`SELECT id FROM pdfs WHERE name LIKE ? OR description LIKE ? LIMIT ?`,
		like, like, lexicalCandidateLimit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func ftsQuery(db *sql.DB, matchQuery string) ([]string, error) {
	rows, err := db.Query(`
		SELECT p.id
		FROM pdfs_fts
		JOIN pdfs p ON p.rowid = pdfs_fts.rowid
		WHERE pdfs_fts MATCH ?
		ORDER BY pdfs_fts.rank
		LIMIT ?`, matchQuery, lexicalCandidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------
// RRF fusion (ver 04-busca-hibrida.md, "Fusão RRF")
// ---------------------------------------------------------------------

const rrfK = 60.0

// FuseRRF combines two rank-ordered id lists (best first) via Reciprocal
// Rank Fusion and returns up to limit ids, best first. Ties break by id
// ascending for determinism. A missing/empty semantic list degrades the
// result to exactly the lexical order — no special-casing required.
func FuseRRF(lexical, semantic []string, limit int) []string {
	score := make(map[string]float64, len(lexical)+len(semantic))
	for rank, id := range lexical {
		score[id] += 1.0 / (rrfK + float64(rank+1))
	}
	for rank, id := range semantic {
		score[id] += 1.0 / (rrfK + float64(rank+1))
	}

	ids := make([]string, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})

	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}
