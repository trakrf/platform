package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// externalKeyPattern is the canonical character class for caller-supplied
// external_keys, declared as the spec-side `pattern` constraint on every
// write schema where external_key (or *_external_key) is writable
// (TRA-615 / BB19 §S5). Source of truth: ExternalKeyPattern in
// internal/util/httputil/validation.go — the server-side validator.
const externalKeyPattern = "^[A-Za-z0-9-]+$"

// externalKeyPatternFields names schema/field pairs that should declare the
// external_key_pattern constraint in the spec. Mirrors the
// `validate:"...,external_key_pattern"` tags on the matching Go structs.
var externalKeyPatternFields = map[string][]string{
	"asset.CreateAssetRequest":               {"external_key", "location_external_key"},
	"asset.CreateAssetWithTagsRequest":       {"external_key", "location_external_key"},
	"asset.RenameAssetRequest":               {"external_key"},
	"location.CreateLocationRequest":         {"external_key", "parent_external_key"},
	"location.CreateLocationWithTagsRequest": {"external_key", "parent_external_key"},
	// TRA-719 / BB35 B2: parent_external_key is now writable on PATCH and
	// must carry the same pattern declaration as Create.
	"location.UpdateLocationRequest": {"parent_external_key"},
	"location.RenameLocationRequest": {"external_key"},
}

// markExternalKeyPattern walks doc.Components.Schemas and sets the
// external_key_pattern on the curated (schema, field) pairs in
// externalKeyPatternFields. The server enforces the same pattern via the
// `external_key_pattern` validator tag — pattern in spec is a documentation /
// codegen alignment, not a second enforcement layer.
func markExternalKeyPattern(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for schemaName, fields := range externalKeyPatternFields {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			continue
		}
		for _, fieldName := range fields {
			prop := ref.Value.Properties[fieldName]
			if prop == nil || prop.Value == nil {
				continue
			}
			prop.Value.Pattern = externalKeyPattern
		}
	}
}

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
	inlinePublicTimeRefs(doc)
	markNullableFields(doc)
	markExternalKeyPattern(doc)
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
	attachMethodNotAllowedToOperations(doc)
	injectDeprecationComponents(doc)
	if err := stripResponseSchemasAdditive(doc, publicResponseSchemas); err != nil {
		return err
	}
	if err := closeWriteSchemasToUnknownFields(doc, publicWriteSchemas); err != nil {
		return err
	}
	if err := markMutuallyExclusiveFieldPairs(doc, mutuallyExclusiveFieldPairs); err != nil {
		return err
	}
	if err := markPrintableStringFields(doc, printableStringFields); err != nil {
		return err
	}
	markIntegerFormats(doc)
	markQueryIntegerBounds(doc)
	markSurrogateIDsInt64(doc)
	markQueryStringPatterns(doc)
	flattenSortQueryToString(doc)
	markDateTimeExamples(doc)
	injectDefaultErrorResponse(doc)
	injectGlobalHeaderRefs(doc)
	stripBearerScopeArrays(doc)
	stripSessionAuthScheme(doc)
	appendSpecVariantsDescription(doc)
	appendMethodPolicyDescription(doc)
	appendIDWidthPolicyDescription(doc)
	rewriteMergePatchContentType(doc)
	annotateReadOnlyTags(doc)
	annotateTagPolymorphism(doc)
	if err := splitTagPolymorphism(doc); err != nil {
		return fmt.Errorf("split tag polymorphism: %w", err)
	}
	if err := hoistInlineEnums(doc); err != nil {
		return fmt.Errorf("hoist inline enums: %w", err)
	}
	if err := renamePublicSpec(doc); err != nil {
		return fmt.Errorf("rename public spec: %w", err)
	}
	doc.Info.Title = "TrakRF API"
	// info.version is the spec version (semver per Zalando / TRA-672); the
	// API surface version lives in the URL path (/api/v1/...). They evolve
	// independently — info.version bumps on every breaking change to this
	// document, /api/v1 is a long-lived URL contract.
	doc.Info.Version = "1.0.0"
	if doc.Info.Contact == nil {
		doc.Info.Contact = &openapi3.Contact{}
	}
	if doc.Info.Contact.Name == "" {
		doc.Info.Contact.Name = "TrakRF Support"
	}
	if doc.Info.Contact.Email == "" {
		doc.Info.Contact.Email = "support@trakrf.id"
	}
	if doc.Info.Contact.URL == "" {
		// Production canonical. The build emits a single artifact for every
		// environment, so the committed spec must pin one value. The backend
		// swaps this to the preview equivalent at serve time when
		// APP_ENV=preview — see swaggerspec.resolvePublicSpec (TRA-717 / BB34
		// F4). Keep the bare-hostname servers[] entries below untouched so
		// the substitution remains targeted to contact.url alone.
		doc.Info.Contact.URL = "https://app.trakrf.id/api"
	}
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
	inlinePublicTimeRefs(doc)
	markNullableFields(doc)
	markExternalKeyPattern(doc)
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
	injectGlobalHeaderRefs(doc)
	stripBearerScopeArrays(doc)
	appendMethodPolicyDescription(doc)
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

	rewriteMappingRef := func(name string) (string, bool) {
		stripped, ok := strings.CutPrefix(name, schemaRefPrefix)
		if !ok {
			return "", false
		}
		newName, found := renames[stripped]
		if !found {
			return "", false
		}
		return schemaRefPrefix + newName, true
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
		// Discriminator mapping refs are plain strings (MappingRef
		// serialises as text, not as a $ref object) so the SchemaRef
		// walk above does not reach them. Rewrite them explicitly so
		// the union's mapping survives publicSchemaRenames.
		if s.Discriminator != nil {
			for k, mr := range s.Discriminator.Mapping {
				if newRef, ok := rewriteMappingRef(mr.Ref); ok {
					mr.Ref = newRef
					s.Discriminator.Mapping[k] = mr
				}
			}
		}
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
// Per TRA-517 AC1 this applies to BOTH "BearerAuth" (the public API key, a
// JWT — TRA-616 renamed from "APIKey") and "SessionAuth" (the SPA session
// JWT — TRA-616 renamed from "BearerAuth"); the platform rejects requests
// without the Bearer prefix, so both must declare http/bearer.
//
// This reverses the TRA-480 §3.3 decision to keep the public scheme as
// type=apiKey for cosmetic SDK-naming alignment — over-the-wire correctness
// wins. The TRA-616 rename to BearerAuth restores the OpenAPI convention
// for HTTP-Bearer schemes so class-emitting codegen tools (typescript-fetch,
// java, python) produce a `Configuration.accessToken`-shaped client rather
// than an `apiKey`-shaped one.
func rewriteBearerSchemes(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	for _, name := range []string{"BearerAuth", "SessionAuth"} {
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

// stripSessionAuthScheme removes the SessionAuth security scheme from the
// public spec's components. SessionAuth is the SPA session JWT and is not
// part of the v1 public API surface (TRA-568). The internal postprocess
// keeps it. Safe to call when the scheme is absent — no-ops in that case.
func stripSessionAuthScheme(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	delete(doc.Components.SecuritySchemes, "SessionAuth")
}

// injectTopLevelSecurity sets the document-level security requirement to
// [{BearerAuth: []}] (TRA-539 §2.6, renamed from APIKey by TRA-616). This
// declares that every operation requires the BearerAuth scheme by default,
// so generated SDK clients authenticate every call automatically.
// Per-operation security overrides (e.g. security: [] on public or login
// endpoints) are respected by generators and are not disturbed here — we
// only set the document-level default if it is currently absent.
func injectTopLevelSecurity(doc *openapi3.T) {
	if len(doc.Security) > 0 {
		return
	}
	doc.Security = openapi3.SecurityRequirements{
		openapi3.SecurityRequirement{"BearerAuth": []string{}},
	}
}

// injectGlobalHeaderRefs (TRA-633 B3) consolidates the X-RateLimit-*,
// Retry-After, WWW-Authenticate, and X-Request-Id headers under
// components.headers and rewrites every operation response to reference
// them.
//
// Live behavior anchors the choice of which responses get which headers:
//
//   - DefaultRateLimitHeaders middleware (router.go) wraps every
//     /api/v1/* response, so X-RateLimit-{Limit,Remaining,Reset} appear
//     on every status — 200 and the full error family alike.
//   - RequestID middleware sets X-Request-Id on every response and
//     errors.ErrorResponse echoes it as error.request_id, which is the
//     durable correlation handle support tickets quote.
//   - Retry-After is emitted only on 429 by the rate limiter.
//   - WWW-Authenticate is emitted only on 401 by httputil.Respond401
//     (RFC 7235 mandates it on 401). 403 carries no challenge — the
//     caller is authenticated but unauthorized.
//
// The pass is idempotent: re-running on a doc whose responses already
// hold the canonical $refs leaves them unchanged. Inline header
// definitions emitted by swag (// @Header annotations) are flattened
// to the $ref form here; the spec output should grep zero per-endpoint
// duplications afterwards.
func injectGlobalHeaderRefs(doc *openapi3.T) {
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.Headers == nil {
		doc.Components.Headers = openapi3.Headers{}
	}

	intSchema := func() *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}}
	}
	strSchema := func() *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}}
	}

	type headerDef struct {
		name        string
		description string
		schema      func() *openapi3.SchemaRef
	}
	defs := []headerDef{
		{"XRateLimitLimit", "Steady-state requests/min for this API key.", intSchema},
		{"XRateLimitRemaining", "Requests remaining before throttling; bounded by X-RateLimit-Limit.", intSchema},
		{"XRateLimitReset", "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit.", intSchema},
		{"RetryAfter", "Seconds to wait before retrying.", intSchema},
		{"WWWAuthenticate", "RFC 7235 authentication challenge. Always `Bearer realm=\"trakrf-api\"` on 401 responses.", strSchema},
		{"XRequestId", "Server-assigned request correlation identifier; mirrored as error.request_id in error envelopes and echoed in server logs. Quote this when filing support tickets.", strSchema},
	}
	for _, d := range defs {
		if _, exists := doc.Components.Headers[d.name]; exists {
			continue
		}
		doc.Components.Headers[d.name] = &openapi3.HeaderRef{
			Value: &openapi3.Header{
				Parameter: openapi3.Parameter{
					Description: d.description,
					Schema:      d.schema(),
				},
			},
		}
	}

	if doc.Paths == nil {
		return
	}

	rateLimitLimit := &openapi3.HeaderRef{Ref: "#/components/headers/XRateLimitLimit"}
	rateLimitRemaining := &openapi3.HeaderRef{Ref: "#/components/headers/XRateLimitRemaining"}
	rateLimitReset := &openapi3.HeaderRef{Ref: "#/components/headers/XRateLimitReset"}
	retryAfter := &openapi3.HeaderRef{Ref: "#/components/headers/RetryAfter"}
	wwwAuthenticate := &openapi3.HeaderRef{Ref: "#/components/headers/WWWAuthenticate"}
	requestID := &openapi3.HeaderRef{Ref: "#/components/headers/XRequestId"}

	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Responses == nil {
				continue
			}
			for code, resp := range op.Responses.Map() {
				if resp == nil || resp.Value == nil {
					continue
				}
				if resp.Value.Headers == nil {
					resp.Value.Headers = openapi3.Headers{}
				}
				resp.Value.Headers["X-RateLimit-Limit"] = rateLimitLimit
				resp.Value.Headers["X-RateLimit-Remaining"] = rateLimitRemaining
				resp.Value.Headers["X-RateLimit-Reset"] = rateLimitReset
				resp.Value.Headers["X-Request-Id"] = requestID
				if code == "429" {
					resp.Value.Headers["Retry-After"] = retryAfter
				}
				if code == "401" {
					resp.Value.Headers["WWW-Authenticate"] = wwwAuthenticate
				}
			}
		}
	}
}

