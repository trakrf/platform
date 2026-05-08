package main

import (
	"fmt"
	"regexp"
	"strings"

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
	consolidateSchemaNamespaces(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	if err := markReadOnlyFields(doc, readOnlyFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	normalizeArrayQueryParams(doc)
	injectTopLevelSecurity(doc)
	injectMethodNotAllowedResponse(doc)
	stripBearerScopeArrays(doc)
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
	consolidateSchemaNamespaces(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	if err := markRequiredFields(doc, internalOnlyRequiredFields); err != nil {
		return err
	}
	if err := markReadOnlyFields(doc, readOnlyFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	normalizeArrayQueryParams(doc)
	stripBearerScopeArrays(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
	return nil
}

// schemaNamespaceRenames consolidates resource families that swaggo
// emits across multiple namespace prefixes onto a single singular
// namespace per TRA-602. swaggo derives the schema namespace from the
// Go package name; resources whose response wrappers live in a plural
// handler package (e.g. `handlers/assets`) and whose inputs/views live
// in a singular model package (e.g. `models/asset`) end up split.
// Codegen tools that mangle `.` to `_` (Java, Go, TypeScript with
// named exports) emit `Asset…` vs `Assets…` types per resource family
// — easy to grab the wrong one. Renaming pre-launch is a free SDK
// regen; post-launch it would be a breaking change.
//
// The audit covers every resource family with a model/handler split:
//
//   - asset / assets, location / locations, report / reports — original
//     TRA-602 scope.
//   - user / users — internal-only split (users.ListResponse vs
//     user.{Create,Update}UserRequest).
//   - organization / orgs — model package is `organization` (full
//     word), handler package is `orgs` (abbreviation). Both fold onto
//     `org.*` (matches the URL prefix /api/v1/orgs/...).
//   - github_com_trakrf_platform_backend_internal_models_user — swag
//     emission artifact: a single User schema falls back to the full
//     Go import path. Folded onto `user.*` so users.ListResponse can
//     reference user.User cleanly after the rename.
//
// errors.*, shared.*, apikey.*, auth.*, bulkimport.*, health.*,
// inventory.*, lookup.*, storage.* are intentionally untouched — no
// model/handler split exists for those families.
var schemaNamespaceRenames = map[string]string{
	"assets.":       "asset.",
	"locations.":    "location.",
	"reports.":      "report.",
	"users.":        "user.",
	"organization.": "org.",
	"orgs.":         "org.",
	"github_com_trakrf_platform_backend_internal_models_user.": "user.",
}

// consolidateSchemaNamespaces renames Components.Schemas keys whose
// prefix is in schemaNamespaceRenames and rewrites every $ref in the
// document accordingly. The pass runs before markRequiredFields and
// markReadOnlyFields so those passes see the consolidated names.
//
// The rename is collision-checked: if a target name already exists
// (e.g. an `asset.AddTagResponse` already in the schemas map), the
// rename for that key is skipped to avoid silent overwrite — but no
// such collisions exist in the current Go package layout.
func consolidateSchemaNamespaces(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}

	renames := buildSchemaRenameSet(doc.Components.Schemas)
	if len(renames) == 0 {
		return
	}

	schemas := doc.Components.Schemas
	for oldName, newName := range renames {
		schemas[newName] = schemas[oldName]
		delete(schemas, oldName)
	}

	rewriteSchemaRefs(doc, renames)
}

// buildSchemaRenameSet returns the (oldName → newName) renames that
// apply to the current schema map. Two collision guards apply:
//
//  1. If the target name already exists as a distinct schema in the
//     input map (e.g. an `asset.AddTagResponse` already in schemas
//     before any rename), skip the rename for that key.
//  2. If two source names rename to the same target (e.g. multiple
//     prefixes mapping to the same destination namespace), skip every
//     source competing for that target. This guard fires when the
//     prefix table introduces a many-to-one mapping like both
//     `orgs.→org.` and `organization.→org.`; the case is benign in the
//     current Go layout (the two source families have disjoint type
//     names) but the guard keeps a future rename pair from silently
//     overwriting one of the sources.
func buildSchemaRenameSet(schemas openapi3.Schemas) map[string]string {
	candidates := map[string]string{}
	conflicts := map[string]bool{}
	targetSources := map[string]string{}

	for oldName := range schemas {
		newName, ok := applyNamespaceRename(oldName)
		if !ok {
			continue
		}
		if _, exists := schemas[newName]; exists {
			continue
		}
		if prior, taken := targetSources[newName]; taken {
			conflicts[newName] = true
			delete(candidates, prior)
			continue
		}
		targetSources[newName] = oldName
		candidates[oldName] = newName
	}

	if len(conflicts) == 0 {
		return candidates
	}
	for old, new := range candidates {
		if conflicts[new] {
			delete(candidates, old)
		}
	}
	return candidates
}

// applyNamespaceRename returns the consolidated form of name and true
// if the name's prefix is in the rename set. Returns the original name
// and false otherwise.
func applyNamespaceRename(name string) (string, bool) {
	for oldPrefix, newPrefix := range schemaNamespaceRenames {
		if rest, ok := strings.CutPrefix(name, oldPrefix); ok {
			return newPrefix + rest, true
		}
	}
	return name, false
}

// rewriteSchemaRefs walks every SchemaRef in the document and rewrites
// its Ref string from the old to the new component name. Covers paths
// (operation parameters, request bodies, responses, response headers),
// nested schemas (Properties, Items, AdditionalProperties, AllOf,
// OneOf, AnyOf, Not), and Components.Responses.
func rewriteSchemaRefs(doc *openapi3.T, renames map[string]string) {
	rewrite := func(ref *openapi3.SchemaRef) {
		if ref == nil || ref.Ref == "" {
			return
		}
		name, ok := strings.CutPrefix(ref.Ref, schemaRefPrefix)
		if !ok {
			return
		}
		if newName, found := renames[name]; found {
			ref.Ref = schemaRefPrefix + newName
		}
	}

	visited := map[*openapi3.Schema]bool{}
	var walk func(ref *openapi3.SchemaRef)
	walk = func(ref *openapi3.SchemaRef) {
		if ref == nil {
			return
		}
		rewrite(ref)
		s := ref.Value
		if s == nil || visited[s] {
			return
		}
		visited[s] = true
		for _, r := range s.Properties {
			walk(r)
		}
		walk(s.Items)
		if s.AdditionalProperties.Schema != nil {
			walk(s.AdditionalProperties.Schema)
		}
		for _, r := range s.AllOf {
			walk(r)
		}
		for _, r := range s.OneOf {
			walk(r)
		}
		for _, r := range s.AnyOf {
			walk(r)
		}
		walk(s.Not)
	}

	if doc.Components != nil {
		for _, ref := range doc.Components.Schemas {
			walk(ref)
		}
		for _, respRef := range doc.Components.Responses {
			if respRef == nil || respRef.Value == nil {
				continue
			}
			for _, h := range respRef.Value.Headers {
				if h != nil && h.Value != nil {
					walk(h.Value.Schema)
				}
			}
			for _, mt := range respRef.Value.Content {
				if mt != nil {
					walk(mt.Schema)
				}
			}
		}
	}

	if doc.Paths == nil {
		return
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			for _, p := range op.Parameters {
				if p == nil || p.Value == nil {
					continue
				}
				walk(p.Value.Schema)
				for _, mt := range p.Value.Content {
					if mt != nil {
						walk(mt.Schema)
					}
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, mt := range op.RequestBody.Value.Content {
					if mt != nil {
						walk(mt.Schema)
					}
				}
			}
			if op.Responses == nil {
				continue
			}
			for _, resp := range op.Responses.Map() {
				if resp == nil || resp.Value == nil {
					continue
				}
				for _, h := range resp.Value.Headers {
					if h != nil && h.Value != nil {
						walk(h.Value.Schema)
					}
				}
				for _, mt := range resp.Value.Content {
					if mt != nil {
						walk(mt.Schema)
					}
				}
			}
		}
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

// injectMethodNotAllowedResponse adds a reusable 405 response under
// components.responses.MethodNotAllowed. The response declares the Allow
// header (RFC 7231 §6.5.5) so a future operation that references this
// component documents the header without each operation re-declaring it.
//
// TRA-588 scopes this to the component definition only — operations are
// not bulk-attached, since most operations don't currently declare 405
// and the spec defaults to "method not in this operation list" implying
// 405. Operations may $ref this component in a follow-up change.
func injectMethodNotAllowedResponse(doc *openapi3.T) {
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.Responses == nil {
		doc.Components.Responses = openapi3.ResponseBodies{}
	}
	if _, exists := doc.Components.Responses["MethodNotAllowed"]; exists {
		return
	}

	desc := "Method not allowed"
	stringType := &openapi3.Types{openapi3.TypeString}
	allowHeader := &openapi3.HeaderRef{
		Value: &openapi3.Header{
			Parameter: openapi3.Parameter{
				Description: "Comma-separated list of HTTP methods supported on this resource (RFC 7231 §6.5.5).",
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: stringType},
				},
			},
		},
	}

	resp := &openapi3.Response{
		Description: &desc,
		Headers: openapi3.Headers{
			"Allow": allowHeader,
		},
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{
					Ref: "#/components/schemas/errors.ErrorResponse",
				},
			},
		},
	}

	doc.Components.Responses["MethodNotAllowed"] = &openapi3.ResponseRef{Value: resp}
}

// nullableFields names schema/field pairs whose response payload may be
// null (or omitted when also non-required). swaggo doesn't emit
// nullable:true for Go *Type pointers, so we add it here. The list is
// curated from BB10/BB11 audit findings (TRA-517 AC2, AC9, AC11).
var nullableFields = map[string][]string{
	"asset.PublicAssetView":            {"location_id", "location_external_key", "description", "valid_to"},
	"apikey.APIKeyListItem":            {"created_by", "created_by_key_id", "last_used_at"},
	"report.PublicAssetHistoryItem":    {"duration_seconds", "location_id", "location_external_key"},
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key", "asset_deleted_at"},
	"location.PublicLocationView":      {"parent_id", "parent_external_key", "description", "valid_to", "updated_at"},
}

// requiredFields names the response fields that are guaranteed present in
// every emission of the corresponding schema. The postprocess injects these
// as the schema's `required:` block so generated clients see them as
// non-optional. Source of truth: the Go struct's `json:` tag — a field is
// required iff its tag does NOT contain `,omitempty`. Pointer-typed fields
// without `,omitempty` are required-and-nullable; they belong here AND in
// `nullableFields`. Fields with `,omitempty` are excluded.
//
// PublicAssetView / PublicLocationView always emit description, valid_to,
// (and updated_at on location) as null when unset per TRA-610 / BB18 §1.8;
// that's why those fields appear here even though their underlying Go
// types are pointer-or-string with no ,omitempty.
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
	"asset.PublicAssetView": {"id", "external_key", "name", "description", "location_id", "location_external_key", "metadata", "is_active", "valid_from", "valid_to", "created_at", "updated_at", "tags"},

	// location
	"location.PublicLocationView": {"id", "external_key", "name", "description", "parent_id", "parent_external_key", "tree_path", "depth", "is_active", "valid_from", "valid_to", "created_at", "updated_at", "tags"},

	// report
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key", "last_seen", "asset_deleted_at"},
	"report.PublicAssetHistoryItem":    {"timestamp", "location_id", "location_external_key", "duration_seconds"},

	// org (post namespace consolidation — TRA-602)
	"org.OrgMeView": {"id", "name"},

	// asset envelopes (post namespace consolidation — TRA-602)
	"asset.AddTagResponse":      {"data"},
	"asset.CreateAssetResponse": {"data"},
	"asset.GetAssetResponse":    {"data"},
	"asset.ListAssetsResponse":  {"data", "limit", "offset", "total_count"},
	"asset.UpdateAssetResponse": {"data"},

	// location envelopes (post namespace consolidation — TRA-602)
	"location.AddTagResponse":          {"data"},
	"location.CreateLocationResponse":  {"data"},
	"location.GetLocationResponse":     {"data"},
	"location.ListAncestorsResponse":   {"data", "limit", "offset", "total_count"},
	"location.ListChildrenResponse":    {"data", "limit", "offset", "total_count"},
	"location.ListDescendantsResponse": {"data", "limit", "offset", "total_count"},
	"location.ListLocationsResponse":   {"data", "limit", "offset", "total_count"},
	"location.UpdateLocationResponse":  {"data"},

	// org envelope (post namespace consolidation — TRA-602)
	"org.GetOrgMeResponse": {"data"},

	// report envelopes (post namespace consolidation — TRA-602)
	"report.AssetHistoryResponse":         {"data", "limit", "offset", "total_count"},
	"report.ListCurrentLocationsResponse": {"data", "limit", "offset", "total_count"},
}

// internalOnlyRequiredFields is the same as requiredFields but for schemas
// that only appear in the internal spec (the public spec prunes them via
// prunePublicSchemas). TRA-578 flipped /orgs/{id}/api-keys to internal-only,
// taking these schemas with it.
var internalOnlyRequiredFields = map[string][]string{
	"apikey.APIKeyListItem":       {"id", "jti", "name", "scopes", "created_by", "created_by_key_id", "created_at"},
	"apikey.APIKeyCreateResponse": {"token", "id", "jti", "name", "scopes", "created_at"},
	"org.CreateAPIKeyResponse":    {"data"},
	"org.ListAPIKeysResponse":     {"data", "limit", "offset", "total_count"},
}

// readOnlyFields names schema/field pairs whose values are server-managed and
// must not be supplied on the write path (TRA-587 / BB16 S8). Generators that
// honor `readOnly: true` (openapi-generator-cli, openapi-typescript) split the
// schema into read and write variants so SDK consumers can't accidentally send
// these fields back on a verbatim read → write round-trip.
//
// Tags are read-only on these views because tag mutation goes through the
// dedicated POST/DELETE /tags subresource endpoints, not the parent PUT.
//
// markReadOnlyFields errors if a configured schema or field is missing from
// the spec — keeps this map honest as struct fields rename or move.
var readOnlyFields = map[string][]string{
	"asset.PublicAssetView":       {"id", "created_at", "updated_at", "tags"},
	"location.PublicLocationView": {"id", "created_at", "updated_at", "tree_path", "depth", "tags"},
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
	ref.Value.Description = "TrakRF error envelope, modeled on RFC 7807 but not 7807-compliant. " +
		"Fields are nested under `error.*` and content-type is `application/json` (not `application/problem+json`). " +
		"Generated clients should branch on `error.type` and `error.title`, not `error.detail`. " +
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

// markReadOnlyFields walks doc.Components.Schemas and sets ReadOnly on the
// configured (schema, field) pairs. Errors if a configured schema or field
// does not exist in the spec, so renames break the build instead of going
// silently stale.
func markReadOnlyFields(doc *openapi3.T, readOnly map[string][]string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		if len(readOnly) == 0 {
			return nil
		}
		return fmt.Errorf("apispec: components.schemas is empty but readOnlyFields has %d entries", len(readOnly))
	}
	for schemaName, fields := range readOnly {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("apispec: readOnlyFields references unknown schema %q", schemaName)
		}
		for _, fieldName := range fields {
			prop, ok := ref.Value.Properties[fieldName]
			if !ok || prop == nil || prop.Value == nil {
				return fmt.Errorf("apispec: readOnlyFields references unknown field %q on schema %q", fieldName, schemaName)
			}
			prop.Value.ReadOnly = true
		}
	}
	return nil
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

// stripBearerScopeArrays strips non-empty scope arrays from operation-level
// SecurityRequirements where the underlying scheme is http or apiKey.
// OpenAPI 3.0 §4.8.30 only permits scope arrays for oauth2 and openIdConnect
// schemes; swaggo's `@Security APIKey[assets:read]` syntax produces invalid
// arrays under http-bearer. To preserve the scope information for human
// readers, the captured scopes are injected into the operation's
// description as a "**Required scope:** `<scope>`" markdown line. The pass
// is idempotent — repeated runs do not double-prepend.
func stripBearerScopeArrays(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Security == nil {
				continue
			}
			scopes := stripScopesFromRequirements(op.Security, doc.Components.SecuritySchemes)
			if len(scopes) == 0 {
				continue
			}
			op.Description = injectScopeMarker(op.Description, scopes)
		}
	}
}

