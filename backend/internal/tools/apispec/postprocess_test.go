package main

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostprocess_InjectsMethodNotAllowedResponse covers TRA-588: the
// public spec must declare a reusable MethodNotAllowed response component
// with an Allow header, ready for operations to $ref. Internal spec gets
// the same treatment.
func TestPostprocess_InjectsMethodNotAllowedResponse(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	require.NoError(t, postprocessPublic(doc))

	require.NotNil(t, doc.Components)
	require.NotNil(t, doc.Components.Responses)

	respRef := doc.Components.Responses["MethodNotAllowed"]
	require.NotNil(t, respRef, "components.responses.MethodNotAllowed must be present")
	require.NotNil(t, respRef.Value)

	require.NotNil(t, respRef.Value.Description)
	assert.Equal(t, "Method not allowed", *respRef.Value.Description)

	allow := respRef.Value.Headers["Allow"]
	require.NotNil(t, allow, "Allow header must be declared on the MethodNotAllowed response")
	require.NotNil(t, allow.Value)
	require.NotNil(t, allow.Value.Schema)
	require.NotNil(t, allow.Value.Schema.Value)
	assert.True(t, allow.Value.Schema.Value.Type.Is(openapi3.TypeString),
		"Allow header schema must be type:string")
	assert.Contains(t, allow.Value.Description, "RFC 7231",
		"Allow header description should cite the relevant RFC clause")

	media := respRef.Value.Content["application/json"]
	require.NotNil(t, media, "MethodNotAllowed must declare application/json content")
	require.NotNil(t, media.Schema)
	assert.Equal(t, "#/components/schemas/errors.ErrorResponse", media.Schema.Ref,
		"content schema must reference the canonical error envelope")

	// TRA-723 / BB36 F3: the service emits the standard rate-limit and
	// request-id headers on every 405 response. The shared component must
	// declare them so every operation that $refs it inherits the full
	// header set — otherwise codegen targets miss four headers on 21 of
	// 22 operations (the only inline 405 was getCurrentOrg's, which has
	// since been removed in favor of the shared $ref).
	expectedHeaderRefs := map[string]string{
		"X-RateLimit-Limit":     "#/components/headers/XRateLimitLimit",
		"X-RateLimit-Remaining": "#/components/headers/XRateLimitRemaining",
		"X-RateLimit-Reset":     "#/components/headers/XRateLimitReset",
		"X-Request-Id":          "#/components/headers/XRequestId",
	}
	for name, ref := range expectedHeaderRefs {
		h := respRef.Value.Headers[name]
		require.NotNil(t, h, "MethodNotAllowed must declare %s header", name)
		assert.Equal(t, ref, h.Ref, "%s must $ref %s", name, ref)
	}
}

// TestPostprocess_InjectMethodNotAllowed_Idempotent verifies running the
// postprocess twice does not duplicate or replace the component.
func TestPostprocess_InjectMethodNotAllowed_Idempotent(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	require.NoError(t, postprocessPublic(doc))
	first := doc.Components.Responses["MethodNotAllowed"]
	require.NotNil(t, first)

	require.NoError(t, postprocessPublic(doc))
	second := doc.Components.Responses["MethodNotAllowed"]
	assert.Same(t, first, second, "second pass must not replace the existing response")
}

// TestPostprocess_AttachesMethodNotAllowedToEveryOperation covers TRA-646
// BB22 S1: codegens that pre-allocate response arms can only model 405 if
// every operation declares it. The MethodNotAllowed component must be
// referenced from every operation that does not already declare 405.
func TestPostprocess_AttachesMethodNotAllowedToEveryOperation(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	require.NoError(t, postprocessPublic(doc))

	require.NotNil(t, doc.Paths)
	opCount := 0
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			opCount++
			require.NotNil(t, op.Responses, "%s %s missing responses", method, path)
			r405 := op.Responses.Value("405")
			require.NotNil(t, r405, "%s %s must declare 405", method, path)
			assert.Equal(t, "#/components/responses/MethodNotAllowed", r405.Ref,
				"%s %s 405 must $ref MethodNotAllowed", method, path)
		}
	}
	require.Greater(t, opCount, 0, "fixture must have at least one operation")
}

// TestPostprocess_AttachesMethodNotAllowed_PreservesExisting verifies an
// operation that already declares 405 is left alone.
func TestPostprocess_AttachesMethodNotAllowed_PreservesExisting(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// Pre-seed an inline 405 on the first operation we find.
	var seeded *openapi3.ResponseRef
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Responses == nil {
				continue
			}
			desc := "operation-specific"
			seeded = &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc}}
			op.Responses.Set("405", seeded)
			break
		}
		if seeded != nil {
			break
		}
	}
	require.NotNil(t, seeded, "fixture must let us seed an operation-level 405")

	require.NoError(t, postprocessPublic(doc))

	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Responses == nil {
				continue
			}
			r405 := op.Responses.Value("405")
			if r405 == seeded {
				return
			}
		}
	}
	t.Fatalf("operation-level 405 was overwritten by the bulk-attach pass")
}

// TestPostprocess_InjectsDeprecationComponents covers TRA-646 BB22 S3.
// The Deprecation/Sunset header components and the Gone (410) response
// must exist as reusable components so codegens can model RFC 8594
// deprecation+sunset before the first endpoint sunset ships.
func TestPostprocess_InjectsDeprecationComponents(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	require.NoError(t, postprocessPublic(doc))

	require.NotNil(t, doc.Components)
	require.NotNil(t, doc.Components.Headers)
	require.NotNil(t, doc.Components.Responses)

	dep := doc.Components.Headers["Deprecation"]
	require.NotNil(t, dep, "components.headers.Deprecation must be present")
	require.NotNil(t, dep.Value)
	assert.Contains(t, dep.Value.Description, "RFC 8594")

	sun := doc.Components.Headers["Sunset"]
	require.NotNil(t, sun, "components.headers.Sunset must be present")
	require.NotNil(t, sun.Value)
	assert.Contains(t, sun.Value.Description, "RFC 8594")

	gone := doc.Components.Responses["Gone"]
	require.NotNil(t, gone, "components.responses.Gone must be present")
	require.NotNil(t, gone.Value)
	require.NotNil(t, gone.Value.Description)
	assert.Contains(t, *gone.Value.Description, "Sunset")
	media := gone.Value.Content["application/json"]
	require.NotNil(t, media)
	assert.Equal(t, "#/components/schemas/errors.ErrorResponse", media.Schema.Ref)
}

// TestPostprocess_StripsResponseSchemasAdditive covers TRA-668 BB27 S8 /
// TRA-672: the explicit `additionalProperties: true` swag emits on every
// response object caused some generators to emit wrapper classes instead
// of clean Record<string,unknown> types. The strip pass removes the
// literal `:true` so the schema falls back to OpenAPI 3.0's permissive
// default, preserving additive evolution without the codegen drag.
func TestPostprocess_StripsResponseSchemasAdditive(t *testing.T) {
	tr := true
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:                 &openapi3.Types{openapi3.TypeObject},
			AdditionalProperties: openapi3.AdditionalProperties{Has: &tr},
		}},
		"location.PublicLocationView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:                 &openapi3.Types{openapi3.TypeObject},
			AdditionalProperties: openapi3.AdditionalProperties{Has: &tr},
		}},
	})

	err := stripResponseSchemasAdditive(doc, []string{"asset.PublicAssetView", "location.PublicLocationView"})
	require.NoError(t, err)

	for _, name := range []string{"asset.PublicAssetView", "location.PublicLocationView"} {
		ref := doc.Components.Schemas[name]
		assert.Nil(t, ref.Value.AdditionalProperties.Has, "%s must have additionalProperties:true stripped", name)
		assert.Nil(t, ref.Value.AdditionalProperties.Schema, "%s structured form must remain unset", name)
	}
}

