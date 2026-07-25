package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalBackend_PutGetListOpenSeekDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	b := NewLocalBackend(root)

	if got := b.Name(); got != "local" {
		t.Fatalf("Name() = %q, want %q", got, "local")
	}

	key := "col1/pdf/doc1.pdf"
	content := []byte("%PDF-1.7 fake content for range reads 0123456789")

	if err := b.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "application/pdf"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, size, err := b.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("Get read: %v", err)
	}
	if size != int64(len(content)) || !bytes.Equal(got, content) {
		t.Fatalf("Get content mismatch: size=%d got=%q", size, got)
	}

	keys, err := b.List(ctx, "col1/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("List = %v, want [%s]", keys, key)
	}

	seeker, seekSize, err := b.OpenSeek(ctx, key)
	if err != nil {
		t.Fatalf("OpenSeek: %v", err)
	}
	if seekSize != int64(len(content)) {
		t.Fatalf("OpenSeek size = %d, want %d", seekSize, len(content))
	}
	const offset = 10
	if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	tail, err := io.ReadAll(seeker)
	seeker.Close()
	if err != nil {
		t.Fatalf("read after seek: %v", err)
	}
	if !bytes.Equal(tail, content[offset:]) {
		t.Fatalf("seek read = %q, want %q", tail, content[offset:])
	}

	if err := b.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := b.Get(ctx, key); err == nil {
		t.Fatal("Get after Delete: want error, got nil")
	}
}

func TestLocalBackend_RejectsUnsafeKeys(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	b := NewLocalBackend(root)

	cases := map[string]string{
		"parent traversal":   "../outside.pdf",
		"nested traversal":   "col1/../../outside.pdf",
		"absolute unix path": "/etc/passwd",
		"windows separator":  `col1\pdf\doc.pdf`,
		"empty key":          "",
	}

	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			if err := b.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "application/pdf"); err == nil {
				t.Fatalf("Put(%q): want error, got nil", key)
			}
			if _, _, err := b.Get(ctx, key); err == nil {
				t.Fatalf("Get(%q): want error, got nil", key)
			}
			if _, _, err := b.OpenSeek(ctx, key); err == nil {
				t.Fatalf("OpenSeek(%q): want error, got nil", key)
			}
			if err := b.Delete(ctx, key); err == nil {
				t.Fatalf("Delete(%q): want error, got nil", key)
			}
		})
	}

	// Nothing must have touched disk: root stays empty.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("root has %d entries after rejected keys, want 0: %v", len(entries), entries)
	}
}

func TestLocalBackend_DeleteRemovesEmptyParentDirs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	b := NewLocalBackend(root)

	key := "col1/pdf/sub/doc1.pdf"
	if err := b.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "application/pdf"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// A sibling file under the top-level "col1" directory must survive —
	// Delete climbs one empty directory at a time and stops at the first
	// non-empty one, and must never remove root itself.
	sibling := "col1/keep.txt"
	if err := b.Put(ctx, sibling, bytes.NewReader([]byte("y")), 1, "text/plain"); err != nil {
		t.Fatalf("Put sibling: %v", err)
	}

	if err := b.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "col1", "pdf")); !os.IsNotExist(err) {
		t.Fatalf("col1/pdf should have been removed (empty), stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "col1")); err != nil {
		t.Fatalf("col1 should still exist (has sibling file): %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root must never be removed: %v", err)
	}
}
