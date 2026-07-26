package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Tag mirrors the tags table.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TagWithCount is a Tag plus how many PDFs currently carry it.
type TagWithCount struct {
	Tag
	Count int `json:"count"`
}

// execer is satisfied by *sql.DB and *sql.Tx — tag lookups and inserts run
// either standalone or inside the transaction that creates a PDF.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// ParseTagString normalizes a raw tag input string into a sorted, deduped,
// lowercase slice of tag names (ver 02-modelo-de-dados.md, "Regra de
// normalização de tags"): split on whitespace, strip '#', '&', '+', dedupe,
// lowercase, sort. The '/' hierarchy separator is preserved.
func ParseTagString(input string) []string {
	replacer := strings.NewReplacer("#", "", "&", "", "+", "")
	seen := make(map[string]struct{})
	var out []string
	for _, field := range strings.Fields(input) {
		name := strings.ToLower(replacer.Replace(field))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ensureTags returns the tag ids for names, creating any tag row that does
// not exist yet (case-insensitive unique on name). Works against either a
// *sql.DB or an in-flight *sql.Tx via the execer interface.
func ensureTags(x execer, names []string) ([]string, error) {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		if _, err := x.Exec(
			`INSERT INTO tags (id, name) VALUES (?, ?) ON CONFLICT(name COLLATE NOCASE) DO NOTHING`,
			id.String(), name,
		); err != nil {
			return nil, err
		}
		var existingID string
		if err := x.QueryRow(`SELECT id FROM tags WHERE name = ? COLLATE NOCASE`, name).Scan(&existingID); err != nil {
			return nil, err
		}
		ids = append(ids, existingID)
	}
	return ids, nil
}

// TagStore provides CRUD over the tags table.
type TagStore struct {
	db *sql.DB
}

// NewTagStore wraps db in a TagStore.
func NewTagStore(db *sql.DB) *TagStore {
	return &TagStore{db: db}
}

// Count returns how many tags exist — used by GET /api/admin/info.
func (s *TagStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&n)
	return n, err
}

// EnsureTags is the standalone (non-transactional) entry point used outside
// PDF creation, e.g. by future callers that need tag ids up front.
func (s *TagStore) EnsureTags(names []string) ([]string, error) {
	return ensureTags(s.db, names)
}

// List returns every tag with how many PDFs currently carry it.
func (s *TagStore) List() ([]TagWithCount, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.name, COUNT(pt.pdf_id)
		FROM tags t
		LEFT JOIN pdf_tags pt ON pt.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TagWithCount
	for rows.Next() {
		var t TagWithCount
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns one tag by id, or ErrNotFound.
func (s *TagStore) Get(id string) (Tag, error) {
	var t Tag
	err := s.db.QueryRow(`SELECT id, name FROM tags WHERE id = ?`, id).Scan(&t.ID, &t.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrNotFound
	}
	return t, err
}

// pdfIDsForTag returns the pdf ids currently associated with tagID —
// captured before a rename/delete/substitute changes them, so the affected
// PDFs can be reindexed in FTS5 afterward.
func pdfIDsForTag(tx *sql.Tx, tagID string) ([]string, error) {
	rows, err := tx.Query(`SELECT pdf_id FROM pdf_tags WHERE tag_id = ?`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// captureOldFTSRows reads the pre-change FTS state for each pdf id, keyed
// by id, skipping any that has no row yet.
func captureOldFTSRows(tx *sql.Tx, pdfIDs []string) (map[string]ftsRow, error) {
	old := make(map[string]ftsRow, len(pdfIDs))
	for _, pid := range pdfIDs {
		row, ok, err := loadFTSRow(tx, pid)
		if err != nil {
			return nil, err
		}
		if ok {
			old[pid] = row
		}
	}
	return old, nil
}

// reindexAffected reindexes every pdf id in old, using its captured
// pre-change state.
func reindexAffected(tx *sql.Tx, old map[string]ftsRow) error {
	for pid, row := range old {
		if err := reindexPDF(tx, pid, row, false); err != nil {
			return err
		}
	}
	return nil
}

// Rename updates a tag's name. Returns ErrConflict if the new name (case
// insensitive) is already taken by another tag. Every PDF carrying this tag
// is reindexed in FTS5 in the same transaction (ver 05-api.md, "Tags").
func (s *TagStore) Rename(id, name string) (Tag, error) {
	if _, err := s.Get(id); err != nil {
		return Tag{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Tag{}, err
	}

	pdfIDs, err := pdfIDsForTag(tx, id)
	if err != nil {
		tx.Rollback()
		return Tag{}, err
	}
	old, err := captureOldFTSRows(tx, pdfIDs)
	if err != nil {
		tx.Rollback()
		return Tag{}, err
	}

	if _, err := tx.Exec(`UPDATE tags SET name = ? WHERE id = ?`, name, id); err != nil {
		tx.Rollback()
		if isUniqueViolation(err) {
			return Tag{}, ErrConflict
		}
		return Tag{}, err
	}

	if err := reindexAffected(tx, old); err != nil {
		tx.Rollback()
		return Tag{}, err
	}

	if err := tx.Commit(); err != nil {
		return Tag{}, err
	}
	return s.Get(id)
}

// Delete removes a tag; pdf_tags rows referencing it cascade automatically.
// Every previously-tagged PDF is reindexed in FTS5 in the same transaction.
func (s *TagStore) Delete(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	pdfIDs, err := pdfIDsForTag(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	old, err := captureOldFTSRows(tx, pdfIDs)
	if err != nil {
		tx.Rollback()
		return err
	}

	if _, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}

	if err := reindexAffected(tx, old); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Substitute moves every PDF association from fromID to toID without
// duplicating a (pdf_id, tag_id) row, then removes fromID (ver 05-api.md,
// "Tags"). Every affected PDF is reindexed in FTS5 in the same transaction.
func (s *TagStore) Substitute(fromID, toID string) error {
	if _, err := s.Get(fromID); err != nil {
		return err
	}
	if _, err := s.Get(toID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	pdfIDs, err := pdfIDsForTag(tx, fromID)
	if err != nil {
		tx.Rollback()
		return err
	}
	old, err := captureOldFTSRows(tx, pdfIDs)
	if err != nil {
		tx.Rollback()
		return err
	}

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO pdf_tags (pdf_id, tag_id) SELECT pdf_id, ? FROM pdf_tags WHERE tag_id = ?`,
		toID, fromID,
	); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, fromID); err != nil {
		tx.Rollback()
		return err
	}

	if err := reindexAffected(tx, old); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