// stripScopesFromRequirements walks every SecurityRequirement in the slice,
// finds entries whose scheme is http or apiKey, captures and zeroes their
// scope arrays. Returns the captured scope names in declaration order with
// duplicates removed.
func stripScopesFromRequirements(reqs *openapi3.SecurityRequirements, schemes openapi3.SecuritySchemes) []string {
	if reqs == nil {
		return nil
	}
	var captured []string
	seen := map[string]bool{}
	for _, req := range *reqs {
		for name, arr := range req {
			if !isBearerLikeScheme(schemes, name) {
				continue
			}
			if len(arr) == 0 {
				continue
			}
			for _, s := range arr {
				if !seen[s] {
					seen[s] = true
					captured = append(captured, s)
				}
			}
			req[name] = []string{}
		}
	}
	return captured
}

// isBearerLikeScheme returns true if the named scheme is one of the OpenAPI
// 3.0 types where non-empty scope arrays are invalid (http, apiKey, mutualTLS).
// Schemes the function can't resolve are conservatively treated as
// bearer-like — they should not carry scope arrays in our spec.
func isBearerLikeScheme(schemes openapi3.SecuritySchemes, name string) bool {
	ref := schemes[name]
	if ref == nil || ref.Value == nil {
		return true
	}
	switch ref.Value.Type {
	case "oauth2", "openIdConnect":
		return false
	default:
		return true
	}
}

const scopeMarkerPrefix = "**Required scope:**"

// injectScopeMarker prepends a "**Required scope:** `<scope>`" line to the
// description. Idempotent: returns the original string if the marker is
// already present at the beginning.
func injectScopeMarker(description string, scopes []string) string {
	if strings.HasPrefix(description, scopeMarkerPrefix) {
		return description
	}
	marker := scopeMarkerPrefix + " `" + strings.Join(scopes, ", ") + "`"
	if description == "" {
		return marker
	}
	return marker + "\n\n" + description
}
