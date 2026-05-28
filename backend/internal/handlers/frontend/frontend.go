package frontend

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// appConfigPlaceholder is replaced in index.html at serve time with an inline
// script carrying runtime config. A struct (not a bare string) so future
// runtime config has a home without re-plumbing the injection.
const appConfigPlaceholder = "<!--__APP_CONFIG__-->"

type appConfig struct {
	EnvironmentLabel string `json:"environmentLabel"`
}

type Handler struct {
	fileServer      http.Handler
	frontendFS      fs.FS
	appConfigScript string
}

// NewHandler creates a new frontend handler instance. environmentLabel is the
// runtime ENVIRONMENT_LABEL value (read at the composition root); it drives the
// banner and non-prod gates in the SPA. Empty means production (no banner).
func NewHandler(frontendFS fs.FS, distPath string, environmentLabel string) *Handler {
	subFS, err := fs.Sub(frontendFS, distPath)
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	return &Handler{
		fileServer:      cacheControlMiddleware(fileServer),
		frontendFS:      frontendFS,
		appConfigScript: buildAppConfigScript(environmentLabel),
	}
}

// buildAppConfigScript renders the inline script that publishes runtime config
// onto window.__APP_CONFIG__. json.Marshal HTML-escapes '<' '>' '&' by default,
// so a label containing "</script>" becomes "</script>" and cannot
// break out of the inline <script> tag.
func buildAppConfigScript(environmentLabel string) string {
	b, err := json.Marshal(appConfig{EnvironmentLabel: environmentLabel})
	if err != nil {
		b = []byte(`{"environmentLabel":""}`)
	}
	return "<script>window.__APP_CONFIG__ = " + string(b) + ";</script>"
}

// ServeFrontend handles serving static frontend assets.
func (h *Handler) ServeFrontend(w http.ResponseWriter, r *http.Request) {
	h.fileServer.ServeHTTP(w, r)
}

// ServeSPA serves index.html for all frontend routes, injecting runtime config
// in place of appConfigPlaceholder. index.html is read fresh per request and
// served no-cache, so the injected config reflects the pod's current env.
func (h *Handler) ServeSPA(w http.ResponseWriter, r *http.Request, indexPath string) {
	indexHTML, err := fs.ReadFile(h.frontendFS, indexPath)
	if err != nil {
		http.Error(w, "Frontend assets not found", http.StatusInternalServerError)
		return
	}

	// Replace exactly one placeholder; a no-op if absent (fail-safe: served
	// unchanged, window.__APP_CONFIG__ stays undefined → SPA defaults to no banner).
	html := strings.Replace(string(indexHTML), appConfigPlaceholder, h.appConfigScript, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
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
