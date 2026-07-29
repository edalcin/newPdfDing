# Changelog

Todas as mudanças notáveis deste projeto são documentadas aqui.

## [Unreleased]

### Added

- Caixa de busca híbrida (léxica + semântica) e lista de tags clicável na biblioteca (`/`), com debounce e filtro combinável — ver [`refatoracao/06-frontend.md`](refatoracao/06-frontend.md#busca-e-filtro-por-tag-na-biblioteca).
- `TagPicker`: combobox de tags na página de detalhes do PDF, com autocompletar das tags existentes e criação de tag nova inline — ver [`refatoracao/06-frontend.md`](refatoracao/06-frontend.md#tagpicker--combobox-de-tags).
- Contagem de PDFs por coleção (`pdf_count`) em `GET /api/collections` e na tela `/collections` — ver [`refatoracao/05-api.md`](refatoracao/05-api.md#coleções).
- `POST /api/pdfs/{id}/text`: backfill de texto extraído para documentos que chegaram sem ele (import do banco legado, watch-dir). O visualizador chama essa rota automaticamente na primeira abertura de um PDF sem texto, tornando-o buscável e embedável sem re-upload — ver [`refatoracao/04-busca-hibrida.md`](refatoracao/04-busca-hibrida.md#backfill-de-texto-pdf_text-ausente).
- Backup e restore do banco SQLite na área administrativa (`/admin`): `GET /api/admin/backup` baixa uma cópia consistente via `VACUUM INTO`; `POST /api/admin/restore` valida o upload (integridade + tabelas obrigatórias) antes de substituir o banco em uso e reinicia o processo (via `SIGTERM` — o mesmo caminho de shutdown gracioso do main), deixando a política de restart do container (`restart: unless-stopped`) reabrir tudo contra o arquivo restaurado — ver [`refatoracao/05-api.md`](refatoracao/05-api.md#admin).

### Fixed

- **Crítico**: `frontend/.gitignore` excluía o diretório `static/pdfjs/` inteiro, incluindo `viewer.html`/`viewer.mjs`/`viewer.css` — os três únicos arquivos ali que são código próprio deste repositório (os demais são copiados de `node_modules/pdfjs-dist` por `scripts/copy-pdfjs.mjs` a cada build). Resultado: nenhum checkout limpo (CI, build Docker) jamais teve esses três arquivos, então `/pdfjs/viewer.html` sempre 404ava em produção, e a SPA renderizava sua própria página 404 *dentro do iframe* do visualizador — o "404 Not Found" ao abrir qualquer PDF. Corrigido restringindo o `.gitignore` aos arquivos realmente gerados e commitando os três arquivos custom.
- **Crítico**: `/sw.js` era servido sem `Cache-Control`, e a Cloudflare na frente da instância de produção o cacheava na borda por 4h (cache por extensão de arquivo). Toda correção feita no service worker ficava presa nesse cache, nunca chegando aos navegadores reais, independente de quantos redeploys da origem acontecessem. Corrigido com `Cache-Control: no-store` em `/sw.js` e `no-cache` no shell da SPA (`index.html`/fallback) — mas o cache de borda já existente em qualquer CDN na frente ainda precisa de purge manual para parar de servir a cópia antiga imediatamente.
- Service worker (`frontend/static/sw.js`): todo `fetch` agora usa `AbortController` com timeout de 8s. Sem isso, uma conexão que travasse (não falhava, só nunca resolvia) deixava a página pendurada indefinidamente — reproduzido em teste de browser real com o login nunca completando.

### Changed

