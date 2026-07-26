# Etapas de Execução

> "Cada etapa é invocada num prompt novo pelo nome exato. A etapa só termina quando seu critério de aceitação for demonstrado. Commitar sempre em `main`."

As 14 etapas abaixo são o contrato de execução da refatoração, na ordem em que devem ser realizadas. Cada uma traz objetivo, entregas, dependências e o critério de aceitação que prova que a etapa está concluída.


## Status

| Etapa | Status |
|---|---|
| ETAPA-1-FUNDACAO | ✅ Concluída |
| ETAPA-2-AUTH | ✅ Concluída |
| ETAPA-3-STORAGE | ✅ Concluída |
| ETAPA-4-DOMINIO-PDF | ✅ Concluída |
| ETAPA-5-ANOTACOES | ✅ Concluída |
| ETAPA-6-BUSCA | ✅ Concluída |
| ETAPA-7-COMPARTILHAMENTO | ✅ Concluída |
| ETAPA-8-BACKGROUND | ✅ Concluída |
| ETAPA-9-UI-BASE | ✅ Concluída |
| ETAPA-10-UI-COMPLETA | ✅ Concluída |
| ETAPA-11-DOCKER-CI | ✅ Concluída |
| ETAPA-12-IMPORTACAO | ✅ Concluída |
| ETAPA-13-VALIDACAO | ⬜ Pendente |

## ETAPA-0-LIMPEZA

**Objetivo**
Executar a limpeza do repositório conforme [09-limpeza-repositorio.md](09-limpeza-repositorio.md), removendo o stack Django antigo por completo.

**Entregas**
- Execução integral da limpeza e reescrita descritas em [09-limpeza-repositorio.md](09-limpeza-repositorio.md).

**Depende de**
- Nenhuma (etapa inicial da sequência).

**Critério de aceitação**
- As quatro varreduras `grep` da seção "Varredura de resíduos" (ver [09-limpeza-repositorio.md](09-limpeza-repositorio.md)) retornam apenas ocorrências em `refatoracao/` e na atribuição do README.
- O repositório não contém mais nenhum `.py`.

## ETAPA-1-FUNDACAO

**Objetivo**
Estabelecer a fundação do backend Go: módulo, configuração, camada de dados e um servidor HTTP mínimo com `/healthz`.

**Entregas**
- `go.mod` via `go mod init`
- `internal/config`
- `internal/store` (`schema.sql`, `migrate.go`)
- `cmd/newpdfding/main.go`
- Servidor chi com rota `/healthz`
- Dockerfile provisório de 2 estágios (sem frontend)
- Variáveis lidas: `LISTEN_ADDR`, `LOG_LEVEL` — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-0-LIMPEZA

**Critério de aceitação**
- `go build ./...` passa.
- O binário sobe, cria o SQLite em `DB_PATH` e `curl localhost:8000/healthz` responde `ok`.
- `sqlite3 $DB_PATH ".tables"` lista todas as tabelas do [02-modelo-de-dados.md](02-modelo-de-dados.md).

## ETAPA-2-AUTH

**Objetivo**
Implementar autenticação por senha única, sessão, rate limiting, CSRF e os headers de segurança fixos.

**Entregas**
- Sessão
- Login/logout
- Rate limit
- CSRF
- Headers de segurança
- Variáveis lidas: `ADMIN_PASSWORD`, `SESSION_IDLE_MINUTES`, `TRUST_PROXY_HEADERS` — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-1-FUNDACAO

**Critério de aceitação**
- `POST /api/auth/login` com senha errada 5× devolve 429 na 6ª.
- Com a senha certa, devolve `Set-Cookie`.
- `GET /api/pdfs` sem cookie devolve 401.
- A resposta traz todos os headers do [08-seguranca.md](08-seguranca.md).

## ETAPA-3-STORAGE

**Objetivo**
Implementar a camada de armazenamento de arquivos em `internal/storage`, com uma única implementação local.

**Entregas**
- `internal/storage`: interfaces `Backend` e `Seeker`
- Implementação `LocalBackend`, com raiz em `FILES`

**Depende de**
- ETAPA-1-FUNDACAO

