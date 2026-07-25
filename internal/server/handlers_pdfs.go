package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/edalcin/newpdfding/internal/security"
	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// fileDirectoryPattern is the closed charset for pdfs.file_directory (ver
// refatoracao/08-seguranca.md, "Validação de entrada") — the absence of '.'
// makes any ".." traversal component impossible to express.
var fileDirectoryPattern = regexp.MustCompile(`^[A-Za-z0-9_\-/]{0,120}$`)

const pdfMagic = "%PDF-"

// ---------------------------------------------------------------------
// JSON representation (ver refatoracao/05-api.md, "Representação de PDF")
// ---------------------------------------------------------------------

type pdfResponse struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	Notes           string      `json:"notes"`
	NotesHTML       string      `json:"notes_html"`
	CollectionID    string      `json:"collection_id"`
	FileDirectory   string      `json:"file_directory"`
	SHA256          string      `json:"sha256"`
	SizeBytes       int64       `json:"size_bytes"`
	NumPages        int         `json:"num_pages"`
	CurrentPage     int         `json:"current_page"`
	Views           int         `json:"views"`
	Revision        int         `json:"revision"`
	Starred         bool        `json:"starred"`
	Archived        bool        `json:"archived"`
	CreatedAt       string      `json:"created_at"`
	LastViewedAt    *string     `json:"last_viewed_at"`
	Tags            []store.Tag `json:"tags"`
	EmbeddingStatus string      `json:"embedding_status"`
}

// toPDFResponse builds the API representation of a PDF, rendering+sanitizing
// notes to HTML. embedding_status is derived by the store layer on every
// read (ver 04-busca-hibrida.md, "Estado de embedding").
func toPDFResponse(p store.PDF) (pdfResponse, error) {
	html, err := security.RenderNotes(p.Notes)
	if err != nil {
		return pdfResponse{}, err
	}
	tags := p.Tags
	if tags == nil {
		tags = []store.Tag{}
	}
	resp := pdfResponse{
		ID: p.ID, Name: p.Name, Description: p.Description,
		Notes: p.Notes, NotesHTML: html,
		CollectionID: p.CollectionID, FileDirectory: p.FileDirectory,
		SHA256: p.SHA256, SizeBytes: p.SizeBytes, NumPages: p.NumPages,
		CurrentPage: p.CurrentPage, Views: p.Views, Revision: p.Revision,
		Starred: p.Starred, Archived: p.Archived, CreatedAt: p.CreatedAt,
		Tags: tags, EmbeddingStatus: p.EmbeddingStatus,
	}
	if p.LastViewedAt.Valid {
		resp.LastViewedAt = &p.LastViewedAt.String
	}
	return resp, nil
}

func writePDF(w http.ResponseWriter, status int, p store.PDF) {
	resp, err := toPDFResponse(p)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, status, resp)
}

// ---------------------------------------------------------------------
// Storage key scheme (ver refatoracao/03-storage.md, "Esquema de chaves")
// ---------------------------------------------------------------------

func pdfFileKey(collectionID, fileDirectory, pdfID string) string {
	dir := fileDirectory
	if dir != "" {
		dir = strings.Trim(dir, "/") + "/"
	}
	return fmt.Sprintf("%s/pdf/%s%s.pdf", collectionID, dir, pdfID)
}

func pdfThumbnailKey(collectionID, pdfID string) string {
	return fmt.Sprintf("%s/thumb/%s.png", collectionID, pdfID)
}

func pdfPreviewKey(collectionID, pdfID string) string {
	return fmt.Sprintf("%s/preview/%s.png", collectionID, pdfID)
}

// ---------------------------------------------------------------------
// GET /api/pdfs
// ---------------------------------------------------------------------

func (s *Server) handleListPDFs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	params := store.ListParams{
		CollectionID: q.Get("collection"),
		Tag:          q.Get("tag"),
		Sort:         q.Get("sort"),
		Cursor:       q.Get("cursor"),
	}

	if v := q.Get("starred"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid starred")
			return
		}
		params.Starred = &b
	}

	// Default view excludes archived PDFs unless the caller explicitly asks
	// for archived=true|false.
	archived := false
	if v := q.Get("archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid archived")
			return
		}
		archived = b
	}
	params.Archived = &archived

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		params.Limit = n
	}

	if query := q.Get("q"); query != "" {
		params.Query = query
		if s.gemini != nil {
			if vec, err := s.gemini.Embed(r.Context(), query); err == nil {
				params.QueryVector = vec
			} else {
				log.Printf("warning: gemini query embed failed, falling back to lexical-only search: %v", err)
			}
		}
	}

	items, next, err := s.pdfs.List(params)
	if errors.Is(err, store.ErrInvalidCursor) {
		writeJSONError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]pdfResponse, 0, len(items))
	for _, p := range items {
		resp, err := toPDFResponse(p)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, resp)
	}

	var nextCursor any
	if next != "" {
		nextCursor = next
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": nextCursor})
}

