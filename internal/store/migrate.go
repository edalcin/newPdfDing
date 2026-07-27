// Package store owns the SQLite connection and the embedded schema.
package store

import (
	"database/sql"
	"embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// Open opens the SQLite database at dbPath, applies the required PRAGMAs
// (ver refatoracao/01-arquitetura.md, "Configuração SQLite obrigatória"), and
// runs the embedded schema.sql. All DDL uses IF NOT EXISTS, so the migration
// is idempotent and safe to run on every boot.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store.Open: %w", err)
	}

	// Single writer connection: WAL allows concurrent readers, but SQLite
	// itself serialises writers — a pool of 1 avoids "database is locked"
	// errors instead of relying solely on busy_timeout.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("store.Open pragma %q: %w", p, err)
		}
	}

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open read schema: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open begin: %w", err)
	}
	if _, err := tx.Exec(string(schema)); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("store.Open exec schema: %w", err)
	}
	if err := addColumnIfMissing(tx, "pdf_annotations", "data", "TEXT NOT NULL DEFAULT ''"); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("store.Open add column: %w", err)
	}
	if err := dropColumnIfExists(tx, "pdfs", "file_directory"); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("store.Open drop column: %w", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open commit: %w", err)
	}

	// Rebuild the FTS5 index from current table state on every boot — cheap
	// at personal-library scale, and it corrects any accumulated divergence
	// without incremental tracking (ver 04-busca-hibrida.md, "Rebuild total
	// no boot"; 01-arquitetura.md, "Mecanismo de migração").
	if err := RebuildFTS(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open rebuild fts: %w", err)
	}

	return db, nil
}

// addColumnIfMissing acrescenta uma coluna a uma tabela já existente. É todo
// o mecanismo de migração do projeto: schema.sql é declarativo e só sabe
// criar tabela nova, nunca evoluir uma antiga.
func addColumnIfMissing(tx *sql.Tx, table, column, decl string) error {
	rows, err := tx.Query("SELECT 1 FROM pragma_table_info(?) WHERE name = ?", table, column)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return nil
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// dropColumnIfExists removes a column from a table that already has it,
// no-op otherwise. Safe to run on every boot for the same reason
// addColumnIfMissing is: this is the only place schema.sql's "declarative,
// create-only" limitation is worked around. ALTER TABLE ... DROP COLUMN
// requires SQLite >= 3.35 (bundled by modernc.org/sqlite well past that).
func dropColumnIfExists(tx *sql.Tx, table, column string) error {
	rows, err := tx.Query("SELECT 1 FROM pragma_table_info(?) WHERE name = ?", table, column)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return rows.Err()
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column))
	return err
}
