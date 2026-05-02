package main

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostprocess_RewritesBearerAuthToHTTPBearer(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// minimal fixture carries only APIKey; synthesize BearerAuth the way
	// swaggo emits it so we can verify the rewrite.
	doc.Components.SecuritySchemes["BearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type: "apiKey", In: "header", Name: "Authorization",
			Description: "Session JWT for internal endpoints.",
		},
	}
	postprocessInternal(doc)

	scheme := doc.Components.SecuritySchemes["BearerAuth"]
	require.NotNil(t, scheme, "BearerAuth scheme must be present")
	require.NotNil(t, scheme.Value)

	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
	assert.Contains(t, scheme.Value.Description, "Session JWT")
}

// TestPostprocess_RewritesAPIKeyToHTTPBearer reverses the TRA-480 §3.3
// decision per TRA-517 AC1. The token is consumed as a Bearer JWT; declaring
// the scheme as type=apiKey makes generated SDKs send the raw value in an
// Authorization header without the "Bearer " prefix, which the platform
// rejects. type=http/scheme=bearer/bearerFormat=JWT is the correct shape.
// We accept the cosmetic SDK-naming churn (e.g. setApiKeyAuth → setBearerAuth)
// in exchange for over-the-wire correctness.
func TestPostprocess_RewritesAPIKeyToHTTPBearer(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	scheme := doc.Components.SecuritySchemes["APIKey"]
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
func TestPostprocess_AnnotatesErrorEnvelope(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := docWithSchemas(openapi3.Schemas{
		"errors.ErrorResponse": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"error": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"type":   stringProp(""),
						"title":  stringProp(""),
						"detail": stringProp(""),
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

	errProps := envelope.Properties["error"].Value.Properties
	assert.NotEmpty(t, errProps["title"].Value.Description, "title field needs its own description")
	assert.NotEmpty(t, errProps["detail"].Value.Description, "detail field needs its own description")
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
				"current_location_id":           &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
				"current_location_external_key": stringProp(""),
				"name":                          stringProp(""), // not on the allowlist
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
		{"asset.PublicAssetView", "current_location_id"},
		{"asset.PublicAssetView", "current_location_external_key"},
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
	assert.Equal(t, []string{}, doc.Security[0]["APIKey"])
}

func TestInjectTopLevelSecurity_PreservesExisting(t *testing.T) {
	doc := &openapi3.T{
		Security: openapi3.SecurityRequirements{
			openapi3.SecurityRequirement{"BearerAuth": []string{"read"}},
		},
	}
	injectTopLevelSecurity(doc)
	require.Len(t, doc.Security, 1)
	assert.Equal(t, []string{"read"}, doc.Security[0]["BearerAuth"])
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

func docWithSchemas(schemas openapi3.Schemas) *openapi3.T {
	return &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: schemas,
			SecuritySchemes: openapi3.SecuritySchemes{
				"APIKey": &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
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

func TestPostprocessPublic_StripsBearerAuth(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	doc.Components.SecuritySchemes["BearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "Session JWT for internal endpoints (platform frontend uses this).",
		},
	}

	require.NoError(t, postprocessPublic(doc))

	_, hasBearer := doc.Components.SecuritySchemes["BearerAuth"]
	assert.False(t, hasBearer, "BearerAuth must be stripped from public components")
	_, hasAPIKey := doc.Components.SecuritySchemes["APIKey"]
	assert.True(t, hasAPIKey, "APIKey must remain in public components")
}

func TestPostprocessInternal_KeepsBearerAuth(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	doc.Components.SecuritySchemes["BearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "Session JWT for internal endpoints (platform frontend uses this).",
		},
	}

	require.NoError(t, postprocessInternal(doc))

	scheme, ok := doc.Components.SecuritySchemes["BearerAuth"]
	require.True(t, ok, "BearerAuth must remain in internal components")
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

// withEmptyRequiredFields clears the package-level requiredFields map for the
// duration of a test and restores it on cleanup. Tests that exercise
// postprocessPublic / postprocessInternal against synthetic minimal docs use
// this so the stale-entry guard in markRequiredFields doesn't bail out before
// the assertions run. Tests that need to verify required-block injection
// directly call markRequiredFields with their own map.
func withEmptyRequiredFields(t *testing.T) {
	t.Helper()
	saved := requiredFields
	requiredFields = map[string][]string{}
	savedInternal := internalOnlyRequiredFields
	internalOnlyRequiredFields = map[string][]string{}
	t.Cleanup(func() {
		requiredFields = saved
		internalOnlyRequiredFields = savedInternal
	})
}