// ---------------------------------------------------------------------
// Upload (single + bulk) — shared pipeline
// ---------------------------------------------------------------------

// uploadItem is one file plus its metadata, shared by single and bulk upload.
type uploadItem struct {
	Name          string
	Description   string
	CollectionID  string
	FileDirectory string
	TagNames      []string
	Text          string
	NumPages      int // 0 when unknown; set by the watch-dir consumer (ver consumer.go)
	File          multipart.File
	Thumbnail     multipart.File
	Preview       multipart.File
}

// duplicatePDFError signals a SHA-256 collision with an existing PDF.
type duplicatePDFError struct{ existing store.PDF }

func (e *duplicatePDFError) Error() string { return "duplicate pdf" }

// uploadValidationError maps directly to an HTTP status for the response.
type uploadValidationError struct {
	status  int
	message string
}

func (e *uploadValidationError) Error() string { return e.message }

// createPDFFromUpload validates, hashes, stores and inserts one uploaded
// PDF. On any failure after a file has been written to storage, every key
// written so far is cleaned up — no orphaned file is ever left behind (ver
// refatoracao/03-storage.md, "Falhas").
func (s *Server) createPDFFromUpload(ctx context.Context, item uploadItem) (store.PDF, error) {
	if !fileDirectoryPattern.MatchString(item.FileDirectory) {
		return store.PDF{}, &uploadValidationError{http.StatusBadRequest, "invalid file_directory"}
	}

	magic := make([]byte, len(pdfMagic))
	if _, err := io.ReadFull(item.File, magic); err != nil || string(magic) != pdfMagic {
		return store.PDF{}, &uploadValidationError{http.StatusUnsupportedMediaType, "file is not a PDF"}
	}
	if _, err := item.File.Seek(0, io.SeekStart); err != nil {
		return store.PDF{}, err
	}

	hasher := sha256.New()
	size, err := io.Copy(hasher, item.File)
	if err != nil {
		return store.PDF{}, err
	}
	if _, err := item.File.Seek(0, io.SeekStart); err != nil {
		return store.PDF{}, err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	if existing, err := s.pdfs.GetBySHA256(sum); err == nil {
		return store.PDF{}, &duplicatePDFError{existing: existing}
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.PDF{}, err
	}

	collectionID := item.CollectionID
	if collectionID == "" {
		def, err := s.collections.Default()
		if err != nil {
			return store.PDF{}, err
		}
		collectionID = def.ID
	} else if ok, err := s.collections.Exists(collectionID); err != nil {
		return store.PDF{}, err
	} else if !ok {
		return store.PDF{}, &uploadValidationError{http.StatusBadRequest, "collection_id does not exist"}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return store.PDF{}, err
	}
	pdfID := id.String()

	name := item.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(pdfID), ".pdf")
	}

	fileKey := pdfFileKey(collectionID, item.FileDirectory, pdfID)
	var thumbKey, previewKey string

	writtenKeys := []string{}
	cleanup := func() {
		for _, k := range writtenKeys {
			_ = s.files.Delete(ctx, k)
		}
	}

	if err := s.files.Put(ctx, fileKey, item.File, size, "application/pdf"); err != nil {
		return store.PDF{}, &uploadValidationError{http.StatusInsufficientStorage, "failed to write file to storage"}
	}
	writtenKeys = append(writtenKeys, fileKey)

	if item.Thumbnail != nil {
		thumbKey = pdfThumbnailKey(collectionID, pdfID)
		if err := s.files.Put(ctx, thumbKey, item.Thumbnail, -1, "image/png"); err != nil {
			cleanup()
			return store.PDF{}, &uploadValidationError{http.StatusInsufficientStorage, "failed to write thumbnail to storage"}
		}
		writtenKeys = append(writtenKeys, thumbKey)
	}
	if item.Preview != nil {
		previewKey = pdfPreviewKey(collectionID, pdfID)
		if err := s.files.Put(ctx, previewKey, item.Preview, -1, "image/png"); err != nil {
			cleanup()
			return store.PDF{}, &uploadValidationError{http.StatusInsufficientStorage, "failed to write preview to storage"}
		}
		writtenKeys = append(writtenKeys, previewKey)
	}

	pdf, err := s.pdfs.Create(store.CreateParams{
		ID:            pdfID,
		Name:          name,
		Description:   item.Description,
		CollectionID:  collectionID,
		FileDirectory: item.FileDirectory,
		StorageKey:    fileKey,
		ThumbnailKey:  thumbKey,
		PreviewKey:    previewKey,
		SHA256:        sum,
		SizeBytes:     size,
		NumPages:      item.NumPages,
		TagNames:      item.TagNames,
		Text:          item.Text,
	})
	if errors.Is(err, store.ErrConflict) {
		cleanup()
		if existing, gerr := s.pdfs.GetBySHA256(sum); gerr == nil {
			return store.PDF{}, &duplicatePDFError{existing: existing}
		}
		return store.PDF{}, err
	}
	if err != nil {
		cleanup()
		return store.PDF{}, err
	}
	return pdf, nil
}

