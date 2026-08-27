# Changelog

Todas as mudanças notáveis deste projeto são documentadas aqui.

## [Unreleased]

### Added

- Caixa de busca híbrida (léxica + semântica) e lista de tags clicável na biblioteca (`/`), com debounce e filtro combinável — ver [`refatoracao/06-frontend.md`](refatoracao/06-frontend.md#busca-e-filtro-por-tag-na-biblioteca).
- `TagPicker`: combobox de tags na página de detalhes do PDF, com autocompletar das tags existentes e criação de tag nova inline — ver [`refatoracao/06-frontend.md`](refatoracao/06-frontend.md#tagpicker--combobox-de-tags).
- Contagem de PDFs por coleção (`pdf_count`) em `GET /api/collections` e na tela `/collections` — ver [`refatoracao/05-api.md`](refatoracao/05-api.md#coleções).
- `POST /api/pdfs/{id}/text`: backfill de texto extraído para documentos que chegaram sem ele (import do banco legado, watch-dir). O visualizador chama essa rota automaticamente na primeira abertura de um PDF sem texto, tornando-o buscável e embedável sem re-upload — ver [`refatoracao/04-busca-hibrida.md`](refatoracao/04-busca-hibrida.md#backfill-de-texto-pdf_text-ausente).
- Backup e restore do banco SQLite na área administrativa (`/admin`): `GET /api/admin/backup` baixa uma cópia consistente via `VACUUM INTO`; `POST /api/admin/restore` valida o upload (integridade + tabelas obrigatórias) antes de substituir o banco em uso e reinicia o processo (via `SIGTERM` — o mesmo caminho de shutdown gracioso do main), deixando a política de restart do container (`restart: unless-stopped`) reabrir tudo contra o arquivo restaurado — ver [`refatoracao/05-api.md`](refatoracao/05-api.md#admin).
- `POST /api/admin/reembed` e o botão **Reembedar pendentes** em `/admin`: enfileiram, de uma só vez, todos os documentos cujo `embedding_status` não é `current` (`none` + `stale`) no worker serial de embedding. É o caminho para aplicar uma troca do modelo de embedding em Configurações → IA ao acervo inteiro, sem clicar documento por documento — ver [`refatoracao/04-busca-hibrida.md`](refatoracao/04-busca-hibrida.md#reembedar-o-acervo-inteiro).

### Fixed

- **Crítico**: `frontend/.gitignore` excluía o diretório `static/pdfjs/` inteiro, incluindo `viewer.html`/`viewer.mjs`/`viewer.css` — os três únicos arquivos ali que são código próprio deste repositório (os demais são copiados de `node_modules/pdfjs-dist` por `scripts/copy-pdfjs.mjs` a cada build). Resultado: nenhum checkout limpo (CI, build Docker) jamais teve esses três arquivos, então `/pdfjs/viewer.html` sempre 404ava em produção, e a SPA renderizava sua própria página 404 *dentro do iframe* do visualizador — o "404 Not Found" ao abrir qualquer PDF. Corrigido restringindo o `.gitignore` aos arquivos realmente gerados e commitando os três arquivos custom.
- **Crítico**: `/sw.js` era servido sem `Cache-Control`, e a Cloudflare na frente da instância de produção o cacheava na borda por 4h (cache por extensão de arquivo). Toda correção feita no service worker ficava presa nesse cache, nunca chegando aos navegadores reais, independente de quantos redeploys da origem acontecessem. Corrigido com `Cache-Control: no-store` em `/sw.js` e `no-cache` no shell da SPA (`index.html`/fallback) — mas o cache de borda já existente em qualquer CDN na frente ainda precisa de purge manual para parar de servir a cópia antiga imediatamente.
- Service worker (`frontend/static/sw.js`): todo `fetch` agora usa `AbortController` com timeout de 8s. Sem isso, uma conexão que travasse (não falhava, só nunca resolvia) deixava a página pendurada indefinidamente — reproduzido em teste de browser real com o login nunca completando.
- Criar um destaque/comentário no visualizador gerava duas linhas duplicadas em `pdf_annotations`: o SDK EmbedPDF emite o evento `create` duas vezes por anotação nova (uma vez otimista, com `committed: false`, e outra após o `commit()` automático confirmar a escrita no engine, com `committed: true`), e `handleAnnotationEvent` (`frontend/src/routes/viewer/[id]/+page.svelte`) chamava a API de criação em ambas, sem checar a flag. Corrigido ignorando eventos `create` com `committed: false`.
- Busca semântica podia pontuar vetores gravados por um modelo de embedding com dimensionalidade diferente: `dotProduct` truncava para o comprimento menor, e o produto escalar sobre o prefixo comum é uma similaridade inventada, capaz de passar do piso de 0,30 e poluir o ranking. Agora comprimentos diferentes pontuam `0` — os vetores do modelo antigo ficam fora dos resultados até serem reembedados.

### Security

- Resolvidas as 14 vulnerabilidades apontadas pelo Dependabot e mais uma achada pelo `npm audit`: `nanoid` 3.3.16 → 3.3.18 (alta, transitiva via `postcss`), `@sveltejs/kit` → 2.70.3 (ReDoS na negociação de conteúdo), `github.com/go-chi/chi/v5` 5.2.5 → 5.3.2 (três vulnerabilidades no roteador, todas alcançáveis pelo código) e a linha do Go para 1.27 (`toolchain go1.27.0` no `go.mod`, `golang:1.27-alpine` no Dockerfile, `go-version: "1.27"` no CI), fechando dez achados de biblioteca padrão em `net/http`, `crypto/tls`, `crypto/x509`, `net/url`, `encoding/asn1` e `net/textproto`. `govulncheck ./...` e `npm audit` agora reportam zero.
- `govulncheck` no CI deixa de ser `continue-on-error`. Era decorativo — reportava no log e o build passava mesmo assim, e foi por isso que 13 achados acumularam sem ninguém notar.
- Os 12 alertas restantes do Dependabot (`django`, `sqlparse`, `pypdf` em `uv.lock`) foram dispensados como *not used*: a stack Django legada foi removida em `5b292f0` e não existe mais nenhum arquivo Python no repositório — nada disso entra na imagem publicada.
- `aquasecurity/trivy-action` deixa de ser referenciada por `@master` e passa a uma tag fixa (`0.36.0`). Action apontada para branch móvel executa código novo a cada run, e o Dependabot não consegue versioná-la.

### Changed

- Todas as dependências passaram para a versão corrente, zerando a fila de PRs do Dependabot: `pdfjs-dist` 5.5.207 → 6.2.108 (major, sem nenhuma mudança de código necessária — a v6 tornou `canvas` obrigatório em `RenderParameters` e `pdf-process.ts` já passava esse campo), `@embedpdf/*` 2.14.4 → 2.15.0, `modernc.org/sqlite` 1.48.2 → 1.57.0, `goldmark` 1.8.4 → 1.8.5, `svelte`, `vite`, `svelte-check`, `@tiptap/*`, `tailwind-variants`, as bases Docker (`node:22-alpine` → `node:24-alpine`, a LTS ativa) e as actions do CI (`checkout` v7, `setup-node` v7, `setup-go` v7, `login-action` v4, `setup-buildx-action` v4, `build-push-action` v7, `metadata-action` v6).
- `typescript` fica em 6.0.3 de propósito: a 7.0.2 viola o peer range declarado por `@sveltejs/kit` e `svelte-check` (`^5.3.3 || ^6.0.0`). Sobe quando o SvelteKit passar a suportar.
- `.github/dependabot.yml` passa a ignorar major do `node`: a imagem de build segue a linha LTS ativa, não a "Current". Sem isso o Dependabot reabre um PR por major nova (26, 28…) cuja resposta correta é sempre "ainda não".