// TestPostprocess_StripsResponseSchemasAdditive_PreservesStructured verifies
// the strip pass does not clobber a schema that already declares a structured
// additionalProperties (e.g. errors.FieldError.params, which carries
// `additionalProperties: {}` from swag).
func TestPostprocess_StripsResponseSchemasAdditive_PreservesStructured(t *testing.T) {
	preset := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
	}}
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": preset,
	})
	// Already has an additionalProperties schema set.
	preset.Value.AdditionalProperties = openapi3.AdditionalProperties{
		Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
	}

	require.NoError(t, stripResponseSchemasAdditive(doc, []string{"asset.PublicAssetView"}))
	assert.NotNil(t, preset.Value.AdditionalProperties.Schema, "structured additionalProperties must survive")
	assert.Nil(t, preset.Value.AdditionalProperties.Has, "Has must remain unset when Schema is preserved")
}

// TestPostprocess_StripsResponseSchemasAdditive_MissingSchemaErrors locks in
// the safety guard: a stale entry in publicResponseSchemas breaks the
// build instead of going silently unenforced.
func TestPostprocess_StripsResponseSchemasAdditive_MissingSchemaErrors(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{})
	err := stripResponseSchemasAdditive(doc, []string{"asset.GhostView"})
	require.Error(t, err, "missing schema must surface as an error")
	assert.Contains(t, err.Error(), "asset.GhostView")
}

func TestPostprocess_RewritesSessionAuthToHTTPBearer(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// minimal fixture carries only BearerAuth; synthesize SessionAuth the way
	// swaggo emits it so we can verify the rewrite.
	doc.Components.SecuritySchemes["SessionAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type: "apiKey", In: "header", Name: "Authorization",
			Description: "Session JWT for internal endpoints.",
		},
	}
	postprocessInternal(doc)

	scheme := doc.Components.SecuritySchemes["SessionAuth"]
	require.NotNil(t, scheme, "SessionAuth scheme must be present")
	require.NotNil(t, scheme.Value)

	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
	assert.Contains(t, scheme.Value.Description, "Session JWT")
}

// TestPostprocess_RewritesBearerAuthToHTTPBearer reverses the TRA-480 §3.3
// decision per TRA-517 AC1. The token is consumed as a Bearer JWT; declaring
// the scheme as type=apiKey makes generated SDKs send the raw value in an
// Authorization header without the "Bearer " prefix, which the platform
// rejects. type=http/scheme=bearer/bearerFormat=JWT is the correct shape.
// TRA-616 renamed the public scheme APIKey → BearerAuth so class-emitting
// codegen tools produce a `Configuration.accessToken`-shaped client.
func TestPostprocess_RewritesBearerAuthToHTTPBearer(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	scheme := doc.Components.SecuritySchemes["BearerAuth"]
	require.NotNil(t, scheme)
	require.NotNil(t, scheme.Value)
	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
	assert.NotEmpty(t, scheme.Value.Description, "description must be preserved across the rewrite")
}

// TestPostprocess_SetsPublicInfoAndServers locks in TRA-517 AC12: servers
// are listed Preview-first so generated clients default to preview during
// integration testing, and each entry's description warns that an API key
// scoped to one environment will fail against the other.
func TestPostprocess_SetsPublicInfoAndServers(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	assert.Equal(t, "TrakRF API", doc.Info.Title)
	assert.Equal(t, "1.0.0", doc.Info.Version,
		"info.version must be semver per Zalando must-use-semantic-versioning (TRA-672)")
	require.NotNil(t, doc.Info.Contact, "info.contact must be present per Zalando must-have-info-contact-url (TRA-672)")
	assert.Equal(t, "https://app.trakrf.id/api", doc.Info.Contact.URL)
	assert.Equal(t, "support@trakrf.id", doc.Info.Contact.Email)
	require.Len(t, doc.Servers, 2)

	assert.Equal(t, "https://app.preview.trakrf.id", doc.Servers[0].URL,
		"preview must be the first server so codegen defaults to it")
	assert.Contains(t, doc.Servers[0].Description, "Preview")
	assert.Contains(t, doc.Servers[0].Description, "fail",
		"preview description must warn that production keys won't authenticate here")

	assert.Equal(t, "https://app.trakrf.id", doc.Servers[1].URL,
		"production server must be app.trakrf.id — the marketing site at trakrf.id does not serve /api/v1/*")
	assert.Contains(t, doc.Servers[1].Description, "Production")
	assert.Contains(t, doc.Servers[1].Description, "fail",
		"production description must warn that preview keys won't authenticate here")
}

func TestPostprocess_SetsInternalInfoAndServers(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessInternal(doc)

	assert.Equal(t, "TrakRF Internal API — not for customer use", doc.Info.Title)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "http://localhost:8080", doc.Servers[0].URL)
	assert.Equal(t, "Local development", doc.Servers[0].Description)
}

func TestPostprocess_NormalizesMetadataEmptySchema(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"Asset": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"name":     &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
				"metadata": &openapi3.SchemaRef{Value: &openapi3.Schema{}},
			},
		}},
	})
	postprocessPublic(doc)

	meta := doc.Components.Schemas["Asset"].Value.Properties["metadata"].Value
	assert.True(t, meta.Type.Is(openapi3.TypeObject))
	require.NotNil(t, meta.AdditionalProperties.Has, "additionalProperties must be explicit bool")
	assert.True(t, *meta.AdditionalProperties.Has)
}

func TestPostprocess_LeavesNonEmptyMetadataAlone(t *testing.T) {
	withEmptyRequiredFields(t)
	structured := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: openapi3.Schemas{
			"owner": &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		},
	}}
	doc := docWithSchemas(openapi3.Schemas{
		"Asset": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:       &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{"metadata": structured},
		}},
	})
	postprocessPublic(doc)

	meta := doc.Components.Schemas["Asset"].Value.Properties["metadata"].Value
	assert.True(t, meta.Type.Is(openapi3.TypeObject))
	assert.Contains(t, meta.Properties, "owner", "pre-declared metadata properties must survive post-processing")
}

func TestPostprocess_ConvertsExtensibleEnumStringToBool(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"Identifier": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"type": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeString},
					Extensions: map[string]any{"x-extensible-enum": "true"},
				}},
			},
		}},
	})
	postprocessPublic(doc)

	typ := doc.Components.Schemas["Identifier"].Value.Properties["type"].Value
	got, ok := typ.Extensions["x-extensible-enum"]
	require.True(t, ok)
	assert.Equal(t, true, got, "x-extensible-enum must be a real bool so consumers don't parse the string")
}

// TestPostprocess_AnnotatesErrorEnvelope locks in TRA-517 AC4: the
// errors.ErrorResponse schema must carry the title/detail contract in its
// schema description, and the title/detail properties must each describe
// their semantics. swaggo doesn't propagate godoc through an outer struct
// that wraps an anonymous nested struct, so this is applied here.
//
// Also locks in TRA-632 / A1: the inner anonymous `error` object's required
// list must include every field the service always emits (type, title,
// status, detail, instance, request_id). Fields with json `,omitempty`
// (fields[]) stay optional.
func TestPostprocess_AnnotatesErrorEnvelope(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"errors.ErrorResponse": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"error": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"type":       stringProp(""),
						"title":      stringProp(""),
						"status":     &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
						"detail":     stringProp(""),
						"instance":   stringProp(""),
						"request_id": stringProp(""),
					},
				}},
			},
		}},
	})
	postprocessPublic(doc)

	envelope := doc.Components.Schemas["errors.ErrorResponse"].Value
	require.NotEmpty(t, envelope.Description, "envelope schema must carry the contract description")
	assert.Contains(t, envelope.Description, "title")
	assert.Contains(t, envelope.Description, "detail")
	assert.Contains(t, envelope.Description, "stable", "description must say title is stable")

	errInner := envelope.Properties["error"].Value
	errProps := errInner.Properties
	assert.NotEmpty(t, errProps["title"].Value.Description, "title field needs its own description")
	assert.NotEmpty(t, errProps["detail"].Value.Description, "detail field needs its own description")

	assert.ElementsMatch(t,
		[]string{"type", "title", "status", "detail", "instance", "request_id"},
		errInner.Required,
		"inner error object must mark every always-emitted field as required",
	)
}

