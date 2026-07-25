package server

import (
	"io/fs"
	"regexp"
)

// SvelteKit's own bootstrap script (the inline <script> every adapter-static
// page needs to kick off module loading) requires either 'unsafe-inline' or
// a hash in script-src. Nonces are not an option: adapter-static emits
// static HTML, never per-request, so there is no safe way to mint a fresh
// nonce per response (ver refatoracao/06-frontend.md, "Divergência de pkd";
// SvelteKit's own docs on CSP + prerendered/static output). SvelteKit is
// configured (frontend/vite.config.ts, csp.mode: 'hash') to compute that
// hash at build time and embed it in a <meta http-equiv="Content-Security-
// Policy"> tag in the built index.html. extractScriptHashes reads that tag
// back out so the same hash can be folded into the real HTTP header — the
// header is the one refatoracao/08-seguranca.md actually mandates; the meta
// tag is just where SvelteKit's build happens to publish the value.
var cspMetaPattern = regexp.MustCompile(`(?i)<meta\s+http-equiv="content-security-policy"\s+content="([^"]*)"`)
var cspHashPattern = regexp.MustCompile(`'sha256-[A-Za-z0-9+/=]+'`)

// extractScriptHashes returns the space-joined sha256 hash tokens SvelteKit
// embedded in the built index.html's CSP meta tag, or "" if none is found
// (e.g. web/dist still holds the placeholder shipped before the frontend
// has ever been built).
func extractScriptHashes(root fs.FS) string {
	data, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return ""
	}
	meta := cspMetaPattern.FindSubmatch(data)
	if meta == nil {
		return ""
	}
	hashes := cspHashPattern.FindAll(meta[1], -1)
	if len(hashes) == 0 {
		return ""
	}
	out := make([]byte, 0, 64)
	for i, h := range hashes {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, h...)
	}
	return string(out)
}
