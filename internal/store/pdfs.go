package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidCursor is returned when a pagination cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid cursor")

// PDF mirrors the pdfs table plus its associated tags.
type PDF struct {
	ID            string
	Name          string
	Description   string
	Notes         string
	CollectionID  string
	FileDirectory string
	StorageKey    string
	ThumbnailKey  string
	PreviewKey    string
	SHA256        string
	SizeBytes     int64
	NumPages      int
	CurrentPage   int
	Views         int
	Revision      int
	Starred       bool
	Archived      bool
	CreatedAt     string
	LastViewedAt  sql.NullString
	Tags          []Tag
}

const pdfColumns = `id, name, description, notes, collection_id, file_directory, storage_key, thumbnail_key, preview_key, sha256, size_bytes, num_pages, current_page, views, revision, starred, archived, created_at, last_viewed_at`

func scanPDF(row interface{ Scan(...any) error }) (PDF, error) {
	var p PDF
	var starred, archived int
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.Notes, &p.CollectionID, &p.FileDirectory,
		&p.StorageKey, &p.ThumbnailKey, &p.PreviewKey, &p.SHA256, &p.SizeBytes, &p.NumPages,
		&p.CurrentPage, &p.Views, &p.Revision, &starred, &archived, &p.CreatedAt, &p.LastViewedAt,
	)
	if err != nil {
		return PDF{}, err
	}
	p.Starred = starred != 0
	p.Archived = archived != 0
	return p, nil
}

// PDFStore provides CRUD, listing and cursor pagination over the pdfs table.
type PDFStore struct {
	db *sql.DB
}

// NewPDFStore wraps db in a PDFStore.
func NewPDFStore(db *sql.DB) *PDFStore {
	return &PDFStore{db: db}
}

// CreateParams is the input to Create.
type CreateParams struct {
	ID            string // caller-generated UUIDv7 — storage keys embed it, so it must exist before Create runs
	Name          string
	Description   string
	Notes         string
	CollectionID  string
	FileDirectory string
	StorageKey    string
	ThumbnailKey  string
	PreviewKey    string
	SHA256        string
	SizeBytes     int64
	TagNames      []string
	Text          string // extracted text, optional — empty means no pdf_text row
}

// Create inserts a PDF row, its tag associations and (if provided) its
// extracted text, all in one transaction.
func (s *PDFStore) Create(p CreateParams) (PDF, error) {
	now := time.Now().UTC().Format(timeFormat)

	tx, err := s.db.Begin()
	if err != nil {
		return PDF{}, err
	}

	_, err = tx.Exec(`INSERT INTO pdfs (
		id, name, description, notes, collection_id, file_directory,
		storage_key, thumbnail_key, preview_key, sha256, size_bytes, num_pages,
		current_page, views, revision, starred, archived, created_at, last_viewed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1, 0, 1, 0, 0, ?, NULL)`,
		p.ID, p.Name, p.Description, p.Notes, p.CollectionID, p.FileDirectory,
		p.StorageKey, p.ThumbnailKey, p.PreviewKey, p.SHA256, p.SizeBytes, now,
	)
	if err != nil {
		tx.Rollback()
		if isUniqueViolation(err) {
			return PDF{}, ErrConflict
		}
		return PDF{}, err
	}

	if len(p.TagNames) > 0 {
		tagIDs, err := ensureTags(tx, p.TagNames)
		if err != nil {
			tx.Rollback()
			return PDF{}, err
		}
		for _, tagID := range tagIDs {
			if _, err := tx.Exec(`INSERT INTO pdf_tags (pdf_id, tag_id) VALUES (?, ?)`, p.ID, tagID); err != nil {
				tx.Rollback()
				return PDF{}, err
			}
		}
	}

	if p.Text != "" {
		if _, err := tx.Exec(`INSERT INTO pdf_text (pdf_id, body) VALUES (?, ?)`, p.ID, p.Text); err != nil {
			tx.Rollback()
			return PDF{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return PDF{}, err
	}
	return s.GetByID(p.ID)
}

// GetByID returns one PDF with its tags loaded, or ErrNotFound.
func (s *PDFStore) GetByID(id string) (PDF, error) {
	row := s.db.QueryRow(`SELECT `+pdfColumns+` FROM pdfs WHERE id = ?`, id)
	p, err := scanPDF(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PDF{}, ErrNotFound
	}
	if err != nil {
		return PDF{}, err
	}
	tags, err := s.tagsFor([]string{p.ID})
	if err != nil {
		return PDF{}, err
	}
	p.Tags = tags[p.ID]
	return p, nil
}

// GetBySHA256 returns the PDF matching hash, or ErrNotFound (ver 05-api.md,
// "Comportamento de duplicidade").
func (s *PDFStore) GetBySHA256(hash string) (PDF, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM pdfs WHERE sha256 = ?`, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return PDF{}, ErrNotFound
	}
	if err != nil {
		return PDF{}, err
	}
	return s.GetByID(id)
}

func (s *PDFStore) tagsFor(pdfIDs []string) (map[string][]Tag, error) {
	out := make(map[string][]Tag, len(pdfIDs))
	if len(pdfIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(pdfIDs))
	args := make([]any, len(pdfIDs))
	for i, id := range pdfIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT pt.pdf_id, t.id, t.name
		FROM pdf_tags pt
		JOIN tags t ON t.id = pt.tag_id
		WHERE pt.pdf_id IN (%s)
		ORDER BY t.name COLLATE NOCASE`, strings.Join(placeholders, ","))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pdfID string
		var t Tag
		if err := rows.Scan(&pdfID, &t.ID, &t.Name); err != nil {
			return nil, err
		}
		out[pdfID] = append(out[pdfID], t)
	}
	return out, rows.Err()
}

