package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/edalcin/newpdfding/internal/store"
)

// embedState is the lifecycle of one queued embedding job (ver
// refatoracao Fase F.2).
type embedState string

const (
	embedQueued     embedState = "queued"
	embedExtracting embedState = "extracting"
	embedEmbedding  embedState = "embedding"
	embedDone       embedState = "done"
	embedFailed     embedState = "failed"
)

type embedJob struct {
	State embedState `json:"state"`
	Error string     `json:"error,omitempty"`
}

// errAlreadyQueued/errQueueFull are returned by embedQueue.enqueue and
// mapped to 409/503 by handleEmbedPDF.
var (
	errAlreadyQueued = errors.New("embedding já em curso")
	errQueueFull     = errors.New("fila de embedding cheia")
)

// embedQueue serializes embedding work onto a single background worker —
// no two Gemini calls ever run concurrently (ver Fase F.2, substitui o
// antigo embedMu).
type embedQueue struct {
	mu   sync.Mutex
	jobs map[string]*embedJob // pdf_id -> estado
	ch   chan string          // buffer 256
	// ctx is the worker's lifetime, recorded by StartEmbedWorker so a bulk
	// enqueue blocked on a full channel unwinds at shutdown instead of
	// leaking its goroutine.
	ctx context.Context
}

func newEmbedQueue() *embedQueue {
	return &embedQueue{jobs: make(map[string]*embedJob), ch: make(chan string, 256), ctx: context.Background()}
}

// enqueue adds pdfID to the queue unless a non-terminal job for it already
// exists.
func (q *embedQueue) enqueue(pdfID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, ok := q.jobs[pdfID]; ok && job.State != embedDone && job.State != embedFailed {
		return errAlreadyQueued
	}
	q.jobs[pdfID] = &embedJob{State: embedQueued}
	select {
	case q.ch <- pdfID:
		return nil
	default:
		delete(q.jobs, pdfID)
		return errQueueFull
	}
}

// enqueueBulk registers ids and feeds them to the worker with a blocking
// send, so a re-embedding of the whole acervo is not capped by the channel
// buffer (enqueue's non-blocking send exists to answer 503 to a single
// click; here there is no client waiting). It self-throttles: at most
// cap(ch) jobs sit ahead of the worker, so the jobs map — polled by GET
// /api/embed/jobs — never grows to the size of the acervo. Ids already
// queued or running are skipped. Runs in its own goroutine.
func (q *embedQueue) enqueueBulk(ids []string) {
	for _, id := range ids {
		q.mu.Lock()
		if job, ok := q.jobs[id]; ok && job.State != embedDone && job.State != embedFailed {
			q.mu.Unlock()
			continue
		}
		q.jobs[id] = &embedJob{State: embedQueued}
		q.mu.Unlock()

		select {
		case q.ch <- id:
		case <-q.ctx.Done():
			q.cancel(id)
			return
		}
	}
}

// cancel removes pdfID's job unconditionally — called when the PDF itself
// is deleted, so a stale job never outlives its document.
func (q *embedQueue) cancel(pdfID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.jobs, pdfID)
}

// exists reports whether pdfID still has a tracked job — the worker checks
// this before starting work a cancel raced past the channel send.
func (q *embedQueue) exists(pdfID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.jobs[pdfID]
	return ok
}

func (q *embedQueue) setState(pdfID string, state embedState, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.jobs[pdfID]; !ok {
		return // cancelled (pdf deleted) while the worker was processing it
	}
	q.jobs[pdfID] = &embedJob{State: state, Error: errMsg}
}

// deleteIfDone removes a completed job 60s after it finished, unless a
// fresh enqueue already replaced it in the meantime.
func (q *embedQueue) deleteIfDone(pdfID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, ok := q.jobs[pdfID]; ok && job.State == embedDone {
		delete(q.jobs, pdfID)
	}
}

// snapshot returns a point-in-time copy of every tracked job, for GET
// /api/embed/jobs.
func (q *embedQueue) snapshot() map[string]embedJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]embedJob, len(q.jobs))
	for id, job := range q.jobs {
		out[id] = *job
	}
	return out
}

// StartEmbedWorker runs the single background goroutine that drains the
// embedding queue serially (ver Fase F.2, mesmo padrão de StartConsumer).
// A nil s.gemini (GEMINI_API_KEY unset) means enqueue is never reachable
// from handleEmbedPDF, so the worker simply has nothing to do.
func (s *Server) StartEmbedWorker(ctx context.Context) {
	s.embeds.ctx = ctx
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-s.embeds.ch:
				s.runEmbedJob(ctx, id)
			}
		}
	}()
}

func (s *Server) runEmbedJob(ctx context.Context, pdfID string) {
	if !s.embeds.exists(pdfID) {
		return // cancelled (pdf deleted) before the worker picked it up
	}
	s.embeds.setState(pdfID, embedExtracting, "")

	pdf, err := s.pdfs.GetByID(pdfID)
	if errors.Is(err, store.ErrNotFound) {
		s.embeds.cancel(pdfID)
		return
	}
	if err != nil {
		s.embeds.setState(pdfID, embedFailed, "erro interno")
		return
	}

	body, err := s.textFor(ctx, pdf)
	if errors.Is(err, errNoText) {
		s.embeds.setState(pdfID, embedFailed, "não foi possível extrair texto deste PDF")
		return
	}
	if err != nil {
		s.embeds.setState(pdfID, embedFailed, "erro interno")
		return
	}

	s.embeds.setState(pdfID, embedEmbedding, "")

	text := store.BuildEmbedText(pdf.Name, pdf.Description, body)
	hash := store.ContentHash(s.pdfs.EmbedModel(), text)

	if info, has, err := s.pdfs.GetEmbeddingInfo(pdfID); err == nil && has && info.ContentHash == hash {
		s.finishEmbedJob(pdfID)
		return
	}

	vec, err := s.gemini.Embed(ctx, s.pdfs.EmbedModel(), text)
	if err != nil {
		s.embeds.setState(pdfID, embedFailed, "falha ao chamar a API de embeddings")
		return
	}
	if err := s.pdfs.UpsertEmbedding(pdfID, hash, vec); err != nil {
		s.embeds.setState(pdfID, embedFailed, "erro interno")
		return
	}
	s.finishEmbedJob(pdfID)
}

func (s *Server) finishEmbedJob(pdfID string) {
	s.embeds.setState(pdfID, embedDone, "")
	time.AfterFunc(60*time.Second, func() { s.embeds.deleteIfDone(pdfID) })
}
