package main

import (
	"fmt"
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
func postprocessPublic(doc *openapi3.T) error {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	normalizeArrayQueryParams(doc)
	injectTopLevelSecurity(doc)
	stripBearerAuthScheme(doc)
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
	return nil
}

// postprocessInternal is the same but labels the doc as internal and
// uses a local development server URL.
func postprocessInternal(doc *openapi3.T) error {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	normalizeArrayQueryParams(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
	return nil
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

// stripBearerAuthScheme removes the BearerAuth security scheme from the
// public spec's components. BearerAuth is the SPA session JWT and is not
// part of the v1 public API surface (TRA-568). The internal postprocess
// keeps it. Safe to call when the scheme is absent — no-ops in that case.
func stripBearerAuthScheme(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	delete(doc.Components.SecuritySchemes, "BearerAuth")
}

// injectTopLevelSecurity sets the document-level security requirement to
// [{APIKey: []}] (TRA-539 §2.6). This declares that every operation
// requires the APIKey scheme by default, so generated SDK clients
// authenticate every call automatically. Per-operation security
// overrides (e.g. security: [] on public or login endpoints) are
// respected by generators and are not disturbed here — we only set the
// document-level default if it is currently absent.
func injectTopLevelSecurity(doc *openapi3.T) {
	if len(doc.Security) > 0 {
		return
	}
	doc.Security = openapi3.SecurityRequirements{
		openapi3.SecurityRequirement{"APIKey": []string{}},
	}
}

// nullableFields names schema/field pairs whose response payload may be
// null (or omitted when also non-required). swaggo doesn't emit
// nullable:true for Go *Type pointers, so we add it here. The list is
// curated from BB10/BB11 audit findings (TRA-517 AC2, AC9, AC11).
var nullableFields = map[string][]string{
	"asset.PublicAssetView":            {"current_location_id", "current_location_external_key"},
	"apikey.APIKeyListItem":            {"created_by", "created_by_key_id", "last_used_at"},
	"report.PublicAssetHistoryItem":    {"duration_seconds", "location_id", "location_external_key"},
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key"},
	"location.PublicLocationView":      {"parent_id", "parent_external_key"},
}

// requiredFields names the response fields that are guaranteed present in
// every emission of the corresponding schema. The postprocess injects these
// as the schema's `required:` block so generated clients see them as
// non-optional. Source of truth: the Go struct's `json:` tag — a field is
// required iff its tag does NOT contain `,omitempty`. Pointer-typed fields
// without `,omitempty` are required-and-nullable; they belong here AND in
// `nullableFields`. Fields with `,omitempty` (e.g. description, valid_to)
// are excluded.
//
// markRequiredFields errors if a configured schema or field is missing from
// the spec — keeps this map honest as struct fields rename or move.
var requiredFields = map[string][]string{
	// errors
	"errors.ErrorResponse": {"error"},
	"errors.FieldError":    {"field", "code", "message"},

	// shared
	"shared.Tag": {"tag_type", "value", "is_active"},

	// asset
	"asset.PublicAssetView": {"id", "external_key", "name", "current_location_id", "current_location_external_key", "metadata", "is_active", "valid_from", "created_at", "updated_at", "tags"},

	// location
	"location.PublicLocationView": {"id", "external_key", "name", "parent_id", "parent_external_key", "path", "depth", "is_active", "valid_from", "created_at", "tags"},

	// report
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key", "last_seen"},
	"report.PublicAssetHistoryItem":    {"timestamp", "location_id", "location_external_key", "duration_seconds"},

	// apikey
	"apikey.APIKeyListItem":       {"id", "jti", "name", "scopes", "created_by", "created_by_key_id", "created_at"},
	"apikey.APIKeyCreateResponse": {"key", "id", "jti", "name", "scopes", "created_at"},

	// orgs
	"orgs.OrgMeView": {"id", "name"},

	// assets envelopes
	"assets.AddTagResponse":      {"data"},
	"assets.CreateAssetResponse": {"data"},
	"assets.GetAssetResponse":    {"data"},
	"assets.ListAssetsResponse":  {"data", "limit", "offset", "total_count"},
	"assets.UpdateAssetResponse": {"data"},

	// locations envelopes
	"locations.AddTagResponse":          {"data"},
	"locations.CreateLocationResponse":  {"data"},
	"locations.GetLocationResponse":     {"data"},
	"locations.ListAncestorsResponse":   {"data", "limit", "offset", "total_count"},
	"locations.ListChildrenResponse":    {"data", "limit", "offset", "total_count"},
	"locations.ListDescendantsResponse": {"data", "limit", "offset", "total_count"},
	"locations.ListLocationsResponse":   {"data", "limit", "offset", "total_count"},
	"locations.UpdateLocationResponse":  {"data"},

	// orgs envelopes
	"orgs.CreateAPIKeyResponse": {"data"},
	"orgs.GetOrgMeResponse":     {"data"},
	"orgs.ListAPIKeysResponse":  {"data", "limit", "offset", "total_count"},

	// reports envelopes
	"reports.AssetHistoryResponse":         {"data", "limit", "offset", "total_count"},
	"reports.ListCurrentLocationsResponse": {"data", "limit", "offset", "total_count"},
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

// markRequiredFields walks doc.Components.Schemas and sets the `required:`
// list on the configured schemas. Errors if a configured schema or field
// does not exist in the spec, so renames break the build instead of going
// silently stale.
func markRequiredFields(doc *openapi3.T, required map[string][]string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		if len(required) == 0 {
			return nil
		}
		return fmt.Errorf("apispec: components.schemas is empty but requiredFields has %d entries", len(required))
	}
	for schemaName, fields := range required {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("apispec: requiredFields references unknown schema %q", schemaName)
		}
		for _, fieldName := range fields {
			if _, ok := ref.Value.Properties[fieldName]; !ok {
				return fmt.Errorf("apispec: requiredFields references unknown field %q on schema %q", fieldName, schemaName)
			}
		}
		ref.Value.Required = append([]string(nil), fields...)
	}
	return nil
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

// normalizeArrayQueryParams walks every operation parameter in doc.Paths and,
// for each in:query parameter whose schema is type:array, sets Style to "form"
// and Explode to false. This corrects the OpenAPI 3 default (style:form,
// explode:true — i.e. ?sort=a&sort=b) to match the actual CSV wire format the
// server expects (?sort=a,-b). kin-openapi's openapi2conv.ToV3 drops Swagger
// 2.0's collectionFormat, leaving the default that tells codegen to send
// multi-value instead of comma-separated.
//
// The pass is idempotent and does not clobber Style/Explode that are already
// set to a non-default (non-zero) value.
func normalizeArrayQueryParams(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	f := false
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			for _, pRef := range op.Parameters {
				if pRef == nil || pRef.Value == nil {
					continue
				}
				p := pRef.Value
				if p.In != "query" {
					continue
				}
				if p.Schema == nil || p.Schema.Value == nil {
					continue
				}
				if !p.Schema.Value.Type.Is(openapi3.TypeArray) {
					continue
				}
				// Only set if not already overridden to a non-default value.
				if p.Style == "" {
					p.Style = "form"
				}
				if p.Explode == nil {
					p.Explode = &f
				}
			}
		}
	}
}