// appendSpecVariantsDescription (TRA-657 BB25 A8) advertises both spec
// variants in info.description on the public spec. The /api Redoc page
// exposes a single download button that pulls only the YAML; consumers
// preferring JSON must already know the canonical /api/openapi.json URL
// or look at a sibling docs page that advertises both. The
// info.description is the natural surface for that pointer. Site-relative
// paths match the rationale in appendMethodPolicyDescription.
func appendSpecVariantsDescription(doc *openapi3.T) {
	const marker = "Spec available as YAML"
	if strings.Contains(doc.Info.Description, marker) {
		return
	}
	advert := "Spec available as YAML (/api/openapi.yaml) and JSON (/api/openapi.json)."
	if doc.Info.Description == "" {
		doc.Info.Description = advert
	} else {
		doc.Info.Description = doc.Info.Description + "\n\n" + advert
	}
}

// appendMethodPolicyDescription (TRA-633 B1, B4 / TRA-649 BB23 S4 /
// TRA-657 BB25 B4) keeps info.description at a one-line pointer to the
// customer-facing reference. The platform router exposes HEAD and OPTIONS
// uniformly via middleware (chimiddleware.GetHead rewrites HEAD→GET; CORS
// middleware short-circuits OPTIONS to 204), so per-path declarations
// would double the operation count for no codegen value. Earlier
// iterations inlined the policy prose directly into info.description, but
// generated SDK class docstrings (AssetsApi.ts, LocationsApi.ts,
// OrgsApi.ts) carried the entire HTTP-method-coverage paragraph at the
// top of every file — TRA-649 / BB23 S4 trimmed that bloat. The full
// policy now lives at /api/http-method-coverage on the docs site.
//
// The link is emitted as a site-relative path (/api/http-method-coverage)
// rather than the absolute https://docs.trakrf.id/... URL: the canonical
// spec is published from docs.preview.trakrf.id during PR review and from
// docs.trakrf.id in production, and a partner doing strict env isolation
// should not see preview-spec links pointing at production docs
// (TRA-657 / BB25 B4).
func appendMethodPolicyDescription(doc *openapi3.T) {
	const marker = "HTTP method coverage"
	if strings.Contains(doc.Info.Description, marker) {
		return
	}
	policy := "HTTP method coverage (HEAD, OPTIONS, 405 / Allow): " +
		"/api/http-method-coverage"
	if doc.Info.Description == "" {
		doc.Info.Description = policy
	} else {
		doc.Info.Description = doc.Info.Description + "\n\n" + policy
	}
}

// appendIDWidthPolicyDescription documents the surrogate-ID wire/storage
// divergence on info.description so integrators reading the Redoc page
// (or generated SDK class docstrings) understand the contract.
//
// Wire format is int64 across every surrogate PK/FK (BB35 B7) to avoid
// a future-breaking SDK regen when the namespace eventually outgrows
// int32. Service-side ID generation stays within int32 for v1 — the
// underlying Postgres column is `int4` — and the parser rejects values
// above 2^31-1 with a 400 validation_error / `too_large`. The wider
// declared wire type is for SDK type-safety on the long horizon, not a
// claim that today's service handles values above 2^31-1.
//
// Site-relative-paths follow the same rationale as
// appendMethodPolicyDescription (preview vs production isolation).
func appendIDWidthPolicyDescription(doc *openapi3.T) {
	const marker = "Surrogate ID width"
	if strings.Contains(doc.Info.Description, marker) {
		return
	}
	policy := "Surrogate ID width: declared `format: int64` on the wire " +
		"so SDK regeneration does not break when the ID namespace eventually " +
		"outgrows int32. Service-side ID generation stays within int32 (2^31-1) " +
		"during v1; values above that bound are rejected with 400 " +
		"validation_error / `too_large`. The wider wire type is a long-horizon " +
		"contract, not a claim that current values exceed int32."
	if doc.Info.Description == "" {
		doc.Info.Description = policy
	} else {
		doc.Info.Description = doc.Info.Description + "\n\n" + policy
	}
}

// injectMethodNotAllowedResponse adds a reusable 405 response under
// components.responses.MethodNotAllowed. The response declares the Allow
// header (RFC 7231 §6.5.5) alongside the rate-limit and request-id headers
// the service emits on every response, so an operation that references
// this component documents the full header set without each operation
// re-declaring them. The companion attachMethodNotAllowedToOperations
// pass bulk-references this component from every operation so codegens
// can model 405 as a possible response on every endpoint (TRA-646 /
// BB22 S1; TRA-723 / BB36 F3 added the rate-limit + request-id headers
// to match what the service actually emits).
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
			"Allow":                 allowHeader,
			"X-RateLimit-Limit":     {Ref: "#/components/headers/XRateLimitLimit"},
			"X-RateLimit-Remaining": {Ref: "#/components/headers/XRateLimitRemaining"},
			"X-RateLimit-Reset":     {Ref: "#/components/headers/XRateLimitReset"},
			"X-Request-Id":          {Ref: "#/components/headers/XRequestId"},
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

