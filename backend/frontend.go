package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed frontend/dist
var frontendFS embed.FS

// serveFrontend returns an http.Handler that serves embedded frontend assets
// with appropriate cache headers for production
func serveFrontend() http.Handler {
	// Strip "frontend/dist" prefix to serve files from root
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	// Wrap with cache control middleware
	return cacheControlMiddleware(fileServer)
}

// cacheControlMiddleware applies cache headers based on asset type
// - index.html: no-cache (always fresh for updated asset references)
// - /assets/*: 1 year immutable (content-hashed filenames)
// - other files: 1 hour moderate cache
func cacheControlMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// index.html and SPA routes: NO cache (must check for new asset hashes)
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else if strings.HasPrefix(path, "/assets/") {
			// Hashed assets: LONG cache (1 year immutable)
			// Safe because Vite generates new filename when content changes
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Other static files (favicon.ico, icons, etc.): moderate cache
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		next.ServeHTTP(w, r)
	})
}

// spaHandler serves index.html for all frontend routes
// This enables React Router to handle client-side routing
func spaHandler(w http.ResponseWriter, r *http.Request) {
	// Read index.html from embedded filesystem
	indexHTML, err := frontendFS.ReadFile("frontend/dist/index.html")
	if err != nil {
		// Should never happen - embedded at build time
		http.Error(w, "Frontend assets not found", http.StatusInternalServerError)
		return
	}

	// Apply no-cache headers for index.html
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	w.Write(indexHTML)
}