// TestPostprocess_AnnotatesTagPolymorphism locks in TRA-666 BB26 C1 and
// TRA-714 BB33 C1: the Tag and TagRequest union schemas carry the
// polymorphism description; splitTagPolymorphism then rewrites each into
// a oneOf over RfidTag/BleTag/BarcodeTag subtypes, and each subtype's
// tag_type carries the discriminator-role field description. Names are
// pre-rename (shared.*) because annotation and split both run before
// renamePublicSpec.
func TestPostprocess_AnnotatesTagPolymorphism(t *testing.T) {
	withEmptyRequiredFields(t)
	tagTypeProp := func() *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeString},
			Enum: []any{"rfid", "ble", "barcode"},
		}}
	}
	doc := docWithSchemas(openapi3.Schemas{
		"shared.Tag": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"id":       &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"tag_type": tagTypeProp(),
				"value":    stringProp(""),
			},
		}},
		"shared.TagRequest": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"tag_type": tagTypeProp(),
				"value":    stringProp(""),
			},
		}},
	})

	require.NoError(t, postprocessPublic(doc))

	// withEmptyRequiredFields disables the public-spec rename pass, so the
	// schemas stay under their dotted pre-rename keys here. The rename is
	// covered separately in rename_public_test.go.
	for _, name := range []string{"shared.Tag", "shared.TagRequest"} {
		ref := doc.Components.Schemas[name]
		require.NotNil(t, ref, "%s schema must survive postprocess", name)
		require.NotNil(t, ref.Value, "%s schema value must not be nil", name)
		assert.NotEmpty(t, ref.Value.Description, "%s must carry a schema-level description", name)
		assert.Contains(t, ref.Value.Description, "olymorphic", "%s description must name the polymorphism", name)
		assert.Contains(t, ref.Value.Description, "tag_type", "%s description must name the discriminator", name)
		assert.Contains(t, ref.Value.Description, "rfid", "%s description must list rfid", name)
		assert.Contains(t, ref.Value.Description, "ble", "%s description must list ble", name)
		assert.Contains(t, ref.Value.Description, "barcode", "%s description must list barcode", name)

		require.Len(t, ref.Value.OneOf, 3, "%s must be split into a oneOf union", name)
		require.NotNil(t, ref.Value.Discriminator, "%s must carry a discriminator", name)
		assert.Equal(t, "tag_type", ref.Value.Discriminator.PropertyName,
			"%s discriminator property must be tag_type", name)
	}

	for _, subName := range []string{"shared.RfidTag", "shared.BleTag", "shared.BarcodeTag", "shared.RfidTagRequest", "shared.BleTagRequest", "shared.BarcodeTagRequest"} {
		sub := doc.Components.Schemas[subName]
		require.NotNil(t, sub, "subtype %s must be present after the split", subName)
		require.NotNil(t, sub.Value, "subtype %s value must not be nil", subName)
		tt := sub.Value.Properties["tag_type"]
		require.NotNil(t, tt, "%s.tag_type must be present", subName)
		require.NotNil(t, tt.Value, "%s.tag_type value must not be nil", subName)
		assert.NotEmpty(t, tt.Value.Description, "%s.tag_type must carry a field-level description", subName)
		assert.Contains(t, tt.Value.Description, "iscriminator",
			"%s.tag_type description must call out its discriminator role", subName)
	}
}

// TestPostprocess_MarksNullableFields locks in TRA-517 AC2/AC9/AC11. Go
// pointer types (*string, *time.Time, *int) marshal to null but swaggo
// doesn't carry that into the OpenAPI 3.0 schema. This is the post-process
// step that adds nullable:true on the curated allowlist of fields.
func TestPostprocess_MarksNullableFields(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"location_id":           &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"location_external_key": stringProp(""),
				"name":                  stringProp(""), // not on the allowlist
			},
		}},
		"apikey.APIKeyListItem": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"created_by_key_id": &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"last_used_at":      stringProp("date-time"),
			},
		}},
		"report.PublicAssetHistoryItem": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"duration_seconds":      &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"location_id":           &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"location_external_key": stringProp(""),
				"event_observed_at":     stringProp("date-time"), // not on the allowlist
			},
		}},
		"report.PublicCurrentLocationItem": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"asset_id":              &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"asset_external_key":    stringProp(""),
				"location_id":           &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"location_external_key": stringProp(""),
				"asset_last_seen":       stringProp("date-time"), // not on the allowlist
			},
		}},
	})
	postprocessPublic(doc)

	cases := []struct {
		schema string
		field  string
	}{
		{"asset.PublicAssetView", "location_id"},
		{"asset.PublicAssetView", "location_external_key"},
		{"apikey.APIKeyListItem", "created_by_key_id"},
		{"apikey.APIKeyListItem", "last_used_at"},
		{"report.PublicAssetHistoryItem", "duration_seconds"},
		{"report.PublicAssetHistoryItem", "location_id"},
		{"report.PublicAssetHistoryItem", "location_external_key"},
		{"report.PublicCurrentLocationItem", "asset_id"},
		{"report.PublicCurrentLocationItem", "asset_external_key"},
		{"report.PublicCurrentLocationItem", "location_id"},
		{"report.PublicCurrentLocationItem", "location_external_key"},
	}
	for _, tc := range cases {
		prop := doc.Components.Schemas[tc.schema].Value.Properties[tc.field]
		assert.True(t, prop.Value.Nullable, "%s.%s must be marked nullable", tc.schema, tc.field)
	}

	// fields NOT on the allowlist must remain non-nullable
	assert.False(t,
		doc.Components.Schemas["asset.PublicAssetView"].Value.Properties["name"].Value.Nullable,
		"name is not nullable")
	assert.False(t,
		doc.Components.Schemas["report.PublicAssetHistoryItem"].Value.Properties["event_observed_at"].Value.Nullable,
		"event_observed_at is not nullable")
	assert.False(t,
		doc.Components.Schemas["report.PublicCurrentLocationItem"].Value.Properties["asset_last_seen"].Value.Nullable,
		"asset_last_seen is not nullable")
}

func TestPostprocess_AddsDateTimeFormatToTimestampFields(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"Asset": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"valid_from":        stringProp(""),
				"valid_to":          stringProp(""),
				"created_at":        stringProp(""),
				"updated_at":        stringProp(""),
				"expires_at":        stringProp(""),
				"last_used_at":      stringProp(""),
				"event_observed_at": stringProp(""),
				"asset_last_seen":   stringProp(""),
				"name":              stringProp(""),     // not a timestamp — must be left alone
				"birth_date":        stringProp("date"), // non-matching name + existing format
			},
		}},
	})
	postprocessPublic(doc)

	props := doc.Components.Schemas["Asset"].Value.Properties
	for _, name := range []string{"valid_from", "valid_to", "created_at", "updated_at", "expires_at", "last_used_at", "event_observed_at", "asset_last_seen"} {
		assert.Equal(t, "date-time", props[name].Value.Format, "%s should gain format: date-time", name)
	}
	assert.Equal(t, "", props["name"].Value.Format, "non-timestamp fields must stay formatless")
	assert.Equal(t, "date", props["birth_date"].Value.Format, "pre-existing format must not be overwritten")
}

// TRA-698 (BB31 §1.4): `pattern` on `format: date-time` breaks
// openapi-generator-cli's Python template — its `@field_validator` runs
// AFTER Pydantic parses the string into a `datetime`, then stringifies
// that datetime (space separator, not `T`) before regex-matching, so
// every read path that returns a timestamp throws a ValidationError. The
// spec-level pattern is redundant on `format: date-time` (RFC 3339 is
// already implied) and must not be emitted by postprocess. Covers both
// component schemas and inline path/query parameters.
func TestPostprocess_DoesNotSetPatternOnDateTimeFields(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"AssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"created_at": stringProp("date-time"),
				"updated_at": stringProp("date-time"),
				"name":       stringProp(""), // not a timestamp — must be left alone
			},
		}},
		"AssetHistoryItem": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"event_observed_at": stringProp("date-time"),
			},
		}},
	})
	// Inline query parameter with format: date-time — covers the
	// /assets/{asset_id}/history `from`/`to` shape.
	doc.Paths = openapi3.NewPaths()
	doc.Paths.Set("/api/v1/assets/{asset_id}/history", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				{Value: &openapi3.Parameter{
					Name: "from",
					In:   "query",
					Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
						Type:   &openapi3.Types{openapi3.TypeString},
						Format: "date-time",
					}},
				}},
			},
		},
	})

	require.NoError(t, postprocessPublic(doc))

	assert.Equal(t, "", doc.Components.Schemas["AssetView"].Value.Properties["created_at"].Value.Pattern,
		"AssetView.created_at must not declare a pattern (BB31 §1.4)")
	assert.Equal(t, "", doc.Components.Schemas["AssetView"].Value.Properties["updated_at"].Value.Pattern,
		"AssetView.updated_at must not declare a pattern (BB31 §1.4)")
	assert.Equal(t, "", doc.Components.Schemas["AssetHistoryItem"].Value.Properties["event_observed_at"].Value.Pattern,
		"AssetHistoryItem.event_observed_at must not declare a pattern (BB31 §1.4)")

	fromParam := doc.Paths.Find("/api/v1/assets/{asset_id}/history").Get.Parameters[0]
	assert.Equal(t, "", fromParam.Value.Schema.Value.Pattern,
		"inline date-time query parameter must not declare a pattern (BB31 §1.4)")
}

