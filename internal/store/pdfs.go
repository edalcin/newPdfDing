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
	StorageKey    string
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
	// EmbeddingStatus is derived on every read, never persisted (ver
	// 04-busca-hibrida.md, "Estado de embedding"): "none"|"current"|"stale".
	EmbeddingStatus string
	// HasText is derived on every single-PDF read (GetByID), never
	// persisted — whether pdf_text has a row for this PDF. Drives the
	// viewer's on-open text backfill (ver SetText) for documents that
	// arrived without extracted text (legacy import, watch-dir consumer).
	HasText bool
}

const pdfColumns = `id, name, description, notes, storage_key, preview_key, sha256, size_bytes, num_pages, current_page, views, revision, starred, archived, created_at, last_viewed_at`

func scanPDF(row interface{ Scan(...any) error }) (PDF, error) {
	var p PDF
	var starred, archived int
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.Notes,
		&p.StorageKey, &p.PreviewKey, &p.SHA256, &p.SizeBytes, &p.NumPages,
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
	db         *sql.DB
	embedModel func() string
}

// NewPDFStore wraps db in a PDFStore. embedModel resolves the current
// embedding model on every call (settings can change it at runtime) and is
// used to derive embedding_status on every read (ver 04-busca-hibrida.md).
func NewPDFStore(db *sql.DB, embedModel func() string) *PDFStore {
	return &PDFStore{db: db, embedModel: embedModel}
}

// CreateParams is the input to Create.
type CreateParams struct {
	ID            string // caller-generated UUIDv7 — storage keys embed it, so it must exist before Create runs
	Name          string
	Description   string
	Notes         string
	StorageKey    string
	PreviewKey    string
	SHA256        string
	SizeBytes     int64
	NumPages      int // 0 when unknown (browser uploads don't report it; watch-dir imports do)
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
		id, name, description, notes,
		storage_key, preview_key, sha256, size_bytes, num_pages,
		current_page, views, revision, starred, archived, created_at, last_viewed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, 1, 0, 0, ?, NULL)`,
		p.ID, p.Name, p.Description, p.Notes,
		p.StorageKey, p.PreviewKey, p.SHA256, p.SizeBytes, p.NumPages, now,
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

	fresh, ok, err := loadFTSRow(tx, p.ID)
	if err != nil {
		tx.Rollback()
		return PDF{}, err
	}
	if ok {
		if err := insertFTSRow(tx, fresh); err != nil {
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
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pdf_text WHERE pdf_id = ?)`, p.ID).Scan(&p.HasText); err != nil {
		return PDF{}, err
	}
	batch := []PDF{p}
	if err := s.attachEmbeddingStatus(batch); err != nil {
		return PDF{}, err
	}
	return batch[0], nil
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
	Tags     []string
	Starred  *bool
	Archived *bool
	Sort     string
	Cursor   string
	Limit    int

	// Embedding, quando não vazio ("none"|"current"|"stale"), mantém só os
	// PDFs naquele estado de embedding. É o único filtro que não vira
	// cláusula WHERE: o estado é derivado a cada leitura de um hash de
	// conteúdo (ver semantic.go, attachEmbeddingStatus), não existe coluna
	// para filtrar. Por isso List varre a sequência ordenada em lotes e
	// descarta o que não casa (ver listFilteredByEmbedding).
	Embedding string

	// Query, when non-empty, switches List into hybrid-search mode (ver
	// 04-busca-hibrida.md): Sort/Cursor are ignored, results are ranked by
	// RRF fusion of lexical (FTS5/LIKE) and semantic (QueryVector cosine)
	// candidates, and next_cursor is always "" — the fused result is a
	// single bounded page, not an infinite-scroll sequence.
	Query       string
	QueryVector []float32
}

// List returns a page of PDFs (tags loaded) plus the opaque cursor for the
// next page, or "" if this was the last page. Pagination follows
// refatoracao/06-frontend.md, "Rolagem infinita": a base64url cursor over
// (sort key, id), compared with SQLite row values so results never repeat
// or skip under concurrent inserts.
func (s *PDFStore) List(p ListParams) ([]PDF, string, error) {
	if p.Query != "" {
		return s.searchList(p)
	}
	if p.Embedding != "" {
		return s.listFilteredByEmbedding(p)
	}
	return s.listPage(p)
}

// embeddingBatch is how many rows listFilteredByEmbedding reads per pass
// while looking for enough matches to fill the caller's page.
const embeddingBatch = 200

