# Modelo de Dados

Este documento define o `schema.sql` completo e final do banco SQLite da aplicação — o implementador copia o bloco abaixo sem decidir nada — além do mapeamento de cada modelo Django do produto atual para o schema novo, das chaves de configuração que substituem o `Profile` de usuário único e da regra de normalização de tags. Três itens do plano original — coleções, assinaturas salvas e o modo árvore de tags — nunca chegaram a ser implementados; ficam documentados como histórico nas seções correspondentes, marcados explicitamente.

## Schema SQL

Conteúdo integral de `internal/store/schema.sql`, embutido no binário via `//go:embed` (ver [Arquitetura](01-arquitetura.md)). Todo DDL usa `IF NOT EXISTS`; não há migrações de rollback.

```sql
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);


CREATE TABLE IF NOT EXISTS tags (
  id   TEXT PRIMARY KEY,                  -- UUIDv7
  name TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_name ON tags(name COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS pdfs (
  id               TEXT PRIMARY KEY,      -- UUIDv7
  name             TEXT NOT NULL,
  description      TEXT NOT NULL DEFAULT '',
  notes            TEXT NOT NULL DEFAULT '',   -- Markdown bruto
  storage_key      TEXT NOT NULL,              -- chave relativa sob FILES
  preview_key      TEXT NOT NULL DEFAULT '',
  sha256           TEXT NOT NULL,
  size_bytes       INTEGER NOT NULL DEFAULT 0,
  num_pages        INTEGER NOT NULL DEFAULT 0,
  current_page     INTEGER NOT NULL DEFAULT 1,
  views            INTEGER NOT NULL DEFAULT 0,
  revision         INTEGER NOT NULL DEFAULT 1,
  starred          INTEGER NOT NULL DEFAULT 0,
  archived         INTEGER NOT NULL DEFAULT 0,
  created_at       TEXT NOT NULL,
  last_viewed_at   TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_pdfs_sha256 ON pdfs(sha256);
CREATE INDEX IF NOT EXISTS idx_pdfs_archived ON pdfs(archived);

CREATE TABLE IF NOT EXISTS pdf_tags (
  pdf_id TEXT NOT NULL REFERENCES pdfs(id) ON DELETE CASCADE,
  tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (pdf_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_pdf_tags_tag ON pdf_tags(tag_id);

CREATE TABLE IF NOT EXISTS pdf_annotations (
  id         TEXT PRIMARY KEY,            -- UUIDv7
  pdf_id     TEXT NOT NULL REFERENCES pdfs(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('comment','highlight')),
  page       INTEGER NOT NULL,
  text       TEXT NOT NULL,               -- trecho selecionado (highlight) ou corpo (comment)
  note       TEXT NOT NULL DEFAULT '',    -- anotação do usuário sobre o trecho
  color      TEXT NOT NULL DEFAULT 'yellow',
  rects      TEXT NOT NULL DEFAULT '',    -- JSON [[x,y,w,h],…] normalizado 0..1 na página; '' = não ancorado
  data       TEXT NOT NULL DEFAULT '',    -- AnnotationTransferItem do EmbedPDF, JSON; '' = linha legada sem geometria de SDK
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_annotations_pdf ON pdf_annotations(pdf_id, kind);

CREATE TABLE IF NOT EXISTS pdf_text (
  pdf_id TEXT PRIMARY KEY REFERENCES pdfs(id) ON DELETE CASCADE,
  body   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS shares (
  id         TEXT PRIMARY KEY,            -- UUIDv7, é o segredo da URL pública
  pdf_id     TEXT NOT NULL UNIQUE REFERENCES pdfs(id) ON DELETE CASCADE,
  views      INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id           TEXT PRIMARY KEY,          -- token base64url de 32 bytes
  created_at   TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS pdfs_fts USING fts5(
  name, description, notes, body, tags,
  content='',
  tokenize='unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS pdf_embeddings (
  pdf_id       TEXT PRIMARY KEY REFERENCES pdfs(id) ON DELETE CASCADE,
  content_hash TEXT NOT NULL,
  embedding    BLOB NOT NULL,
  created_at   TEXT NOT NULL
);
```

