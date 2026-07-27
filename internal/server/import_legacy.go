package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/edalcin/newpdfding/internal/store"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ImportReport summarizes one run of ImportLegacy.
type ImportReport struct {
	Tags        int
	PDFs        int
	Skipped     int // PDFs skipped: missing file, duplicate sha256, or read error
	Annotations int
	Shares      int
}

// ImportLegacy runs the single-shot import of a legacy Django database into
// the current schema (ver refatoracao/ETAPAS.md, ETAPA-12-IMPORTACAO;
// refatoracao/02-modelo-de-dados.md, "Mapeamento de modelos Django →
// tabelas novas"). It reads legacyDBPath (the old db.sqlite3) and copies
// referenced files from legacyMediaDir into the active storage backend.
// Workspace, WorkspaceUser and pdf_collection have no destination — every
// legacy workspace's tags collapse into this single-user schema's flat list,
// merging by case-insensitive name. Imported PDFs never get an embedding
// (pdf_embeddings stays untouched), matching "Nenhum embedding automático".
func (s *Server) ImportLegacy(ctx context.Context, legacyDBPath, legacyMediaDir string) (ImportReport, error) {
	var report ImportReport

	legacyDB, err := sql.Open("sqlite", legacyDBPath)
	if err != nil {
		return report, fmt.Errorf("open legacy db: %w", err)
	}
	defer legacyDB.Close()
	if err := legacyDB.PingContext(ctx); err != nil {
		return report, fmt.Errorf("open legacy db: %w", err)
	}

	tagIDs, err := s.importLegacyTags(legacyDB)
	if err != nil {
		return report, fmt.Errorf("import tags: %w", err)
	}
	report.Tags = len(tagIDs)

	pdfIDs, skipped, err := s.importLegacyPDFs(ctx, legacyDB, legacyMediaDir)
	if err != nil {
		return report, fmt.Errorf("import pdfs: %w", err)
	}
	report.PDFs = len(pdfIDs)
	report.Skipped = skipped

	annotations, err := s.importLegacyAnnotations(legacyDB, "pdf_pdfcomment", "comment", pdfIDs)
	if err != nil {
		return report, fmt.Errorf("import comments: %w", err)
	}
	highlights, err := s.importLegacyAnnotations(legacyDB, "pdf_pdfhighlight", "highlight", pdfIDs)
	if err != nil {
		return report, fmt.Errorf("import highlights: %w", err)
	}
	report.Annotations = annotations + highlights

	shares, err := s.importLegacyShares(legacyDB, pdfIDs)
	if err != nil {
		return report, fmt.Errorf("import shares: %w", err)
	}
	report.Shares = shares

	return report, nil
}

