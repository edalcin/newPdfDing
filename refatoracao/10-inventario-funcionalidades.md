# Inventário de Funcionalidades — Paridade newPdfDing → Go/SvelteKit

Este é o contrato de "nenhuma funcionalidade perdida" da refatoração. Cada linha é um item do levantamento feito sobre o código Django hoje em produção — enumeração canônica de rotas em `pdfding/pdf/urls.py`, `pdfding/admin/urls.py`, `pdfding/core/urls.py` e `pdfding/users/urls.py` — apontando para onde a mesma capacidade passa a viver na arquitetura Go + SvelteKit e em qual etapa de [ETAPAS.md](ETAPAS.md) ela é entregue. Nenhuma funcionalidade listada abaixo é descartada sem estar também na seção final de remoções intencionais.

## Tabela de paridade

### Biblioteca, upload e organização

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Upload individual | `pdfding/pdf/views/pdf_views.py:Add` | `POST /api/pdfs` (multipart `file`, `thumbnail`, `preview`, `text`, `name`, `description`, `tags`, `collection_id`, `file_directory`) — [API](05-api.md); pré-processamento (thumbnail/preview/texto) roda no pdf.js do navegador antes do envio — [Frontend](06-frontend.md) | ETAPA-4-DOMINIO-PDF |
| Upload em lote | `pdfding/pdf/views/pdf_views.py:BulkAdd` | `POST /api/pdfs/bulk` (multipart múltiplo) — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Detecção de duplicidade por SHA-256 | `pdfding/pdf/services/pdf_services.py:compute_file_sha256`, `pdfding/pdf/services/workspace_services.py:check_if_pdf_with_hash_exists` | hash SHA-256 calculado no stream do servidor; índice único `idx_pdfs_sha256` sobre `pdfs.sha256` — [Modelo de Dados](02-modelo-de-dados.md); resposta `409` com `{"error":"PDF já existe","pdf_id","name"}` em `POST /api/pdfs`, `/bulk` e na consumo — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Biblioteca com 4 layouts (compact/list/grid/minimal) | `pdfding/users/models.py:Profile.layout` | componentes de layout da rota `/` (biblioteca), persistidos em `settings['ui.layout']` — [Modelo de Dados](02-modelo-de-dados.md), [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |
| 7 ordenações da biblioteca | `pdfding/users/models.py:Profile.pdf_sorting` | `settings['pdf.sorting']` (`newest\|oldest\|name_asc\|name_desc\|most_viewed\|least_viewed\|recently_viewed`) — [Modelo de Dados](02-modelo-de-dados.md); aplicado via `GET /api/pdfs?sort=` — [API](05-api.md); seletor na barra da biblioteca — [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |

### Busca e navegação

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Busca (hoje RapidFuzz) | `pdfding/pdf/views/pdf_views.py:OverviewMixin.fuzzy_filter_pdfs` (`rapidfuzz.fuzz.WRatio`/`partial_ratio`) | índice FTS5 (`pdfs_fts`) com fallback `LIKE`, fundido por RRF com embeddings sob demanda — [Busca Híbrida](04-busca-hibrida.md); `GET /api/pdfs?q=` — [API](05-api.md) | ETAPA-6-BUSCA |
| Filtro por tag | `pdfding/pdf/views/pdf_views.py:OverviewMixin.filter_objects` | `GET /api/pdfs?tag=` — [API](05-api.md); tabela de junção `pdf_tags` — [Modelo de Dados](02-modelo-de-dados.md); combinado como `WHERE` após a fusão RRF — [Busca Híbrida](04-busca-hibrida.md) | ETAPA-4-DOMINIO-PDF |
| Modo árvore de tags com `/` | `pdfding/users/models.py:Profile.tag_tree_mode` | `settings['ui.tag_tree_mode']` — [Modelo de Dados](02-modelo-de-dados.md); tela `/tags` — [Frontend](06-frontend.md) | ETAPA-10-UI-COMPLETA |
| Página de detalhes | `pdfding/pdf/views/pdf_views.py:Details` | rota `/pdf/[id]` — [Frontend](06-frontend.md); dados via `GET /api/pdfs/{id}` — [API](05-api.md) | ETAPA-10-UI-COMPLETA |

### Metadados do documento

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Notas em Markdown sanitizado | `pdfding/pdf/models/pdf_models.py:Pdf.notes_html` | coluna `pdfs.notes` (Markdown bruto) — [Modelo de Dados](02-modelo-de-dados.md); editada via `PATCH /api/pdfs/{id}` — [API](05-api.md); renderizada com `goldmark` e sanitizada com `bluemonday.UGCPolicy()` — [Segurança](08-seguranca.md); editor rico TipTap na página de detalhes — [Frontend](06-frontend.md) | ETAPA-4-DOMINIO-PDF |
| Descrição | `pdfding/pdf/views/pdf_views.py:Edit` (campo `description`) | coluna `pdfs.description` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Renomear | `pdfding/pdf/views/pdf_views.py:Edit` (campo `name`) | coluna `pdfs.name` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Mover de coleção | `pdfding/pdf/services/collection_services.py:change_collection_of_pdf` | coluna `pdfs.collection_id` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` campo `collection_id` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Subdiretórios | `pdfding/pdf/models/pdf_models.py:Pdf.file_directory` | coluna `pdfs.file_directory` — [Modelo de Dados](02-modelo-de-dados.md); validada contra `^[A-Za-z0-9_\-/]{0,120}$` — [Segurança](08-seguranca.md); `PATCH /api/pdfs/{id}` campo `file_directory` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |

### Ações sobre o PDF

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Estrela | `pdfding/pdf/views/pdf_views.py:Star` | coluna `pdfs.starred` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` campo `starred` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Arquivar | `pdfding/pdf/views/pdf_views.py:Archive` | coluna `pdfs.archived` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` campo `archived` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Ações em lote (delete/archive/unarchive/star/unstar) | `pdfding/pdf/views/pdf_bulk_action_views.py:BulkActions` | `POST /api/pdfs/bulk-actions` `{action: "delete"\|"archive"\|"unarchive"\|"star"\|"unstar", ids: []}` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Excluir | `pdfding/pdf/views/pdf_views.py:Delete` | `DELETE /api/pdfs/{id}` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Baixar | `pdfding/pdf/views/pdf_views.py:Download` | `GET /api/pdfs/{id}/download` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |

### Entrega de arquivo e progresso de leitura

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Thumbnail | `pdfding/pdf/views/pdf_views.py:ServeThumbnail` | `GET /api/pdfs/{id}/thumbnail`; gerada pelo pdf.js do navegador no upload, ou enviada tardiamente via `POST /api/pdfs/{id}/thumbnail` — [API](05-api.md); chave `{collection_id}/thumb/{pdf_id}.png` — [Storage](03-storage.md) | ETAPA-4-DOMINIO-PDF |
| Preview | `pdfding/pdf/views/pdf_views.py:ServePreview`, `pdfding/pdf/views/pdf_views.py:ShowPreview` | `GET /api/pdfs/{id}/preview` — [API](05-api.md); chave `{collection_id}/preview/{pdf_id}.png` — [Storage](03-storage.md) | ETAPA-4-DOMINIO-PDF |
| Contador de visualizações | `pdfding/pdf/models/pdf_models.py:Pdf.views` | coluna `pdfs.views` — [Modelo de Dados](02-modelo-de-dados.md); incrementada ao servir o arquivo — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |
| Progresso de leitura e página atual | `pdfding/pdf/views/pdf_views.py:UpdatePage`, `pdfding/pdf/models/pdf_models.py:Pdf.progress` | coluna `pdfs.current_page` — [Modelo de Dados](02-modelo-de-dados.md); `PATCH /api/pdfs/{id}` campo `current_page`, salvo com debounce de 2s pela ponte do viewer — [API](05-api.md), [Frontend](06-frontend.md) | ETAPA-4-DOMINIO-PDF |
| Barras de progresso | `pdfding/users/models.py:Profile.show_progress_bars` | `settings['ui.show_progress_bars']` — [Modelo de Dados](02-modelo-de-dados.md); exibidas nos cards da biblioteca — [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |

### Viewer e revisão de arquivo

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Viewer pdf.js | `pdfding/pdf/views/pdf_views.py:ViewerView` | rota `/viewer/[id]`; pdf.js 5.5.207 embutido como asset estático em `frontend/static/pdfjs/`, aberto em iframe com ponte `postMessage` — [Frontend](06-frontend.md) | ETAPA-10-UI-COMPLETA |
| Salvar PDF editado com revisão | `pdfding/pdf/views/pdf_views.py:UpdatePdf`, `pdfding/pdf/models/pdf_models.py:Pdf.revision` | `PUT /api/pdfs/{id}/file` incrementa `pdfs.revision` — [API](05-api.md), [Modelo de Dados](02-modelo-de-dados.md) | ETAPA-5-ANOTACOES |

### Anotações e assinaturas

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Comentários | `pdfding/pdf/models/pdf_models.py:PdfComment` | tabela `pdf_annotations` com `kind='comment'` — [Modelo de Dados](02-modelo-de-dados.md); `POST /api/pdfs/{id}/annotations` `{kind:"comment", page, text}` — [API](05-api.md) | ETAPA-5-ANOTACOES |
| Destaques | `pdfding/pdf/models/pdf_models.py:PdfHighlight` | tabela `pdf_annotations` com `kind='highlight'` — [Modelo de Dados](02-modelo-de-dados.md); `POST /api/pdfs/{id}/annotations` `{kind:"highlight", page, text}` — [API](05-api.md) | ETAPA-5-ANOTACOES |
| Visão geral de destaques e comentários paginada | `pdfding/pdf/views/pdf_views.py:HighlightOverview`, `pdfding/pdf/views/pdf_views.py:CommentOverview` | `GET /api/annotations?kind=highlight\|comment&cursor=` — [API](05-api.md); telas `/highlights` e `/comments` — [Frontend](06-frontend.md) | ETAPA-5-ANOTACOES |
| Visão de anotações por PDF | `pdfding/pdf/views/pdf_views.py:DetailsHighlightOverview`, `pdfding/pdf/views/pdf_views.py:DetailsCommentOverview` | `GET /api/annotations?kind=&pdf_id=&cursor=` — [API](05-api.md) | ETAPA-5-ANOTACOES |
| Exportação YAML/JSON de anotações | `pdfding/pdf/views/pdf_views.py:ExportAnnotations` | `GET /api/annotations/export?kind=&pdf_id=&format=json\|yaml` — [API](05-api.md) | ETAPA-5-ANOTACOES |
| Ordenação de anotações | `pdfding/users/models.py:Profile.annotation_sorting` | `settings['annotation.sorting']` — [Modelo de Dados](02-modelo-de-dados.md); aplicada na listagem `GET /api/annotations` — [API](05-api.md) | ETAPA-5-ANOTACOES |
| Assinaturas | `pdfding/users/models.py:Profile.signatures`, `pdfding/users/views.py:Signatures` | tabela `signatures` — [Modelo de Dados](02-modelo-de-dados.md); `GET/POST/DELETE /api/signatures` — [API](05-api.md); aplicação na página via ponte do viewer — [Frontend](06-frontend.md) | ETAPA-5-ANOTACOES |

### Preferências do viewer

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Modo invertido | `pdfding/users/models.py:Profile.pdf_inverted_mode` | `settings['viewer.inverted']` — [Modelo de Dados](02-modelo-de-dados.md); acionado pela ponte `postMessage` do viewer — [Frontend](06-frontend.md) | ETAPA-10-UI-COMPLETA |
| Manter tela ligada | `pdfding/users/models.py:Profile.pdf_keep_screen_awake` | `settings['viewer.keep_awake']` — [Modelo de Dados](02-modelo-de-dados.md); `navigator.wakeLock` acionado pela ponte do viewer — [Frontend](06-frontend.md) | ETAPA-10-UI-COMPLETA |

### Tags e coleções

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Criar/editar/excluir tag | `pdfding/pdf/views/pdf_views.py:EditTag`, `pdfding/pdf/views/pdf_views.py:DeleteTag` | `PATCH /api/tags/{id}` (renomear), `DELETE /api/tags/{id}`; criação implícita ao associar `tags` em `POST`/`PATCH /api/pdfs` — [API](05-api.md); tabelas `tags`/`pdf_tags` — [Modelo de Dados](02-modelo-de-dados.md) | ETAPA-4-DOMINIO-PDF |
| Administração de tags: renomear/excluir/fundir | `pdfding/admin/views.py:RenameTag`, `pdfding/admin/views.py:DeleteTag`, `pdfding/admin/views.py:SubstituteTag` | mesmos endpoints de tags (`PATCH /api/tags/{id}`, `DELETE /api/tags/{id}`, `POST /api/tags/substitute` para fundir) — [API](05-api.md); a distinção de privilégio "admin" desaparece — produto é de usuário único | ETAPA-4-DOMINIO-PDF |
| Coleções CRUD | `pdfding/pdf/views/collection_views.py` | `GET/POST/PATCH/DELETE /api/collections` — [API](05-api.md); tabela `collections` — [Modelo de Dados](02-modelo-de-dados.md) | ETAPA-4-DOMINIO-PDF |
| Coleção padrão | `pdfding/pdf/models/collection_models.py:Collection.default_collection` | coluna `collections.is_default`; seed automático de uma coleção `Default` no boot se nenhuma existir — [Modelo de Dados](02-modelo-de-dados.md); `DELETE /api/collections/{id}` rejeita a padrão com `409` — [API](05-api.md) | ETAPA-4-DOMINIO-PDF |

### Compartilhamento e administração

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Link de compartilhamento | `pdfding/pdf/views/pdf_views.py:SharePdf`, `pdfding/pdf/views/pdf_views.py:UnsharePdf` | `POST /api/pdfs/{id}/share` → `{id, url}`; `DELETE /api/pdfs/{id}/share` — [API](05-api.md); tabela `shares` — [Modelo de Dados](02-modelo-de-dados.md) | ETAPA-7-COMPARTILHAMENTO |
| Viewer público | `pdfding/pdf/views/pdf_views.py:ViewSharedPdf`, `pdfding/pdf/views/pdf_views.py:ServeSharedPdf` | `GET /s/{share_id}` (SPA em modo compartilhado) e `GET /api/shared/{share_id}/file` (stream com `Range`) — [API](05-api.md); ponte do viewer montada somente-leitura — [Frontend](06-frontend.md) | ETAPA-7-COMPARTILHAMENTO |
| Contador de views do share | `pdfding/pdf/models/shared_models.py:SharedPdf.views` | coluna `shares.views`, incrementada em `GET /api/shared/{share_id}` — [Modelo de Dados](02-modelo-de-dados.md), [API](05-api.md) | ETAPA-7-COMPARTILHAMENTO |
| Revogação de share pelo admin | `pdfding/admin/views.py:RevokeShare` | `DELETE /api/pdfs/{id}/share` e listagem `GET /api/shares` — [API](05-api.md); distinção de admin desaparece (usuário único) | ETAPA-7-COMPARTILHAMENTO |
| Informações da instância | `pdfding/admin/views.py:Information` | `GET /api/admin/info` (versão, contagens de PDFs/tags/coleções, espaço sob `FILES`, contagem por `embedding_status`) — [API](05-api.md); tela `/admin` — [Frontend](06-frontend.md) | ETAPA-10-UI-COMPLETA |

### Tema e paginação

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Tema claro/escuro/sistema | `pdfding/users/models.py:Profile.dark_mode` | `settings['ui.theme']` (`system\|light\|dark`); toggle na barra superior, espelhado em `localStorage` — [Modelo de Dados](02-modelo-de-dados.md), [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |
| Itens por página | `pdfding/users/models.py:Profile.pdfs_per_page` | substituído por rolagem infinita: `settings['ui.per_page']` passa a ser o tamanho do lote buscado por `IntersectionObserver` via `GET /api/pdfs?limit=&cursor=` — [Modelo de Dados](02-modelo-de-dados.md), [API](05-api.md), [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |

### Automação e manutenção em segundo plano

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| Watch-dir de consumo | `pdfding/pdf/tasks.py:consume_task` | goroutine com `time.Ticker` a cada `CONSUME_INTERVAL_MINUTES`, lendo `CONSUME_DIR` — [Arquitetura](01-arquitetura.md) (`internal/server/consumer.go`); variáveis `CONSUME_*` — [Docker/CI/Deploy](07-docker-ci-deploy.md) | ETAPA-8-BACKGROUND |
| Tags automáticas na consumo | `pdfding/pdf/tasks.py:consume_task` (usa `settings.CONSUME_TAG_STRING`) | variável `CONSUME_TAGS` aplicada às tags do PDF importado pelo `consumer.go` — [Docker/CI/Deploy](07-docker-ci-deploy.md), [Arquitetura](01-arquitetura.md) | ETAPA-8-BACKGROUND |
| Regeneração de thumbnails | `pdfding/pdf/management/commands/regenerate_thumbnails.py` | substituído por `POST /api/admin/reindex` (reconstrói somente o índice FTS5) — [API](05-api.md), [Busca Híbrida](04-busca-hibrida.md) — combinado com geração de thumbnail sob demanda pelo pdf.js do navegador na primeira abertura do viewer — [Frontend](06-frontend.md) | ETAPA-8-BACKGROUND |

### Infraestrutura

| Funcionalidade | Onde está hoje (arquivo:símbolo) | Destino | Etapa |
|---|---|---|---|
| `/healthz` | `pdfding/core/views.py:HealthView` | `GET /healthz` público → `200 "ok"` — [API](05-api.md); servido pelo chi já no boot — [Arquitetura](01-arquitetura.md) | ETAPA-1-FUNDACAO |
| Service worker/PWA | `pdfding/core/views.py:ServiceWorkerView` | `static/manifest.webmanifest` + `static/sw.js`, dois caches versionados (network-first/cache-first conforme a rota) — [Frontend](06-frontend.md) | ETAPA-9-UI-BASE |

## Funcionalidades intencionalmente removidas

Removidas por decisão explícita do briefing ou do plano de refatoração — não têm linha na tabela de paridade acima porque não têm destino algum na nova arquitetura.

| Funcionalidade removida | Motivo |
|---|---|
| Cor de tema configurável | briefing — paleta única, sem `theme_color`/cores customizadas por usuário |
| Multi-usuário e papéis de workspace | briefing — produto de usuário único; `Workspace`/`WorkspaceUser` eliminados (decisão 9) |
| Verificação de e-mail | briefing — não há mais conta de usuário nem fluxo de e-mail, só `ADMIN_PASSWORD` |
| PostgreSQL | simplicidade — SQLite puro Go é o único banco suportado |
| Paginação numerada | substituída por rolagem infinita em todas as listagens |
| `ALLOW_PDF_SUB_DIRECTORIES` como interruptor | subdiretórios passam a ser sempre permitidos, sem variável de ambiente para desligar |
| Backup S3/MinIO agendado (`pdfding/backup/tasks.py`) | decisão do usuário de eliminar toda armazenagem/integração com Amazon S3 desta refatoração; o produto não envia mais nada para nuvem. Backup externo, se necessário, é responsabilidade do operador do host — ver [Storage](03-storage.md) |
| Criptografia de backup (`pdfding/backup/service.py`) | dependia do job de backup em nuvem, removido junto |
| Restauração manual de backup (`pdfding/backup/management/commands/recover_data.py`) | dependia do backup em S3/MinIO, removido; sem backup automático não há do que restaurar via este comando |