// respondUploadError maps createPDFFromUpload's error into the response
// contract fixed in refatoracao/05-api.md, "Comportamento de duplicidade".
func respondUploadError(w http.ResponseWriter, err error) {
	var dup *duplicatePDFError
	var verr *uploadValidationError
	switch {
	case errors.As(err, &dup):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "PDF já existe", "pdf_id": dup.existing.ID, "name": dup.existing.Name,
		})
	case errors.As(err, &verr):
		writeJSONError(w, verr.status, verr.message)
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}

func formFile(r *http.Request, field string) multipart.File {
	f, _, err := r.FormFile(field)
	if err != nil {
		return nil
	}
	return f
}

func closeIfNotNil(f multipart.File) {
	if f != nil {
		f.Close()
	}
}

// ---------------------------------------------------------------------
// POST /api/pdfs
// ---------------------------------------------------------------------

func (s *Server) handleCreatePDF(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadMB*1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload exceeds MAX_UPLOAD_MB")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "malformed multipart payload")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	thumb := formFile(r, "thumbnail")
	defer closeIfNotNil(thumb)
	preview := formFile(r, "preview")
	defer closeIfNotNil(preview)

	item := uploadItem{
		Name:          r.FormValue("name"),
		Description:   r.FormValue("description"),
		CollectionID:  r.FormValue("collection_id"),
		FileDirectory: r.FormValue("file_directory"),
		TagNames:      store.ParseTagString(r.FormValue("tags")),
		Text:          r.FormValue("text"),
		NumPages:      parsePositiveIntForm(r.FormValue("num_pages")),
		File:          file,
		Thumbnail:     thumb,
		Preview:       preview,
	}

	pdf, err := s.createPDFFromUpload(r.Context(), item)
	if err != nil {
		respondUploadError(w, err)
		return
	}
	writePDF(w, http.StatusCreated, pdf)
}

// ---------------------------------------------------------------------
// POST /api/pdfs/bulk
// ---------------------------------------------------------------------