// attachMethodNotAllowedToOperations references the MethodNotAllowed
// component from every operation that does not already declare a "405"
// response (TRA-646 / BB22 S1). Codegens that pre-allocate response arms
// from the spec need the per-operation declaration; the universal
// behavior is documented in info.description but is not machine-readable
// without each operation enumerating it.
func attachMethodNotAllowedToOperations(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	if doc.Components == nil || doc.Components.Responses == nil {
		return
	}
	if _, ok := doc.Components.Responses["MethodNotAllowed"]; !ok {
		return
	}
	ref := &openapi3.ResponseRef{Ref: "#/components/responses/MethodNotAllowed"}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			if op.Responses == nil {
				op.Responses = openapi3.NewResponses()
			}
			if existing := op.Responses.Value("405"); existing != nil {
				continue
			}
			op.Responses.Set("405", ref)
		}
	}
}

// injectDeprecationComponents adds the response/header pair the
// RFC 8594 deprecation+sunset flow needs (TRA-646 / BB22 S3). No path
// references these yet — they are forward-looking so codegens can model
// "endpoint returns 410 after sunset" and "Deprecation/Sunset headers may
// appear on a deprecated endpoint" before the first endpoint sunset.
//
// Components added:
//   - components.headers.Deprecation: RFC 8594 boolean/date header.
//   - components.headers.Sunset:      RFC 8594 sunset date header.
//   - components.responses.Gone:      410 response after sunset.
func injectDeprecationComponents(doc *openapi3.T) {
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.Headers == nil {
		doc.Components.Headers = openapi3.Headers{}
	}
	if doc.Components.Responses == nil {
		doc.Components.Responses = openapi3.ResponseBodies{}
	}

	stringType := &openapi3.Types{openapi3.TypeString}

	if _, exists := doc.Components.Headers["Deprecation"]; !exists {
		doc.Components.Headers["Deprecation"] = &openapi3.HeaderRef{
			Value: &openapi3.Header{
				Parameter: openapi3.Parameter{
					Description: "RFC 8594 deprecation indicator. Either the literal value `true` or an HTTP-date marking when the endpoint became deprecated. Present on every response from a deprecated endpoint until the sunset date.",
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: stringType},
					},
				},
			},
		}
	}
	if _, exists := doc.Components.Headers["Sunset"]; !exists {
		doc.Components.Headers["Sunset"] = &openapi3.HeaderRef{
			Value: &openapi3.Header{
				Parameter: openapi3.Parameter{
					Description: "RFC 8594 sunset date. HTTP-date marking when the endpoint will stop responding (200 series replaced by 410 Gone). Pairs with Deprecation; appears on every response from a deprecated endpoint.",
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: stringType, Format: "http-date"},
					},
				},
			},
		}
	}

	if _, exists := doc.Components.Responses["Gone"]; !exists {
		desc := "Endpoint sunset. The endpoint was deprecated and has now passed its Sunset date; clients should migrate to the documented replacement (RFC 8594)."
		doc.Components.Responses["Gone"] = &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &desc,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/errors.ErrorResponse",
						},
					},
				},
			},
		}
	}
}

// publicResponseSchemas names the public response models that the
// Versioning page commits to additive-stable evolution. The list is
// retained as a hygiene roster: stripResponseSchemasAdditive walks it to
// remove the literal `additionalProperties: true` that swag's
// `--parseDependency` emits on every object (TRA-668 BB27 S8 / TRA-672 —
// the explicit `:true` caused some generators to emit wrapper classes
// instead of clean Record<string,unknown> shapes). OpenAPI 3.0's default
// is already permissive, so dropping the flag preserves additive evolution
// without the codegen drag. Supersedes the prior TRA-646 / BB22 S4
// behavior, which set the flag explicitly for the opposite reason.
//
// Includes view models, response envelopes, error envelopes, and
// shared.Tag (carried in tag responses). Internal-only schemas are
// excluded — internalOnlyRequiredFields names them.
var publicResponseSchemas = []string{
	// view models
	"asset.PublicAssetView",
	"location.PublicLocationView",
	"report.PublicAssetHistoryItem",
	"report.PublicCurrentLocationItem",
	"org.OrgMeView",

	// asset envelopes
	"asset.AddTagResponse",
	"asset.CreateAssetResponse",
	"asset.GetAssetResponse",
	"asset.ListAssetsResponse",
	"asset.RenameAssetResponse",
	"asset.UpdateAssetResponse",

	// location envelopes
	"location.AddTagResponse",
	"location.CreateLocationResponse",
	"location.GetLocationResponse",
	"location.ListAncestorsResponse",
	"location.ListChildrenResponse",
	"location.ListDescendantsResponse",
	"location.ListLocationsResponse",
	"location.RenameLocationResponse",
	"location.UpdateLocationResponse",

	// org envelope
	"org.GetOrgMeResponse",

	// report envelopes
	"report.AssetHistoryResponse",
	"report.ListCurrentLocationsResponse",

	// shared payloads carried in responses
	"shared.Tag",

	// error envelopes — also returned over the wire
	"errors.ErrorResponse",
	"errors.FieldError",
}

// publicWriteSchemas names the public request bodies that the server
// decodes with strict-unknown-field enforcement (DisallowUnknownFields).
// closeWriteSchemasToUnknownFields sets `additionalProperties: false` on
// each so the spec advertises the runtime contract — Schemathesis-class
// "API rejected schema-compliant request" failures (TRA-678) trace back
// to integrators sending unknown fields against a permissive spec.
//
// Read schemas are NOT in this list — additive evolution requires
// generated clients to ignore unknown fields on responses. Internal-only
// request bodies are excluded; only the public surface is gated.
//
// TRA-719 / BB35 B1: UpdateAssetRequest and UpdateLocationRequest are
// intentionally omitted. TRA-710 (BB33 F2) made the service uniformly
// accept-if-matches for read-only fields, so a verbatim GET → PATCH
// round-trip is valid. Strict client-side generators (Pydantic
// `extra="forbid"`, Java/Kotlin strict mode) would reject the round-trip
// at the schema level if `additionalProperties: false` were advertised.
// The server-side `unknown_field` rejection still fires for genuinely
// unrecognized properties — the contract lives at the validator, not at
// the schema declaration.
var publicWriteSchemas = []string{
	"asset.CreateAssetWithTagsRequest",
	"asset.RenameAssetRequest",
	"location.CreateLocationWithTagsRequest",
	"location.RenameLocationRequest",
	"shared.TagRequest",
}

// mutuallyExclusiveFieldPairs declares (schema, fieldA, fieldB) tuples
// where the surrogate id and natural-key alternate cannot be supplied
// together on Create (TRA-678). Encoded via a JSON Schema `not: required:
// [a, b]` clause so generators understand the constraint and Schemathesis-
// class "API rejected schema-compliant request" failures stop firing when
// the server's "both must agree" check rejects a fuzz-generated payload.
//
// Update / PATCH bodies are intentionally NOT listed: the JSON-Merge-Patch
// semantic uses explicit null on either field as a clear-this-FK signal,
// so a payload that sends `{a: null, b: null}` to clear the FK must remain
// valid — a `not: required` constraint would reject it. The PATCH handler
// implements its own per-field reconciliation.
var mutuallyExclusiveFieldPairs = []struct {
	Schema string
	FieldA string
	FieldB string
}{
	{"asset.CreateAssetWithTagsRequest", "location_id", "location_external_key"},
	{"location.CreateLocationWithTagsRequest", "parent_id", "parent_external_key"},
}