Conteúdo idêntico a `internal/store/schema.sql`. Não existe tabela `collections` nem `signatures` — ambas foram planejadas (ver mapeamento abaixo) e nunca implementadas. A coluna `pdfs.thumbnail_key` também nunca existiu; só `preview_key`. As colunas `note`, `color`, `rects` e `data` de `pdf_annotations` suportam a geometria de destaque e a anotação de texto do visualizador EmbedPDF (`data` chega vazia em linhas legadas sem essa geometria).

## Mapeamento de modelos Django → tabelas novas

Uma linha por modelo Django do produto atual. `Workspace`, `WorkspaceUser` e `User` são eliminados (usuário único); `Profile` deixa de ser um modelo e vira chaves na tabela `settings`.

| Modelo Django (atual) | Destino | Observação |
|---|---|---|
| `Pdf` | `pdfs` (+ `pdf_tags`, `pdf_text`) | Campos centrais preservados 1:1; os campos novos `storage_key`, `preview_key`, `sha256`, `revision` suportam o esquema de armazenamento e a detecção de duplicidade — ver [Storage](03-storage.md); `notes` continua Markdown bruto, sanitizado na leitura — ver [Segurança](08-seguranca.md). Não existe `thumbnail_key`: thumbnail foi planejado e nunca implementado. |
| `PdfComment` | `pdf_annotations` (`kind = 'comment'`) | Fundida com `PdfHighlight` numa única tabela com coluna `kind` — decisão 10: os dois modelos Django eram subclasses da mesma abstrata `PdfAnnotation`, com campos idênticos. |
| `PdfHighlight` | `pdf_annotations` (`kind = 'highlight'`) | Mesma fusão descrita na linha de `PdfComment`. |
| `Tag` | `tags` (+ `pdf_tags` para a relação N:N com `pdfs`) | Nome único case-insensitive (`idx_tags_name COLLATE NOCASE`); regra de normalização preservada — ver "Regra de normalização de tags" abaixo. |
| `Collection` | *Nunca implementado* | Planejado como entidade de topo do produto depois da eliminação do `Workspace` — decisão 9 — incluindo uma "coleção padrão" semeada no boot. A tabela `collections`, a coluna `pdfs.collection_id`, as rotas `/api/collections` e a tela `/collections` nunca foram construídas. Ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md). |
| `Workspace` | *Eliminado* | Usuário único remove a necessidade de isolamento multi-tenant — decisão 9. |
| `WorkspaceUser` | *Eliminado* | Associação usuário↔workspace deixa de existir junto com o multiusuário — decisão 9. |
| `Profile` | `settings` (maior parte dos campos) | Mapeamento campo a campo — ver "Campos do Profile mapeados para settings" abaixo. O campo `signatures` (JSON) tinha uma tabela própria (`signatures`) planejada — **nunca implementada**: não existe no schema atual nem rota `/api/signatures`. |
| `SharedPdf` | `shares` | `id` é o próprio segredo da URL pública (`/s/{share_id}`); `pdf_id` é `UNIQUE` — no máximo um share ativo por PDF. |
| `User` | *Eliminado* (`django.contrib.auth`) | Usuário único autenticado por `ADMIN_PASSWORD` — ver [Segurança](08-seguranca.md); `sessions` não referencia nenhum usuário. |

### Campos do Profile mapeados para settings

O `Profile` do Django atual (`pdfding/users/models.py`) tem os campos `annotation_sorting`, `current_collection_id`, `current_workspace_id`, `custom_theme_color`, `custom_theme_color_secondary`, `dark_mode`, `layout`, `language`, `pdf_inverted_mode`, `pdf_keep_screen_awake`, `pdf_sorting`, `show_progress_bars`, `signatures` (JSON), `tags_open`, `tag_tree_mode`, `theme_color`, `user`. Cada um se resolve assim:

