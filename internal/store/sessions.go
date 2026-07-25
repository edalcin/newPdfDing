package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when a store lookup finds no matching row.
var ErrNotFound = errors.New("not found")

const timeFormat = time.RFC3339Nano

// SessionStore provides CRUD over the sessions table (ver
// refatoracao/08-seguranca.md, "Autenticação").
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore wraps db in a SessionStore.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// Create inserts a new session row for the given token id.
func (s *SessionStore) Create(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := s.db.Exec(`INSERT INTO sessions (id, created_at, last_seen_at) VALUES (?, ?, ?)`, id, now, now)
	return err
}

// Touch reports whether the session exists and has not exceeded
// idleMinutes since its last activity. A live session has its last_seen_at
// slid forward to now (ver 08-seguranca.md, "Expiração"); an expired one is
// deleted immediately instead of waiting for the hourly sweep.
func (s *SessionStore) Touch(id string, idleMinutes int) (bool, error) {
	var lastSeen string
	err := s.db.QueryRow(`SELECT last_seen_at FROM sessions WHERE id = ?`, id).Scan(&lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	seenAt, err := time.Parse(timeFormat, lastSeen)
	if err != nil {
		return false, err
	}
	if time.Since(seenAt) > time.Duration(idleMinutes)*time.Minute {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
		return false, nil
	}

	now := time.Now().UTC().Format(timeFormat)
	if _, err := s.db.Exec(`UPDATE sessions SET last_seen_at = ? WHERE id = ?`, now, id); err != nil {
		return false, err
	}
	return true, nil
}

// Delete removes a session row (logout).
func (s *SessionStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteExpired removes every session whose last_seen_at is older than
// idleMinutes and reports how many rows were deleted. Called hourly from
// main (ver 08-seguranca.md, "Limpeza").
func (s *SessionStore) DeleteExpired(idleMinutes int) (int64, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(idleMinutes) * time.Minute).Format(timeFormat)
	res, err := s.db.Exec(`DELETE FROM sessions WHERE last_seen_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
