package api

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type staticHandler struct {
	root   string
	logger *slog.Logger
}

func newStaticHandler(root string, logger *slog.Logger) http.Handler {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &staticHandler{root: abs, logger: logger}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается")
		return
	}

	urlPath := path.Clean("/" + r.URL.Path)
	if urlPath == "/" {
		h.serveFile(w, r, "index.html")
		return
	}

	rel := strings.TrimPrefix(urlPath, "/")
	file := filepath.Join(h.root, filepath.FromSlash(rel))

	// Защита от выхода за пределы каталога web.
	if !strings.HasPrefix(file, h.root+string(os.PathSeparator)) && file != h.root {
		http.NotFound(w, r)
		return
	}

	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		h.serveFile(w, r, rel)
		return
	}

	if path.Ext(rel) == "" {
		candidate := rel + ".html"
		full := filepath.Join(h.root, filepath.FromSlash(candidate))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			h.serveFile(w, r, candidate)
			return
		}
	}

	h.serveFileStatus(w, r, "404.html", http.StatusNotFound)
}

func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	h.serveFileStatus(w, r, name, http.StatusOK)
}

func (h *staticHandler) serveFileStatus(w http.ResponseWriter, r *http.Request, name string, status int) {
	full := filepath.Join(h.root, filepath.FromSlash(name))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if status != http.StatusOK {
		data, err := os.ReadFile(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
		return
	}

	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, info.ModTime(), f)
}
