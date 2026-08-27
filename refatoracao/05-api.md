# 05 — Contrato de API

Contrato REST completo do backend. Todas as rotas ficam sob `/api`, com uma única exceção declarada (`GET /healthz`, fora do prefixo por convenção de healthcheck de infraestrutura). Toda rota exige sessão válida (cookie), exceto as marcadas **Público** na coluna Auth de cada tabela abaixo. Arquitetura geral em [Arquitetura](01-arquitetura.md), schema completo em [Modelo de dados](02-modelo-de-dados.md).

## Convenções gerais

- **Formato**: todas as respostas são JSON, com três exceções: rotas que servem binário (`.../file`, `.../thumbnail`, `.../preview`, `.../download`, `.../shared/{id}/file`), `GET /healthz` (texto puro `ok`) e `GET /s/{share_id}` (HTML da SPA).
- **Erros**: toda resposta de erro usa o envelope `{"error": "mensagem"}` com o status HTTP adequado (as rotas binárias e `/healthz` retornam apenas o status, sem corpo JSON). Ver [Legenda de status HTTP](#legenda-de-status-http).
- **Sucesso**: criação → `201` com o recurso criado; atualização → `200` com o recurso atualizado; remoção → `204` sem corpo; listagens paginadas → `200` `{"items":[...], "next_cursor": "<cursor>|null"}`.
- **Autenticação**: cookie de sessão (`HttpOnly; Secure; SameSite=Lax`). Mecanismo completo, expiração e rate limit de login em [Segurança](08-seguranca.md).
- **CSRF**: todo método não idempotente (`POST`/`PUT`/`PATCH`/`DELETE`) exige o header `X-CSRF-Token` além do cookie, exceto as rotas públicas de compartilhamento (somente leitura). Mecanismo completo em [Segurança](08-seguranca.md).
- **Paginação por cursor**: usada em `GET /api/pdfs` e `GET /api/annotations`. O `cursor` é uma string opaca; formato exato e regra de comparação documentados em [Frontend](06-frontend.md). As demais listagens (`tags`, `collections`, `signatures`, `shares`) não são paginadas — devolvem array completo.
- **Upload**: corpo limitado por `MAX_UPLOAD_MB` (ver [Docker/CI/Deploy](07-docker-ci-deploy.md)); tipo validado pelos magic bytes `%PDF-` lidos do próprio stream (ver [Segurança](08-seguranca.md)).
- **Representação de PDF**: toda resposta que devolve um PDF traz as colunas de `pdfs` (ver [Modelo de dados](02-modelo-de-dados.md)) mais `tags: [{"id","name"}]`, `notes_html` (o Markdown bruto de `notes` renderizado por `goldmark` e sanitizado por `bluemonday.UGCPolicy()` antes de sair no JSON — ver [Segurança](08-seguranca.md)), `embedding_status: "none"|"current"|"stale"` (ver [Embedding sob demanda](#embedding-sob-demanda)) e `has_text: bool` (se existe linha em `pdf_text` para o documento — só populado em `GET /api/pdfs/{id}`, não na listagem `GET /api/pdfs`; usado pelo viewer para decidir se faz o backfill de texto, ver [PDFs](#pdfs), nota sobre `POST .../text`). `storage_key`, `thumbnail_key` e `preview_key` são detalhes internos e não são expostos — o cliente obtém conteúdo pelas rotas dedicadas (`.../file`, `.../thumbnail`, `.../preview`, `.../download`).
- **"Admin"** nas rotas abaixo é um rótulo funcional (tela de administração), não uma permissão distinta — o produto tem um único usuário (ver [Visão geral](00-visao-geral.md)).

## Legenda de status HTTP

| Código | Significado |
|---|---|
| `400` | Validação de payload ou parâmetros falhou |
| `401` | Sem sessão válida (cookie ausente, inválido ou expirado) |
| `404` | Recurso inexistente |
| `409` | Conflito (duplicata, nome em uso, coleção padrão protegida, embedding já atualizado ou em curso) |
| `413` | Corpo da requisição acima de `MAX_UPLOAD_MB` |
| `415` | Tipo de conteúdo inválido (ex.: arquivo sem assinatura `%PDF-`) |
| `429` | Limite de tentativas excedido (rate limit) |
| `500` | Erro interno do servidor |

Código adicional, específico das rotas que gravam arquivo em disco (fora da lista genérica acima — ver [Storage](03-storage.md)):

| Código | Significado |
|---|---|
| `507` | Disco cheio durante a gravação (`Put`); nenhuma linha é inserida no banco |

## Auth

| Método | Caminho | Auth | Payload | Resposta |
|---|---|---|---|---|
| `POST` | `/api/auth/login` | Público¹ | `{"password": "string"}` | `200` + `Set-Cookie` com o token de sessão |
| `POST` | `/api/auth/logout` | Sessão | — | `200`; cookie expirado e linha removida de `sessions` |
| `GET` | `/api/auth/session` | Público¹ | — | `200` `{"authenticated": true\|false}` |

Erros: `POST /login` → `400` (payload malformado), `401` (senha incorreta), `429` (6ª tentativa dentro da janela de bloqueio — ver [Segurança](08-seguranca.md)), `500`. `POST /logout` → `401`, `500`. `GET /session` → `500`.

¹ `login` e `session` precisam funcionar sem sessão prévia — são, respectivamente, o mecanismo que **cria** a sessão e a checagem que a SPA usa antes de saber se uma existe. `logout` já exige o cookie, pois precisa saber qual sessão invalidar.

## PDFs

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/pdfs` | Query: `q, tag, collection, starred, archived, sort, cursor, limit` | `200` `{"items":[<PDF>...], "next_cursor"}` |
| `POST` | `/api/pdfs` | Multipart: `file` (obrigatório), `thumbnail`, `preview`, `text` (opcionais), `name`, `description`, `tags`, `collection_id` | `201` PDF criado |
| `POST` | `/api/pdfs/bulk` | Multipart com múltiplos `file` (um conjunto de metadados por arquivo) | `201` `{"results":[{"status":"created","pdf_id"}\|{"status":"duplicate","pdf_id","name"}, ...]}` |
| `GET` | `/api/pdfs/{id}` | — | `200` PDF completo |
| `PATCH` | `/api/pdfs/{id}` | JSON parcial: `name, description, notes, tags, collection_id, starred, archived, current_page` | `200` PDF atualizado |
| `DELETE` | `/api/pdfs/{id}` | — | `204` |
| `GET` | `/api/pdfs/{id}/file` | Header `Range` opcional | `200`/`206`, `application/pdf`, `Accept-Ranges: bytes` |
| `GET` | `/api/pdfs/{id}/thumbnail` | — | `200` `image/png` |
| `GET` | `/api/pdfs/{id}/preview` | — | `200` `image/png` |
| `POST` | `/api/pdfs/{id}/thumbnail` | Multipart: PNG gerado tardiamente pelo browser | `200` `{"thumbnail":"ok"}` |
| `POST` | `/api/pdfs/{id}/text` | `{"text": "string"}` | `200` `{"text":"ok"}` |
| `PUT` | `/api/pdfs/{id}/file` | Corpo: PDF editado | `200` `{"revision": <n>}` |
| `GET` | `/api/pdfs/{id}/download` | — | `200`, `Content-Disposition: attachment; filename="<name>.pdf"` |
| `POST` | `/api/pdfs/bulk-actions` | `{"action": "delete"\|"archive"\|"unarchive"\|"star"\|"unstar", "ids": ["..."]}` | `200` `{"updated": <n>}` |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/pdfs` → `400` (parâmetro inválido, ex. `sort`/`cursor` malformado), `401`, `500`.
- `POST /api/pdfs` → `400`, `401`, `409` (SHA-256 duplicado — ver [Comportamento de duplicidade](#comportamento-de-duplicidade)), `413`, `415`, `500`, `507`.
- `POST /api/pdfs/bulk` → `400`, `401`, `413`, `415`, `500`, `507` (duplicatas por item **não** geram `409` de requisição — ficam no array `results`, ver abaixo).
- `GET /api/pdfs/{id}` → `401`, `404`, `500`.
- `PATCH /api/pdfs/{id}` → `400`, `401`, `404`, `500`.
- `DELETE /api/pdfs/{id}` → `401`, `404`, `500`.
- `GET .../file`, `.../thumbnail`, `.../preview`, `.../download` → `401`, `404` (PDF inexistente no banco, **ou** arquivo ausente em disco — log de aviso citando `pdf_id` e chave, ver [Storage](03-storage.md)), `500`.
- `POST .../thumbnail` → `400`, `401`, `404`, `413`, `415`, `500`, `507`.
- `POST .../text` → `400` (`text` vazio ou payload malformado), `401`, `404`, `413`, `500`.
- `PUT .../file` → `400`, `401`, `404`, `413`, `415`, `500`, `507`.
- `POST /api/pdfs/bulk-actions` → `400` (`action` inválida), `401`, `500`.

Notas:
- `sort` aceita `newest|oldest|name_asc|name_desc|most_viewed|least_viewed|recently_viewed` — mesma lista fechada de `pdf.sorting`, default `newest` (ver [Modelo de dados](02-modelo-de-dados.md)).
- `tags` no upload é uma string com tags separadas por espaço, normalizada como em `Tag.parse_tag_string` (ver [Modelo de dados](02-modelo-de-dados.md)).
- `DELETE /api/pdfs/{id}` remove em cascata `pdf_tags`, `pdf_annotations`, `shares` e `pdf_embeddings` (todos `ON DELETE CASCADE`) e apaga PDF/thumbnail/preview do storage (ver [Storage](03-storage.md)).
- Editar `name`, `description` ou o texto extraído pode tornar `embedding_status` igual a `stale` (ver [Embedding sob demanda](#embedding-sob-demanda)).
- `PATCH` alterando `current_page` é o mecanismo usado pelo viewer para salvar a página atual, com debounce no cliente (ver [Frontend](06-frontend.md)).
- `POST .../text` backfila `pdf_text` para documentos que chegaram sem texto extraído (import do banco legado — ver `ETAPA-12-IMPORTACAO` em [ETAPAS.md](ETAPAS.md) —, ou watch-dir), reindexando FTS5 na mesma transação (ver [Busca híbrida](04-busca-hibrida.md), "Reindexação"). O viewer chama essa rota automaticamente ao abrir um PDF cujo `has_text` é `false`, extraindo o texto no navegador com o mesmo pdf.js do upload (ver [Frontend](06-frontend.md)). Também pode tornar `embedding_status` igual a `stale` se o documento já tivesse um embedding com hash calculado sobre o corpo vazio.

## Embedding sob demanda

| Método | Caminho | Auth | Payload | Resposta de sucesso |
|---|---|---|---|---|
| `POST` | `/api/pdfs/{id}/embed` | Sessão | — (sem corpo) | `200` `{"embedding_status":"current","dimensions":<n>}` |

Embeda **apenas** o documento `{id}`, de forma síncrona. Não existe endpoint de embedding em lote — embedar é sempre um documento por acionamento, e não existe worker, `time.Ticker` ou varredura automática (ver [Busca híbrida](04-busca-hibrida.md)). Um `sync.Mutex` no servidor serializa acionamentos concorrentes para nunca haver duas chamadas simultâneas à API Gemini.

Erros:

| Código | Motivo |
|---|---|
| `401` | Sem sessão |
| `404` | Documento inexistente |
| `409` | Já está `current` (o botão não deveria estar clicável — o servidor recusa mesmo assim), ou há outro embedding em curso no processo |
| `412` | `GEMINI_API_KEY` ausente — corpo `{"error":"busca semântica desabilitada"}` |
| `422` | Documento sem texto extraído (`pdf_text` vazio) — não há o que embedar |
| `502` | A API Gemini falhou — mensagem sanitizada no corpo, **nada é gravado** |
| `500` | Erro interno |

`GET /api/pdfs` e `GET /api/pdfs/{id}` devolvem o campo derivado `embedding_status` (`none`, `current` ou `stale`) que habilita/desabilita o botão na interface — ver [PDFs](#pdfs) e [Busca híbrida](04-busca-hibrida.md).

## Modelos de IA

Três rotas usadas pela área "Configurações → IA" e pelos botões "Descrever com IA"/"Sugerir tags" da página do PDF ([Frontend](06-frontend.md)); mecânica do `GeminiClient` (autenticação por header, `ListModels`, `GenerateText`) em [Busca Híbrida](04-busca-hibrida.md#autenticação-por-header).

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/ai/models` | — | `200` `{"embed":[{"name","display_name"}...],"text":[...]}` — listas nunca `null`, sempre `[]` quando vazias |
| `POST` | `/api/pdfs/{id}/describe` | — | `200` `{"description":"<texto>"}` — não persiste; o frontend salva pelo `PATCH /api/pdfs/{id}` já existente |
| `POST` | `/api/pdfs/{id}/suggest-tags` | — | `200` `{"tags":["..."]}` — sempre um array, nunca `null`; contém **apenas** nomes de tags já existentes no acervo (filtro determinístico no servidor, não confiança no prompt) |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/ai/models` → `401`; `412` `GEMINI_API_KEY` ausente; `502` a listagem falhou na API Gemini (mensagem sanitizada, nunca o corpo bruto do upstream).
- `POST /api/pdfs/{id}/describe` e `POST /api/pdfs/{id}/suggest-tags` → `401`; `404` PDF inexistente; `412` `GEMINI_API_KEY` ausente, ou nenhum modelo de texto escolhido em Configurações → IA (`settings['ai.text_model']` vazio); `422` documento sem texto extraído (mesma extração sob demanda do backfill do viewer — ver [Busca Híbrida](04-busca-hibrida.md#backfill-de-texto-pdf_text-ausente)); `502` a chamada generativa falhou na API Gemini; `500` erro interno.

`suggest-tags` nunca inventa uma tag: a resposta do modelo é normalizada (minúsculas, sem espaços nas bordas) e cruzada contra `GET /api/tags`, descartando qualquer linha que não bata com uma tag existente, cortada em 5 sugestões.

## Anotações

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/annotations` | Query: `kind=comment\|highlight` (opcional), `pdf_id` (opcional), `cursor` | `200` `{"items":[<Anotação>...], "next_cursor"}` |
| `POST` | `/api/pdfs/{id}/annotations` | `{"kind":"comment"\|"highlight", "page": <n>, "text": "string"}` | `201` anotação criada |
| `PATCH` | `/api/annotations/{id}` | `{"text": "string"}` | `200` anotação atualizada |
| `DELETE` | `/api/annotations/{id}` | — | `204` |
| `GET` | `/api/annotations/export` | Query: `kind`, `pdf_id`, `format=json\|yaml` | `200` download (`Content-Disposition: attachment`) |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/annotations` → `400` (`kind` inválido), `401`, `500`.
- `POST /api/pdfs/{id}/annotations` → `400` (`kind` fora de `comment`/`highlight`, `page` ausente), `401`, `404` (PDF inexistente), `500`.
- `PATCH /api/annotations/{id}` → `400`, `401`, `404`, `500`.
- `DELETE /api/annotations/{id}` → `401`, `404`, `500`.
- `GET /api/annotations/export` → `400` (`format` inválido), `401`, `500`.

## Tags

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/tags` | — | `200` `[{"id","name","count"}]` |
| `PATCH` | `/api/tags/{id}` | `{"name": "string"}` | `200` tag renomeada |
| `DELETE` | `/api/tags/{id}` | — | `204` |
| `POST` | `/api/tags/substitute` | `{"from_id": "string", "to_id": "string"}` | `200` tags fundidas |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/tags` → `401`, `500`.
- `PATCH /api/tags/{id}` → `400`, `401`, `404`, `409` (nome já em uso — `idx_tags_name`), `500`.
- `DELETE /api/tags/{id}` → `401`, `404`, `500`.
- `POST /api/tags/substitute` → `400` (`from_id == to_id`), `401`, `404` (algum dos dois ids inexistente), `500`.

Notas: nomes de tag podem conter `/` para hierarquia (modo árvore); a árvore é construída no cliente (ver [Frontend](06-frontend.md)). `POST /api/tags/substitute` move todas as associações de `from_id` para `to_id` sem duplicar linha em `pdf_tags` (chave composta `pdf_id, tag_id`) e remove `from_id`. Toda mutação que afeta PDFs (renomear, excluir, fundir) dispara reindexação FTS5 dos documentos afetados, na mesma transação (ver [Busca híbrida](04-busca-hibrida.md)).

## Coleções

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/collections` | — | `200` `[<Coleção>...]` — cada item traz `pdf_count: <n>`, contagem atual de PDFs na coleção |
| `POST` | `/api/collections` | `{"name": "string", "description": "string"}` | `201` coleção criada |
| `PATCH` | `/api/collections/{id}` | `{"name"?, "description"?}` | `200` coleção atualizada |
| `DELETE` | `/api/collections/{id}` | — | `204` |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/collections` → `401`, `500`.
- `POST /api/collections` → `400`, `401`, `409` (nome já em uso — `idx_collections_name`), `500`.
- `PATCH /api/collections/{id}` → `400`, `401`, `404`, `409`, `500`.
- `DELETE /api/collections/{id}` → `401`, `404`, `409` (rejeita a coleção padrão, `is_default=1`), `500`.

Atenção: `pdfs.collection_id` tem `ON DELETE CASCADE` (ver [Modelo de dados](02-modelo-de-dados.md)) — excluir uma coleção não padrão exclui em cascata todos os seus PDFs, tags associadas, anotações e embeddings. A proteção `409` na coleção padrão existe justamente para o acervo nunca ficar sem coleção de destino.

`pdf_count` só sai em `GET /api/collections` (calculado por `LEFT JOIN` + `COUNT` sobre `pdfs`, ver [Modelo de dados](02-modelo-de-dados.md)) — as respostas de `POST`/`PATCH` (`collectionResponse` sem contagem recalculada) não o incluem; o cliente mantém o valor já exibido ao atualizar nome/descrição em vez de sobrescrevê-lo com o zero-value da resposta.

## Compartilhamento

| Método | Caminho | Auth | Payload | Resposta |
|---|---|---|---|---|
| `POST` | `/api/pdfs/{id}/share` | Sessão | — | `201` `{"id","url"}` |
| `DELETE` | `/api/pdfs/{id}/share` | Sessão | — | `204` |
| `GET` | `/api/shares` | Sessão | — | `200` `[{"id","pdf_id","pdf_name","views","created_at"}]` |
| `GET` | `/s/{share_id}` | Público | — | `200` HTML da SPA em modo compartilhado |
| `GET` | `/api/shared/{share_id}` | Público | — | `200` metadados do PDF; incrementa `views` |
| `GET` | `/api/shared/{share_id}/file` | Público | Header `Range` opcional | `200`/`206` stream do PDF |

Erros por rota:
- `POST /api/pdfs/{id}/share` → `401`, `404`, `409` (PDF já compartilhado — `shares.pdf_id` é `UNIQUE`), `500`.
- `DELETE /api/pdfs/{id}/share` → `401`, `404`, `500`.
- `GET /api/shares` → `401`, `500`.
- `GET /s/{share_id}` → `404` (share inexistente ou revogado), `500`.
- `GET /api/shared/{share_id}` → `404`, `500`.
- `GET /api/shared/{share_id}/file` → `404`, `500`.

O compartilhamento público não tem senha nem expiração — mesmo comportamento do produto atual; é adição de escopo posterior, não parte desta refatoração.

## Assinaturas

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/signatures` | — | `200` `[{"id","name","data","created_at"}]` |
| `POST` | `/api/signatures` | `{"name": "string", "data": "data URL PNG"}` | `201` assinatura criada |
| `DELETE` | `/api/signatures/{id}` | — | `204` |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/signatures` → `401`, `500`.
- `POST /api/signatures` → `400` (`data` não é uma data URL PNG válida), `401`, `500`.
- `DELETE /api/signatures/{id}` → `401`, `404`, `500`.

## Preferências

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/settings` | — | `200` mapa completo chave→valor |
| `PATCH` | `/api/settings` | Mapa parcial `{"chave": "valor", ...}` | `200` mapa atualizado |

Todas exigem **Sessão**. Chaves e valores válidos são a lista fechada de [Modelo de dados](02-modelo-de-dados.md) — `PATCH` rejeita qualquer chave ou valor fora dela.

Erros por rota:
- `GET /api/settings` → `401`, `500`.
- `PATCH /api/settings` → `400` (chave desconhecida ou valor fora da lista fechada), `401`, `500`.

## Admin

| Método | Caminho | Payload | Resposta |
|---|---|---|---|
| `GET` | `/api/admin/info` | — | `200` `{"version","pdfs_count","tags_count","collections_count","files_bytes","embedding_status_counts":{"none","current","stale"}}` |
| `POST` | `/api/admin/reindex` | — | `200` `{"reindexed": true}` |
| `POST` | `/api/admin/reembed` | — | `202` `{"queued": <n>, "model": "<modelo em vigor>"}` |
| `GET` | `/api/admin/backup` | — | `200` binário `application/vnd.sqlite3`, `Content-Disposition: attachment` |
| `POST` | `/api/admin/restore` | corpo bruto: arquivo `.db`/`.sqlite3` | `200` `{"restored": true, "restarting": true}` |

Todas exigem **Sessão**.

Erros por rota:
- `GET /api/admin/info` → `401`, `500`.
- `POST /api/admin/reindex` → `401`, `500`.
- `POST /api/admin/reembed` → `401`, `412` (sem `GEMINI_API_KEY`), `500`.
- `GET /api/admin/backup` → `401`, `500`.
- `POST /api/admin/restore` → `400` (upload vazio, não é SQLite, `PRAGMA integrity_check` falhou, ou faltam as tabelas `pdfs`/`tags`/`settings`), `401`, `413` (excede `MAX_UPLOAD_MB`), `500`.

`POST /api/admin/reindex` reconstrói **somente** o índice FTS5 (`delete-all` + `INSERT ... SELECT`, ver [Busca híbrida](04-busca-hibrida.md)) — não dispara embedding de nada, porque embedding é sempre sob demanda. Não existe endpoint de storage: não há backend para escolher (ver [Storage](03-storage.md)).

`POST /api/admin/reembed` enfileira **todos** os documentos cujo `embedding_status` não é `current` — os `none` (nunca embedados) e os `stale` (conteúdo ou modelo de embedding mudou) — no mesmo worker serial de `POST /api/pdfs/{id}/embed`, e responde na hora com quantos entraram na fila. É o caminho para aplicar uma troca de modelo em Configurações → IA ao acervo inteiro sem clicar em "Reembedar" documento por documento: trocar `settings['ai.embed_model']` marca todo vetor existente como `stale` (o `content_hash` inclui o nome do modelo — ver [Busca híbrida](04-busca-hibrida.md), "Hash de conteúdo"). O progresso é lido pelo mesmo `GET /api/embed/jobs` do botão individual. A enfileiração é auto-regulada: no máximo o tamanho do buffer do canal fica à frente do worker, então a fila nunca cresce até o tamanho do acervo. Continua não existindo automatismo: nada dispara essa rota sozinho.

`GET /api/admin/backup` gera o arquivo com `VACUUM INTO` (primitiva nativa de backup online do SQLite): produz uma cópia coerente de arquivo único, sem WAL, mesmo com o servidor respondendo outras requisições — não pausa escritores. Contém apenas o banco (metadados, tags, anotações, configurações, embeddings); os PDFs em si vivem em `FILES` e não fazem parte do backup.

`POST /api/admin/restore` valida o upload (`PRAGMA integrity_check` + presença das tabelas obrigatórias) antes de tocar no banco em uso. Validado, fecha a conexão ativa, remove os sidecars `-wal`/`-shm` do banco anterior e substitui o arquivo em `DB_PATH` pelo upload. Em seguida envia `SIGTERM` a si mesmo — o mesmo caminho de shutdown gracioso que o processo já usa para o sinal do SO (ver `cmd/newpdfding/main.go`) — para que a política de restart do container (`restart: unless-stopped` em `compose.yaml`) suba um processo novo com todo store e worker reabertos contra o arquivo restaurado. Fora de um orquestrador com restart automático, o processo simplesmente encerra após o `SIGTERM` e precisa ser iniciado de novo manualmente.

## Saúde

| Método | Caminho | Auth | Payload | Resposta |
|---|---|---|---|---|
| `GET` | `/healthz` | Público | — | `200` corpo texto `ok` |

Única rota fora do prefixo `/api` — convenção padrão para healthchecks de infraestrutura, usada pelo `HEALTHCHECK` do Dockerfile (ver [Docker/CI/Deploy](07-docker-ci-deploy.md)). Não usa o envelope `{"error":...}` porque não retorna erro estruturado.

Erros: `500` (ex.: banco inacessível).

## Comportamento de duplicidade

Detecção por SHA-256, com índice único `idx_pdfs_sha256` (ver [Modelo de dados](02-modelo-de-dados.md)). O hash é sempre calculado no servidor durante o próprio streaming do upload — nunca aceito do cliente.

| Caminho de entrada | Comportamento em duplicata |
|---|---|
| `POST /api/pdfs` | `409` `{"error":"PDF já existe","pdf_id":"<id>","name":"<nome>"}`; nada é gravado — nem linha em `pdfs`, nem arquivo em disco. |
| `POST /api/pdfs/bulk` | Regra idêntica, aplicada **por item**: cada arquivo do lote é verificado individualmente; o item duplicado entra no array `results` com `{"status":"duplicate","pdf_id":"<id>","name":"<nome>"}`, sem interromper o processamento dos demais itens do lote. |
| Watch-dir (`CONSUME_DIR`, processo de background — não é uma rota HTTP; ver [Storage](03-storage.md) e [Docker/CI/Deploy](07-docker-ci-deploy.md)) | Ao detectar hash já existente, apenas grava um log e apaga o arquivo de `CONSUME_DIR`; nenhum registro é criado e não há resposta HTTP para reportar. |

## Ver também

- [Modelo de dados](02-modelo-de-dados.md) — schema completo, chaves de `settings`, normalização de tags.
- [Storage](03-storage.md) — entrega de arquivo, `Range`, falhas de disco.
- [Busca híbrida](04-busca-hibrida.md) — FTS5, embeddings, fusão RRF, `embedding_status`.
- [Frontend](06-frontend.md) — formato do cursor de paginação, ponte `postMessage` do viewer.
- [Docker/CI/Deploy](07-docker-ci-deploy.md) — `MAX_UPLOAD_MB` e demais variáveis de ambiente.
- [Segurança](08-seguranca.md) — cookies, CSRF, rate limit, validação de upload.
