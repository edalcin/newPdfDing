package store

import "errors"

// ErrNotFound is returned when a store lookup finds no matching row.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write would violate a uniqueness
// constraint (e.g. a duplicate tag/collection name or PDF sha256) or a
// protected-row rule (e.g. deleting the default collection).
var ErrConflict = errors.New("conflict")
