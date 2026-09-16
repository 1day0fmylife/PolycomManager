package webui

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// files contains the production Vite bundle copied by `make web` / `make build`.
//
//go:embed all:dist
var files embed.FS

type spaHandler struct {
	root  fs.FS
	index []byte
}

func Handler() http.Handler {
	root, err := fs.Sub(files, "dist")
	if err != nil {
		panic(fmt.Errorf("initialize embedded web UI: %w", err))
	}

	index, err := fs.ReadFile(root, "index.html")
	if err != nil || len(index) == 0 {
		panic(fmt.Errorf("embedded web UI is incomplete: dist/index.html is missing; rebuild with `make build`: %w", err))
	}

	return spaHandler{root: root, index: index}
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// API routes are registered before this catch-all browser handler.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requested := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requested == "." || requested == "" || requested == "index.html" {
		h.serveIndex(w, r)
		return
	}

	info, err := fs.Stat(h.root, requested)
	if err == nil && !info.IsDir() {
		h.serveAsset(w, r, requested)
		return
	}

	// A request with a filename extension is a real static-asset request, not a
	// client-side React route. Returning index.html here can hide stale/missing
	// Vite chunks behind misleading MIME/type errors.
	if path.Ext(requested) != "" {
		http.Error(w, "web UI asset not found", http.StatusNotFound)
		return
	}

	// Extension-less paths are handled by the React router.
	h.serveIndex(w, r)
}

func (h spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(h.index))
}

func (h spaHandler) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(h.root, name)
	if err != nil {
		http.Error(w, "web UI asset not found", http.StatusNotFound)
		return
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Vite emits content-hashed assets, so they can safely be cached. Files not
	// carrying a hash still work; the cache policy only affects performance.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(data))
}
