package main

import (
	"regexp"

	"github.com/getkin/kin-openapi/openapi3"
)

// postprocessPublic rewrites the doc for customer-facing publication:
// converts the security schemes from swaggo's 2.0 "apiKey" form to 3.0
// HTTP-Bearer, sets the customer-facing info and server URLs, marks fields
// nullable that swaggo can't infer from Go pointer types, and normalizes
// swaggo artefacts that confuse OpenAPI codegen (empty metadata schemas,
// stringified x-extensible-enum flags, missing date-time formats on
// timestamp fields). Production is app.trakrf.id (the TrakRF application
// serves both the UI and /api/v1/*); trakrf.id is the marketing site and
// must not appear here — a Bearer token sent there would hit the marketing
// HTML page and silently succeed. Servers are ordered Preview-first so
// generated clients default to preview during integration testing
// (TRA-517 AC12).
func postprocessPublic(doc *openapi3.T) {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	doc.Info.Title = "TrakRF API"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{
			URL:         "https://app.preview.trakrf.id",
			Description: "Preview (per-PR deploys). Preview-scoped API keys authenticate here only — they will fail with 401 against Production, and Production keys will fail here.",
		},
		{
			URL:         "https://app.trakrf.id",
			Description: "Production. Production-scoped API keys authenticate here only — they will fail with 401 against Preview, and Preview keys will fail here.",
		},
	}
}

// postprocessInternal is the same but labels the doc as internal and
// uses a local development server URL.
func postprocessInternal(doc *openapi3.T) {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
}

// rewriteBearerSchemes promotes the security schemes from swaggo's
// apiKey/header emission to http/bearer/JWT so generated SDKs send
// "Authorization: Bearer <token>" instead of just the raw token.
// Per TRA-517 AC1 this applies to BOTH "APIKey" (the public API key, a
// JWT) and "BearerAuth" (the session JWT) — the platform rejects requests
// without the Bearer prefix, so both must declare http/bearer.
//
// This reverses the TRA-480 §3.3 decision to keep APIKey as type=apiKey
// for cosmetic SDK-naming alignment. Over-the-wire correctness wins.
func rewriteBearerSchemes(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	for _, name := range []string{"APIKey", "BearerAuth"} {
		ref := doc.Components.SecuritySchemes[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		desc := ref.Value.Description
		ref.Value = &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  desc,
		}
	}
}

// nullableFields names schema/field pairs whose response payload may be
// null (or omitted when also non-required). swaggo doesn't emit
// nullable:true for Go *Type pointers, so we add it here. The list is
// curated from BB10/BB11 audit findings (TRA-517 AC2, AC9, AC11).
var nullableFields = map[string][]string{
	"asset.PublicAssetView":         {"current_location", "valid_to"},
	"apikey.APIKeyListItem":         {"created_by_key_id", "last_used_at"},
	"report.PublicAssetHistoryItem": {"duration_seconds"},
}

// annotateErrorEnvelope adds a schema-level description to errors.ErrorResponse
// (and per-property descriptions on the title and detail fields) documenting
// the title vs detail contract from TRA-517 AC4. swaggo doesn't propagate
// godoc on a struct that wraps an anonymous nested struct, so the
// description has to be applied here.
func annotateErrorEnvelope(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	ref := doc.Components.Schemas["errors.ErrorResponse"]
	if ref == nil || ref.Value == nil {
		return
	}
	ref.Value.Description = "RFC 7807 Problem Details envelope. Generated clients should branch on `error.type` and `error.title`, not `error.detail`. " +
		"`error.title` is a stable, machine-readable summary that does not vary between calls for the same condition. " +
		"`error.detail` is the specific, human-readable cause of this particular failure and may be empty when title alone fully describes the condition."

	errProp := ref.Value.Properties["error"]
	if errProp == nil || errProp.Value == nil {
		return
	}
	if title := errProp.Value.Properties["title"]; title != nil && title.Value != nil {
		title.Value.Description = "Stable, machine-readable summary suitable for client-side branching. Does not vary between calls for the same condition."
	}
	if detail := errProp.Value.Properties["detail"]; detail != nil && detail.Value != nil {
		detail.Value.Description = "Specific, human-readable cause of this particular failure. May be empty when title alone fully describes the condition. Do not branch on this value."
	}
}

// markNullableFields walks doc.Components.Schemas and sets nullable:true
// on the curated (schema, field) pairs in nullableFields. Fields not
// declared in `required` may be both omitted and emitted as null; the
// nullable marker tells generated clients null is a legal value.
func markNullableFields(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for schemaName, fields := range nullableFields {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			continue
		}
		for _, fieldName := range fields {
			prop := ref.Value.Properties[fieldName]
			if prop == nil || prop.Value == nil {
				continue
			}
			prop.Value.Nullable = true
		}
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
