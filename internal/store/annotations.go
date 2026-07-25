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

// Create inserts a new annotation for pdfID.
func (s *AnnotationStore) Create(pdfID, kind string, page int, text string) (Annotation, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Annotation{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(
		`INSERT INTO pdf_annotations (id, pdf_id, kind, page, text, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id.String(), pdfID, kind, page, text, now,
	)
	if err != nil {
		return Annotation{}, err
	}
	return s.Get(id.String())
}

// Get returns one annotation by id, or ErrNotFound.
func (s *AnnotationStore) Get(id string) (Annotation, error) {
	var a Annotation
	err := s.db.QueryRow(
		`SELECT id, pdf_id, kind, page, text, created_at FROM pdf_annotations WHERE id = ?`, id,
	).Scan(&a.ID, &a.PDFID, &a.Kind, &a.Page, &a.Text, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Annotation{}, ErrNotFound
	}
	return a, err
}

// UpdateText updates an annotation's text.
func (s *AnnotationStore) UpdateText(id, text string) (Annotation, error) {
	if _, err := s.Get(id); err != nil {
		return Annotation{}, err
	}
	if _, err := s.db.Exec(`UPDATE pdf_annotations SET text = ? WHERE id = ?`, text, id); err != nil {
		return Annotation{}, err
	}
	return s.Get(id)
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

	query := fmt.Sprintf(`SELECT id, pdf_id, kind, page, text, created_at FROM pdf_annotations WHERE %s ORDER BY created_at DESC, id DESC`, whereSQL)

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
		if err := rows.Scan(&a.ID, &a.PDFID, &a.Kind, &a.Page, &a.Text, &a.CreatedAt); err != nil {
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