// markMutuallyExclusiveFieldPairs walks each (schema, a, b) tuple and
// installs a `not: { required: [a, b] }` clause on the schema. Idempotent;
// repeated runs don't double-stack the `not` constraint.
func markMutuallyExclusiveFieldPairs(doc *openapi3.T, pairs []struct{ Schema, FieldA, FieldB string }) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		if len(pairs) == 0 {
			return nil
		}
		return fmt.Errorf("apispec: components.schemas is empty but mutuallyExclusiveFieldPairs has %d entries", len(pairs))
	}
	for _, pair := range pairs {
		ref := doc.Components.Schemas[pair.Schema]
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("apispec: mutuallyExclusiveFieldPairs references unknown schema %q", pair.Schema)
		}
		not := &openapi3.Schema{Required: []string{pair.FieldA, pair.FieldB}}
		ref.Value.Not = &openapi3.SchemaRef{Value: not}
	}
	return nil
}

// printableStringRegex is the JSON Schema regex that mirrors the server-
// side `no_control_chars` validator (TRA-678): allows tab, LF, CR, and any
// code point outside the C0 controls and DEL. Schemathesis honors `pattern`
// at item-string generation, so adding it on a property prevents the fuzz
// generator from emitting payloads that the server will reject with 400.
// Without this annotation Schemathesis treats the "API rejected schema-
// compliant request" 400 (validator firing on control chars) as a contract
// gap.
//
// TRA-687: use a raw string so the value carries literal `\xNN` escape
// sequences rather than actual control bytes. The bytes survive YAML/JSON
// serialization unmolested, so `openapi-generator-cli generate -g python`
// can emit them into a Python source file without tripping Python's
// "source code string cannot contain null bytes" SyntaxError. Python `re`,
// JavaScript, and Go's RE2 all interpret `\xNN` at match time, so runtime
// behavior is preserved.
const printableStringRegex = `^[^\x00-\x08\x0B\x0C\x0E-\x1F\x7F]*$`

// printableStringFields names (schema, field) pairs that the no_control_chars
// validator gates server-side. Mirror in the spec so generated fuzz payloads
// don't trip the validator with class-A NUL / control-char strings.
var printableStringFields = map[string][]string{
	"asset.CreateAssetWithTagsRequest":       {"name", "description"},
	"asset.UpdateAssetRequest":               {"name", "description"},
	"location.CreateLocationWithTagsRequest": {"name", "description"},
	"location.UpdateLocationRequest":         {"name", "description"},
	"shared.TagRequest":                      {"value"},
	"shared.Tag":                             {"value"},
}

// markPrintableStringFields sets `pattern: printableStringRegex` on each
// listed (schema, field) pair. Missing schemas/fields are skipped — same
// lenient pattern as markNullableFields. Idempotent: if an explicit
// pattern is already declared, it is preserved.
func markPrintableStringFields(doc *openapi3.T, fields map[string][]string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil
	}
	for schemaName, props := range fields {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			continue
		}
		for _, p := range props {
			prop := ref.Value.Properties[p]
			if prop == nil || prop.Value == nil {
				continue
			}
			if prop.Value.Pattern == "" {
				prop.Value.Pattern = printableStringRegex
			}
		}
	}
	return nil
}

// closeWriteSchemasToUnknownFields sets `additionalProperties: false` on
// every schema in `schemas` (TRA-678). Missing schemas are skipped rather
// than fatal — matches the markNullableFields lenient pattern so the pass
// is reusable in unit tests that construct minimal in-memory docs.
// Idempotent.
func closeWriteSchemasToUnknownFields(doc *openapi3.T, schemas []string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil
	}
	f := false
	for _, name := range schemas {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		ref.Value.AdditionalProperties = openapi3.AdditionalProperties{Has: &f}
	}
	return nil
}

// markQueryStringPatterns applies the right pattern to free-form text
// query filters. Two pattern classes:
//
//   - `q` (substring search): printable-string — control chars rejected,
//     everything else accepted. Mirrors the body-side no_control_chars
//     validator. TRA-678.
//   - `*external_key` (identifier filter): strict external_key pattern
//     (`^[A-Za-z0-9-]+$`) — matches the server-side validator applied at
//     POST/PATCH time and at filter parse time via
//     ValidateExternalKeyFilterValues. Without this, a generated client
//     validating against the spec would accept `?external_key=abc/def`
//     locally and then surface a server-side 400 the client believes
//     "shouldn't happen." TRA-713 / TRA-717 / BB33 F5 + BB34 F5.
//
// Schemathesis sees the right shape for each: control-char fuzz on `q`
// is negative_data_rejection (positive on q would be wrong); slash/hash
// fuzz on `*external_key` is negative_data_rejection (positive would
// silently 200-with-empty pre-TRA-713, 400 post-TRA-713).
func markQueryStringPatterns(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	apply := func(p *openapi3.Parameter) {
		if p == nil || p.In != "query" || p.Schema == nil || p.Schema.Value == nil {
			return
		}
		s := p.Schema.Value
		var pattern string
		switch {
		case p.Name == "q":
			pattern = printableStringRegex
		case strings.HasSuffix(p.Name, "external_key"):
			pattern = externalKeyPattern
		default:
			return
		}
		if s.Type.Is(openapi3.TypeArray) && s.Items != nil && s.Items.Value != nil {
			items := s.Items.Value
			if items.Type.Is(openapi3.TypeString) && items.Pattern == "" {
				items.Pattern = pattern
			}
			return
		}
		if s.Type.Is(openapi3.TypeString) && s.Pattern == "" {
			s.Pattern = pattern
		}
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, pRef := range item.Parameters {
			if pRef != nil {
				apply(pRef.Value)
			}
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			for _, pRef := range op.Parameters {
				if pRef != nil {
					apply(pRef.Value)
				}
			}
		}
	}
}

// markQueryIntegerBounds applies int4 surrogate-id bounds to query
// parameters whose name matches a surrogate-id (suffix `_id`) and to
// pagination's `offset` (TRA-678). The server rejects 0 on id-keyed
// query filters and overflows on offset values that exceed int4, so the
// spec must advertise the runtime bounds.
//
// Array-typed id filters (e.g. `location_id` as `array<integer>` with
// repeat semantics) are walked into Items. Scalar `offset` is set as-is.
//
// The pass is idempotent and does not clobber bounds that are already
// declared at a non-default value.
func markQueryIntegerBounds(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	intMax := float64(2147483647)
	one := float64(1)
	zero := float64(0)
	applyIDBounds := func(s *openapi3.Schema) {
		if s == nil || s.Type == nil || !s.Type.Is(openapi3.TypeInteger) {
			return
		}
		if s.Min == nil {
			s.Min = &one
		}
		if s.Max == nil {
			s.Max = &intMax
		}
	}
	applyOffsetBounds := func(s *openapi3.Schema) {
		if s == nil || s.Type == nil || !s.Type.Is(openapi3.TypeInteger) {
			return
		}
		if s.Min == nil {
			s.Min = &zero
		}
		if s.Max == nil {
			s.Max = &intMax
		}
	}
	walkParam := func(p *openapi3.Parameter) {
		if p == nil || p.In != "query" || p.Schema == nil || p.Schema.Value == nil {
			return
		}
		s := p.Schema.Value
		switch {
		case p.Name == "offset":
			applyOffsetBounds(s)
		case strings.HasSuffix(p.Name, "_id"):
			if s.Type.Is(openapi3.TypeArray) && s.Items != nil && s.Items.Value != nil {
				applyIDBounds(s.Items.Value)
			} else {
				applyIDBounds(s)
			}
		}
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, pRef := range item.Parameters {
			if pRef != nil {
				walkParam(pRef.Value)
			}
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			for _, pRef := range op.Parameters {
				if pRef != nil {
					walkParam(pRef.Value)
				}
			}
		}
	}
}

// stripResponseSchemasAdditive removes `additionalProperties: true` from
// each public response model (TRA-668 BB27 S8 / TRA-672). The literal
// `:true` came from swag's `--parseDependency` emission on every object;
// some generators emit wrapper classes around `:true` schemas instead of
// clean Record<string,unknown> types. OpenAPI 3.0's default is already
// permissive, so dropping the flag keeps additive evolution without the
// codegen drag.
//
// Only the literal `true` is stripped. Structured `additionalProperties`
// schemas (e.g., errors.FieldError.params) are preserved untouched.
//
// Errors if a configured schema is missing — keeps the roster honest as
// schemas rename or move.
func stripResponseSchemasAdditive(doc *openapi3.T, schemas []string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		if len(schemas) == 0 {
			return nil
		}
		return fmt.Errorf("apispec: components.schemas is empty but publicResponseSchemas has %d entries", len(schemas))
	}
	for _, name := range schemas {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("apispec: publicResponseSchemas references unknown schema %q", name)
		}
		ap := &ref.Value.AdditionalProperties
		if ap.Schema != nil {
			continue
		}
		if ap.Has != nil && *ap.Has {
			ap.Has = nil
		}
	}
	return nil
}

