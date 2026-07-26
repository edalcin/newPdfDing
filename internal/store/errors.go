package store

import (
	"errors"
	"strings"
)

// ErrNotFound is returned when a store lookup finds no matching row.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write would violate a uniqueness
// constraint (e.g. a duplicate tag name or PDF sha256).
var ErrConflict = errors.New("conflict")

// isUniqueViolation reports whether err came from a SQLite UNIQUE
// constraint. modernc.org/sqlite (the pure-Go driver used here) does not
// expose a typed sentinel like mattn/go-sqlite3's sqlite3.Error, so this
// falls back to matching the driver's error string.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