// sortSpec pairs a sortable ORDER BY expression with its direction and
// whether its cursor value is numeric (views) or textual (everything else).
type sortSpec struct {
	expr  string
	dir   string
	isInt bool
}

var sortSpecs = map[string]sortSpec{
	"newest":          {"created_at", "DESC", false},
	"oldest":          {"created_at", "ASC", false},
	"name_asc":        {"name COLLATE NOCASE", "ASC", false},
	"name_desc":       {"name COLLATE NOCASE", "DESC", false},
	"most_viewed":     {"views", "DESC", true},
	"least_viewed":    {"views", "ASC", true},
	"recently_viewed": {"COALESCE(last_viewed_at, '')", "DESC", false},
}

// ListParams is the input to List. Nil Starred/Archived means "no filter";
// Archived defaults to false at the handler layer (ver handlers_pdfs.go) so
// the default library view excludes archived PDFs.
type ListParams struct {
	CollectionID string
	Tag          string
	Starred      *bool
	Archived     *bool
	Sort         string
	Cursor       string
	Limit        int
}

// List returns a page of PDFs (tags loaded) plus the opaque cursor for the
// next page, or "" if this was the last page. Pagination follows
// refatoracao/06-frontend.md, "Rolagem infinita": a base64url cursor over
// (sort key, id), compared with SQLite row values so results never repeat
// or skip under concurrent inserts.
func (s *PDFStore) List(p ListParams) ([]PDF, string, error) {
	spec, ok := sortSpecs[p.Sort]
	if !ok {
		spec = sortSpecs["newest"]
	}

	var where []string
	var args []any

	if p.CollectionID != "" {
		where = append(where, "collection_id = ?")
		args = append(args, p.CollectionID)
	}
	if p.Tag != "" {
		where = append(where, `id IN (SELECT pt.pdf_id FROM pdf_tags pt JOIN tags t ON t.id = pt.tag_id WHERE t.name = ? COLLATE NOCASE)`)
		args = append(args, p.Tag)
	}
	if p.Starred != nil {
		where = append(where, "starred = ?")
		args = append(args, boolToInt(*p.Starred))
	}
	if p.Archived != nil {
		where = append(where, "archived = ?")
		args = append(args, boolToInt(*p.Archived))
	}

	if p.Cursor != "" {
		val, id, err := decodeCursor(p.Cursor)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		op := "<"
		if spec.dir == "ASC" {
			op = ">"
		}
		where = append(where, fmt.Sprintf("(%s, id) %s (?, ?)", spec.expr, op))
		if spec.isInt {
			n, convErr := strconv.ParseInt(val, 10, 64)
			if convErr != nil {
				return nil, "", ErrInvalidCursor
			}
			args = append(args, n, id)
		} else {
			args = append(args, val, id)
		}
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit+1)

	whereSQL := "1=1"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}
	query := fmt.Sprintf(`SELECT %s FROM pdfs WHERE %s ORDER BY %s %s, id %s LIMIT ?`,
		pdfColumns, whereSQL, spec.expr, spec.dir, spec.dir)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	var items []PDF
	for rows.Next() {
		p, err := scanPDF(rows)
		if err != nil {
			rows.Close()
			return nil, "", err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, "", err
	}
	rows.Close()

	var nextCursor string
	if len(items) > limit {
		last := items[limit-1]
		nextCursor = encodeCursor(cursorValue(spec, last), last.ID)
		items = items[:limit]
	}

	ids := make([]string, len(items))
	for i, p := range items {
		ids[i] = p.ID
	}
	tagsByID, err := s.tagsFor(ids)
	if err != nil {
		return nil, "", err
	}
	for i := range items {
		items[i].Tags = tagsByID[items[i].ID]
	}

	return items, nextCursor, nil
}

func cursorValue(spec sortSpec, p PDF) string {
	switch spec.expr {
	case "created_at":
		return p.CreatedAt
	case "name COLLATE NOCASE":
		return p.Name
	case "views":
		return strconv.Itoa(p.Views)
	case "COALESCE(last_viewed_at, '')":
		if p.LastViewedAt.Valid {
			return p.LastViewedAt.String
		}
		return ""
	default:
		return ""
	}
}

// UpdateParams is a partial update — nil fields are left unchanged.
type UpdateParams struct {
	Name          *string
	Description   *string
	Notes         *string
	Tags          *[]string
	CollectionID  *string
	FileDirectory *string
	Starred       *bool
	Archived      *bool
	CurrentPage   *int
}