// markIntegerFormats walks every schema in the document and sets
// `format: int32` on any integer property that lacks an explicit format.
// Zalando's `must-define-a-format-for-integer-types` rule requires this;
// codegen libraries (typescript-fetch, openapi-typescript, openapi-go) use
// the format to pick the right wire encoding. Every TrakRF integer column
// is Postgres `integer` (int4) — IDs, pagination counts, depth, duration
// seconds — and the HTTP status code in ErrorResponse.error.status is also
// int32. int64 fields don't exist in the public surface, so this pass
// safely defaults all unspecified integers to int32 (TRA-672 audit).
//
// Properties that already declare a format (e.g., a future int64 addition
// would carry `format: int64` from its struct tag or annotation) are left
// untouched.
func markIntegerFormats(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for _, ref := range doc.Components.Schemas {
		if ref == nil || ref.Value == nil {
			continue
		}
		setIntegerFormatRecursive(ref.Value)
	}
}

func setIntegerFormatRecursive(s *openapi3.Schema) {
	if s == nil {
		return
	}
	if s.Type != nil && s.Type.Is(openapi3.TypeInteger) && s.Format == "" {
		s.Format = "int32"
	}
	for _, prop := range s.Properties {
		if prop != nil && prop.Value != nil {
			setIntegerFormatRecursive(prop.Value)
		}
	}
	if s.Items != nil && s.Items.Value != nil {
		setIntegerFormatRecursive(s.Items.Value)
	}
	if s.AdditionalProperties.Schema != nil && s.AdditionalProperties.Schema.Value != nil {
		setIntegerFormatRecursive(s.AdditionalProperties.Schema.Value)
	}
}

// isSurrogateIDName reports whether `name` denotes a surrogate primary or
// foreign key — `id` exactly, or any field ending in `_id` (snake). Used
// to gate the int64 width promotion in markSurrogateIDsInt64 (TRA-719 /
// BB35 B7). The public spec uses snake-case throughout; camelCase
// path-param variants (`userId`, `inviteId`, `jobId`) live on the
// internal surface only, so a snake-only check covers the customer
// contract without false positives on unrelated camel names.
func isSurrogateIDName(name string) bool {
	return name == "id" || strings.HasSuffix(name, "_id")
}

// markSurrogateIDsInt64 promotes every integer surrogate PK/FK to
// `format: int64` and removes the `maximum: 2147483647` upper bound that
// markIntegerFormats / markQueryIntegerBounds installed by default
// (TRA-719 / BB35 B7).
//
// IDs across the public surface are randomly distributed across the int32
// namespace. Service-side ID generation stays within int32 for v1 — this
// is a wire-type declaration only — but pinning the spec width at int64
// pre-launch avoids the breaking-client change when the namespace
// eventually outgrows int32 (Java/Kotlin Integer→Long, OpenAPI codegen
// targets that distinguish int32/int64 in their wrappers).
//
// Walks both Components.Schemas properties and operation parameters; runs
// AFTER markIntegerFormats and markQueryIntegerBounds so it sees the
// post-default state and can confidently drop the int32 ceiling.
// Idempotent: re-running on already-promoted fields is a no-op.
func markSurrogateIDsInt64(doc *openapi3.T) {
	promote := func(s *openapi3.Schema) {
		if s == nil || s.Type == nil || !s.Type.Is(openapi3.TypeInteger) {
			return
		}
		s.Format = "int64"
		// Drop the int32 ceiling — the wire type already bounds the value
		// and a literal Max keeps strict generators rejecting valid int64
		// payloads. Min is left intact (1 for FKs, retains the no-zero-FK
		// invariant).
		if s.Max != nil && *s.Max == float64(2147483647) {
			s.Max = nil
		}
	}

	if doc.Components != nil && doc.Components.Schemas != nil {
		var walk func(s *openapi3.Schema)
		visited := map[*openapi3.Schema]bool{}
		walk = func(s *openapi3.Schema) {
			if s == nil || visited[s] {
				return
			}
			visited[s] = true
			for propName, propRef := range s.Properties {
				if propRef == nil || propRef.Value == nil {
					continue
				}
				if isSurrogateIDName(propName) {
					promote(propRef.Value)
					// Array-typed FK collections (rare today, but cheap to
					// cover) carry the integer in Items.
					if propRef.Value.Items != nil && propRef.Value.Items.Value != nil {
						promote(propRef.Value.Items.Value)
					}
				}
				walk(propRef.Value)
			}
			if s.Items != nil && s.Items.Value != nil {
				walk(s.Items.Value)
			}
			for _, r := range s.AllOf {
				if r != nil {
					walk(r.Value)
				}
			}
			for _, r := range s.OneOf {
				if r != nil {
					walk(r.Value)
				}
			}
			for _, r := range s.AnyOf {
				if r != nil {
					walk(r.Value)
				}
			}
		}
		for _, ref := range doc.Components.Schemas {
			if ref != nil {
				walk(ref.Value)
			}
		}
	}

	if doc.Paths != nil {
		walkParam := func(p *openapi3.Parameter) {
			if p == nil || p.Schema == nil || p.Schema.Value == nil {
				return
			}
			if !isSurrogateIDName(p.Name) {
				return
			}
			s := p.Schema.Value
			if s.Type != nil && s.Type.Is(openapi3.TypeArray) && s.Items != nil && s.Items.Value != nil {
				promote(s.Items.Value)
			} else {
				promote(s)
			}
		}
		for _, item := range doc.Paths.Map() {
			if item == nil {
				continue
			}
			for _, pRef := range item.Parameters {
				if pRef != nil {
					walkParam(pRef.Value)
				}
			}
			for _, op := range item.Operations() {
				if op == nil {
					continue
				}
				for _, pRef := range op.Parameters {
					if pRef != nil {
						walkParam(pRef.Value)
					}
				}
			}
		}
	}
}

// dateTimeExample / dateExample are the RFC 3339 stand-in values inserted
// onto schema properties whose `format` is `date-time` / `date` and which
// lack an `example`. Zalando's
// `must-use-standard-formats-for-date-and-time-properties-example` rule
// requires the example so codegen-generated docs and tests round-trip a
// recognizable payload (TRA-672).
const (
	// dateTimeExample carries the canonical outbound wire shape (RFC 3339
	// with three-digit millisecond fractional precision, UTC) per
	// TRA-717 / BB34 F3 rework so spec examples match what the server
	// actually emits.
	dateTimeExample = "2025-04-29T12:34:56.000Z"
	dateExample     = "2025-04-29"
)

// markDateTimeExamples walks every schema and seeds an `example:` on date
// and date-time string properties that don't already declare one. Covers
// both component schemas and inline path-parameter schemas (the
// /assets/{asset_id}/history `from`/`to` query params live on operations,
// not in components). Existing examples are preserved.
func markDateTimeExamples(doc *openapi3.T) {
	if doc.Components != nil && doc.Components.Schemas != nil {
		for _, ref := range doc.Components.Schemas {
			if ref == nil || ref.Value == nil {
				continue
			}
			setDateTimeExampleRecursive(ref.Value)
		}
	}
	if doc.Paths != nil {
		for _, item := range doc.Paths.Map() {
			if item == nil {
				continue
			}
			for _, p := range item.Parameters {
				if p != nil && p.Value != nil && p.Value.Schema != nil && p.Value.Schema.Value != nil {
					setDateTimeExampleRecursive(p.Value.Schema.Value)
				}
			}
			for _, op := range item.Operations() {
				if op == nil {
					continue
				}
				for _, p := range op.Parameters {
					if p != nil && p.Value != nil && p.Value.Schema != nil && p.Value.Schema.Value != nil {
						setDateTimeExampleRecursive(p.Value.Schema.Value)
					}
				}
			}
		}
	}
}

