package api

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func init() {
	// Not in Go's built-in table; browsers want the right type for the PWA manifest.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// dist holds the Vite build, copied in by `make web` or the Dockerfile.
// Only .gitkeep is committed, so a plain `go build` still compiles.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the embedded web build rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// SPA serves static files from fsys and falls back to index.html for client-side
// routes. Hashed assets are cached forever; index.html is always revalidated.
func SPA(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if st, err := fs.Stat(fsys, p); err == nil && !st.IsDir() {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			if p == "index.html" {
				serveIndex(w, r, fsys)
				return
			}
			files.ServeHTTP(w, r)
			return
		}
		// Missing files with an extension are real 404s; anything else is an app route.
		if path.Ext(p) != "" && p != "index.html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		serveIndex(w, r, fsys)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "web UI not built: run `make web`", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}
