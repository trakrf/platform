package main

import "github.com/getkin/kin-openapi/openapi3"

// postprocessPublic rewrites the doc for customer-facing publication:
// converts the APIKey security scheme from swaggo's 2.0 "apiKey" form
// to 3.0 HTTP-Bearer with bearerFormat JWT, and sets the customer-facing
// info and server URLs. Production is app.trakrf.id (the TrakRF application
// serves both the UI and /api/v1/*); trakrf.id is the marketing site and
// must not appear here — a Bearer token sent there would hit the marketing
// HTML page and silently succeed.
func postprocessPublic(doc *openapi3.T) {
	rewriteAPIKeyScheme(doc)
	doc.Info.Title = "TrakRF API"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "https://app.trakrf.id", Description: "Production"},
		{URL: "https://app.preview.trakrf.id", Description: "Preview (per-PR deploys)"},
	}
}

// postprocessInternal is the same but labels the doc as internal and
// uses a local development server URL. The APIKey scheme rewrite is
// applied here too so both specs share a consistent 3.0 surface.
func postprocessInternal(doc *openapi3.T) {
	rewriteAPIKeyScheme(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
}

func rewriteAPIKeyScheme(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	ref := doc.Components.SecuritySchemes["APIKey"]
	if ref == nil || ref.Value == nil {
		return
	}
	desc := ref.Value.Description
	ref.Value = &openapi3.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  desc,
	}
}