func setDateTimeExampleRecursive(s *openapi3.Schema) {
	if s == nil {
		return
	}
	if s.Type != nil && s.Type.Is(openapi3.TypeString) && s.Example == nil {
		switch s.Format {
		case "date-time":
			s.Example = dateTimeExample
		case "date":
			s.Example = dateExample
		}
	}
	for _, prop := range s.Properties {
		if prop != nil && prop.Value != nil {
			setDateTimeExampleRecursive(prop.Value)
		}
	}
	if s.Items != nil && s.Items.Value != nil {
		setDateTimeExampleRecursive(s.Items.Value)
	}
	if s.AdditionalProperties.Schema != nil && s.AdditionalProperties.Schema.Value != nil {
		setDateTimeExampleRecursive(s.AdditionalProperties.Schema.Value)
	}
}

// injectDefaultErrorResponse adds a `default` response entry to every
// operation that lacks one, pointing at the ErrorResponse envelope.
// Zalando's `must-specify-default-response` rule requires it so codegen
// libraries (typescript-fetch, axios-codegen) have a catch-all response
// type for unmodeled status codes — without it, the generator can't
// type-narrow on success vs error in a single Promise chain (TRA-672).
func injectDefaultErrorResponse(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	desc := "Unmodeled error. Generated clients should treat any response not otherwise enumerated as a structured ErrorResponse."
	build := func() *openapi3.ResponseRef {
		return &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &desc,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/errors.ErrorResponse",
						},
					},
				},
			},
		}
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Responses == nil {
				continue
			}
			if op.Responses.Default() != nil {
				continue
			}
			op.Responses.Set("default", build())
		}
	}
}

