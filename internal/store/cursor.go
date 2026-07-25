package store

import (
	"encoding/base64"
	"strings"
)

// encodeCursor packs a sort value and an id into the opaque cursor format
// fixed in refatoracao/06-frontend.md, "Rolagem infinita":
// base64url("{sort_value}|{id}").
func encodeCursor(value, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value + "|" + id))
}

// decodeCursor reverses encodeCursor. It splits on the last '|' so a sort
// value containing '|' (e.g. a PDF name) never gets misparsed — ids are
// UUIDs and never contain '|'.
func decodeCursor(cursor string) (value, id string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", ErrInvalidCursor
	}
	idx := strings.LastIndex(string(raw), "|")
	if idx < 0 {
		return "", "", ErrInvalidCursor
	}
	return string(raw[:idx]), string(raw[idx+1:]), nil
}
