package main

import (
	"regexp"

	"github.com/getkin/kin-openapi/openapi3"
)

// postprocessPublic rewrites the doc for customer-facing publication:
// converts the BearerAuth scheme from swaggo's 2.0 "apiKey" form to 3.0
// HTTP-Bearer (so the scheme name matches its type — TRA-480 §3.3), sets
// the customer-facing info and server URLs, and normalizes swaggo artefacts
// that confuse OpenAPI codegen (empty metadata schemas, stringified
// x-extensible-enum flags, missing date-time formats on timestamp fields).
// Production is app.trakrf.id (the TrakRF application serves both the UI
// and /api/v1/*); trakrf.id is the marketing site and must not appear here
// — a Bearer token sent there would hit the marketing HTML page and
// silently succeed.
func postprocessPublic(doc *openapi3.T) {
	rewriteBearerAuthScheme(doc)
	normalizeSchemaQuirks(doc)
	doc.Info.Title = "TrakRF API"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "https://app.trakrf.id", Description: "Production"},
		{URL: "https://app.preview.trakrf.id", Description: "Preview (per-PR deploys)"},
	}
}

// postprocessInternal is the same but labels the doc as internal and
// uses a local development server URL.
func postprocessInternal(doc *openapi3.T) {
	rewriteBearerAuthScheme(doc)
	normalizeSchemaQuirks(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
}

// rewriteBearerAuthScheme promotes the session-JWT scheme named "BearerAuth"
// from swaggo's apiKey/header form to http/bearer/JWT so the scheme name
// matches its type. The "APIKey" scheme stays as swaggo emitted it
// (type: apiKey, in: header, name: Authorization) — the name literally says
// "apiKey", so keeping the type aligned avoids confusing SDK codegen.
func rewriteBearerAuthScheme(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	ref := doc.Components.SecuritySchemes["BearerAuth"]
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

// normalizeSchemaQuirks walks every schema in doc.Components.Schemas (and
// every nested Properties map) and corrects three swaggo emission bugs that
// break OpenAPI codegen:
//
//  1. "metadata" properties that render as the empty schema `{}` become
//     `{type: object, additionalProperties: true}` — the intended shape for
//     a free-form JSON object.
//  2. x-extensible-enum extensions emitted as the string "true"/"false"
//     (from swaggo's `extensions:"x-extensible-enum=true"` struct tag,
//     which treats everything as a string) become actual booleans.
//  3. Timestamp-shaped string properties (valid_from, valid_to, *_at,
//     timestamp, last_seen) gain `format: date-time` when swaggo emitted
//     them as plain strings — this happens for Go `time.Time` fields
//     wrapped in custom types (e.g. shared.FlexibleDate with
//     `swaggertype:"string"`), where swaggo drops the default format.
func normalizeSchemaQuirks(doc *openapi3.T) {
	if doc.Components == nil {
		return
	}
	visited := map[*openapi3.Schema]bool{}
	for _, ref := range doc.Components.Schemas {
		walkSchema(ref, visited)
	}
}

func walkSchema(ref *openapi3.SchemaRef, visited map[*openapi3.Schema]bool) {
	if ref == nil || ref.Value == nil || visited[ref.Value] {
		return
	}
	visited[ref.Value] = true

	fixExtensibleEnumBool(ref.Value)

	for name, prop := range ref.Value.Properties {
		if prop == nil {
			continue
		}
		if name == "metadata" && isEmptySchema(prop) {
			upgradeToFreeFormObject(prop)
		}
		if prop.Value != nil && isTimestampField(name) && prop.Value.Type.Is(openapi3.TypeString) && prop.Value.Format == "" {
			prop.Value.Format = "date-time"
		}
		walkSchema(prop, visited)
	}
	if ref.Value.Items != nil {
		walkSchema(ref.Value.Items, visited)
	}
	if ref.Value.AdditionalProperties.Schema != nil {
		walkSchema(ref.Value.AdditionalProperties.Schema, visited)
	}
	for _, s := range ref.Value.AllOf {
		walkSchema(s, visited)
	}
	for _, s := range ref.Value.OneOf {
		walkSchema(s, visited)
	}
	for _, s := range ref.Value.AnyOf {
		walkSchema(s, visited)
	}
}

func isEmptySchema(ref *openapi3.SchemaRef) bool {
	if ref == nil || ref.Ref != "" {
		return false
	}
	v := ref.Value
	if v == nil {
		return false
	}
	if v.Type != nil && len(*v.Type) > 0 || len(v.Properties) > 0 || v.Items != nil {
		return false
	}
	if v.AdditionalProperties.Has != nil || v.AdditionalProperties.Schema != nil {
		return false
	}
	if len(v.AllOf)+len(v.OneOf)+len(v.AnyOf) > 0 || v.Not != nil {
		return false
	}
	return true
}

func upgradeToFreeFormObject(ref *openapi3.SchemaRef) {
	t := true
	ref.Value = &openapi3.Schema{
		Type:                 &openapi3.Types{openapi3.TypeObject},
		AdditionalProperties: openapi3.AdditionalProperties{Has: &t},
	}
}

func fixExtensibleEnumBool(s *openapi3.Schema) {
	const key = "x-extensible-enum"
	raw, ok := s.Extensions[key]
	if !ok {
		return
	}
	switch v := raw.(type) {
	case string:
		switch v {
		case "true":
			s.Extensions[key] = true
		case "false":
			s.Extensions[key] = false
		}
	}
}

var timestampFieldNames = regexp.MustCompile(`^(valid_from|valid_to|timestamp|last_seen|.*_at)$`)

func isTimestampField(name string) bool {
	return timestampFieldNames.MatchString(name)
}