// importLegacyTags imports pdf_tag, reusing TagStore.EnsureTags — its
// unique index on name (COLLATE NOCASE) already merges legacy per-workspace
// duplicates. Returns legacy id -> new id.
func (s *Server) importLegacyTags(legacyDB *sql.DB) (map[string]string, error) {
	rows, err := legacyDB.Query(`SELECT id, name FROM pdf_tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var legacyID, name string
		if err := rows.Scan(&legacyID, &name); err != nil {
			return nil, err
		}
		ids, err := s.tags.EnsureTags([]string{name})
		if err != nil {
			return nil, fmt.Errorf("tag %q: %w", name, err)
		}
		out[legacyID] = ids[0]
	}
	return out, rows.Err()
}

// importLegacyPDFs imports pdf_pdf plus its pdf_pdf_tags associations,
// copying the pdf/preview files from legacyMediaDir into the
// active storage backend. A PDF whose file is missing on disk or whose
// sha256 already exists is skipped and logged, not fatal. Returns legacy id
// -> new id for every PDF actually imported.
func (s *Server) importLegacyPDFs(ctx context.Context, legacyDB *sql.DB, legacyMediaDir string) (map[string]string, int, error) {
	rows, err := legacyDB.Query(`SELECT
		id, archived, creation_date, current_page, description,
		file, last_viewed_date, name, notes, number_of_pages,
		preview, revision, starred, views
		FROM pdf_pdf`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	type legacyPDF struct {
		id, createdAt, description, file,
		lastViewedAt, name, notes string
		preview                                     sql.NullString
		archived, starred                           bool
		currentPage, numberOfPages, revision, views int
	}

	var legacyPDFs []legacyPDF
	for rows.Next() {
		var p legacyPDF
		if err := rows.Scan(
			&p.id, &p.archived, &p.createdAt, &p.currentPage, &p.description,
			&p.file, &p.lastViewedAt, &p.name, &p.notes, &p.numberOfPages,
			&p.preview, &p.revision, &p.starred, &p.views,
		); err != nil {
			return nil, 0, err
		}
		legacyPDFs = append(legacyPDFs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	out := map[string]string{}
	skipped := 0
	for _, p := range legacyPDFs {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, 0, err
		}
		pdfID := id.String()

		fileKey := pdfFileKey(pdfID)
		sum, size, err := s.copyLegacyFile(ctx, legacyMediaDir, p.file, fileKey)
		if err != nil {
			log.Printf("import: pdf %q: %v, skipping", p.name, err)
			skipped++
			continue
		}

		var previewKey string
		if p.preview.Valid && p.preview.String != "" {
			previewKey = pdfPreviewKey(pdfID)
			if _, _, err := s.copyLegacyFile(ctx, legacyMediaDir, p.preview.String, previewKey); err != nil {
				log.Printf("import: pdf %q: preview copy failed: %v (continuing without it)", p.name, err)
				previewKey = ""
			}
		}

		var tagNames []string
		trows, err := legacyDB.Query(`SELECT t.name FROM pdf_pdf_tags pt JOIN pdf_tag t ON t.id = pt.tag_id WHERE pt.pdf_id = ?`, p.id)
		if err != nil {
			return nil, 0, err
		}
		for trows.Next() {
			var name string
			if err := trows.Scan(&name); err != nil {
				trows.Close()
				return nil, 0, err
			}
			tagNames = append(tagNames, name)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return nil, 0, err
		}

		numPages := p.numberOfPages
		if numPages < 0 {
			numPages = 0
		}
		lastViewedAt := p.lastViewedAt
		if p.views == 0 {
			lastViewedAt = ""
		}

		_, err = s.pdfs.Import(store.ImportParams{
			ID:            pdfID,
			Name:          p.name,
			Description:   p.description,
			Notes:         p.notes,
			StorageKey:    fileKey,
			PreviewKey:    previewKey,
			SHA256:        sum,
			SizeBytes:     size,
			NumPages:      numPages,
			CurrentPage:   p.currentPage,
			Views:         p.views,
			Revision:      p.revision + 1, // legacy default is 0; new schema's floor is 1
			Starred:       p.starred,
			Archived:      p.archived,
			TagNames:      tagNames,
			CreatedAt:     p.createdAt,
			LastViewedAt:  lastViewedAt,
		})
		if errors.Is(err, store.ErrConflict) {
			_ = s.files.Delete(ctx, fileKey)
			if previewKey != "" {
				_ = s.files.Delete(ctx, previewKey)
			}
			log.Printf("import: pdf %q duplicates an already-imported sha256, skipping", p.name)
			skipped++
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("pdf %q: %w", p.name, err)
		}
		out[p.id] = pdfID
	}
	return out, skipped, nil
}

// copyLegacyFile streams relPath (relative to legacyMediaDir) into the
// active storage backend under key, returning its sha256 and size.
func (s *Server) copyLegacyFile(ctx context.Context, legacyMediaDir, relPath, key string) (sha256sum string, size int64, err error) {
	if relPath == "" {
		return "", 0, errors.New("empty file path")
	}
	f, err := os.Open(filepath.Join(legacyMediaDir, filepath.FromSlash(relPath)))
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hasher := sha256.New()
	tee := io.TeeReader(f, hasher)
	buf, err := io.ReadAll(tee)
	if err != nil {
		return "", 0, err
	}
	if err := s.files.Put(ctx, key, bytes.NewReader(buf), int64(len(buf)), "application/octet-stream"); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), int64(len(buf)), nil
}

// importLegacyAnnotations imports every row of legacyTable (pdf_pdfcomment
// or pdf_pdfhighlight) whose pdf_id was actually imported, tagging it with
// kind. Returns how many were imported.
func (s *Server) importLegacyAnnotations(legacyDB *sql.DB, legacyTable, kind string, pdfIDs map[string]string) (int, error) {
	rows, err := legacyDB.Query(fmt.Sprintf(`SELECT id, creation_date, page, pdf_id, text FROM %s`, legacyTable))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, createdAt, legacyPDFID, text string
		var page int
		if err := rows.Scan(&id, &createdAt, &page, &legacyPDFID, &text); err != nil {
			return 0, err
		}
		newPDFID, ok := pdfIDs[legacyPDFID]
		if !ok {
			continue // pdf was skipped (missing file / duplicate) — drop its annotations too
		}
		if _, err := s.annotations.Import(id, newPDFID, kind, page, text, createdAt); err != nil {
			return 0, fmt.Errorf("%s %s: %w", kind, id, err)
		}
		count++
	}
	return count, rows.Err()
}

// importLegacyShares imports pdf_sharedpdf for every PDF that was actually
// imported. Legacy-only fields with no destination in the new shares table
// (password, max_views, deletion_date, qr code file) have no new schema
// counterpart (ver 02-modelo-de-dados.md) and are intentionally dropped.
func (s *Server) importLegacyShares(legacyDB *sql.DB, pdfIDs map[string]string) (int, error) {
	rows, err := legacyDB.Query(`SELECT id, creation_date, views, pdf_id FROM pdf_sharedpdf`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, createdAt, legacyPDFID string
		var views int
		if err := rows.Scan(&id, &createdAt, &views, &legacyPDFID); err != nil {
			return 0, err
		}
		newPDFID, ok := pdfIDs[legacyPDFID]
		if !ok {
			continue
		}
		if _, err := s.shares.Import(id, newPDFID, views, createdAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				log.Printf("import: pdf %s already has a share, skipping share %s", newPDFID, id)
				continue
			}
			return 0, fmt.Errorf("share %s: %w", id, err)
		}
		count++
	}
	return count, rows.Err()
}
