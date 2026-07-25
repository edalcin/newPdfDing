package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidKey is returned when a key would resolve outside the backend
// root — path traversal, absolute paths, or a Windows-style separator (ver
// refatoracao/03-storage.md, "Proteção contra path traversal").
var ErrInvalidKey = errors.New("storage: invalid key")

// LocalBackend stores files on the local filesystem under root ($FILES).
// It implements both Backend and Seeker.
type LocalBackend struct {
	root string
}

// NewLocalBackend returns a LocalBackend rooted at root.
func NewLocalBackend(root string) *LocalBackend {
	return &LocalBackend{root: filepath.Clean(root)}
}

func (b *LocalBackend) Name() string { return "local" }

// resolve validates key and returns its absolute filesystem path. Every key
// goes through the same three steps before any I/O touches disk (ver
// 03-storage.md): join with root, clean, and require root as a strict
// prefix of the result. A leading check rejects any '\' in the key outright
// — Go's path/filepath does not treat '\' as a separator on non-Windows
// runtimes, so a Linux build would otherwise accept it as a literal (and
// misleading) filename character instead of the traversal attempt it is.
func (b *LocalBackend) resolve(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, '\\') || strings.ContainsRune(key, 0) {
		return "", ErrInvalidKey
	}
	// Keys are always POSIX-relative under root. filepath.Join would merely
	// neutralise a leading '/' by concatenation (never escape root through
	// it), but the contract in 03-storage.md is to reject absolute keys
	// outright, not silently rewrite them.
	if strings.HasPrefix(key, "/") || filepath.IsAbs(key) {
		return "", ErrInvalidKey
	}

	joined := filepath.Join(b.root, key)
	clean := filepath.Clean(joined)

	if clean != b.root && !strings.HasPrefix(clean, b.root+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	return clean, nil
}

// Put creates the parent directory tree and writes body to key.
func (b *LocalBackend) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("storage/local mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage/local create: %w", err)
	}
	if _, err := io.Copy(f, body); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("storage/local write: %w", err)
	}
	return f.Close()
}

// Get opens key and returns its content and size. The caller must close it.
func (b *LocalBackend) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := b.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("storage/local open: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("storage/local stat: %w", err)
	}
	return f, fi.Size(), nil
}

// OpenSeek opens key for positional (Range) reads, per the Seeker interface.
func (b *LocalBackend) OpenSeek(_ context.Context, key string) (io.ReadSeekCloser, int64, error) {
	path, err := b.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("storage/local open: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("storage/local stat: %w", err)
	}
	return f, fi.Size(), nil
}

// Delete removes key, then walks up removing empty parent directories one
// level at a time, stopping at (never removing) root.
func (b *LocalBackend) Delete(_ context.Context, key string) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("storage/local remove: %w", err)
	}

	for dir := filepath.Dir(path); dir != b.root; dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			break // not empty, or some other error — stop climbing
		}
	}
	return nil
}

// List returns every key under root whose path starts with prefix (empty
// prefix lists everything).
func (b *LocalBackend) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.Walk(b.root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.root, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage/local list: %w", err)
	}
	return keys, nil
}