func TestInjectTopLevelSecurity_AddsDefaultWhenAbsent(t *testing.T) {
	doc := &openapi3.T{}
	injectTopLevelSecurity(doc)
	require.Len(t, doc.Security, 1)
	assert.Equal(t, []string{}, doc.Security[0]["BearerAuth"])
}

func TestInjectTopLevelSecurity_PreservesExisting(t *testing.T) {
	doc := &openapi3.T{
		Security: openapi3.SecurityRequirements{
			openapi3.SecurityRequirement{"SessionAuth": []string{"read"}},
		},
	}
	injectTopLevelSecurity(doc)
	require.Len(t, doc.Security, 1)
	assert.Equal(t, []string{"read"}, doc.Security[0]["SessionAuth"])
}

func TestMarkRequiredFields_AddsRequiredBlock(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"thing.View": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"id":   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
						"name": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
						"note": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					},
				}},
			},
		},
	}
	required := map[string][]string{"thing.View": {"id", "name"}}

	if err := markRequiredFields(doc, required); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := doc.Components.Schemas["thing.View"].Value.Required
	want := []string{"id", "name"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("required = %v, want %v", got, want)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("doc no longer validates: %v", err)
	}
}

func TestMarkRequiredFields_ErrorsOnMissingSchema(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	required := map[string][]string{"thing.Missing": {"id"}}

	err := markRequiredFields(doc, required)
	if err == nil {
		t.Fatalf("expected error for missing schema, got nil")
	}
}

func TestMarkRequiredFields_ErrorsOnMissingField(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"thing.View": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{"id": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}}},
				}},
			},
		},
	}
	required := map[string][]string{"thing.View": {"id", "ghost"}}

	err := markRequiredFields(doc, required)
	if err == nil {
		t.Fatalf("expected error for missing field, got nil")
	}
}

// TestPostprocess_MarksReadOnlyFields covers TRA-587 / BB16 S8: round-trip-safe
// server-managed fields on read views are tagged readOnly so codegen splits
// read and write types and a verbatim GET → PATCH round-trip is type-safe.
//
// `tags` and `external_key` are intentionally excluded from the readOnly list
// (TRA-686 / BB29 F7+F8, history TRA-643): each has a dedicated mutation path,
// and the PATCH validator rejects them with per-category codes (tags →
// invalid_value, external_key → read_only). Keeping these fields writable in
// the spec preserves the runtime signal — codegen tools won't strip them, so
// an SDK that mistakenly sends them surfaces the failure.
func TestPostprocess_MarksReadOnlyFields(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"id":         &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"created_at": stringProp("date-time"),
				"updated_at": stringProp("date-time"),
				"tags": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:  &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
				}},
				"name":         stringProp(""), // not on the allowlist — must remain writable
				"external_key": stringProp(""), // mutable lookup key (TRA-555) — must remain writable
			},
		}},
		"location.PublicLocationView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"id":         &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"created_at": stringProp("date-time"),
				"updated_at": stringProp("date-time"),
				"tags": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:  &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
				}},
				"name": stringProp(""), // not on the allowlist
			},
		}},
	})

	readOnly := map[string][]string{
		"asset.PublicAssetView":       {"id", "created_at", "updated_at"},
		"location.PublicLocationView": {"id", "created_at", "updated_at"},
	}
	if err := markReadOnlyFields(doc, readOnly); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for schemaName, fields := range readOnly {
		schema := doc.Components.Schemas[schemaName].Value
		for _, field := range fields {
			assert.True(t, schema.Properties[field].Value.ReadOnly,
				"%s.%s must be marked readOnly", schemaName, field)
		}
	}

	// fields NOT on the allowlist must remain writable
	assert.False(t,
		doc.Components.Schemas["asset.PublicAssetView"].Value.Properties["name"].Value.ReadOnly,
		"name must remain writable")
	assert.False(t,
		doc.Components.Schemas["asset.PublicAssetView"].Value.Properties["external_key"].Value.ReadOnly,
		"external_key must remain writable")
	assert.False(t,
		doc.Components.Schemas["location.PublicLocationView"].Value.Properties["name"].Value.ReadOnly,
		"name must remain writable")

	// `tags` is managed via subresource and rejected by the PUT validator
	// (TRA-643 / BB22 F1); it must NOT be marked readOnly so codegen keeps it
	// in the request shape and SDK consumers see the rejection signal.
	assert.False(t,
		doc.Components.Schemas["asset.PublicAssetView"].Value.Properties["tags"].Value.ReadOnly,
		"asset tags must remain writable so SDK callers see the rejection")
	assert.False(t,
		doc.Components.Schemas["location.PublicLocationView"].Value.Properties["tags"].Value.ReadOnly,
		"location tags must remain writable so SDK callers see the rejection")
}

func TestMarkReadOnlyFields_ErrorsOnMissingSchema(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	readOnly := map[string][]string{"thing.Missing": {"id"}}

	err := markReadOnlyFields(doc, readOnly)
	if err == nil {
		t.Fatalf("expected error for missing schema, got nil")
	}
}

func TestMarkReadOnlyFields_ErrorsOnMissingField(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"thing.View": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{"id": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}}},
				}},
			},
		},
	}
	readOnly := map[string][]string{"thing.View": {"ghost"}}

	err := markReadOnlyFields(doc, readOnly)
	if err == nil {
		t.Fatalf("expected error for missing field, got nil")
	}
}

func docWithSchemas(schemas openapi3.Schemas) *openapi3.T {
	return &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: schemas,
			SecuritySchemes: openapi3.SecuritySchemes{
				"BearerAuth": &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
					Type: "apiKey", In: "header", Name: "Authorization",
				}},
			},
		},
	}
}

func stringProp(format string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeString}, Format: format,
	}}
}

func TestPostprocessPublic_StripsSessionAuth(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	doc.Components.SecuritySchemes["SessionAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "Session JWT for internal endpoints (platform frontend uses this).",
		},
	}

	require.NoError(t, postprocessPublic(doc))

	_, hasSession := doc.Components.SecuritySchemes["SessionAuth"]
	assert.False(t, hasSession, "SessionAuth must be stripped from public components")
	_, hasBearer := doc.Components.SecuritySchemes["BearerAuth"]
	assert.True(t, hasBearer, "BearerAuth must remain in public components")
}

func TestPostprocessInternal_KeepsSessionAuth(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	doc.Components.SecuritySchemes["SessionAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "Session JWT for internal endpoints (platform frontend uses this).",
		},
	}

	require.NoError(t, postprocessInternal(doc))

	scheme, ok := doc.Components.SecuritySchemes["SessionAuth"]
	require.True(t, ok, "SessionAuth must remain in internal components")
	require.NotNil(t, scheme.Value)
	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
}