| Campo `Profile` (Django) | Destino | Observação |
|---|---|---|
| `annotation_sorting` | `settings['annotation.sorting']` | — |
| `current_collection_id` | *Nunca implementado* | Dependia de Coleções, que nunca foram implementadas — decisão 9. |
| `current_workspace_id` | *Eliminado* | Sem `Workspace` — decisão 9. |
| `custom_theme_color` | *Eliminado* | Paleta única de tema (briefing). |
| `custom_theme_color_secondary` | *Eliminado* | Paleta única de tema (briefing). |
| `dark_mode` | `settings['ui.theme']` | Campo booleano do Django vira enum de três estados (`system\|light\|dark`). |
| `layout` | `settings['ui.layout']` | — |
| `language` | *Eliminado* | Produto não tem i18n no alvo; não consta no briefing nem no inventário fechado de chaves — registrar, se questionado, como funcionalidade não migrada. |
| `pdf_inverted_mode` | `settings['viewer.inverted']` | — |
| `pdf_keep_screen_awake` | `settings['viewer.keep_awake']` | — |
| `pdf_sorting` | `settings['pdf.sorting']` | — |
| `show_progress_bars` | `settings['ui.show_progress_bars']` | — |
| `signatures` (JSON) | *Nunca implementado* | Tabela `signatures` planejada, nunca implementada — não existe no schema atual nem rota `/api/signatures`. |
| `tags_open` | `settings['ui.tags_open']` | — |
| `tag_tree_mode` | *Nunca implementado* | Toggle de modo árvore planejado, nunca implementado — não existe chave `ui.tag_tree_mode` nem toggle na UI (ver "Regra de normalização de tags" abaixo). |
| `theme_color` | *Eliminado* | Paleta única de tema (briefing). |
| `user` | *Eliminado* | FK para `User`, também eliminado — usuário único. |

## Chaves de configuração (`settings`)

Lista fechada — nenhuma outra chave é válida; `PATCH /api/settings` (ver [API](05-api.md)) valida contra esta lista (`internal/store/settings.go:settingDefs`). Todas as chaves são armazenadas como texto (`settings.value TEXT NOT NULL`); os campos booleanos usam os literais `"0"`/`"1"`.

| Chave | Default | Valores válidos | Observação |
|---|---|---|---|
| `ui.theme` | `system` | `system\|light\|dark` | Cor de tema não existe mais — paleta única, ver [Frontend](06-frontend.md). |
| `ui.layout` | `grid` | `compact\|list\|grid\|minimal` | — |
| `ui.per_page` | `25` | número inteiro positivo | — |
| `ui.tags_open` | `1` | `0\|1` (booleano) | — |
| `ui.show_progress_bars` | `1` | `0\|1` (booleano) | — |
| `pdf.sorting` | `newest` | `newest\|oldest\|name_asc\|name_desc\|most_viewed\|least_viewed\|recently_viewed` | — |
| `annotation.sorting` | `newest` | — | — |
| `viewer.inverted` | `0` | `0\|1` (booleano) | — |
| `viewer.keep_awake` | `0` | `0\|1` (booleano) | — |
| `ai.text_model` | `""` (vazio) | livre — nome de modelo Gemini, ou vazio | Modelo usado por "Descrever com IA" e "Sugerir tags". Vazio = os dois recursos ficam desabilitados (`412`) até o usuário escolher um em Configurações → IA — sem default embutido, porque o catálogo de modelos depende da chave do usuário. |

Duas chaves do plano original nunca chegaram a ser implementadas e **não fazem parte** desta lista fechada: `current.collection_id` (dependia de Coleções, nunca implementadas) e `ui.tag_tree_mode` (modo árvore de tags, nunca implementado). `PATCH /api/settings` rejeita as duas com `400`, como rejeitaria qualquer chave desconhecida.

## Regra de normalização de tags

A normalização de tags mantém o comportamento do produto Django atual, implementado hoje em `ParseTagString` (`internal/store/tags.go`, equivalente a `Tag.parse_tag_string` em `pdfding/pdf/models/tag_models.py`). A string de entrada (uma ou mais tags) é normalizada nesta ordem:

1. Separar por espaço.
2. Remover os caracteres `#`, `&`, `+`.
3. Deduplicar.
4. Converter para minúsculas.
5. Ordenar.

O separador `/` (ex.: `trabalho/projetos/2024`) é preservado pela normalização, pensado para uma futura hierarquia de tags. O **modo árvore** que exploraria essa hierarquia (chave `ui.tag_tree_mode`, toggle planejado na tela `/tags`) nunca foi implementado — hoje a lista de tags na SPA é sempre plana, ver [Frontend](06-frontend.md).

## Coleção padrão — nunca implementada

O plano original previa uma "coleção padrão" semeada no boot, verificando `is_default = 1` numa tabela `collections` e criando uma linha `Default` na ausência de qualquer coleção. Essa etapa nunca chegou a ser codificada: não existe tabela `collections`, não existe seeding no boot, e o servidor não faz nenhuma verificação equivalente. Ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md).
