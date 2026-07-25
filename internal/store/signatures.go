package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Signature mirrors the signatures table — the Profile.signatures JSON
// field became its own table (ver 02-modelo-de-dados.md, "Campos do
// Profile mapeados para settings").
type Signature struct {
	ID        string
	Name      string
	Data      string // data URL PNG
	CreatedAt string
}

// SignatureStore provides CRUD over the signatures table.
type SignatureStore struct {
	db *sql.DB
}

// NewSignatureStore wraps db in a SignatureStore.
func NewSignatureStore(db *sql.DB) *SignatureStore {
	return &SignatureStore{db: db}
}

// List returns every signature, newest first.
func (s *SignatureStore) List() ([]Signature, error) {
	rows, err := s.db.Query(`SELECT id, name, data, created_at FROM signatures ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Signature
	for rows.Next() {
		var sig Signature
		if err := rows.Scan(&sig.ID, &sig.Name, &sig.Data, &sig.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sig)
	}
	return out, rows.Err()
}

// Create inserts a new signature.
func (s *SignatureStore) Create(name, data string) (Signature, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Signature{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	if _, err := s.db.Exec(
		`INSERT INTO signatures (id, name, data, created_at) VALUES (?, ?, ?, ?)`,
		id.String(), name, data, now,
	); err != nil {
		return Signature{}, err
	}
	return s.Get(id.String())
}

// Get returns one signature by id, or ErrNotFound.
func (s *SignatureStore) Get(id string) (Signature, error) {
	var sig Signature
	err := s.db.QueryRow(
		`SELECT id, name, data, created_at FROM signatures WHERE id = ?`, id,
	).Scan(&sig.ID, &sig.Name, &sig.Data, &sig.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Signature{}, ErrNotFound
	}
	return sig, err
}

// Delete removes a signature.
func (s *SignatureStore) Delete(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM signatures WHERE id = ?`, id)
	return err
}