// nullableFields names schema/field pairs whose response payload may be
// null (or omitted when also non-required). swaggo doesn't emit
// nullable:true for Go *Type pointers, so we add it here. The list is
// curated from BB10/BB11 audit findings (TRA-517 AC2, AC9, AC11).
//
// Write-side asymmetry: every field declared nullable on a *PublicXxxView
// read schema MUST also be declared nullable on the matching write schemas
// (Update + CreateWithTags) where the field is writable. The PUT / POST
// handlers translate explicit JSON null into a column-clear via Clear*
// sentinels (TRA-614 / BB19 §S1). Read-only fields (id, *_at, tags) are
// not writable and don't appear on the write side.
var nullableFields = map[string][]string{
	// --- read views (response payloads) ---
	"asset.PublicAssetView":            {"location_id", "location_external_key", "description", "valid_to", "deleted_at"},
	"apikey.APIKeyListItem":            {"created_by", "created_by_key_id", "last_used_at"},
	"report.PublicAssetHistoryItem":    {"duration_seconds", "location_id", "location_external_key"},
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key", "asset_deleted_at"},
	"location.PublicLocationView":      {"parent_id", "parent_external_key", "description", "valid_to", "deleted_at"},

	// --- write schemas (request payloads) — TRA-614 / BB19 §S1 ---
	// Mirror the read-view asymmetry: anything nullable above is nullable
	// here too where writable. valid_to was already correct via the
	// ClearValidTo sentinel; description was added by TRA-614.
	//
	// TRA-705 (BB32 §C6): valid_from, is_active, and metadata are NOT
	// nullable on either Create or Update. Omission already serves "use
	// server default" on these fields; accepting null on Create only
	// (the previous TRA-675 "null-as-now" carve-out) muddied the
	// semantics and forced a documented Date Fields asymmetry note that
	// integrators tripped on. Both sides now reject `null` with
	// invalid_value.
	// TRA-681: location_external_key dropped from UpdateAssetRequest — the
	// asset-side natural-key form is read-only on PATCH.
	// TRA-719 / BB35 B2: parent_external_key restored to
	// UpdateLocationRequest now that PATCH dispatches it through the same
	// FK-resolution path as Create.
	"asset.UpdateAssetRequest":               {"description", "location_id", "valid_to"},
	"asset.CreateAssetRequest":               {"description", "location_id", "location_external_key", "valid_to"},
	"asset.CreateAssetWithTagsRequest":       {"description", "location_id", "location_external_key", "valid_to", "tags"},
	"location.UpdateLocationRequest":         {"description", "parent_id", "parent_external_key", "valid_to"},
	"location.CreateLocationRequest":         {"description", "parent_id", "parent_external_key", "valid_to"},
	"location.CreateLocationWithTagsRequest": {"description", "parent_id", "parent_external_key", "valid_to", "tags"},

	// shared.TagRequest.tag_type is optional and defaults to "rfid" server-side
	// when null or omitted (TRA-678). The spec marks it nullable to match the
	// runtime accept-null-as-default behavior; rejecting null here would force
	// every integrator that loops over an enum-or-null pattern to special-case
	// the field. Value is not nullable — that's the actual identifier.
	"shared.TagRequest": {"tag_type"},
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
	"shared.Tag": {"id", "tag_type", "value"},

	// asset
	"asset.PublicAssetView": {"id", "external_key", "name", "description", "location_id", "location_external_key", "metadata", "is_active", "valid_from", "valid_to", "created_at", "updated_at", "deleted_at", "tags"},

	// location
	"location.PublicLocationView": {"id", "external_key", "name", "description", "parent_id", "parent_external_key", "is_active", "valid_from", "valid_to", "created_at", "updated_at", "deleted_at", "tags"},

	// report
	"report.PublicCurrentLocationItem": {"asset_id", "asset_external_key", "location_id", "location_external_key", "asset_last_seen", "asset_deleted_at"},
	"report.PublicAssetHistoryItem":    {"event_observed_at", "location_id", "location_external_key", "duration_seconds"},

	// org (post namespace consolidation — TRA-602)
	// TRA-719 / BB35 A5: scopes + api_key_id are required so integrators
	// have a stable, typed surface for self-inspection.
	"org.OrgMeView": {"id", "name", "scopes", "api_key_id"},

	// asset envelopes (post namespace consolidation — TRA-602)
	"asset.AddTagResponse":      {"data"},
	"asset.CreateAssetResponse": {"data"},
	"asset.GetAssetResponse":    {"data"},
	"asset.ListAssetsResponse":  {"data", "limit", "offset", "total_count"},
	"asset.RenameAssetResponse": {"data", "descendant_count_affected"},
	"asset.UpdateAssetResponse": {"data"},

	// location envelopes (post namespace consolidation — TRA-602)
	"location.AddTagResponse":          {"data"},
	"location.CreateLocationResponse":  {"data"},
	"location.GetLocationResponse":     {"data"},
	"location.ListAncestorsResponse":   {"data", "limit", "offset", "total_count"},
	"location.ListChildrenResponse":    {"data", "limit", "offset", "total_count"},
	"location.ListDescendantsResponse": {"data", "limit", "offset", "total_count"},
	"location.ListLocationsResponse":   {"data", "limit", "offset", "total_count"},
	"location.RenameLocationResponse":  {"data", "descendant_count_affected"},
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
// `tags`, `external_key`, and `parent_external_key` (locations) are
// intentionally NOT marked readOnly. They have dedicated mutation paths —
// POST/DELETE /tags for tags and POST /rename for the natural keys — and
// the PATCH validator rejects each with a per-category code (tags →
// invalid_value, the natural-key forms → read_only) rather than silently
// dropping them (TRA-686 / BB29 F7+F8, history TRA-643 / TRA-664). Keeping
// these fields out of the readOnly list preserves the runtime signal —
// codegen tools won't strip them from request shapes, so an SDK that
// mistakenly sends them surfaces the failure.
//
// markReadOnlyFields errors if a configured schema or field is missing from
// the spec — keeps this map honest as struct fields rename or move.
var readOnlyFields = map[string][]string{
	"asset.PublicAssetView":            {"id", "created_at", "updated_at", "deleted_at"},
	"location.PublicLocationView":      {"id", "created_at", "updated_at", "deleted_at"},
	"org.OrgMeView":                    {"id"},
	"shared.Tag":                       {"id"},
	"report.PublicCurrentLocationItem": {"asset_deleted_at"},
}

// annotateErrorEnvelope adds a schema-level description to errors.ErrorResponse
// (and per-property descriptions on the title and detail fields) documenting
// the title vs detail contract from TRA-517 AC4. swaggo doesn't propagate
// godoc on a struct that wraps an anonymous nested struct, so the
// description has to be applied here.
//
// Also sets the required: list on the inner anonymous `error` object. The
// generator only writes a top-level required: for the wrapper struct, so the
// inner Type/Title/Status/Detail/Instance/RequestID fields — which the
// service always emits — never get marked required. Fields with json
// `,omitempty` (e.g. Fields []FieldError) stay optional. TRA-632 / A1.
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
	errProp.Value.Required = []string{"type", "title", "status", "detail", "instance", "request_id"}
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

// asset_last_seen is the wire-level rename of the legacy last_seen field
// (TRA-717 / BB34 F2). Listed explicitly because it does not match the
// `_at` suffix, while event_observed_at (the wire-level rename of the
// legacy timestamp field) does.
var timestampFieldNames = regexp.MustCompile(`^(valid_from|valid_to|asset_last_seen|.*_at)$`)

func isTimestampField(name string) bool {
	return timestampFieldNames.MatchString(name)
}

// publicTimeRefName matches the swag-emitted $ref for the
// shared.PublicTime wrapper type in either the original namespace
// (`shared.PublicTime`) or the post-consolidation form (`PublicTime`)
// per consolidateSchemaNamespaces (TRA-660 / BB25 C1).
var publicTimeRefName = regexp.MustCompile(`/(shared\.PublicTime|PublicTime)$`)

// inlinePublicTimeRefs rewrites every property that $refs the
// shared.PublicTime wrapper as an inline `type: string, format: date-time`
// schema, then drops the wrapper component itself. swag emits the
// wrapper as a structured component with an embedded `time.Time`
// pseudo-property because PublicTime is a struct that wraps time.Time;
// the inline form is what consumers expect for an outbound timestamp
// (TRA-717 / BB34 F3 rework).
//
// Runs after consolidateSchemaNamespaces so the post-consolidation
// component name is matched too. Existing `nullable` set on the
// referencing property by markNullableFields is preserved.
func inlinePublicTimeRefs(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for _, ref := range doc.Components.Schemas {
		rewritePublicTimeRefsInSchema(ref)
	}
	if doc.Paths != nil {
		for _, item := range doc.Paths.Map() {
			if item == nil {
				continue
			}
			for _, p := range item.Parameters {
				if p != nil && p.Value != nil && p.Value.Schema != nil {
					rewritePublicTimeRefsInSchema(p.Value.Schema)
				}
			}
			for _, op := range item.Operations() {
				if op == nil {
					continue
				}
				for _, p := range op.Parameters {
					if p != nil && p.Value != nil && p.Value.Schema != nil {
						rewritePublicTimeRefsInSchema(p.Value.Schema)
					}
				}
				if op.RequestBody != nil && op.RequestBody.Value != nil {
					for _, mt := range op.RequestBody.Value.Content {
						if mt != nil && mt.Schema != nil {
							rewritePublicTimeRefsInSchema(mt.Schema)
						}
					}
				}
				for _, resp := range op.Responses.Map() {
					if resp == nil || resp.Value == nil {
						continue
					}
					for _, mt := range resp.Value.Content {
						if mt != nil && mt.Schema != nil {
							rewritePublicTimeRefsInSchema(mt.Schema)
						}
					}
				}
			}
		}
	}
	for name := range doc.Components.Schemas {
		if publicTimeRefName.MatchString("/" + name) {
			delete(doc.Components.Schemas, name)
		}
	}
}

func rewritePublicTimeRefsInSchema(ref *openapi3.SchemaRef) {
	if ref == nil {
		return
	}
	if ref.Ref != "" && publicTimeRefName.MatchString(ref.Ref) {
		nullable := false
		if ref.Value != nil {
			nullable = ref.Value.Nullable
		}
		ref.Ref = ""
		ref.Value = &openapi3.Schema{
			Type:     &openapi3.Types{openapi3.TypeString},
			Format:   "date-time",
			Nullable: nullable,
		}
		return
	}
	if ref.Value == nil {
		return
	}
	for _, prop := range ref.Value.Properties {
		rewritePublicTimeRefsInSchema(prop)
	}
	if ref.Value.Items != nil {
		rewritePublicTimeRefsInSchema(ref.Value.Items)
	}
	if ref.Value.AdditionalProperties.Schema != nil {
		rewritePublicTimeRefsInSchema(ref.Value.AdditionalProperties.Schema)
	}
	for _, s := range ref.Value.AllOf {
		rewritePublicTimeRefsInSchema(s)
	}
	for _, s := range ref.Value.OneOf {
		rewritePublicTimeRefsInSchema(s)
	}
	for _, s := range ref.Value.AnyOf {
		rewritePublicTimeRefsInSchema(s)
	}
}

// flattenSortQueryToString rewrites every `sort` query parameter from
// `type: array, items: enum`-form to `type: string, pattern: csv-of-enums`
// (TRA-678). The wire format never changes — server still parses CSV the
// same way — but the schema shape stops triggering Schemathesis 4.x's
// conflicting interpretations of `?sort=`:
//
//   - positive_data_acceptance treats `?sort=` as a valid empty array, so
//     the server must return 2xx (rejecting it would be "API rejected
//     schema-compliant request").
//   - negative_data_rejection treats `?sort=` as type-mismatched against
//     `type: array` (empty string isn't an array), so the server must
//     return 4xx (accepting it would be "API accepted schema-violating
//     request").
//
// No server response can satisfy both. Pivoting the spec to `type: string`
// keeps the wire-level CSV behavior, eliminates the conflict, and
// preserves item-level enum validation via a pattern that enumerates the
// allowed (optionally `-`-prefixed) values.
func flattenSortQueryToString(doc *openapi3.T) {
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
			for _, pRef := range op.Parameters {
				if pRef == nil || pRef.Value == nil {
					continue
				}
				p := pRef.Value
				if p.In != "query" || p.Name != "sort" {
					continue
				}
				if p.Schema == nil || p.Schema.Value == nil {
					continue
				}
				s := p.Schema.Value
				if !s.Type.Is(openapi3.TypeArray) || s.Items == nil || s.Items.Value == nil {
					continue
				}
				items := s.Items.Value
				if len(items.Enum) == 0 {
					continue
				}
				enumStrs := make([]string, 0, len(items.Enum))
				for _, e := range items.Enum {
					if str, ok := e.(string); ok {
						enumStrs = append(enumStrs, str)
					}
				}
				if len(enumStrs) == 0 {
					continue
				}
				// Build alternation, preserving the existing leading-`-`
				// shape (e.g. `external_key` vs `-external_key`). Allow
				// empty string + CSV of any-of-enums.
				alt := strings.Join(enumStrs, "|")
				pattern := fmt.Sprintf("^(|(?:%s)(?:,(?:%s))*)$", alt, alt)
				s.Type = &openapi3.Types{openapi3.TypeString}
				s.Items = nil
				s.Pattern = pattern
				s.Default = ""
			}
		}
	}
}

// normalizeArrayQueryParams walks every operation parameter in doc.Paths and,
// for each in:query parameter whose schema is type:array, sets Style to "form"
// and Explode to false. This corrects the OpenAPI 3 default (style:form,
// explode:true — i.e. ?sort=a&sort=b) to match the actual CSV wire format the
// server expects (?sort=a,-b). kin-openapi's openapi2conv.ToV3 drops Swagger
// 2.0's collectionFormat, leaving the default that tells codegen to send
// multi-value instead of comma-separated.
//
// Also sets AllowEmptyValue=true on array-typed query parameters: the
// canonical "no value" CSV encoding is `?sort=` (empty value), and OpenAPI
// 3 otherwise treats empty as missing — which Schemathesis flags as
// "API accepted schema-violating request" against the items enum (TRA-678).
// The flag is deprecated in 3.1 but widely honored by 3.0 generators and
// Schemathesis 4.x in particular.
//
// The pass is idempotent and does not clobber Style/Explode/AllowEmptyValue
// that are already set to a non-default (non-zero) value.
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
				// Default to empty array so `?sort=` (empty CSV value)
				// resolves to [] — both checks in Schemathesis 4.x agree
				// that [] satisfies the array type with no item-enum
				// constraint. Without a default, the empty value is
				// ambiguous: positive_data_acceptance treats it as []
				// (valid) but negative_data_rejection treats it as [""]
				// (invalid), and the server cannot satisfy both. TRA-678.
				if p.Schema.Value.Default == nil {
					p.Schema.Value.Default = []any{}
				}
			}
		}
	}
}