// TestNormalizeArrayQueryParams verifies that in:query parameters with
// type:array receive style:form and explode:false, that non-array (string)
// params are untouched, and that a param whose Style is already non-default is
// not overwritten.
func TestNormalizeArrayQueryParams(t *testing.T) {
	f := false
	boolFalse := &f

	makeDoc := func(params openapi3.Parameters) *openapi3.T {
		doc := &openapi3.T{
			OpenAPI: "3.0.0",
			Info:    &openapi3.Info{Title: "Test", Version: "v1"},
			Paths:   openapi3.NewPaths(),
		}
		doc.Paths.Set("/things", &openapi3.PathItem{
			Get: &openapi3.Operation{
				Parameters: params,
			},
		})
		return doc
	}

	t.Run("sets style+explode on array query param", func(t *testing.T) {
		doc := makeDoc(openapi3.Parameters{
			{Value: &openapi3.Parameter{
				Name:   "sort",
				In:     "query",
				Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}}},
			}},
			{Value: &openapi3.Parameter{
				Name:   "filter",
				In:     "query",
				Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
			}},
		})

		normalizeArrayQueryParams(doc)

		params := doc.Paths.Value("/things").Get.Parameters
		arrayParam := params[0].Value
		stringParam := params[1].Value

		assert.Equal(t, "form", arrayParam.Style, "array param must get style:form")
		require.NotNil(t, arrayParam.Explode, "array param must get explode set")
		assert.Equal(t, false, *arrayParam.Explode, "array param must get explode:false")

		assert.Equal(t, "", stringParam.Style, "string param style must remain empty")
		assert.Nil(t, stringParam.Explode, "string param explode must remain nil")
	})

	t.Run("does not overwrite existing non-default Style", func(t *testing.T) {
		doc := makeDoc(openapi3.Parameters{
			{Value: &openapi3.Parameter{
				Name:    "tags",
				In:      "query",
				Style:   "spaceDelimited",
				Explode: boolFalse,
				Schema:  &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}}},
			}},
		})

		normalizeArrayQueryParams(doc)

		p := doc.Paths.Value("/things").Get.Parameters[0].Value
		assert.Equal(t, "spaceDelimited", p.Style, "pre-existing Style must not be overwritten")
		require.NotNil(t, p.Explode)
		assert.Equal(t, false, *p.Explode, "pre-existing Explode must not be overwritten")
	})
}

// TestPostprocess_StripsBearerScopeArrays_InjectsDescription locks in
// TRA-585 S2. OpenAPI 3.0 §4.8.30 forbids non-empty scope arrays on
// non-oauth2 / non-openIdConnect schemes. Swaggo's
// `@Security BearerAuth[assets:read]` syntax produces an invalid spec under
// http-bearer. The pass strips the arrays and prepends a
// "**Required scope:** `<scope>`" line to the operation description.
func TestPostprocess_StripsBearerScopeArrays_InjectsDescription(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	require.NotNil(t, op)
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"BearerAuth": []string{"assets:read"}},
	)

	require.NoError(t, postprocessPublic(doc))

	require.Len(t, *op.Security, 1)
	assert.Equal(t, []string{}, (*op.Security)[0]["BearerAuth"],
		"scope array must be empty after the pass — non-empty arrays are invalid for http-bearer")

	assert.True(t, strings.HasPrefix(op.Description, "**Required scope:** `assets:read`"),
		"description must start with the scope marker, got %q", op.Description)
	assert.Contains(t, op.Description, "Paginated list of assets.",
		"original description content must be preserved")
}

// TestPostprocess_StripsBearerScopeArrays_Idempotent verifies the pass is
// safe to run twice.
func TestPostprocess_StripsBearerScopeArrays_Idempotent(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"BearerAuth": []string{"assets:read"}},
	)

	require.NoError(t, postprocessPublic(doc))
	first := op.Description
	require.NoError(t, postprocessPublic(doc))
	assert.Equal(t, first, op.Description,
		"second invocation must not double-prepend the scope marker")
}

// TestPostprocess_StripsBearerScopeArrays_EmitsEmptyForScopelessBearer locks
// in the TRA-712 BB33 F7 acceptance: an op with bearer auth but no declared
// scopes still gets an x-required-scopes extension, set to an empty array.
// Absence would be ambiguous about whether scopes were considered;
// empty array clearly signals "any authenticated key works" to codegen
// ingestors trying to mint minimal-scope keys (e.g. /api/v1/orgs/me).
func TestPostprocess_StripsBearerScopeArrays_EmitsEmptyForScopelessBearer(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"BearerAuth": []string{}},
	)

	require.NoError(t, postprocessPublic(doc))
	assert.Equal(t, "Paginated list of assets.", op.Description,
		"no scopes => no marker injected")
	require.NotNil(t, op.Extensions, "extensions must be populated")
	got, ok := op.Extensions["x-required-scopes"]
	require.True(t, ok, "x-required-scopes must be present on bearer-gated ops, even scopeless")
	assert.Equal(t, []string{}, got, "scopeless bearer op gets empty array")
}

// TestPostprocess_StripsBearerScopeArrays_EmitsExtension locks in TRA-685 F4.
// Captured scopes must surface as `x-required-scopes` on the operation so
// scope-aware partners can read the required scopes machine-readably.
// Standard codegen won't auto-surface this (matching prior behavior), but
// the spec stays honest about the auth model.
func TestPostprocess_StripsBearerScopeArrays_EmitsExtension(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	require.NotNil(t, op)
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"BearerAuth": []string{"assets:read"}},
	)

	require.NoError(t, postprocessPublic(doc))

	require.NotNil(t, op.Extensions, "extensions must be populated")
	got, ok := op.Extensions["x-required-scopes"]
	require.True(t, ok, "x-required-scopes must be present on scope-gated operations")
	assert.Equal(t, []string{"assets:read"}, got)
}

// TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs locks in TRA-585 S1.
// The errors page declares the envelope is "modeled on RFC 7807 but not
// 7807-compliant" — the spec description must match instead of claiming
// full RFC 7807 compliance.
func TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	doc.Components.Schemas["errors.ErrorResponse"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"error": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: map[string]*openapi3.SchemaRef{
						"title":  {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
						"detail": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					},
				}},
			},
		},
	}
	require.NoError(t, postprocessPublic(doc))

	desc := doc.Components.Schemas["errors.ErrorResponse"].Value.Description
	assert.Contains(t, desc, "modeled on RFC 7807 but not 7807-compliant",
		"description must match the docs/api/errors page wording (TRA-585 S1)")
	assert.Contains(t, desc, "application/json",
		"description must call out that content-type is application/json, not application/problem+json")
	assert.Contains(t, desc, "nested under `error.*`",
		"description must call out the non-7807 nesting")
	assert.NotContains(t, desc, "RFC 7807 Problem Details envelope.",
		"old wording must be gone — it implies full compliance")
}

