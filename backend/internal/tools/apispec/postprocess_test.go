package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostprocess_RewritesBearerAuthToHTTPBearer(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// minimal fixture carries only APIKey; synthesize BearerAuth the way
	// swaggo emits it so we can verify the rewrite.
	doc.Components.SecuritySchemes["BearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type: "apiKey", In: "header", Name: "Authorization",
			Description: "Session JWT for internal endpoints.",
		},
	}
	postprocessPublic(doc)

	scheme := doc.Components.SecuritySchemes["BearerAuth"]
	require.NotNil(t, scheme, "BearerAuth scheme must be present")
	require.NotNil(t, scheme.Value)

	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
	assert.Contains(t, scheme.Value.Description, "Session JWT")
}

// TestPostprocess_PreservesAPIKeyApiKeyType guards the TRA-480 §3.3 fix:
// swaggo emits APIKey with type=apiKey, which matches the literal name. We
// must NOT rewrite it — a prior implementation promoted APIKey to
// http/bearer, which confused SDK generators that lifted the name into
// identifiers implying an apiKey type.
func TestPostprocess_PreservesAPIKeyApiKeyType(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	scheme := doc.Components.SecuritySchemes["APIKey"]
	require.NotNil(t, scheme)
	require.NotNil(t, scheme.Value)
	assert.Equal(t, "apiKey", scheme.Value.Type, "APIKey scheme name implies apiKey type — keep them aligned")
	assert.Equal(t, "header", scheme.Value.In)
	assert.Equal(t, "Authorization", scheme.Value.Name)
}

func TestPostprocess_SetsPublicInfoAndServers(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	assert.Equal(t, "TrakRF API", doc.Info.Title)
	assert.Equal(t, "v1", doc.Info.Version)
	require.Len(t, doc.Servers, 2)
	assert.Equal(t, "https://app.trakrf.id", doc.Servers[0].URL,
		"production server must be app.trakrf.id — the marketing site at trakrf.id does not serve /api/v1/*")
	assert.Equal(t, "Production", doc.Servers[0].Description)
	assert.Equal(t, "https://app.preview.trakrf.id", doc.Servers[1].URL)
}

func TestPostprocess_SetsInternalInfoAndServers(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessInternal(doc)

	assert.Equal(t, "TrakRF Internal API — not for customer use", doc.Info.Title)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "http://localhost:8080", doc.Servers[0].URL)
	assert.Equal(t, "Local development", doc.Servers[0].Description)
}

func TestPostprocess_NormalizesMetadataEmptySchema(t *testing.T) {
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

func TestPostprocess_AddsDateTimeFormatToTimestampFields(t *testing.T) {
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
