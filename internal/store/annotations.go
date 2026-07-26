package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Annotation mirrors the pdf_annotations table (kind is 'comment' or
// 'highlight' — the two Django models PdfComment/PdfHighlight merged into
// one table, ver 02-modelo-de-dados.md, decisão 10).
type Annotation struct {
	ID        string
	PDFID     string
	Kind      string
	Page      int
	Text      string
	Note      string
	Color     string
	Rects     string
	CreatedAt string
}

// AnnotationStore provides CRUD and cursor listing over pdf_annotations.
type AnnotationStore struct {
	db *sql.DB
}

// NewAnnotationStore wraps db in an AnnotationStore.
func NewAnnotationStore(db *sql.DB) *AnnotationStore {
	return &AnnotationStore{db: db}
}

// Create inserts a new annotation for pdfID. color must already be
// validated by the caller (handlers_annotations.go); rects is '' for an
// unanchored annotation.
func (s *AnnotationStore) Create(pdfID, kind string, page int, text, note, color, rects string) (Annotation, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Annotation{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(
		`INSERT INTO pdf_annotations (id, pdf_id, kind, page, text, note, color, rects, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), pdfID, kind, page, text, note, color, rects, now,
	)
	if err != nil {
		return Annotation{}, err
	}
	return s.Get(id.String())
}

// Import inserts an annotation with a caller-controlled id and created_at,
// for the one-shot legacy database import (ver ETAPA-12-IMPORTACAO).
func (s *AnnotationStore) Import(id, pdfID, kind string, page int, text, createdAt string) (Annotation, error) {
	if _, err := s.db.Exec(
		`INSERT INTO pdf_annotations (id, pdf_id, kind, page, text, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, pdfID, kind, page, text, createdAt,
	); err != nil {
		return Annotation{}, err
	}
	return s.Get(id)
}

// Get returns one annotation by id, or ErrNotFound.
func (s *AnnotationStore) Get(id string) (Annotation, error) {
	var a Annotation
	err := s.db.QueryRow(
		`SELECT id, pdf_id, kind, page, text, note, color, rects, created_at FROM pdf_annotations WHERE id = ?`, id,
	).Scan(&a.ID, &a.PDFID, &a.Kind, &a.Page, &a.Text, &a.Note, &a.Color, &a.Rects, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Annotation{}, ErrNotFound
	}
	return a, err
}

// Update applies a partial update to an annotation's text and/or note —
// nil fields are left unchanged (ver 05-api.md, "PATCH /api/annotations/{id}").
func (s *AnnotationStore) Update(id string, text, note *string) (Annotation, error) {
	if _, err := s.Get(id); err != nil {
		return Annotation{}, err
	}
	var sets []string
	var args []any
	if text != nil {
		sets = append(sets, "text = ?")
		args = append(args, *text)
	}
	if note != nil {
		sets = append(sets, "note = ?")
		args = append(args, *note)
	}
	if len(sets) > 0 {
		args = append(args, id)
		query := fmt.Sprintf(`UPDATE pdf_annotations SET %s WHERE id = ?`, strings.Join(sets, ", "))
		if _, err := s.db.Exec(query, args...); err != nil {
			return Annotation{}, err
		}
	}
	return s.Get(id)
}

// UpdateAnchor stores the geometry resolved for an annotation that was
// created without one (legacy rows). Returns ErrNotFound if absent.
func (s *AnnotationStore) UpdateAnchor(id, rects string) error {
	res, err := s.db.Exec(`UPDATE pdf_annotations SET rects = ? WHERE id = ?`, rects, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an annotation.
func (s *AnnotationStore) Delete(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM pdf_annotations WHERE id = ?`, id)
	return err
}

// AnnotationListParams filters List; Kind/PDFID empty means "no filter".
type AnnotationListParams struct {
	Kind   string
	PDFID  string
	Cursor string
	Limit  int
}

// List returns a cursor page of annotations ordered newest first (created_at
// DESC, id DESC — the same tuple-comparison pagination as PDFStore.List, ver
// refatoracao/06-frontend.md, "Rolagem infinita").
func (s *AnnotationStore) List(p AnnotationListParams) ([]Annotation, string, error) {
	items, next, err := s.query(p, false)
	return items, next, err
}

// ListAll returns every matching annotation, unpaginated — used by the
// export endpoint (ver 05-api.md, "GET /api/annotations/export").
func (s *AnnotationStore) ListAll(kind, pdfID string) ([]Annotation, error) {
	items, _, err := s.query(AnnotationListParams{Kind: kind, PDFID: pdfID}, true)
	return items, err
}

func (s *AnnotationStore) query(p AnnotationListParams, all bool) ([]Annotation, string, error) {
	var where []string
	var args []any

	if p.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, p.Kind)
	}
	if p.PDFID != "" {
		where = append(where, "pdf_id = ?")
		args = append(args, p.PDFID)
	}
	if p.Cursor != "" {
		val, id, err := decodeCursor(p.Cursor)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		where = append(where, "(created_at, id) < (?, ?)")
		args = append(args, val, id)
	}

	whereSQL := "1=1"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(`SELECT id, pdf_id, kind, page, text, note, color, rects, created_at FROM pdf_annotations WHERE %s ORDER BY created_at DESC, id DESC`, whereSQL)

	limit := p.Limit
	if !all {
		if limit <= 0 {
			limit = 25
		}
		query += " LIMIT ?"
		args = append(args, limit+1)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var items []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.PDFID, &a.Kind, &a.Page, &a.Text, &a.Note, &a.Color, &a.Rects, &a.CreatedAt); err != nil {
			return nil, "", err
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if all {
		return items, "", nil
	}

	var nextCursor string
	if len(items) > limit {
		last := items[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		items = items[:limit]
	}
	return items, nextCursor, nil
}
