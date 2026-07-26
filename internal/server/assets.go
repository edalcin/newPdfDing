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
//
// index.html itself is ALWAYS served with Cache-Control: no-cache — every
// hashed asset it references (/_app/immutable/...) changes name on every
// `npm run build`, so a browser-cached copy of index.html from a previous
// deploy points at chunk files that no longer exist in the current build,
// breaking client-side navigation with a 404 the moment SvelteKit's router
// tries to lazy-load a page component. no-cache forces revalidation on
// every load instead of trusting a stale cached copy.
//
// /sw.js gets the same no-cache treatment for a sharper reason: browsers
// (and any CDN/proxy in front, e.g. Cloudflare) MUST see a fresh copy of the
// service worker script on every request for the update mechanism to work
// at all — an intermediary caching it for hours means every fix shipped to
// sw.js silently never reaches real clients until that cache entry expires,
// no matter how many times the origin is redeployed (ver
// refatoracao/06-frontend.md, "PWA e service worker").
func spaHandler(root fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(root))
	return func(w http.ResponseWriter, r *http.Request) {
		isShell := r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/sw.js"
		if _, err := fs.Stat(root, stripLeadingSlash(r.URL.Path)); err != nil {
			r = cloneRequestWithPath(r, "/")
			isShell = true
		}
		if isShell {
			w.Header().Set("Cache-Control", "no-cache")
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
