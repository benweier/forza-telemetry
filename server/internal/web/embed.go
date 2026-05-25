// Package web embeds the built TanStack Start client assets (SPA mode).
// In dev, run the client dev server separately on :3000; in prod,
// `just copy-client-dist` copies client/dist/client/ into ./dist so
// `go:embed all:dist` ships them inside the Go binary.
//
// TanStack Start in SPA mode emits a shell HTML at `_shell.html` (instead of
// the traditional index.html). All unknown client-router paths must fall
// through to that shell — the client router hydrates and renders the route.
package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

const shellPath = "_shell.html"

// Handler serves the embedded SPA build with a `_shell.html` fallback for
// unknown client-side routes (TanStack Start SPA convention). If the build
// is empty, serves a placeholder so the binary still runs.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return placeholder()
	}
	if _, err := fs.Stat(sub, shellPath); err != nil {
		return placeholder()
	}
	fileSrv := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			serveShell(w, sub)
			return
		}
		if _, err := fs.Stat(sub, path[1:]); err != nil {
			serveShell(w, sub)
			return
		}
		fileSrv.ServeHTTP(w, r)
	})
}

func serveShell(w http.ResponseWriter, sub fs.FS) {
	f, err := sub.Open(shellPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, f)
}

func placeholder() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body><pre>forza-telemetry: client not built. Run 'just build' to populate server/internal/web/dist.</pre></body></html>`))
	})
}