// TestConsolidateSchemaNamespaces_RenamesPluralPrefixes covers TRA-602
// and the post-launch audit extension. Schemas in the consolidated set
// (assets./locations./reports./users./orgs./organization./long-form
// user import path) are renamed to the singular target. errors.*,
// shared.*, apikey.*, and other already-singular namespaces are
// untouched. $refs anywhere in the document are rewritten in lockstep.
func TestConsolidateSchemaNamespaces_RenamesPluralPrefixes(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"asset.PublicAssetView":                                        {Value: &openapi3.Schema{}},
				"assets.CreateAssetResponse":                                   {Value: &openapi3.Schema{}},
				"assets.AddTagResponse":                                        {Value: &openapi3.Schema{}},
				"location.PublicLocationView":                                  {Value: &openapi3.Schema{}},
				"locations.UpdateLocationResponse":                             {Value: &openapi3.Schema{}},
				"reports.AssetHistoryResponse":                                 {Value: &openapi3.Schema{}},
				"users.ListResponse":                                           {Value: &openapi3.Schema{}},
				"user.CreateUserRequest":                                       {Value: &openapi3.Schema{}},
				"github_com_trakrf_platform_backend_internal_models_user.User": {Value: &openapi3.Schema{}},
				"organization.UserOrg":                                         {Value: &openapi3.Schema{}},
				"orgs.GetOrgMeResponse":                                        {Value: &openapi3.Schema{}},
				"errors.ErrorResponse":                                         {Value: &openapi3.Schema{}},
				"shared.Tag":                                                   {Value: &openapi3.Schema{}},
				"apikey.APIKeyListItem":                                        {Value: &openapi3.Schema{}},
			},
		},
	}
	// $ref pointing at a renamed schema (operation response) plus one
	// at a non-renamed schema (errors envelope) so we can verify both.
	doc.Paths.Set("/things", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{Value: &openapi3.Response{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/assets.CreateAssetResponse"},
						},
					},
				}}),
				openapi3.WithStatus(400, &openapi3.ResponseRef{Value: &openapi3.Response{
					Content: openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/errors.ErrorResponse"},
						},
					},
				}}),
			),
		},
	})

	consolidateSchemaNamespaces(doc)

	schemas := doc.Components.Schemas
	// Renamed targets present.
	assert.Contains(t, schemas, "asset.CreateAssetResponse", "assets.CreateAssetResponse must be renamed to asset.CreateAssetResponse")
	assert.Contains(t, schemas, "asset.AddTagResponse")
	assert.Contains(t, schemas, "location.UpdateLocationResponse")
	assert.Contains(t, schemas, "report.AssetHistoryResponse")
	assert.Contains(t, schemas, "user.ListResponse", "users.ListResponse must collapse onto user.*")
	assert.Contains(t, schemas, "user.User", "long-form user import path must collapse onto user.User")
	assert.Contains(t, schemas, "org.UserOrg", "organization.* must collapse onto org.*")
	assert.Contains(t, schemas, "org.GetOrgMeResponse", "orgs.* must collapse onto org.*")
	// Old plural names gone.
	assert.NotContains(t, schemas, "assets.CreateAssetResponse", "old plural name must be removed")
	assert.NotContains(t, schemas, "assets.AddTagResponse")
	assert.NotContains(t, schemas, "locations.UpdateLocationResponse")
	assert.NotContains(t, schemas, "reports.AssetHistoryResponse")
	assert.NotContains(t, schemas, "users.ListResponse")
	assert.NotContains(t, schemas, "github_com_trakrf_platform_backend_internal_models_user.User")
	assert.NotContains(t, schemas, "organization.UserOrg")
	assert.NotContains(t, schemas, "orgs.GetOrgMeResponse")
	// Already-singular and out-of-scope namespaces untouched.
	assert.Contains(t, schemas, "asset.PublicAssetView")
	assert.Contains(t, schemas, "location.PublicLocationView")
	assert.Contains(t, schemas, "user.CreateUserRequest", "pre-existing user.* schemas survive the consolidation")
	assert.Contains(t, schemas, "errors.ErrorResponse")
	assert.Contains(t, schemas, "shared.Tag")
	assert.Contains(t, schemas, "apikey.APIKeyListItem")

	// Operation $refs rewritten.
	op := doc.Paths.Find("/things").Get
	require.NotNil(t, op)
	require.NotNil(t, op.Responses)
	r200 := op.Responses.Value("200").Value
	assert.Equal(t, "#/components/schemas/asset.CreateAssetResponse",
		r200.Content["application/json"].Schema.Ref,
		"$ref to renamed schema must be rewritten")
	r400 := op.Responses.Value("400").Value
	assert.Equal(t, "#/components/schemas/errors.ErrorResponse",
		r400.Content["application/json"].Schema.Ref,
		"$ref to non-renamed schema must be left alone")
}

// TestConsolidateSchemaNamespaces_NoOpWithoutTargets verifies the pass
// is a no-op when no plural-prefix schemas exist (e.g. minimal fixture).
func TestConsolidateSchemaNamespaces_NoOpWithoutTargets(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"errors.ErrorResponse":  {Value: &openapi3.Schema{}},
				"shared.Tag":            {Value: &openapi3.Schema{}},
				"asset.PublicAssetView": {Value: &openapi3.Schema{}},
			},
		},
	}

	consolidateSchemaNamespaces(doc)

	assert.Len(t, doc.Components.Schemas, 3, "schemas map size must be unchanged")
	assert.Contains(t, doc.Components.Schemas, "errors.ErrorResponse")
	assert.Contains(t, doc.Components.Schemas, "shared.Tag")
	assert.Contains(t, doc.Components.Schemas, "asset.PublicAssetView")
}

// TestConsolidateSchemaNamespaces_RewritesNestedRefs confirms nested
// $refs (Properties, AllOf, Items) are rewritten alongside top-level
// schema renames.
func TestConsolidateSchemaNamespaces_RewritesNestedRefs(t *testing.T) {
	envelope := &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: openapi3.Schemas{
			"data": {Ref: "#/components/schemas/asset.PublicAssetView"},
			"page": {Value: &openapi3.Schema{
				AllOf: openapi3.SchemaRefs{
					{Ref: "#/components/schemas/shared.Pagination"},
				},
			}},
			"items": {Value: &openapi3.Schema{
				Type:  &openapi3.Types{openapi3.TypeArray},
				Items: &openapi3.SchemaRef{Ref: "#/components/schemas/reports.AssetHistoryResponse"},
			}},
		},
	}
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"asset.PublicAssetView":        {Value: &openapi3.Schema{}},
				"shared.Pagination":            {Value: &openapi3.Schema{}},
				"reports.AssetHistoryResponse": {Value: &openapi3.Schema{}},
				"assets.ListAssetsResponse":    {Value: envelope},
			},
		},
	}

	consolidateSchemaNamespaces(doc)

	renamed := doc.Components.Schemas["asset.ListAssetsResponse"].Value
	require.NotNil(t, renamed)
	assert.Equal(t, "#/components/schemas/asset.PublicAssetView",
		renamed.Properties["data"].Ref, "data $ref to non-renamed schema is preserved")
	assert.Equal(t, "#/components/schemas/shared.Pagination",
		renamed.Properties["page"].Value.AllOf[0].Ref, "AllOf $ref unchanged")
	assert.Equal(t, "#/components/schemas/report.AssetHistoryResponse",
		renamed.Properties["items"].Value.Items.Ref, "nested array Items $ref must be rewritten")
}

// TestConsolidateSchemaNamespaces_SkipsCollidingTargets locks in the
// collision guards in buildSchemaRenameSet:
//
//  1. A target name that already exists as a distinct schema (a real
//     pre-existing user.User vs renaming users.User to user.User) — the
//     rename for that conflicting source is skipped to avoid silent
//     overwrite.
//  2. Multiple sources mapping to the same target (e.g. orgs.X and
//     organization.X both folding onto org.X) — every contributing
//     source is dropped from the rename set so neither overwrites the
//     other. Production case has disjoint type names per source, but
//     the guard prevents silent breakage if that ever changes.
func TestConsolidateSchemaNamespaces_SkipsCollidingTargets(t *testing.T) {
	t.Run("pre-existing target schema blocks rename", func(t *testing.T) {
		doc := &openapi3.T{
			OpenAPI: "3.0.0",
			Info:    &openapi3.Info{Title: "Test", Version: "v1"},
			Paths:   openapi3.NewPaths(),
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{
					"users.ListResponse": {Value: &openapi3.Schema{Description: "rename source"}},
					"user.ListResponse":  {Value: &openapi3.Schema{Description: "pre-existing target"}},
				},
			},
		}

		consolidateSchemaNamespaces(doc)

		require.Contains(t, doc.Components.Schemas, "users.ListResponse",
			"source must not be renamed when target already exists")
		require.Contains(t, doc.Components.Schemas, "user.ListResponse")
		assert.Equal(t, "pre-existing target",
			doc.Components.Schemas["user.ListResponse"].Value.Description,
			"pre-existing target must not be overwritten")
	})

	t.Run("two sources renaming to the same target are both skipped", func(t *testing.T) {
		doc := &openapi3.T{
			OpenAPI: "3.0.0",
			Info:    &openapi3.Info{Title: "Test", Version: "v1"},
			Paths:   openapi3.NewPaths(),
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{
					"orgs.Conflict":         {Value: &openapi3.Schema{Description: "from orgs"}},
					"organization.Conflict": {Value: &openapi3.Schema{Description: "from organization"}},
					"orgs.OnlyOrgs":         {Value: &openapi3.Schema{}},
				},
			},
		}

		consolidateSchemaNamespaces(doc)

		require.Contains(t, doc.Components.Schemas, "orgs.Conflict",
			"colliding source must remain when two prefixes target the same name")
		require.Contains(t, doc.Components.Schemas, "organization.Conflict",
			"colliding source must remain when two prefixes target the same name")
		require.NotContains(t, doc.Components.Schemas, "org.Conflict",
			"the contested target must not be created when sources collide")
		require.Contains(t, doc.Components.Schemas, "org.OnlyOrgs",
			"non-colliding sources in the same prefix family still rename")
	})
}