**Critério de aceitação**
- Teste Go que grava, lê, lista, faz `OpenSeek` com offset e apaga uma chave em `LocalBackend`.
- Teste que prova que chaves contendo `../`, caminho absoluto ou `\` são rejeitadas antes de qualquer I/O.
- Teste que prova que `Delete` remove os diretórios-pai vazios e para na raiz.
- `grep -rn "aws-sdk\|s3" internal/storage/` sem resultado.

## ETAPA-4-DOMINIO-PDF

**Objetivo**
Implementar o domínio central — coleções, tags e PDFs —, com upload, deduplicação e listagem paginada por cursor.

**Entregas**
- Coleções
- Tags
- PDFs: CRUD, upload multipart com thumbnail/preview/texto, dedup 409, estrela, arquivo, ações em lote, progresso
- Listagem com cursor
- Variáveis lidas: `MAX_UPLOAD_MB` — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-2-AUTH
- ETAPA-3-STORAGE

**Critério de aceitação**
- `curl -F` sobe um PDF e devolve 201.
- O mesmo arquivo enviado de novo devolve 409 com `pdf_id`.
- `GET /api/pdfs?limit=2&cursor=...` pagina sem repetir nem pular itens.

## ETAPA-5-ANOTACOES

**Objetivo**
Implementar anotações (comentários/destaques), suas listagens e exportação, assinaturas e a revisão de arquivo do PDF.

**Entregas**
- Anotações (comentário/destaque)
- Listagens
- Exportação YAML/JSON
- Assinaturas
- Revisão de arquivo (`PUT .../file`)

**Depende de**
- ETAPA-4-DOMINIO-PDF

**Critério de aceitação**
- Criar 3 anotações, listar por `kind`, exportar em ambos os formatos e conferir o conteúdo.
- `PUT` de um PDF novo incrementa `revision` para 2.

## ETAPA-6-BUSCA

**Objetivo**
Implementar a busca híbrida (FTS5 + embeddings sob demanda com fusão RRF), sem nenhum automatismo de embedding.

**Entregas**
- FTS5
- Endpoint `POST /api/pdfs/{id}/embed`
- Campo derivado `embedding_status`
- Cosseno
- Fusão RRF
- Filtros combinados
- **Nenhum worker, nenhum agendamento, nenhum embedding disparado por upload ou edição.**
- Variáveis lidas: `GEMINI_API_KEY`, `EMBED_MODEL` — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-4-DOMINIO-PDF

**Critério de aceitação**
- Com 3 PDFs de conteúdo distinto e nenhum embedado, `GET /api/pdfs?q=<termo do corpo>` retorna o PDF certo em primeiro lugar (só FTS5) e os três vêm com `embedding_status: "none"`.
- `POST /api/pdfs/{id}/embed` num deles devolve 200 e o `GET` seguinte mostra `current` para ele e `none` para os outros dois — provando que nada foi embedado sozinho.
- Repetir o `POST` no mesmo devolve 409.
- Um `PATCH` alterando a descrição desse documento faz o `GET` passar a devolver `stale`.
- Com o documento embedado, uma consulta por sinônimo que **não** ocorre no texto o retorna.
- Com `GEMINI_API_KEY` ausente, `POST .../embed` devolve 412 e a busca textual continua funcionando.
- `grep -rn "Ticker\|EMBED_SWEEP" internal/` sem resultado.

## ETAPA-7-COMPARTILHAMENTO

**Objetivo**
Implementar o compartilhamento público de PDFs: criação, revogação, rotas públicas e contador de visualizações.

**Entregas**
- Criar/revogar share
- Rotas públicas
- Contador
- Variáveis lidas: `BASE_URL` (montagem do link público de compartilhamento) — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-4-DOMINIO-PDF

**Critério de aceitação**
- `POST /api/pdfs/{id}/share` devolve URL.
- `GET /api/shared/{share}` sem cookie devolve 200 e incrementa `views`.
- Após `DELETE`, devolve 404.

## ETAPA-8-BACKGROUND

**Objetivo**
Implementar o único processo periódico de background do produto: consumo automático de PDFs por watch-dir.

**Entregas**
- Watch-dir de consumo
- Variáveis lidas: `CONSUME_ENABLE`, `CONSUME_DIR`, `CONSUME_INTERVAL_MINUTES`, `CONSUME_TAGS`, `CONSUME_SKIP_EXISTING` — ver [07-docker-ci-deploy.md](07-docker-ci-deploy.md).

**Depende de**
- ETAPA-4-DOMINIO-PDF
- ETAPA-3-STORAGE

**Critério de aceitação**
- Colocar um PDF em `CONSUME_DIR` e, no ciclo seguinte, ele aparece em `GET /api/pdfs` e sumiu do diretório.
- `grep -rn "aws-sdk\|minio\|S3\|Backup" internal/` sem resultado — não existe job de backup nesta refatoração.

## ETAPA-9-UI-BASE

**Objetivo**
Construir a base do frontend SvelteKit: scaffold, embutimento no binário Go, login, biblioteca e upload.

**Entregas**
- Scaffold SvelteKit + `adapter-static` + Tailwind + shadcn-svelte + Boxicons
- `go:embed`
- Login
- Biblioteca com rolagem infinita e os 4 layouts
- Upload com pdf.js no navegador
- Tema claro/escuro
- PWA

**Depende de**
- ETAPA-5-ANOTACOES

**Critério de aceitação**
- `npm run build` gera `frontend/build`.
- A imagem Docker serve a SPA em `/`.
- Upload pela UI cria o PDF com thumbnail.
- A lista carrega páginas ao rolar.
- O toggle de tema persiste após recarregar.
- Lighthouse reconhece o app como instalável.

## ETAPA-10-UI-COMPLETA

**Objetivo**
Completar o frontend com as telas e funcionalidades restantes, fechando a paridade com o produto atual.

**Entregas**
- Detalhes com TipTap
- Viewer pdf.js com a ponte `postMessage`
- Telas de destaques/comentários
- Coleções
- Tags
- Configurações
- Administração
- Viewer público

**Depende de**
- ETAPA-9-UI-BASE
- ETAPA-6-BUSCA
- ETAPA-7-COMPARTILHAMENTO

**Critério de aceitação**
- Percorrer manualmente a tabela de paridade do [10-inventario-funcionalidades.md](10-inventario-funcionalidades.md) e marcar todas as linhas.
- Abrir um PDF, criar um destaque, fechar e reabrir mostrando o destaque persistido.

## ETAPA-11-DOCKER-CI

**Objetivo**
Finalizar a imagem Docker de produção, o pipeline de CI/CD e a documentação de deploy.

**Entregas**
- Dockerfile final de 3 estágios em distroless
- Workflow com testes + Trivy
- Dependabot
- `.env.example`
- `UNRAID.md`
- `README.md`
- `compose.yaml`

**Depende de**
- ETAPA-10-UI-COMPLETA

**Critério de aceitação**
- `docker build` conclui e `docker images` mostra < 60 MB.
- O container sobe como `nonroot`.
- O workflow verde no GitHub publica `ghcr.io/edalcin/newpdfding:latest`.
- Trivy sem CRITICAL/HIGH.

## ETAPA-12-IMPORTACAO

**Objetivo**
Implementar a importação única do banco Django legado para o schema novo.

**Entregas**
- Comando único `newpdfding -import-legacy <caminho do db.sqlite3 antigo> <caminho do media antigo>`
- Reinserção de PDFs, tags, coleções, anotações e shares no schema novo
- Cópia dos arquivos para o backend ativo
- Nenhum embedding automático: documentos importados entram com `embedding_status: "none"`

**Depende de**
- ETAPA-11-DOCKER-CI

**Critério de aceitação**
- Importar uma cópia do banco de produção e conferir que as contagens de PDFs, tags e anotações batem com as do banco antigo.
- `SELECT count(*) FROM pdf_embeddings` é `0`.

## ETAPA-13-VALIDACAO

**Objetivo**
Validar de ponta a ponta a refatoração — resíduos, segurança, paridade e changelog — antes de encerrar.

**Entregas**
- Varredura final de resíduos
- Checklist de segurança do [08-seguranca.md](08-seguranca.md)
- Tabela de paridade do [10-inventario-funcionalidades.md](10-inventario-funcionalidades.md) 100% marcada
- `CHANGELOG.md`

**Depende de**
- ETAPA-12-IMPORTACAO

**Critério de aceitação**
- Os quatro `grep` limpos.
- Checklist completo.
- Tabela sem linha aberta.
