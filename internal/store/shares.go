package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Share mirrors the shares table. Its id IS the public URL secret (ver
// 02-modelo-de-dados.md, "shares" — "id é o próprio segredo da URL
// pública").
type Share struct {
	ID        string
	PDFID     string
	Views     int
	CreatedAt string
}

// ShareWithPDFName joins in the shared PDF's name for GET /api/shares.
type ShareWithPDFName struct {
	Share
	PDFName string
}

// ShareStore provides CRUD over the shares table.
type ShareStore struct {
	db *sql.DB
}

// NewShareStore wraps db in a ShareStore.
func NewShareStore(db *sql.DB) *ShareStore {
	return &ShareStore{db: db}
}

// Create shares pdfID publicly. Returns ErrConflict if the PDF already has
// an active share (shares.pdf_id is UNIQUE — ver 05-api.md, "Compartilhamento").
func (s *ShareStore) Create(pdfID string) (Share, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Share{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(`INSERT INTO shares (id, pdf_id, views, created_at) VALUES (?, ?, 0, ?)`, id.String(), pdfID, now)
	if isUniqueViolation(err) {
		return Share{}, ErrConflict
	}
	if err != nil {
		return Share{}, err
	}
	return s.Get(id.String())
}

// Get returns one share by id (the public secret), or ErrNotFound.
func (s *ShareStore) Get(id string) (Share, error) {
	var sh Share
	err := s.db.QueryRow(`SELECT id, pdf_id, views, created_at FROM shares WHERE id = ?`, id).
		Scan(&sh.ID, &sh.PDFID, &sh.Views, &sh.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	return sh, err
}

// GetByPDFID returns the share for pdfID, or ErrNotFound if it isn't shared.
func (s *ShareStore) GetByPDFID(pdfID string) (Share, error) {
	var sh Share
	err := s.db.QueryRow(`SELECT id, pdf_id, views, created_at FROM shares WHERE pdf_id = ?`, pdfID).
		Scan(&sh.ID, &sh.PDFID, &sh.Views, &sh.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	return sh, err
}

// DeleteByPDFID revokes pdfID's share. Returns ErrNotFound if it wasn't shared.
func (s *ShareStore) DeleteByPDFID(pdfID string) error {
	res, err := s.db.Exec(`DELETE FROM shares WHERE pdf_id = ?`, pdfID)
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

// RecordView increments a share's view counter (ver 05-api.md, "GET
// /api/shared/{share_id}" — "incrementa views").
func (s *ShareStore) RecordView(id string) error {
	_, err := s.db.Exec(`UPDATE shares SET views = views + 1 WHERE id = ?`, id)
	return err
}

// List returns every active share with its PDF's name, newest first.
func (s *ShareStore) List() ([]ShareWithPDFName, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.pdf_id, s.views, s.created_at, p.name
		FROM shares s
		JOIN pdfs p ON p.id = s.pdf_id
		ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShareWithPDFName
	for rows.Next() {
		var sh ShareWithPDFName
		if err := rows.Scan(&sh.ID, &sh.PDFID, &sh.Views, &sh.CreatedAt, &sh.PDFName); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}
