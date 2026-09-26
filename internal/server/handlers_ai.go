package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/edalcin/newpdfding/internal/security"
	"github.com/edalcin/newpdfding/internal/store"
	"github.com/go-chi/chi/v5"
)

// aiBodyChars caps how much extracted text feeds a generative prompt —
// maior que embedBodyChars (2000, usado para embeddings) porque descrever
// e classificar exigem mais contexto que um vetor.
const aiBodyChars = 12000

// aiDescriptionChars caps the description written back to the database.
// A saída do modelo é entrada não confiável: o prompt pede 60 palavras, mas
// nada garante isso.
const aiDescriptionChars = 1200

// aiMaxSuggestedTags caps how many tag suggestions handleAISuggestTags
// returns, mirroring the limit given to the model in the prompt.
const aiMaxSuggestedTags = 5

// aiTextModel is the generative model chosen in Configurações → IA, or ""
// when the user has not chosen one — sem default embutido, porque nomes de
// modelo dependem da chave de API do usuário.
func (s *Server) aiTextModel() string { return s.settings.Get("ai.text_model") }

// requireTextModel writes the 412 envelope and returns "" when generative
// features are not usable.
func (s *Server) requireTextModel(w http.ResponseWriter) string {
	if s.gemini == nil {
		writeJSONError(w, http.StatusPreconditionFailed, "GEMINI_API_KEY não configurada")
		return ""
	}
	model := s.aiTextModel()
	if model == "" {
		writeJSONError(w, http.StatusPreconditionFailed, "selecione o modelo de texto em Configurações → IA")
		return ""
	}
	return model
}

