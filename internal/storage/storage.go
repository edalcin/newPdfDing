// Package storage abstracts file storage behind the Backend interface. It
// knows nothing about the domain — it operates exclusively on a "key
// string" (ver refatoracao/01-arquitetura.md, "Camadas e regra de
// dependência"). This project has a single implementation, LocalBackend
// (ver refatoracao/03-storage.md) — the interface exists to concentrate
// path validation and positional (Range) access in one place, not to allow
// swapping backends.
package storage

import (
	"context"
	"io"
)

// Backend is the exact signature fixed in refatoracao/01-arquitetura.md.
type Backend interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Name() string // "local"
}

// Seeker is implemented by backends that support positional reads, used to
// serve HTTP Range requests via http.ServeContent (ver 03-storage.md,
// "Entrega de arquivos ao browser").
type Seeker interface {
	OpenSeek(ctx context.Context, key string) (io.ReadSeekCloser, int64, error)
}