// handleBulkCreatePDFs accepts several "file" parts in one multipart
// request. Per-file metadata is indexed by upload order: name_0/name_1/...,
// description_N, tags_N, collection_id_N, file_directory_N (05-api.md does
// not pin an exact wire format beyond "multipart with multiple file fields,
// one set of metadata per file" — this is that scheme).
func (s *Server) handleBulkCreatePDFs(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadMB*1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload exceeds MAX_UPLOAD_MB")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "malformed multipart payload")
		return
	}
	defer r.MultipartForm.RemoveAll()

	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		writeJSONError(w, http.StatusBadRequest, "at least one file is required")
		return
	}

	type result struct {
		Status string `json:"status"`
		PDFID  string `json:"pdf_id,omitempty"`
		Name   string `json:"name,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(headers))

	for i, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			results = append(results, result{Status: "error", Error: "failed to read uploaded file"})
			continue
		}

		suffix := "_" + strconv.Itoa(i)
		item := uploadItem{
			Name:          r.FormValue("name" + suffix),
			Description:   r.FormValue("description" + suffix),
			CollectionID:  r.FormValue("collection_id" + suffix),
			FileDirectory: r.FormValue("file_directory" + suffix),
			TagNames:      store.ParseTagString(r.FormValue("tags" + suffix)),
			Text:          r.FormValue("text" + suffix),
			NumPages:      parsePositiveIntForm(r.FormValue("num_pages" + suffix)),
			File:          f,
		}

		pdf, err := s.createPDFFromUpload(r.Context(), item)
		f.Close()
		if err != nil {
			var dup *duplicatePDFError
			if errors.As(err, &dup) {
				results = append(results, result{Status: "duplicate", PDFID: dup.existing.ID, Name: dup.existing.Name})
				continue
			}
			var verr *uploadValidationError
			if errors.As(err, &verr) {
				results = append(results, result{Status: "error", Error: verr.message})
				continue
			}
			results = append(results, result{Status: "error", Error: "internal error"})
			continue
		}
		results = append(results, result{Status: "created", PDFID: pdf.ID})
	}

	writeJSON(w, http.StatusCreated, map[string]any{"results": results})
}

// ---------------------------------------------------------------------
// GET/PATCH/DELETE /api/pdfs/{id}
// ---------------------------------------------------------------------

func (s *Server) handleGetPDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePDF(w, http.StatusOK, pdf)
}

func (s *Server) handlePatchPDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name          *string   `json:"name"`
		Description   *string   `json:"description"`
		Notes         *string   `json:"notes"`
		Tags          *[]string `json:"tags"`
		CollectionID  *string   `json:"collection_id"`
		FileDirectory *string   `json:"file_directory"`
		Starred       *bool     `json:"starred"`
		Archived      *bool     `json:"archived"`
		CurrentPage   *int      `json:"current_page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	if req.FileDirectory != nil && !fileDirectoryPattern.MatchString(*req.FileDirectory) {
		writeJSONError(w, http.StatusBadRequest, "invalid file_directory")
		return
	}
	if req.CollectionID != nil {
		ok, err := s.collections.Exists(*req.CollectionID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "collection_id does not exist")
			return
		}
	}
	if req.CurrentPage != nil && *req.CurrentPage < 1 {
		writeJSONError(w, http.StatusBadRequest, "current_page must be >= 1")
		return
	}

	params := store.UpdateParams{
		Name: req.Name, Description: req.Description, Notes: req.Notes,
		CollectionID: req.CollectionID, FileDirectory: req.FileDirectory,
		Starred: req.Starred, Archived: req.Archived, CurrentPage: req.CurrentPage,
	}
	if req.Tags != nil {
		normalized := store.ParseTagString(strings.Join(*req.Tags, " "))
		params.Tags = &normalized
	}

	pdf, err := s.pdfs.Update(id, params)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePDF(w, http.StatusOK, pdf)
}

func (s *Server) handleDeletePDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.Delete(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.deletePDFFiles(r.Context(), pdf)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePDFFiles(ctx context.Context, p store.PDF) {
	for _, key := range []string{p.StorageKey, p.ThumbnailKey, p.PreviewKey} {
		if key == "" {
			continue
		}
		if err := s.files.Delete(ctx, key); err != nil {
			log.Printf("warning: failed to delete storage key %q for pdf_id=%s: %v", key, p.ID, err)
		}
	}
}

// ---------------------------------------------------------------------
// File / thumbnail / preview / download
// ---------------------------------------------------------------------

func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	s.serveStoredFile(w, r, "application/pdf", true, func(p store.PDF) string { return p.StorageKey })
}

func (s *Server) handleDownloadPDF(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	f, _, err := s.files.OpenSeek(r.Context(), pdf.StorageKey)
	if err != nil {
		log.Printf("warning: pdf file missing on disk pdf_id=%s key=%s: %v", pdf.ID, pdf.StorageKey, err)
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	_ = s.pdfs.RecordView(id)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, sanitizeFilename(pdf.Name)))
	http.ServeContent(w, r, pdf.Name+".pdf", modTime(pdf.CreatedAt), f)
}

func (s *Server) handleServeThumbnail(w http.ResponseWriter, r *http.Request) {
	s.serveImage(w, r, func(p store.PDF) string { return p.ThumbnailKey })
}

func (s *Server) handleServePreview(w http.ResponseWriter, r *http.Request) {
	s.serveImage(w, r, func(p store.PDF) string { return p.PreviewKey })
}

