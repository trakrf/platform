package serve

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestComputeAllowedMethods(t *testing.T) {
	noop := func(w http.ResponseWriter, r *http.Request) {}

	cases := []struct {
		name   string
		setup  func(r *chi.Mux)
		path   string
		expect []string
	}{
		{
			name:   "single GET synthesizes HEAD",
			setup:  func(r *chi.Mux) { r.Get("/x", noop) },
			path:   "/x",
			expect: []string{"GET", "HEAD"},
		},
		{
			name: "GET and POST",
			setup: func(r *chi.Mux) {
				r.Get("/x", noop)
				r.Post("/x", noop)
			},
			path:   "/x",
			expect: []string{"GET", "HEAD", "POST"},
		},
		{
			name: "PUT and DELETE only — no HEAD",
			setup: func(r *chi.Mux) {
				r.Put("/x", noop)
				r.Delete("/x", noop)
			},
			path:   "/x",
			expect: []string{"PUT", "DELETE"},
		},
		{
			name: "full canonical set ordering",
			setup: func(r *chi.Mux) {
				r.Delete("/x", noop)
				r.Patch("/x", noop)
				r.Put("/x", noop)
				r.Post("/x", noop)
				r.Get("/x", noop)
				r.Options("/x", noop)
			},
			path:   "/x",
			expect: []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		},
		{
			name:   "path matches no route",
			setup:  func(r *chi.Mux) { r.Get("/known", noop) },
			path:   "/unknown",
			expect: []string{},
		},
		{
			name: "path-param route",
			setup: func(r *chi.Mux) {
				r.Get("/api/v1/assets/{asset_id}", noop)
				r.Put("/api/v1/assets/{asset_id}", noop)
			},
			path:   "/api/v1/assets/123",
			expect: []string{"GET", "HEAD", "PUT"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			tc.setup(r)
			got := computeAllowedMethods(r, tc.path)
			if len(tc.expect) == 0 {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.expect, got)
		})
	}
}
