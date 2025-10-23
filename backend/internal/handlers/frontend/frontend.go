package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	fileServer http.Handler
	frontendFS embed.FS
}

// NewHandler creates a new frontend handler instance.
func NewHandler(frontendFS embed.FS, distPath string) *Handler {
	subFS, err := fs.Sub(frontendFS, distPath)
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	return &Handler{
		fileServer: cacheControlMiddleware(fileServer),
		frontendFS: frontendFS,
	}
}

// ServeFrontend handles serving static frontend assets.
func (h *Handler) ServeFrontend(w http.ResponseWriter, r *http.Request) {
	h.fileServer.ServeHTTP(w, r)
}

// ServeSPA serves index.html for all frontend routes.
func (h *Handler) ServeSPA(w http.ResponseWriter, r *http.Request, indexPath string) {
	indexHTML, err := h.frontendFS.ReadFile(indexPath)
	if err != nil {
		http.Error(w, "Frontend assets not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	w.Write(indexHTML)
}

// RegisterRoutes registers frontend serving routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Handle("/*", http.HandlerFunc(h.ServeFrontend))
}

func cacheControlMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else if strings.HasPrefix(path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		next.ServeHTTP(w, r)
	})
}
