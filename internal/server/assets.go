package server

import (
	"embed"
	"io/fs"
	"net/http"
)

// webFS embeds the built SvelteKit SPA (ver refatoracao/06-frontend.md,
// "Saída de build e integração com go:embed"). web/dist is populated by
// `npm run build` (frontend) + copy, or by the Dockerfile build stage
// (ver refatoracao/07-docker-ci-deploy.md) — never committed, always
// generated. A placeholder index.html ships so a fresh checkout still
// builds before the frontend has been built once.
//
//go:embed all:web/dist
var webFS embed.FS

func webRoot() fs.FS {
	sub, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic("server: could not sub webFS: " + err.Error())
	}
	return sub
}

// spaHandler serves the embedded SPA build, falling back to index.html for
// any path that isn't a real static asset — the SvelteKit router owns
// client-side navigation (adapter-static's fallback: 'index.html', ver
// 06-frontend.md). API routes are mounted separately in buildRouter and
// never reach this handler.
func spaHandler(root fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(root))
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(root, stripLeadingSlash(r.URL.Path)); err != nil {
			r = cloneRequestWithPath(r, "/")
		}
		fileServer.ServeHTTP(w, r)
	}
}

func stripLeadingSlash(p string) string {
	if p == "" || p == "/" {
		return "."
	}
	if p[0] == '/' {
		return p[1:]
	}
	return p
}

func cloneRequestWithPath(r *http.Request, path string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	return r2
}
