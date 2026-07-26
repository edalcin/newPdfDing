package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Collection mirrors the collections table (ver 02-modelo-de-dados.md).
type Collection struct {
	ID          string
	Name        string
	Description string
	IsDefault   bool
	CreatedAt   string
}

// CollectionStore provides CRUD over the collections table.
type CollectionStore struct {
	db *sql.DB
}

// NewCollectionStore wraps db in a CollectionStore.
func NewCollectionStore(db *sql.DB) *CollectionStore {
	return &CollectionStore{db: db}
}

// Count returns how many collections exist — used by GET /api/admin/info.
func (s *CollectionStore) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM collections`).Scan(&n)
	return n, err
}

// EnsureDefault seeds the default collection on boot if none exists yet
// (ver 02-modelo-de-dados.md, "Seeding da coleção padrão no boot"). Called
// once from main after store.Open.
func (s *CollectionStore) EnsureDefault() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM collections WHERE is_default = 1`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(
		`INSERT INTO collections (id, name, description, is_default, created_at) VALUES (?, 'Default', '', 1, ?)`,
		id.String(), now,
	)
	return err
}

// Default returns the collection with is_default = 1.
func (s *CollectionStore) Default() (Collection, error) {
	return s.scanOne(s.db.QueryRow(`SELECT id, name, description, is_default, created_at FROM collections WHERE is_default = 1 LIMIT 1`))
}

// CollectionWithCount is a Collection plus how many PDFs currently belong
// to it — used by GET /api/collections so the UI can show library size per
// collection without a separate round trip.
type CollectionWithCount struct {
	Collection
	PdfCount int
}

// List returns every collection with its PDF count, alphabetically by name.
func (s *CollectionStore) List() ([]CollectionWithCount, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.name, c.description, c.is_default, c.created_at, COUNT(p.id)
		FROM collections c
		LEFT JOIN pdfs p ON p.collection_id = c.id
		GROUP BY c.id
		ORDER BY c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CollectionWithCount
	for rows.Next() {
		var c CollectionWithCount
		var isDefault int
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &isDefault, &c.CreatedAt, &c.PdfCount); err != nil {
			return nil, err
		}
		c.IsDefault = isDefault != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one collection by id, or ErrNotFound.
func (s *CollectionStore) Get(id string) (Collection, error) {
	return s.scanOne(s.db.QueryRow(`SELECT id, name, description, is_default, created_at FROM collections WHERE id = ?`, id))
}

// Exists reports whether a collection with the given id exists.
func (s *CollectionStore) Exists(id string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM collections WHERE id = ?`, id).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create inserts a new collection. Returns ErrConflict if the name is
// already in use (idx_collections_name, case-insensitive).
func (s *CollectionStore) Create(name, description string) (Collection, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Collection{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(
		`INSERT INTO collections (id, name, description, is_default, created_at) VALUES (?, ?, ?, 0, ?)`,
		id.String(), name, description, now,
	)
	if isUniqueViolation(err) {
		return Collection{}, ErrConflict
	}
	if err != nil {
		return Collection{}, err
	}
	return s.Get(id.String())
}

// Import returns the existing collection matching name (case-insensitive —
// e.g. a legacy "Default" workspace collection merges into the seeded
// default collection), or creates one with the given description and
// created_at if none exists yet. Used by the one-shot legacy database
// import (ver ETAPA-12-IMPORTACAO), where every legacy workspace's
// collections collapse into this single-user schema's flat collection list.
func (s *CollectionStore) Import(name, description, createdAt string) (Collection, error) {
	if existing, err := s.findByName(name); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Collection{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Collection{}, err
	}
	_, err = s.db.Exec(
		`INSERT INTO collections (id, name, description, is_default, created_at) VALUES (?, ?, ?, 0, ?)`,
		id.String(), name, description, createdAt,
	)
	if isUniqueViolation(err) {
		// Race with a concurrent Import of the same name — reuse the winner.
		return s.findByName(name)
	}
	if err != nil {
		return Collection{}, err
	}
	return s.Get(id.String())
}

func (s *CollectionStore) findByName(name string) (Collection, error) {
	return s.scanOne(s.db.QueryRow(`SELECT id, name, description, is_default, created_at FROM collections WHERE name = ? COLLATE NOCASE`, name))
}

// Update applies a partial update (nil fields left unchanged). Returns
// ErrNotFound or ErrConflict (duplicate name).
func (s *CollectionStore) Update(id string, name, description *string) (Collection, error) {
	if name != nil {
		if _, err := s.db.Exec(`UPDATE collections SET name = ? WHERE id = ?`, *name, id); err != nil {
			if isUniqueViolation(err) {
				return Collection{}, ErrConflict
			}
			return Collection{}, err
		}
	}
	if description != nil {
		if _, err := s.db.Exec(`UPDATE collections SET description = ? WHERE id = ?`, *description, id); err != nil {
			return Collection{}, err
		}
	}
	return s.Get(id)
}

// Delete removes a non-default collection. Deleting cascades to its PDFs
// (ON DELETE CASCADE), which in turn cascade to pdf_tags, pdf_annotations,
// shares and pdf_embeddings (ver 05-api.md, "Coleções"). Returns
// ErrConflict if the collection is the protected default one.
func (s *CollectionStore) Delete(id string) error {
	c, err := s.Get(id)
	if err != nil {
		return err
	}
	if c.IsDefault {
		return ErrConflict
	}
	_, err = s.db.Exec(`DELETE FROM collections WHERE id = ?`, id)
	return err
}

func (s *CollectionStore) scanOne(row *sql.Row) (Collection, error) {
	var c Collection
	var isDefault int
	err := row.Scan(&c.ID, &c.Name, &c.Description, &isDefault, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, ErrNotFound
	}
	if err != nil {
		return Collection{}, err
	}
	c.IsDefault = isDefault != 0
	return c, nil
}

// isUniqueViolation reports whether err came from a SQLite UNIQUE
// constraint. modernc.org/sqlite (the pure-Go driver used here) does not
// expose a typed sentinel like mattn/go-sqlite3's sqlite3.Error, so this
// falls back to matching the driver's error string.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
