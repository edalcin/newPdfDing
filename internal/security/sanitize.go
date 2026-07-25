package security

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var ugcPolicy = bluemonday.UGCPolicy()

// RenderNotes renders raw Markdown (pdfs.notes) to sanitized HTML. The
// client never receives unsanitized HTML, regardless of what the browser
// trusts via CSP (ver refatoracao/08-seguranca.md, "Validação de entrada").
func RenderNotes(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return ugcPolicy.Sanitize(buf.String()), nil
}