// truncateChars cuts s to at most n runes, leaving it unchanged if it
// already fits.
func truncateChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ---------------------------------------------------------------------
// GET /api/ai/models
// ---------------------------------------------------------------------

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeJSONError(w, http.StatusPreconditionFailed, "GEMINI_API_KEY não configurada")
		return
	}
	text, err := s.gemini.ListModels(r.Context())
	if err != nil {
		log.Printf("warning: gemini list models: %v", err)
		writeJSONError(w, http.StatusBadGateway, "falha ao listar modelos da API Gemini")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

// ---------------------------------------------------------------------
// POST /api/pdfs/{id}/describe
// ---------------------------------------------------------------------

func (s *Server) handleAIDescribe(w http.ResponseWriter, r *http.Request) {
	model := s.requireTextModel(w)
	if model == "" {
		return
	}

	pdf, err := s.pdfs.GetByID(chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	body, err := s.textFor(r.Context(), pdf)
	if errors.Is(err, errNoText) {
		writeJSONError(w, http.StatusUnprocessableEntity, "este PDF não tem texto extraível")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	body = truncateChars(body, aiBodyChars)

	system := "Você resume documentos PDF. Responda sempre em português do Brasil, em um único parágrafo corrido de no máximo 60 palavras, sem título, sem lista e sem markdown."
	prompt := "Título do documento: " + pdf.Name + "\n\nTrecho do conteúdo:\n" + body

	out, err := s.gemini.GenerateText(r.Context(), model, system, prompt)
	if err != nil {
		log.Printf("warning: gemini describe pdf_id=%s: %v", pdf.ID, err)
		writeJSONError(w, http.StatusBadGateway, "falha ao gerar a descrição")
		return
	}
	description := strings.Join(strings.Fields(out), " ")
	if description == "" {
		writeJSONError(w, http.StatusBadGateway, "a IA não retornou uma descrição")
		return
	}
	description = truncateChars(description, aiDescriptionChars)

	writeJSON(w, http.StatusOK, map[string]string{"description": description})
}

// ---------------------------------------------------------------------
// POST /api/pdfs/{id}/suggest-tags
// ---------------------------------------------------------------------

func (s *Server) handleAISuggestTags(w http.ResponseWriter, r *http.Request) {
	model := s.requireTextModel(w)
	if model == "" {
		return
	}

	pdf, err := s.pdfs.GetByID(chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	body, err := s.textFor(r.Context(), pdf)
	if errors.Is(err, errNoText) {
		writeJSONError(w, http.StatusUnprocessableEntity, "este PDF não tem texto extraível")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	body = truncateChars(body, aiBodyChars)

	existing, err := s.tags.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(existing) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"tags": []string{}})
		return
	}
	existingSet := make(map[string]bool, len(existing))
	names := make([]string, len(existing))
	for i, t := range existing {
		existingSet[t.Name] = true
		names[i] = t.Name
	}

	system := "Você classifica documentos usando um vocabulário fechado de tags. Escolha apenas tags da lista fornecida, nunca invente tags. Responda com uma tag por linha, sem numeração e sem explicação. Se nenhuma tag servir, responda apenas NENHUMA."
	prompt := "Tags disponíveis:\n" + strings.Join(names, "\n") +
		"\n\nTítulo do documento: " + pdf.Name +
		"\n\nTrecho do conteúdo:\n" + body +
		"\n\nEscolha no máximo 5 tags."

	out, err := s.gemini.GenerateText(r.Context(), model, system, prompt)
	if err != nil {
		log.Printf("warning: gemini suggest tags pdf_id=%s: %v", pdf.ID, err)
		writeJSONError(w, http.StatusBadGateway, "falha ao sugerir tags")
		return
	}

	seen := make(map[string]bool)
	tags := make([]string, 0, aiMaxSuggestedTags)
	for _, line := range strings.Split(out, "\n") {
		name := strings.ToLower(strings.TrimSpace(line))
		if name == "" || name == "nenhuma" || seen[name] || !existingSet[name] {
			continue
		}
		seen[name] = true
		tags = append(tags, name)
		if len(tags) == aiMaxSuggestedTags {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// ---------------------------------------------------------------------
// POST /api/pdfs/{id}/chat
// ---------------------------------------------------------------------

// Limites do Chat do documento. O servidor não guarda a conversa: o
// navegador reenvia o histórico a cada pergunta, então tudo aqui é entrada
// não confiável.
const (
	chatBodyChars    = 400000 // texto extraído enviado ao modelo por pergunta
	chatQuestionMax  = 2000
	chatMaxExchanges = 20
	chatMaxBodyBytes = 200 << 10
)

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, chatMaxBodyBytes)
	var req struct {
		History []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"history"`
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "conversa longa demais — copie para Notas e recomece")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" || utf8.RuneCountInString(question) > chatQuestionMax {
		writeJSONError(w, http.StatusBadRequest, "a pergunta deve ter entre 1 e 2000 caracteres")
		return
	}
	if len(req.History) >= chatMaxExchanges {
		writeJSONError(w, http.StatusBadRequest, "conversa longa demais — copie para Notas e recomece")
		return
	}
	turns := make([]store.ChatTurn, 0, 2*len(req.History)+1)
	for _, h := range req.History {
		if strings.TrimSpace(h.Question) == "" || strings.TrimSpace(h.Answer) == "" {
			writeJSONError(w, http.StatusBadRequest, "malformed payload")
			return
		}
		turns = append(turns, store.ChatTurn{Role: "user", Text: h.Question}, store.ChatTurn{Role: "model", Text: h.Answer})
	}
	turns = append(turns, store.ChatTurn{Role: "user", Text: question})

	model := s.requireTextModel(w)
	if model == "" {
		return
	}
	pdf, err := s.pdfs.GetByID(chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "pdf not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	body, err := s.textFor(r.Context(), pdf)
	if errors.Is(err, errNoText) {
		writeJSONError(w, http.StatusUnprocessableEntity, "este PDF não tem texto extraível")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	truncated := utf8.RuneCountInString(body) > chatBodyChars
	body = truncateChars(body, chatBodyChars)

	system := "Você responde perguntas sobre um único documento PDF, usando somente o conteúdo dele, reproduzido abaixo. " +
		"Se a resposta não estiver no documento, diga que o documento não trata disso — não complete com conhecimento externo. " +
		"Responda sempre em português do Brasil, de forma direta. Pode usar markdown (listas, negrito, tabelas).\n\n" +
		"Título do documento: " + pdf.Name + "\n\nConteúdo do documento:\n" + body

	answer, err := s.gemini.GenerateChat(r.Context(), model, system, turns)
	if err != nil {
		log.Printf("warning: gemini chat pdf_id=%s: %v", pdf.ID, err)
		writeJSONError(w, http.StatusBadGateway, "falha ao gerar a resposta")
		return
	}
	answer = strings.TrimSpace(answer)
	html, err := security.RenderNotes(answer)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": answer, "answer_html": html, "truncated": truncated})
}