// stripBearerScopeArrays strips non-empty scope arrays from operation-level
// SecurityRequirements where the underlying scheme is http or apiKey.
// OpenAPI 3.0 §4.8.30 only permits scope arrays for oauth2 and openIdConnect
// schemes; swaggo's `@Security BearerAuth[assets:read]` syntax produces invalid
// arrays under http-bearer. To preserve the scope information:
//
//  1. captured scopes are injected into the operation's description as a
//     "**Required scope:** `<scope>`" markdown line (human readers).
//  2. captured scopes are emitted as `x-required-scopes: [<scope>, ...]`
//     on the operation (machine-readable; TRA-685 F4 / TRA-712 BB33 F7).
//     Operations gated by bearer auth but without specific scopes get an
//     empty array, signalling "any authenticated key works" to codegen
//     ingestors trying to mint minimal-scope keys — versus absent, which
//     would be ambiguous about whether scopes were ever considered.
//     Standard codegen won't auto-surface this, but scope-aware partners
//     can read it.
//
// The pass is idempotent — repeated runs do not double-prepend the marker
// nor double-write the extension.
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
			scopes, hasBearer := stripScopesFromRequirements(op.Security, doc.Components.SecuritySchemes)
			if !hasBearer {
				continue
			}
			if len(scopes) > 0 {
				op.Description = injectScopeMarker(op.Description, scopes)
			}
			if op.Extensions == nil {
				op.Extensions = map[string]any{}
			}
			op.Extensions["x-required-scopes"] = append([]string{}, scopes...)
		}
	}
}

// stripScopesFromRequirements walks every SecurityRequirement in the slice,
// finds entries whose scheme is http or apiKey, captures and zeroes their
// scope arrays. Returns the captured scope names in declaration order with
// duplicates removed, plus a flag indicating whether any bearer-like
// requirement was present at all (regardless of scopes) so callers can
// emit an empty x-required-scopes for scopeless bearer ops.
func stripScopesFromRequirements(reqs *openapi3.SecurityRequirements, schemes openapi3.SecuritySchemes) ([]string, bool) {
	if reqs == nil {
		return nil, false
	}
	var captured []string
	seen := map[string]bool{}
	hasBearer := false
	for _, req := range *reqs {
		for name, arr := range req {
			if !isBearerLikeScheme(schemes, name) {
				continue
			}
			hasBearer = true
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
	return captured, hasBearer
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

// mergePatchPaths is the set of operations that accept RFC 7396 JSON Merge
// Patch (TRA-663 / BB26). Their request bodies are rewritten from
// `application/json` (what swag emits from `@Accept json`) to
// `application/merge-patch+json` so generated SDKs signal merge semantics
// to integrators and the spec verb matches the runtime semantics.
var mergePatchPaths = []string{
	"/api/v1/assets/{asset_id}",
	"/api/v1/locations/{location_id}",
}

// rewriteMergePatchContentType changes the request-body Content-Type from
// `application/json` to `application/merge-patch+json` on the PATCH
// operations in mergePatchPaths. Empty body and 200/400/etc. responses are
// untouched — only the request body media type changes.
func rewriteMergePatchContentType(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	for _, p := range mergePatchPaths {
		item := doc.Paths.Find(p)
		if item == nil || item.Patch == nil || item.Patch.RequestBody == nil {
			continue
		}
		body := item.Patch.RequestBody.Value
		if body == nil || body.Content == nil {
			continue
		}
		mt, ok := body.Content["application/json"]
		if !ok {
			continue
		}
		body.Content = openapi3.Content{"application/merge-patch+json": mt}
	}
}

// readOnlyTagsDescription documents the read/write split between the tag
// arrays embedded in resource views (read-only) and the /tags subresource
// (write). Without this note, the natural round-trip (GET → modify tags
// → PATCH) silently drops tag edits — see TRA-663 BB26 §S3.
const readOnlyTagsDescription = "Tags currently attached to this resource. Read-only on PATCH; mutate via POST /{resource}/{id}/tags and DELETE /{resource}/{id}/tags/{tag_id}."

// readOnlyTagsSchemas names the schemas whose `tags` property carries the
// read/write split description. Pre-rename names — runs before
// renamePublicSpec consolidates them to AssetView / LocationView.
var readOnlyTagsSchemas = []string{
	"asset.PublicAssetView",
	"location.PublicLocationView",
}

// annotateReadOnlyTags sets the description on the `tags` array property of
// the resource views in readOnlyTagsSchemas. Silently skipped for any
// missing schema — runs late, so consolidateSchemaNamespaces should already
// have folded the dotted names; if a name is absent the postprocess
// pipeline has changed in a way that warrants a separate look.
func annotateReadOnlyTags(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for _, schemaName := range readOnlyTagsSchemas {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			continue
		}
		prop, ok := ref.Value.Properties["tags"]
		if !ok || prop == nil || prop.Value == nil {
			continue
		}
		prop.Value.Description = readOnlyTagsDescription
	}
}

// tagSchemaDescription names the polymorphism explicitly so an AI ingestor
// (or human) reading the spec without domain context sees from the noun
// alone that Tag covers multiple identifier kinds. TRA-666 BB26 C1: the
// resource and discriminator names stay ("tag" is the canonical TrakRF
// noun for RFID/BLE/barcode identifiers; tag_type earns its prefix on
// flat surfaces like DB columns, query params, and logs), so the fix
// lives in the description where generated client class docstrings carry
// it forward.
// TRA-719 / BB35 B6: surface the discriminator semantics, the open-set
// versioning policy, and the codegen artifact in the schema-level
// description so generated SDK class docstrings carry the contract to
// integrators reading them directly.
const tagSchemaDescription = "Polymorphic identifier attached to an asset or location, discriminated by `tag_type` into one of three variants: " +
	"`RfidTag` (RFID transponder, `tag_type: rfid`), `BleTag` (Bluetooth Low Energy beacon, `tag_type: ble`), or `BarcodeTag` (1D/2D barcode, `tag_type: barcode`). " +
	"The `/assets/{asset_id}/tags` and `/locations/{location_id}/tags` subresources accept and return all three variants; " +
	"`tag_type` together with `value` form the natural key (a tag value is unique within its kind, not across kinds). " +
	"\n\n" +
	"`tag_type` is an **open** enumeration per the versioning policy — new variants may be added in additive minor revisions without a `/api/v2` cut. " +
	"Integrators should treat unknown `tag_type` values as forward-compatible: pass the row through untouched rather than rejecting it. " +
	"\n\n" +
	"**Codegen note:** some strict-typed generators (e.g. datamodel-codegen for Pydantic) materialize the single-value `tag_type` constants on each variant as separate enum classes — `TagType`, `TagType2`, `TagType4` or similar. " +
	"That is a generator artifact and not part of the contract; the wire shape is a single discriminated union with three variants today. Treat the generated classes as implementation detail."

const tagTypeFieldDescription = "Discriminator for the polymorphic Tag resource. " +
	"`rfid` denotes an RFID transponder, `ble` denotes a Bluetooth Low Energy beacon, " +
	"and `barcode` denotes a 1D/2D barcode. Together with `value`, this forms the natural key — " +
	"the same `value` may exist under different `tag_type`s without conflict."

// annotateTagPolymorphism sets the schema-level description on shared.Tag /
// shared.TagRequest and the field-level description on their tag_type
// discriminator. Pre-rename names — runs before renamePublicSpec
// consolidates them to Tag / TagRequest. Silently skipped for any missing
// schema, matching the pattern in annotateReadOnlyTags.
func annotateTagPolymorphism(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	for _, schemaName := range []string{"shared.Tag", "shared.TagRequest"} {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			continue
		}
		ref.Value.Description = tagSchemaDescription
		if prop, ok := ref.Value.Properties["tag_type"]; ok && prop != nil && prop.Value != nil {
			prop.Value.Description = tagTypeFieldDescription
		}
	}
}
