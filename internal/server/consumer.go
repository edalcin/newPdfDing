package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/ledongthuc/pdf"
)

// StartConsumer runs the watch-dir import loop — the single periodic
// background process in the product (ver refatoracao/00-visao-geral.md,
// decisão 8; refatoracao/ETAPAS.md, ETAPA-8-BACKGROUND). It does nothing if
// CONSUME_ENABLE is not set. Runs until ctx is cancelled.
func (s *Server) StartConsumer(ctx context.Context) {
	if !s.cfg.ConsumeEnable {
		return
	}
	if err := os.MkdirAll(s.cfg.ConsumeDir, 0o750); err != nil {
		log.Printf("consume: failed to create CONSUME_DIR %s: %v", s.cfg.ConsumeDir, err)
		return
	}

	go func() {
		ticker := time.NewTicker(time.Duration(s.cfg.ConsumeInterval) * time.Minute)
		defer ticker.Stop()
		s.consumeOnce(ctx) // first pass immediately, then on each tick
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.consumeOnce(ctx)
			}
		}
	}()
}

// consumeOnce scans CONSUME_DIR once for *.pdf files and imports each.
func (s *Server) consumeOnce(ctx context.Context) {
	entries, err := os.ReadDir(s.cfg.ConsumeDir)
	if err != nil {
		log.Printf("consume: failed to read CONSUME_DIR %s: %v", s.cfg.ConsumeDir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		s.consumeFile(ctx, filepath.Join(s.cfg.ConsumeDir, e.Name()))
	}
}

// consumeFile imports one PDF from CONSUME_DIR, reusing the exact same
// validation/dedup/storage pipeline as the HTTP upload (ver
// createPDFFromUpload in handlers_pdfs.go) — the only difference is text
// extraction, done here in pure Go since there is no browser involved (ver
// 00-visao-geral.md, decisão 2).
func (s *Server) consumeFile(ctx context.Context, path string) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("consume: failed to open %s: %v", path, err)
		return
	}

	text, numPages, err := extractPDFText(path)
	if err != nil {
		// Not fatal: import proceeds without extracted text/page count —
		// createPDFFromUpload's own magic-byte check still rejects non-PDF
		// files below.
		log.Printf("consume: text extraction failed for %s: %v", path, err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	item := uploadItem{
		Name:     name,
		TagNames: store.ParseTagString(s.cfg.ConsumeTags),
		Text:     text,
		NumPages: numPages,
		File:     f,
	}

	pdfRow, createErr := s.createPDFFromUpload(ctx, item)
	f.Close()

	var dup *duplicatePDFError
	switch {
	case createErr == nil:
		log.Printf("consume: imported %q as pdf_id=%s", path, pdfRow.ID)
		if rmErr := os.Remove(path); rmErr != nil {
			log.Printf("consume: imported %q but failed to remove it from CONSUME_DIR: %v", path, rmErr)
		}
	case errors.As(createErr, &dup):
		log.Printf("consume: %q duplicates pdf_id=%s by sha256", path, dup.existing.ID)
		if !s.cfg.ConsumeSkipExisting {
			// Operator asked to be told, not to have files silently
			// discarded — leave it for manual review.
			return
		}
		if rmErr := os.Remove(path); rmErr != nil {
			log.Printf("consume: duplicate %q but failed to remove it from CONSUME_DIR: %v", path, rmErr)
		}
	default:
		log.Printf("consume: failed to import %q: %v (left in place, retried next cycle)", path, createErr)
	}
}

// extractPDFText pulls the plain text body and page count from a PDF on
// disk using a pure-Go parser (ver 01-arquitetura.md, dependência
// github.com/ledongthuc/pdf — "usada apenas no caminho da watch-dir").
func extractPDFText(path string) (string, int, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	var buf bytes.Buffer
	body, err := r.GetPlainText()
	if err != nil {
		return "", r.NumPage(), err
	}
	if _, err := buf.ReadFrom(body); err != nil {
		return "", r.NumPage(), err
	}
	return buf.String(), r.NumPage(), nil
}

// extractPDFTextFromStorage copies key out of the storage backend into a
// temp file and runs extractPDFText on it — ledongthuc/pdf needs an
// io.ReaderAt with a known size, which the Backend interface does not give.
func (s *Server) extractPDFTextFromStorage(ctx context.Context, key string) (string, int, error) {
	rc, _, err := s.files.Get(ctx, key)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", "npd-*.pdf")
	if err != nil {
		return "", 0, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}

	return extractPDFText(tmpPath)
}

// errNoText means the PDF has no extractable text layer (scan puro sem OCR).
var errNoText = errors.New("no extractable text")

// textFor returns the PDF's extracted text, extracting and persisting it on
// first use when pdf_text is empty. Retorna errNoText quando o documento não
// tem camada de texto — o chamador decide a mensagem ao usuário.
func (s *Server) textFor(ctx context.Context, pdf store.PDF) (string, error) {
	if body, _ := s.pdfs.GetText(pdf.ID); body != "" {
		return body, nil
	}
	text, _, err := s.extractPDFTextFromStorage(ctx, pdf.StorageKey)
	if err != nil || text == "" {
		return "", errNoText
	}
	if err := s.pdfs.SetText(pdf.ID, text); err != nil {
		return "", err
	}
	return text, nil
}