// TestConsolidateSchemaNamespaces_HandlesEmptyComponents covers the
// guard for documents with no Components or empty Schemas.
func TestConsolidateSchemaNamespaces_HandlesEmptyComponents(t *testing.T) {
	doc := &openapi3.T{OpenAPI: "3.0.0", Info: &openapi3.Info{Title: "T", Version: "v1"}}
	assert.NotPanics(t, func() { consolidateSchemaNamespaces(doc) })

	doc2 := &openapi3.T{
		OpenAPI:    "3.0.0",
		Info:       &openapi3.Info{Title: "T", Version: "v1"},
		Components: &openapi3.Components{},
	}
	assert.NotPanics(t, func() { consolidateSchemaNamespaces(doc2) })
}

// TestInjectGlobalHeaderRefs covers TRA-633 B3: rate-limit and
// request-correlation headers must live in components.headers, and every
// operation response must reference them. Inline header definitions on
// individual responses are flattened to the canonical $ref.
func TestInjectGlobalHeaderRefs(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v1"},
		Paths:   &openapi3.Paths{},
	}
	desc200 := "OK"
	desc401 := "unauthorized"
	desc429 := "rate_limited"
	// 200 carries a stale inline X-RateLimit-Limit; the pass must replace
	// it with a $ref to the components.headers entry.
	resp200 := &openapi3.ResponseRef{Value: &openapi3.Response{
		Description: &desc200,
		Headers: openapi3.Headers{
			"X-RateLimit-Limit": &openapi3.HeaderRef{Value: &openapi3.Header{Parameter: openapi3.Parameter{
				Description: "stale inline copy",
				Schema:      &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
			}}},
		},
	}}
	resp401 := &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc401}}
	resp429 := &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc429}}

	responses := openapi3.NewResponses()
	responses.Set("200", resp200)
	responses.Set("401", resp401)
	responses.Set("429", resp429)
	doc.Paths.Set("/widgets", &openapi3.PathItem{
		Get: &openapi3.Operation{Responses: responses},
	})

	injectGlobalHeaderRefs(doc)

	require.NotNil(t, doc.Components)
	require.NotNil(t, doc.Components.Headers)
	for _, name := range []string{"XRateLimitLimit", "XRateLimitRemaining", "XRateLimitReset", "RetryAfter", "WWWAuthenticate", "XRequestId"} {
		ref := doc.Components.Headers[name]
		require.NotNil(t, ref, "components.headers.%s must be defined", name)
		require.NotNil(t, ref.Value)
		require.NotNil(t, ref.Value.Schema)
	}

	for _, code := range []string{"200", "401", "429"} {
		resp := doc.Paths.Find("/widgets").Get.Responses.Value(code)
		require.NotNil(t, resp, "response %s must be present", code)
		for _, name := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "X-Request-Id"} {
			h := resp.Value.Headers[name]
			require.NotNil(t, h, "response %s missing %s", code, name)
			assert.Equal(t, "#/components/headers/"+canonicalizeHeaderName(name), h.Ref,
				"response %s header %s must be a $ref", code, name)
			assert.Nil(t, h.Value, "$ref headers must not carry inline values (response %s, %s)", code, name)
		}
	}
	retryAfter := doc.Paths.Find("/widgets").Get.Responses.Value("429").Value.Headers["Retry-After"]
	require.NotNil(t, retryAfter, "429 must declare Retry-After")
	assert.Equal(t, "#/components/headers/RetryAfter", retryAfter.Ref)
	assert.Nil(t, doc.Paths.Find("/widgets").Get.Responses.Value("200").Value.Headers["Retry-After"],
		"non-429 responses must not declare Retry-After")

	wwwAuth := doc.Paths.Find("/widgets").Get.Responses.Value("401").Value.Headers["WWW-Authenticate"]
	require.NotNil(t, wwwAuth, "401 must declare WWW-Authenticate (RFC 7235)")
	assert.Equal(t, "#/components/headers/WWWAuthenticate", wwwAuth.Ref)
	assert.Nil(t, doc.Paths.Find("/widgets").Get.Responses.Value("200").Value.Headers["WWW-Authenticate"],
		"non-401 responses must not declare WWW-Authenticate")
	assert.Nil(t, doc.Paths.Find("/widgets").Get.Responses.Value("429").Value.Headers["WWW-Authenticate"],
		"non-401 responses must not declare WWW-Authenticate")
}

// canonicalizeHeaderName maps an HTTP header name to its components.headers
// component name. Used by the test only.
func canonicalizeHeaderName(h string) string {
	return strings.ReplaceAll(strings.ReplaceAll(h, "-", ""), "_", "")
}

// TestInjectGlobalHeaderRefs_Idempotent verifies a second pass does not
// duplicate or corrupt the components or response wiring.
func TestInjectGlobalHeaderRefs_Idempotent(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v1"},
		Paths:   &openapi3.Paths{},
	}
	desc := "OK"
	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc}})
	doc.Paths.Set("/x", &openapi3.PathItem{Get: &openapi3.Operation{Responses: responses}})

	injectGlobalHeaderRefs(doc)
	first := doc.Components.Headers["XRequestId"]
	require.NotNil(t, first)
	injectGlobalHeaderRefs(doc)
	assert.Same(t, first, doc.Components.Headers["XRequestId"], "components.headers entry must be reused, not replaced")

	headers := doc.Paths.Find("/x").Get.Responses.Value("200").Value.Headers
	assert.Len(t, headers, 4, "200 must declare exactly the 4 global headers (no Retry-After)")
}

// TestAppendSpecVariantsDescription covers TRA-657 BB25 A8: both spec
// variants (YAML and JSON) must be advertised in info.description so the
// /api Redoc page — whose download button only fetches YAML — has a
// discoverable pointer to the JSON URL.
func TestAppendSpecVariantsDescription(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v1", Description: "Existing prose."},
	}
	appendSpecVariantsDescription(doc)

	assert.Contains(t, doc.Info.Description, "Existing prose.", "existing description must be preserved")
	assert.Contains(t, doc.Info.Description, "/api/openapi.yaml", "YAML variant URL must be advertised")
	assert.Contains(t, doc.Info.Description, "/api/openapi.json", "JSON variant URL must be advertised")
	assert.NotContains(t, doc.Info.Description, "docs.trakrf.id",
		"link must be site-relative, not env-pinned to production docs (TRA-657 / BB25 B4)")

	// Idempotency: a second call must not duplicate the section.
	before := doc.Info.Description
	appendSpecVariantsDescription(doc)
	assert.Equal(t, before, doc.Info.Description, "second call must be a no-op")

	// Empty seed: function must populate without a leading separator.
	emptyDoc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v1"},
	}
	appendSpecVariantsDescription(emptyDoc)
	assert.True(t, strings.HasPrefix(emptyDoc.Info.Description, "Spec available as YAML"),
		"empty seed must produce description without leading newlines")
}

// TestAppendMethodPolicyDescription covers TRA-633 B1/B4 + TRA-649 / BB23
// S4: HEAD/OPTIONS behavior is documented once, but only as a one-line
// link to the customer-facing docs page. Inlining the full policy prose
// caused generated SDK class docstrings to balloon (the entire
// HTTP-method-coverage paragraph appeared at the top of AssetsApi.ts,
// LocationsApi.ts, OrgsApi.ts on every regen).
func TestAppendMethodPolicyDescription(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v1", Description: "Existing prose."},
	}
	appendMethodPolicyDescription(doc)

	assert.Contains(t, doc.Info.Description, "Existing prose.", "existing description must be preserved")
	assert.Contains(t, doc.Info.Description, "HTTP method coverage", "method-coverage pointer must be present")
	assert.Contains(t, doc.Info.Description, "/api/http-method-coverage",
		"description must link out to the docs page for method-coverage details")
	assert.NotContains(t, doc.Info.Description, "docs.trakrf.id",
		"link must be site-relative, not env-pinned to production docs (TRA-657 / BB25 B4)")
	assert.NotContains(t, doc.Info.Description, "transparently strips the response body",
		"prose body must live in docs, not the spec (TRA-649 S4)")

	// Idempotency: a second call must not duplicate the section.
	before := doc.Info.Description
	appendMethodPolicyDescription(doc)
	assert.Equal(t, before, doc.Info.Description, "second call must be a no-op")
}

