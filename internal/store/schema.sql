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
  file_directory   TEXT NOT NULL DEFAULT '',   -- subdiretório lógico opcional
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
