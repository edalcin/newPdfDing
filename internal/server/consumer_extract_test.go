package server

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCapTextTruncatesAtLimit covers the extraction memory-cap invariant:
// a source larger than extractedTextCapBytes yields exactly the cap's
// worth of text (truncated, not an error), and a smaller source passes
// through unchanged.
func TestCapTextTruncatesAtLimit(t *testing.T) {
	big := strings.Repeat("x", extractedTextCapBytes+1024*1024) // 1 MiB over cap
	got, err := capText(strings.NewReader(big))
	if err != nil {
		t.Fatalf("capText: %v", err)
	}
	if len(got) != extractedTextCapBytes {
		t.Fatalf("expected capped text of %d bytes, got %d", extractedTextCapBytes, len(got))
	}
	if got != big[:extractedTextCapBytes] {
		t.Fatalf("capped text is not a clean prefix of the source")
	}

	small := "well within the cap"
	got, err = capText(strings.NewReader(small))
	if err != nil {
		t.Fatalf("capText small: %v", err)
	}
	if got != small {
		t.Fatalf("expected short input unchanged, got %q", got)
	}
}

// TestExtractPDFTextFromStorage_TempFileUnderFilesTmp covers the temp
// staging path used to run ledongthuc/pdf against a storage-backed key:
// the staging file must land under <cfg.Files>/tmp — never the OS temp
// dir/tmpfs — and it must be gone as soon as extraction returns, on both
// the success and the error path.
func TestExtractPDFTextFromStorage_TempFileUnderFilesTmp(t *testing.T) {
	srv, _ := testServer(t, false)
	ctx := context.Background()
	wantDir := filepath.Join(srv.cfg.Files, "tmp")

	// Intercept createTempFile to record exactly which directory receives
	// the staging file, without racing real disk-copy timing.
	var gotDir string
	var lastPath string
	orig := createTempFile
	createTempFile = func(dir, pattern string) (*os.File, error) {
		gotDir = dir
		f, err := orig(dir, pattern)
		if f != nil {
			lastPath = f.Name()
		}
		return f, err
	}
	t.Cleanup(func() { createTempFile = orig })

	// Success path: a real, parseable PDF (fixture from ledongthuc/pdf's
	// own test suite, so GetPlainText() is guaranteed to succeed against
	// this exact parser version).
	pdfBytes, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	key := "watch/sample.pdf"
	if err := srv.files.Put(ctx, key, bytes.NewReader(pdfBytes), int64(len(pdfBytes)), "application/pdf"); err != nil {
		t.Fatalf("seed storage: %v", err)
	}

	text, numPages, err := srv.extractPDFTextFromStorage(ctx, key)
	if err != nil {
		t.Fatalf("extractPDFTextFromStorage (success path): %v", err)
	}
	if text == "" {
		t.Fatalf("expected non-empty extracted text from the fixture PDF")
	}
	if numPages < 1 {
		t.Fatalf("expected at least 1 page, got %d", numPages)
	}
	if gotDir != wantDir {
		t.Fatalf("temp file staged in %q, want %q", gotDir, wantDir)
	}
	if gotDir == "" || gotDir == os.TempDir() {
		t.Fatalf("temp file staged directly in the OS temp dir %q, want a subdirectory of cfg.Files", gotDir)
	}
	if _, err := os.Stat(lastPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged temp file %s to be removed after a successful extraction, stat err = %v", lastPath, err)
	}

	// Error path: content that isn't a parseable PDF, so pdf.Open fails —
	// the staged file must still be cleaned up.
	gotDir, lastPath = "", ""
	badKey := "watch/not-a-pdf.pdf"
	badBytes := []byte("%PDF-1.7\nthis is not a real pdf structure")
	if err := srv.files.Put(ctx, badKey, bytes.NewReader(badBytes), int64(len(badBytes)), "application/pdf"); err != nil {
		t.Fatalf("seed storage (bad pdf): %v", err)
	}
	if _, _, err := srv.extractPDFTextFromStorage(ctx, badKey); err == nil {
		t.Fatalf("expected an error extracting a non-PDF payload")
	}
	if gotDir != wantDir {
		t.Fatalf("temp file (error path) staged in %q, want %q", gotDir, wantDir)
	}
	if lastPath == "" {
		t.Fatalf("expected a temp file to have been created before the extraction error")
	}
	if _, err := os.Stat(lastPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged temp file %s to be removed after a failed extraction, stat err = %v", lastPath, err)
	}

	// Nothing was ever left behind in <cfg.Files>/tmp itself, on either path.
	entries, err := os.ReadDir(wantDir)
	if err != nil {
		t.Fatalf("read %s: %v", wantDir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected %s to be empty after extraction, found %v", wantDir, entries)
	}
}

// TestCleanOrphanedTempFiles covers the best-effort sweep of stale
// npd-*.pdf staging files: an old file is removed, a fresh one is left
// alone (it may belong to a concurrent extraction still in flight).
func TestCleanOrphanedTempFiles(t *testing.T) {
	dir := t.TempDir()

	oldPath := filepath.Join(dir, "npd-old12345.pdf")
	if err := os.WriteFile(oldPath, []byte("stale"), 0o640); err != nil {
		t.Fatalf("write old temp file: %v", err)
	}
	oldTime := time.Now().Add(-orphanedTempFileAge - time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	freshPath := filepath.Join(dir, "npd-fresh6789.pdf")
	if err := os.WriteFile(freshPath, []byte("in flight"), 0o640); err != nil {
		t.Fatalf("write fresh temp file: %v", err)
	}

	cleanOrphanedTempFiles(dir)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old orphaned temp file to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("expected fresh temp file to be left alone, stat err = %v", err)
	}
}