// withEmptyRequiredFields clears the package-level requiredFields and
// readOnlyFields maps for the duration of a test and restores them on
// cleanup. Tests that exercise postprocessPublic / postprocessInternal
// against synthetic minimal docs use this so the stale-entry guards in
// markRequiredFields and markReadOnlyFields don't bail out before the
// assertions run. Tests that verify either pass directly call
// markRequiredFields / markReadOnlyFields with their own map.
func withEmptyRequiredFields(t *testing.T) {
	t.Helper()
	saved := requiredFields
	requiredFields = map[string][]string{}
	savedInternal := internalOnlyRequiredFields
	internalOnlyRequiredFields = map[string][]string{}
	savedReadOnly := readOnlyFields
	readOnlyFields = map[string][]string{}
	savedAdditive := publicResponseSchemas
	publicResponseSchemas = nil
	// TRA-681: mutuallyExclusiveFieldPairs is strict (errors on unknown
	// schema) like requiredFields / readOnlyFields above. Clear it here so
	// minimal in-memory test docs that don't seed the Create* schemas don't
	// trip the strict guard inside postprocessPublic. Coverage for the pass
	// itself lives in postprocess_test.go alongside the other strict-map
	// passes.
	savedMutuallyExclusive := mutuallyExclusiveFieldPairs
	mutuallyExclusiveFieldPairs = nil
	// TRA-660: existing tests pre-seed schemas under the dotted Go-package
	// names and assert on those names after calling postprocessPublic. The
	// public-spec rename pass would rewrite those keys, so disable it here
	// — tests for the rename pass live in rename_public_test.go and don't
	// use this helper.
	savedSchemaRenames := publicSchemaRenames
	publicSchemaRenames = map[string]string{}
	savedOpIDRenames := publicOperationIdRenames
	publicOperationIdRenames = map[string]string{}
	savedTagDescriptions := publicTagDescriptions
	publicTagDescriptions = nil
	// TRA-691: hoistInlineEnums is strict (errors on unknown source schema)
	// like the other strict-map passes. Minimal in-memory test docs don't
	// seed shared.Tag / errors.ErrorResponse / errors.FieldError, so clear
	// the extraction list here. Coverage for the pass itself lives in
	// hoist_enums_test.go.
	savedInlineEnumExtractions := inlineEnumExtractions
	inlineEnumExtractions = nil
	t.Cleanup(func() {
		requiredFields = saved
		internalOnlyRequiredFields = savedInternal
		readOnlyFields = savedReadOnly
		publicResponseSchemas = savedAdditive
		mutuallyExclusiveFieldPairs = savedMutuallyExclusive
		publicSchemaRenames = savedSchemaRenames
		publicOperationIdRenames = savedOpIDRenames
		publicTagDescriptions = savedTagDescriptions
		inlineEnumExtractions = savedInlineEnumExtractions
	})
}

// TRA-717 / BB34 F5 (+ BB33 F5 carry-over): list-endpoint filters named
// `*external_key` declare the strict identifier pattern (^[A-Za-z0-9-]+$),
// not the loose printable-string pattern. A generated client validating
// against the spec must reject `?external_key=abc/def` at the client
// boundary instead of letting it through and surfacing a server-side 400
// the client believes "shouldn't happen." `q` keeps the printable pattern
// because it really is free-form substring search.
func TestMarkQueryStringPatterns_ExternalKeyFiltersUseStrictPattern(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "v"},
		Paths:   openapi3.NewPaths(),
	}
	arrayParam := func(name string) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name: name,
			In:   "query",
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
				Type: &openapi3.Types{openapi3.TypeArray},
				Items: &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeString},
				}},
			}},
		}}
	}
	stringParam := func(name string) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name: name,
			In:   "query",
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
				Type: &openapi3.Types{openapi3.TypeString},
			}},
		}}
	}
	doc.Paths.Set("/api/v1/assets", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				arrayParam("external_key"),
				arrayParam("location_external_key"),
				stringParam("q"),
			},
		},
	})
	doc.Paths.Set("/api/v1/locations", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				arrayParam("external_key"),
				arrayParam("parent_external_key"),
			},
		},
	})
	doc.Paths.Set("/api/v1/reports/asset-locations", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				arrayParam("location_external_key"),
			},
		},
	})

	markQueryStringPatterns(doc)

	itemsPattern := func(path, name string) string {
		for _, p := range doc.Paths.Find(path).Get.Parameters {
			if p.Value.Name == name {
				if p.Value.Schema.Value.Items != nil {
					return p.Value.Schema.Value.Items.Value.Pattern
				}
				return p.Value.Schema.Value.Pattern
			}
		}
		return ""
	}

	strict := externalKeyPattern
	for _, c := range []struct {
		path, name string
	}{
		{"/api/v1/assets", "external_key"},
		{"/api/v1/assets", "location_external_key"},
		{"/api/v1/locations", "external_key"},
		{"/api/v1/locations", "parent_external_key"},
		{"/api/v1/reports/asset-locations", "location_external_key"},
	} {
		assert.Equal(t, strict, itemsPattern(c.path, c.name),
			"%s ?%s items.pattern must be the strict external_key pattern (TRA-717 F5)", c.path, c.name)
	}

	// q is free-form substring search — keeps the printable-string pattern
	assert.Equal(t, printableStringRegex, itemsPattern("/api/v1/assets", "q"),
		"q must keep printable-string pattern (control chars rejected, slashes allowed)")
}

// TestInlinePublicTimeRefs covers TRA-717 / BB34 F3 rework: any property
// $ref'ing the shared.PublicTime wrapper (in either the original or
// post-consolidation namespace) is rewritten as an inline
// `type: string, format: date-time` schema and the wrapper component is
// dropped. Existing nullable on the referencing property is preserved.
func TestInlinePublicTimeRefs(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"shared.PublicTime": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{"time.Time": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}}},
				}},
				"AssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"created_at": {Ref: "#/components/schemas/shared.PublicTime"},
						"deleted_at": {Ref: "#/components/schemas/shared.PublicTime", Value: &openapi3.Schema{Nullable: true}},
					},
				}},
			},
		},
		Paths: openapi3.NewPaths(),
	}

	inlinePublicTimeRefs(doc)

	_, kept := doc.Components.Schemas["shared.PublicTime"]
	assert.False(t, kept, "shared.PublicTime component must be deleted")

	av := doc.Components.Schemas["AssetView"].Value
	created := av.Properties["created_at"]
	require.NotNil(t, created)
	assert.Equal(t, "", created.Ref, "created_at must be inlined, not $ref")
	require.NotNil(t, created.Value)
	assert.True(t, created.Value.Type.Is(openapi3.TypeString))
	assert.Equal(t, "date-time", created.Value.Format)
	assert.False(t, created.Value.Nullable, "non-nullable property stays non-nullable")

	deleted := av.Properties["deleted_at"]
	require.NotNil(t, deleted)
	assert.Equal(t, "", deleted.Ref)
	require.NotNil(t, deleted.Value)
	assert.True(t, deleted.Value.Type.Is(openapi3.TypeString))
	assert.Equal(t, "date-time", deleted.Value.Format)
	assert.True(t, deleted.Value.Nullable, "nullable on the referencing property must be preserved across the rewrite")
}

// Post-consolidateSchemaNamespaces, the component name is bare
// `PublicTime` (no `shared.` prefix). The rewrite must match that form
// too.
func TestInlinePublicTimeRefs_PostConsolidationName(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"PublicTime": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
				"AssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Properties: openapi3.Schemas{
						"valid_from": {Ref: "#/components/schemas/PublicTime"},
					},
				}},
			},
		},
		Paths: openapi3.NewPaths(),
	}

	inlinePublicTimeRefs(doc)

	_, kept := doc.Components.Schemas["PublicTime"]
	assert.False(t, kept)
	vf := doc.Components.Schemas["AssetView"].Value.Properties["valid_from"]
	assert.Equal(t, "", vf.Ref)
	assert.Equal(t, "date-time", vf.Value.Format)
}
