# Modelo de Dados

Este documento define o `schema.sql` completo e final do banco SQLite da aplicação — o implementador copia o bloco abaixo sem decidir nada — além do mapeamento de cada modelo Django do produto atual para o schema novo, das chaves de configuração que substituem o `Profile` de usuário único, da regra de normalização de tags e da semente de coleção padrão aplicada no boot.

## Schema SQL

Conteúdo integral de `internal/store/schema.sql`, embutido no binário via `//go:embed` (ver [Arquitetura](01-arquitetura.md)). Todo DDL usa `IF NOT EXISTS`; não há migrações de rollback.

```sql
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS collections (
  id          TEXT PRIMARY KEY,           -- UUIDv7
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  is_default  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_name ON collections(name COLLATE NOCASE);

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
  collection_id    TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  file_directory   TEXT NOT NULL DEFAULT '',   -- subdiretório lógico opcional
  storage_key      TEXT NOT NULL,              -- chave relativa sob FILES
  thumbnail_key    TEXT NOT NULL DEFAULT '',
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
CREATE INDEX IF NOT EXISTS idx_pdfs_collection ON pdfs(collection_id);
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
  text       TEXT NOT NULL,
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

CREATE TABLE IF NOT EXISTS signatures (
  id         TEXT PRIMARY KEY,            -- UUIDv7
  name       TEXT NOT NULL,
  data       TEXT NOT NULL,               -- data URL PNG
  created_at TEXT NOT NULL
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

## Mapeamento de modelos Django → tabelas novas

Uma linha por modelo Django do produto atual. `Workspace`, `WorkspaceUser` e `User` são eliminados (usuário único); `Profile` deixa de ser um modelo e vira chaves na tabela `settings` (mais a tabela `signatures`).

| Modelo Django (atual) | Destino | Observação |
|---|---|---|
| `Pdf` | `pdfs` (+ `pdf_tags`, `pdf_text`) | Campos centrais preservados 1:1; os campos novos `storage_key`, `thumbnail_key`, `preview_key`, `sha256`, `revision` suportam o esquema de armazenamento e a detecção de duplicidade — ver [Storage](03-storage.md); `notes` continua Markdown bruto, sanitizado na leitura — ver [Segurança](08-seguranca.md). |
| `PdfComment` | `pdf_annotations` (`kind = 'comment'`) | Fundida com `PdfHighlight` numa única tabela com coluna `kind` — decisão 10: os dois modelos Django eram subclasses da mesma abstrata `PdfAnnotation`, com campos idênticos. |
| `PdfHighlight` | `pdf_annotations` (`kind = 'highlight'`) | Mesma fusão descrita na linha de `PdfComment`. |
| `Tag` | `tags` (+ `pdf_tags` para a relação N:N com `pdfs`) | Nome único case-insensitive (`idx_tags_name COLLATE NOCASE`); regra de normalização preservada — ver "Regra de normalização de tags" abaixo. |
| `Collection` | `collections` | Com `Workspace` eliminado, `Collection` sobe à entidade de topo do produto — decisão 9; a coleção padrão é semeada no boot — ver "Seeding da coleção padrão no boot" abaixo. |
| `Workspace` | *Eliminado* | Usuário único remove a necessidade de isolamento multi-tenant — decisão 9; `Collection` herda o papel de entidade de topo. |
| `WorkspaceUser` | *Eliminado* | Associação usuário↔workspace deixa de existir junto com o multiusuário — decisão 9. |
| `Profile` | `settings` (maior parte dos campos) + tabela `signatures` (campo `signatures`) | Mapeamento campo a campo — ver "Campos do Profile mapeados para settings" abaixo. |
| `SharedPdf` | `shares` | `id` é o próprio segredo da URL pública (`/s/{share_id}`); `pdf_id` é `UNIQUE` — no máximo um share ativo por PDF. |
| `User` | *Eliminado* (`django.contrib.auth`) | Usuário único autenticado por `ADMIN_PASSWORD` — ver [Segurança](08-seguranca.md); `sessions` não referencia nenhum usuário. |

### Campos do Profile mapeados para settings

O `Profile` do Django atual (`pdfding/users/models.py`) tem os campos `annotation_sorting`, `current_collection_id`, `current_workspace_id`, `custom_theme_color`, `custom_theme_color_secondary`, `dark_mode`, `layout`, `language`, `pdf_inverted_mode`, `pdf_keep_screen_awake`, `pdf_sorting`, `show_progress_bars`, `signatures` (JSON), `tags_open`, `tag_tree_mode`, `theme_color`, `user`. Cada um se resolve assim:

| Campo `Profile` (Django) | Destino | Observação |
|---|---|---|
| `annotation_sorting` | `settings['annotation.sorting']` | — |
| `current_collection_id` | `settings['current.collection_id']` | — |
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
| `signatures` (JSON) | Tabela `signatures` | Vira tabela própria, não uma chave de `settings`. |
| `tags_open` | `settings['ui.tags_open']` | — |
| `tag_tree_mode` | `settings['ui.tag_tree_mode']` | — |
| `theme_color` | *Eliminado* | Paleta única de tema (briefing). |
| `user` | *Eliminado* | FK para `User`, também eliminado — usuário único. |

## Chaves de configuração (`settings`)

Lista fechada — nenhuma outra chave é válida; `PATCH /api/settings` (ver [API](05-api.md)) valida contra esta lista. Todas as chaves são armazenadas como texto (`settings.value TEXT NOT NULL`); os campos booleanos usam os literais `"0"`/`"1"`.

| Chave | Default | Valores válidos | Observação |
|---|---|---|---|
| `ui.theme` | `system` | `system\|light\|dark` | Cor de tema não existe mais — paleta única, ver [Frontend](06-frontend.md). |
| `ui.layout` | `grid` | `compact\|list\|grid\|minimal` | — |
| `ui.per_page` | `25` | número inteiro positivo | — |
| `ui.tags_open` | `1` | `0\|1` (booleano) | — |
| `ui.tag_tree_mode` | `0` | `0\|1` (booleano) | Hierarquia de tags por `/`; ver "Regra de normalização de tags" abaixo. |
| `ui.show_progress_bars` | `1` | `0\|1` (booleano) | — |
| `pdf.sorting` | `newest` | `newest\|oldest\|name_asc\|name_desc\|most_viewed\|least_viewed\|recently_viewed` | — |
| `annotation.sorting` | `newest` | — | — |
| `viewer.inverted` | `0` | `0\|1` (booleano) | — |
| `viewer.keep_awake` | `0` | `0\|1` (booleano) | — |
| `current.collection_id` | `""` (vazio) | UUIDv7 de um `collections.id` existente, ou vazio | Vazio = nenhuma coleção selecionada. |

## Regra de normalização de tags

A normalização de tags mantém o comportamento atual, implementado hoje em `Tag.parse_tag_string` (`pdfding/pdf/models/tag_models.py`). A string de entrada (uma ou mais tags) é normalizada nesta ordem:

1. Separar por espaço.
2. Remover os caracteres `#`, `&`, `+`.
3. Deduplicar.
4. Converter para minúsculas.
5. Ordenar.

A hierarquia por `/` (ex.: `trabalho/projetos/2024`) é preservada para o modo árvore, controlado pela chave `ui.tag_tree_mode`.

## Seeding da coleção padrão no boot

No boot da aplicação, depois de aplicar o `schema.sql`, o servidor verifica se existe alguma linha em `collections` com `is_default = 1`. Se não existir, cria uma:

| Coluna | Valor |
|---|---|
| `id` | novo UUIDv7 |
| `name` | `Default` |
| `description` | `''` |
| `is_default` | `1` |
| `created_at` | timestamp atual |

A verificação é idempotente: em boots subsequentes a condição `is_default = 1` já está satisfeita e nenhuma segunda coleção padrão é criada.
