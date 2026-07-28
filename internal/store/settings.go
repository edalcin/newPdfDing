package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// settingDefs is the closed list of valid settings keys, their defaults and
// valid values (ver 02-modelo-de-dados.md, "Chaves de configuração
// (settings)"). PATCH /api/settings rejects any key or value outside this
// list.
type settingDef struct {
	def         string
	valid       map[string]bool // nil = free-form — validated by validator below
	positiveInt bool            // "número inteiro positivo" (ver 02-modelo-de-dados.md, ui.per_page)
}

var settingDefs = map[string]settingDef{
	"ui.theme":              {def: "system", valid: map[string]bool{"system": true, "light": true, "dark": true}},
	"ui.layout":             {def: "grid", valid: map[string]bool{"compact": true, "list": true, "grid": true, "minimal": true}},
	"ui.per_page":           {def: "25", positiveInt: true},
	"ui.tags_open":          {def: "1", valid: map[string]bool{"0": true, "1": true}},
	"ui.show_progress_bars": {def: "1", valid: map[string]bool{"0": true, "1": true}},
	"pdf.sorting":           {def: "newest", valid: map[string]bool{"newest": true, "oldest": true, "name_asc": true, "name_desc": true, "most_viewed": true, "least_viewed": true, "recently_viewed": true}},
	"annotation.sorting":    {def: "newest"},
	"viewer.inverted":       {def: "0", valid: map[string]bool{"0": true, "1": true}},
	"viewer.keep_awake":     {def: "0", valid: map[string]bool{"0": true, "1": true}},
	"ai.embed_model":        {def: ""},
	"ai.text_model":         {def: ""},
}

// SettingsStore provides validated key-value persistence over the settings
// table.
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore wraps db in a SettingsStore.
func NewSettingsStore(db *sql.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// All returns the full settings map — every closed key, filled with its
// stored value or, if unset, its default (ver "GET /api/settings").
func (s *SettingsStore) All() (map[string]string, error) {
	out := make(map[string]string, len(settingDefs))
	for key, def := range settingDefs {
		out[key] = def.def
	}
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if _, known := settingDefs[key]; known {
			out[key] = value
		}
	}
	return out, rows.Err()
}

// Get returns the stored value for key, or the key's default when unset,
// unknown, or unreadable — callers that only need one preference and have
// no error path to report (ver Server.embedModelName).
func (s *SettingsStore) Get(key string) string {
	def := settingDefs[key].def
	var value string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return def
	}
	return value
}

// ErrInvalidSetting is returned by Patch for an unknown key or an
// out-of-list value.
var ErrInvalidSetting = errors.New("invalid setting")

// Patch validates and upserts a partial settings map, returning the full
// updated map.
func (s *SettingsStore) Patch(updates map[string]string) (map[string]string, error) {
	for key, value := range updates {
		def, known := settingDefs[key]
		if !known {
			return nil, fmt.Errorf("%w: unknown key %q", ErrInvalidSetting, key)
		}
		if def.valid != nil && !def.valid[value] {
			return nil, fmt.Errorf("%w: invalid value %q for %q", ErrInvalidSetting, value, key)
		}
		if def.positiveInt {
			n, convErr := strconv.Atoi(value)
			if convErr != nil || n <= 0 {
				return nil, fmt.Errorf("%w: %q must be a positive integer, got %q", ErrInvalidSetting, key, value)
			}
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	for key, value := range updates {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key, value,
		); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.All()
}
