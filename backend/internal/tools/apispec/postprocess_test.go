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
	assert.Equal(t, "v1", doc.Info.Version)
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
				"timestamp":             stringProp("date-time"), // not on the allowlist
			},
		}},
		"report.PublicCurrentLocationItem": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"asset_id":              &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"asset_external_key":    stringProp(""),
				"location_id":           &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"location_external_key": stringProp(""),
				"last_seen":             stringProp("date-time"), // not on the allowlist
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
		doc.Components.Schemas["report.PublicAssetHistoryItem"].Value.Properties["timestamp"].Value.Nullable,
		"timestamp is not nullable")
	assert.False(t,
		doc.Components.Schemas["report.PublicCurrentLocationItem"].Value.Properties["last_seen"].Value.Nullable,
		"last_seen is not nullable")
}

func TestPostprocess_AddsDateTimeFormatToTimestampFields(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"Asset": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"valid_from":   stringProp(""),
				"valid_to":     stringProp(""),
				"created_at":   stringProp(""),
				"updated_at":   stringProp(""),
				"expires_at":   stringProp(""),
				"last_used_at": stringProp(""),
				"timestamp":    stringProp(""),
				"last_seen":    stringProp(""),
				"name":         stringProp(""),     // not a timestamp — must be left alone
				"birth_date":   stringProp("date"), // non-matching name + existing format
			},
		}},
	})
	postprocessPublic(doc)

	props := doc.Components.Schemas["Asset"].Value.Properties
	for _, name := range []string{"valid_from", "valid_to", "created_at", "updated_at", "expires_at", "last_used_at", "timestamp", "last_seen"} {
		assert.Equal(t, "date-time", props[name].Value.Format, "%s should gain format: date-time", name)
	}
	assert.Equal(t, "", props["name"].Value.Format, "non-timestamp fields must stay formatless")
	assert.Equal(t, "date", props["birth_date"].Value.Format, "pre-existing format must not be overwritten")
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

// TestPostprocess_MarksReadOnlyFields covers TRA-587 / BB16 S8: server-managed
// fields on read views must be tagged readOnly so codegen splits read and
// write types and a verbatim GET → PUT round-trip is type-safe.
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
				"tree_path":  stringProp(""),
				"depth":      &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"tags": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:  &openapi3.Types{openapi3.TypeArray},
					Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
				}},
				"name": stringProp(""), // not on the allowlist
			},
		}},
	})

	readOnly := map[string][]string{
		"asset.PublicAssetView":       {"id", "created_at", "updated_at", "tags"},
		"location.PublicLocationView": {"id", "created_at", "updated_at", "tree_path", "depth", "tags"},
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

// TestPostprocess_StripsBearerScopeArrays_NoOpWithoutScopes verifies an op
// with an already-empty scope array is left untouched.
func TestPostprocess_StripsBearerScopeArrays_NoOpWithoutScopes(t *testing.T) {
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
	t.Cleanup(func() {
		requiredFields = saved
		internalOnlyRequiredFields = savedInternal
		readOnlyFields = savedReadOnly
	})
}