func (s *Server) serveStoredFile(w http.ResponseWriter, r *http.Request, contentType string, countView bool, key func(store.PDF) string) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	f, _, err := s.files.OpenSeek(r.Context(), key(pdf))
	if err != nil {
		log.Printf("warning: pdf file missing on disk pdf_id=%s key=%s: %v", pdf.ID, key(pdf), err)
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	if countView {
		_ = s.pdfs.RecordView(id)
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, id, modTime(pdf.CreatedAt), f)
}

func (s *Server) serveImage(w http.ResponseWriter, r *http.Request, key func(store.PDF) string) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	k := key(pdf)
	if k == "" {
		writeJSONError(w, http.StatusNotFound, "image not set")
		return
	}
	rc, _, err := s.files.Get(r.Context(), k)
	if err != nil {
		log.Printf("warning: pdf image missing on disk pdf_id=%s key=%s: %v", pdf.ID, k, err)
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/png")
	if _, err := io.Copy(w, rc); err != nil {
		log.Printf("warning: failed to stream image pdf_id=%s key=%s: %v", pdf.ID, k, err)
	}
}

// handleUploadThumbnail accepts a browser-generated PNG thumbnail after the
// initial upload (ver 05-api.md, "POST .../thumbnail" — the pdf.js
// client-side flow may only be able to render the first page once the
// viewer opens, not at upload time).
func (s *Server) handleUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadMB*1024*1024)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload exceeds MAX_UPLOAD_MB")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "malformed multipart payload")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "thumbnail is required")
		return
	}
	defer file.Close()

	key := pdfThumbnailKey(pdf.CollectionID, pdf.ID)
	if err := s.files.Put(r.Context(), key, file, header.Size, "image/png"); err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, "failed to write thumbnail to storage")
		return
	}
	if err := s.pdfs.SetThumbnailKey(pdf.ID, key); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thumbnail": "ok"})
}

// ---------------------------------------------------------------------
// POST /api/pdfs/bulk-actions
// ---------------------------------------------------------------------

var validBulkActions = map[string]bool{
	"delete": true, "archive": true, "unarchive": true, "star": true, "unstar": true,
}

func (s *Server) handleBulkActions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validBulkActions[req.Action] || len(req.IDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid action or ids")
		return
	}

	if req.Action == "delete" {
		deleted, err := s.pdfs.BulkDelete(req.IDs)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, p := range deleted {
			s.deletePDFFiles(r.Context(), p)
		}
		writeJSON(w, http.StatusOK, map[string]int{"updated": len(deleted)})
		return
	}

	n, err := s.pdfs.BulkUpdate(req.IDs, req.Action)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"updated": n})
}

// ---------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------

// parsePositiveIntForm parses a form field as a page count, treating any
// missing/invalid/non-positive value as "unknown" (0) rather than an
// upload error — num_pages is best-effort metadata from client-side pdf.js
// (browser upload) or the watch-dir's pure-Go extraction (ver
// consumer.go), never a required field.
func parsePositiveIntForm(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func modTime(rfc3339Nano string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, rfc3339Nano)
	if err != nil {
		return time.Time{}
	}
	return t
}

// sanitizeFilename strips characters that would break a Content-Disposition
// header value from a user-supplied PDF name.
func sanitizeFilename(name string) string {
	var b bytes.Buffer
	for _, r := range name {
		if r == '"' || r == '\\' || r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "document"
	}
	return b.String()
}

// ---------------------------------------------------------------------
// PUT /api/pdfs/{id}/file — file revision
// ---------------------------------------------------------------------

// handlePutPDFFile replaces a PDF's content in place and bumps its
// revision (ver 05-api.md, "PUT .../file"; 10-inventario-funcionalidades.md,
// "Salvar PDF editado com revisão").
func (s *Server) handlePutPDFFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdf, err := s.pdfs.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadMB*1024*1024)
	var buf bytes.Buffer
	n, err := io.Copy(&buf, r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload exceeds MAX_UPLOAD_MB")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	data := buf.Bytes()
	if len(data) < len(pdfMagic) || string(data[:len(pdfMagic)]) != pdfMagic {
		writeJSONError(w, http.StatusUnsupportedMediaType, "file is not a PDF")
		return
	}

	hasher := sha256.New()
	hasher.Write(data)
	sum := hex.EncodeToString(hasher.Sum(nil))

	if err := s.files.Put(r.Context(), pdf.StorageKey, bytes.NewReader(data), n, "application/pdf"); err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, "failed to write file to storage")
		return
	}

	revision, err := s.pdfs.Revise(id, sum, n)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"revision": revision})
}