// listFilteredByEmbedding walks the same ordered sequence listPage
// paginates, in batches, keeping only the rows whose derived
// embedding_status matches p.Embedding. The cursor it returns is the cursor
// of the last row it kept, so the next page resumes exactly after it and
// the infinite scroll contract is unchanged.
func (s *PDFStore) listFilteredByEmbedding(p ListParams) ([]PDF, string, error) {
	spec, ok := sortSpecs[p.Sort]
	if !ok {
		spec = sortSpecs["newest"]
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 25
	}

	batch := p
	batch.Embedding = ""
	batch.Limit = embeddingBatch
	batch.Cursor = p.Cursor

	out := make([]PDF, 0, limit)
	for {
		items, next, err := s.listPage(batch)
		if err != nil {
			return nil, "", err
		}
		for _, item := range items {
			if item.EmbeddingStatus != p.Embedding {
				continue
			}
			out = append(out, item)
			if len(out) == limit {
				return out, encodeCursor(cursorValue(spec, item), item.ID), nil
			}
		}
		if next == "" {
			return out, "", nil
		}
		batch.Cursor = next
	}
}

// listPage is the unfiltered browse page: one SQL query, one cursor.
func (s *PDFStore) listPage(p ListParams) ([]PDF, string, error) {
	spec, ok := sortSpecs[p.Sort]
	if !ok {
		spec = sortSpecs["newest"]
	}

	var where []string
	var args []any

	for _, t := range p.Tags {
		where = append(where, `id IN (SELECT pt.pdf_id FROM pdf_tags pt JOIN tags tg ON tg.id = pt.tag_id WHERE tg.name = ? COLLATE NOCASE)`)
		args = append(args, t)
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
	if err := s.attachEmbeddingStatus(items); err != nil {
		return nil, "", err
	}

	return items, nextCursor, nil
}

// searchList implements the q!="" branch of List: fuse lexical + semantic
// candidates by RRF, apply the same tag/starred/archived filters
// as browse mode on top of the fused order, and return up to Limit (default
// 50) results — a single bounded page, never paginated (ver
// 04-busca-hibrida.md, "Fusão RRF", "Filtros combinados com a busca").
func (s *PDFStore) searchList(p ListParams) ([]PDF, string, error) {
	lexical, err := SearchLexical(s.db, p.Query)
	if err != nil {
		return nil, "", err
	}

	var semantic []string
	if len(p.QueryVector) > 0 {
		semantic, err = SemanticSearch(s.db, p.QueryVector)
		if err != nil {
			return nil, "", err
		}
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}

	fused := FuseRRF(lexical, semantic, 200)
	if len(fused) == 0 {
		return []PDF{}, "", nil
	}

	items, err := s.fetchFilteredOrdered(fused, p, limit)
	if err != nil {
		return nil, "", err
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
	if err := s.attachEmbeddingStatus(items); err != nil {
		return nil, "", err
	}
	// O filtro de embedding é aplicado depois do status derivado, sobre a
	// página única já fundida — mesma semântica de browse, sem cursor.
	if p.Embedding != "" {
		kept := make([]PDF, 0, len(items))
		for _, item := range items {
			if item.EmbeddingStatus == p.Embedding {
				kept = append(kept, item)
			}
		}
		items = kept
	}

	return items, "", nil
}

// fetchFilteredOrdered fetches the pdfs in orderedIDs matching the browse
// filters, then reorders the result to match orderedIDs (SQL IN does not
// preserve order) and cuts it to limit.
func (s *PDFStore) fetchFilteredOrdered(orderedIDs []string, p ListParams, limit int) ([]PDF, error) {
	placeholders, args := idPlaceholders(orderedIDs)
	where := []string{fmt.Sprintf("id IN (%s)", placeholders)}

	for _, t := range p.Tags {
		where = append(where, `id IN (SELECT pt.pdf_id FROM pdf_tags pt JOIN tags tg ON tg.id = pt.tag_id WHERE tg.name = ? COLLATE NOCASE)`)
		args = append(args, t)
	}
	if p.Starred != nil {
		where = append(where, "starred = ?")
		args = append(args, boolToInt(*p.Starred))
	}
	if p.Archived != nil {
		where = append(where, "archived = ?")
		args = append(args, boolToInt(*p.Archived))
	}

	query := fmt.Sprintf(`SELECT %s FROM pdfs WHERE %s`, pdfColumns, strings.Join(where, " AND "))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]PDF)
	for rows.Next() {
		p, err := scanPDF(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		byID[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	items := make([]PDF, 0, limit)
	for _, id := range orderedIDs {
		if p, ok := byID[id]; ok {
			items = append(items, p)
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
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
	if p.Starred != nil {
		add("starred", boolToInt(*p.Starred))
	}
	if p.Archived != nil {
		add("archived", boolToInt(*p.Archived))
	}
	if p.CurrentPage != nil {
		add("current_page", *p.CurrentPage)
	}

	// Reindexing FTS5 needs the OLD indexed values (name/description/notes/
	// tags/body) before this write changes them, and the NEW values after —
	// all inside one transaction (ver 04-busca-hibrida.md, "Reindexação").
	tx, err := s.db.Begin()
	if err != nil {
		return PDF{}, err
	}

	oldRow, hadRow, err := loadFTSRow(tx, id)
	if err != nil {
		tx.Rollback()
		return PDF{}, err
	}

	if len(sets) > 0 {
		args = append(args, id)
		query := fmt.Sprintf(`UPDATE pdfs SET %s WHERE id = ?`, strings.Join(sets, ", "))
		if _, err := tx.Exec(query, args...); err != nil {
			tx.Rollback()
			return PDF{}, err
		}
	}

	if p.Tags != nil {
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
	}

	if hadRow {
		if err := reindexPDF(tx, id, oldRow, false); err != nil {
			tx.Rollback()
			return PDF{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return PDF{}, err
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

	tx, err := s.db.Begin()
	if err != nil {
		return PDF{}, err
	}
	oldRow, hadRow, err := loadFTSRow(tx, id)
	if err != nil {
		tx.Rollback()
		return PDF{}, err
	}
	if _, err := tx.Exec(`DELETE FROM pdfs WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return PDF{}, err
	}
	if hadRow {
		if err := deleteFTSRow(tx, oldRow); err != nil {
			tx.Rollback()
			return PDF{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PDF{}, err
	}
	return p, nil
}

// SetPreviewKey records the storage key of a browser-generated preview PNG
// uploaded after the initial upload (ver 05-api.md, "POST .../preview").
// Returns ErrNotFound if the PDF does not exist.
func (s *PDFStore) SetPreviewKey(id, key string) error {
	res, err := s.db.Exec(`UPDATE pdfs SET preview_key = ? WHERE id = ?`, key, id)
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

// SetText upserts the extracted text body for a PDF and reindexes FTS5 in
// the same transaction (ver 05-api.md, "POST .../text" — the viewer
// backfills text on first open for documents that arrived without it:
// documents migrated from the legacy Django database (one-time import,
// since removed) or the watch-dir consumer's pure-Go extraction gap).
// Returns ErrNotFound if the PDF does not exist.
func (s *PDFStore) SetText(id, text string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	oldRow, hadRow, err := loadFTSRow(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	if !hadRow {
		tx.Rollback()
		return ErrNotFound
	}

	if _, err := tx.Exec(
		`INSERT INTO pdf_text (pdf_id, body) VALUES (?, ?) ON CONFLICT(pdf_id) DO UPDATE SET body = excluded.body`,
		id, text,
	); err != nil {
		tx.Rollback()
		return err
	}

	if err := reindexPDF(tx, id, oldRow, false); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// RecordView increments views and slides last_viewed_at to now — called
// when the raw file is served (ver 10-inventario-funcionalidades.md,
// "Contador de visualizações").
func (s *PDFStore) RecordView(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := s.db.Exec(`UPDATE pdfs SET views = views + 1, last_viewed_at = ? WHERE id = ?`, now, id)
	return err
}

// Revise increments revision and updates sha256/size_bytes — called by
// PUT .../file when the PDF content itself is replaced (ver 05-api.md,
// "PUT .../file").
func (s *PDFStore) Revise(id, sha256 string, sizeBytes int64) (int, error) {
	if _, err := s.db.Exec(
		`UPDATE pdfs SET revision = revision + 1, sha256 = ?, size_bytes = ? WHERE id = ?`,
		sha256, sizeBytes, id,
	); err != nil {
		if isUniqueViolation(err) {
			return 0, ErrConflict
		}
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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	for _, p := range deleted {
		oldRow, hadRow, err := loadFTSRow(tx, p.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if hadRow {
			if err := deleteFTSRow(tx, oldRow); err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM pdfs WHERE id IN (%s)`, placeholders), args...); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