// Update applies a partial update to a PDF, replacing its full tag set when
// Tags is non-nil (ver 05-api.md, "PDFs").
func (s *PDFStore) Update(id string, p UpdateParams) (PDF, error) {
	if _, err := s.GetByID(id); err != nil {
		return PDF{}, err
	}

	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	if p.CollectionID != nil {
		add("collection_id", *p.CollectionID)
	}
	if p.FileDirectory != nil {
		add("file_directory", *p.FileDirectory)
	}
	if p.Starred != nil {
		add("starred", boolToInt(*p.Starred))
	}
	if p.Archived != nil {
		add("archived", boolToInt(*p.Archived))
	}
	if p.CurrentPage != nil {
		add("current_page", *p.CurrentPage)
	}

	if len(sets) > 0 {
		args = append(args, id)
		query := fmt.Sprintf(`UPDATE pdfs SET %s WHERE id = ?`, strings.Join(sets, ", "))
		if _, err := s.db.Exec(query, args...); err != nil {
			return PDF{}, err
		}
	}

	if p.Tags != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return PDF{}, err
		}
		if _, err := tx.Exec(`DELETE FROM pdf_tags WHERE pdf_id = ?`, id); err != nil {
			tx.Rollback()
			return PDF{}, err
		}
		if len(*p.Tags) > 0 {
			tagIDs, err := ensureTags(tx, *p.Tags)
			if err != nil {
				tx.Rollback()
				return PDF{}, err
			}
			for _, tagID := range tagIDs {
				if _, err := tx.Exec(`INSERT INTO pdf_tags (pdf_id, tag_id) VALUES (?, ?)`, id, tagID); err != nil {
					tx.Rollback()
					return PDF{}, err
				}
			}
		}
		if err := tx.Commit(); err != nil {
			return PDF{}, err
		}
	}

	return s.GetByID(id)
}

// Delete removes a PDF row (cascading pdf_tags, pdf_annotations, shares,
// pdf_embeddings) and returns the row as it was, so the caller can clean up
// its storage keys.
func (s *PDFStore) Delete(id string) (PDF, error) {
	p, err := s.GetByID(id)
	if err != nil {
		return PDF{}, err
	}
	if _, err := s.db.Exec(`DELETE FROM pdfs WHERE id = ?`, id); err != nil {
		return PDF{}, err
	}
	return p, nil
}

// SetThumbnailKey updates the thumbnail_key of a PDF (ver 05-api.md, "POST
// .../thumbnail" — a browser-generated thumbnail arriving after upload).
func (s *PDFStore) SetThumbnailKey(id, key string) error {
	res, err := s.db.Exec(`UPDATE pdfs SET thumbnail_key = ? WHERE id = ?`, key, id)
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

// RecordView increments views and slides last_viewed_at to now — called
// when the raw file is served (ver 10-inventario-funcionalidades.md,
// "Contador de visualizações").
func (s *PDFStore) RecordView(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := s.db.Exec(`UPDATE pdfs SET views = views + 1, last_viewed_at = ? WHERE id = ?`, now, id)
	return err
}

// Revise increments revision — called by PUT .../file when the PDF content
// itself is replaced (ver 05-api.md, "PUT .../file").
func (s *PDFStore) Revise(id string) (int, error) {
	if _, err := s.db.Exec(`UPDATE pdfs SET revision = revision + 1 WHERE id = ?`, id); err != nil {
		return 0, err
	}
	var revision int
	if err := s.db.QueryRow(`SELECT revision FROM pdfs WHERE id = ?`, id).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, nil
}

// BulkUpdate applies action ("archive"|"unarchive"|"star"|"unstar") to every
// id, returning how many rows were affected.
func (s *PDFStore) BulkUpdate(ids []string, action string) (int64, error) {
	var col string
	var val int
	switch action {
	case "archive":
		col, val = "archived", 1
	case "unarchive":
		col, val = "archived", 0
	case "star":
		col, val = "starred", 1
	case "unstar":
		col, val = "starred", 0
	default:
		return 0, fmt.Errorf("store: unknown bulk action %q", action)
	}
	placeholders, args := idPlaceholders(ids)
	query := fmt.Sprintf(`UPDATE pdfs SET %s = %d WHERE id IN (%s)`, col, val, placeholders)
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BulkDelete removes every PDF in ids and returns their rows (so the caller
// can clean up storage keys) plus how many were actually deleted.
func (s *PDFStore) BulkDelete(ids []string) ([]PDF, error) {
	placeholders, args := idPlaceholders(ids)
	rows, err := s.db.Query(fmt.Sprintf(`SELECT `+pdfColumns+` FROM pdfs WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	var deleted []PDF
	for rows.Next() {
		p, err := scanPDF(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		deleted = append(deleted, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if _, err := s.db.Exec(fmt.Sprintf(`DELETE FROM pdfs WHERE id IN (%s)`, placeholders), args...); err != nil {
		return nil, err
	}
	return deleted, nil
}

func idPlaceholders(ids []string) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return strings.Join(placeholders, ","), args
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
