package main

import (
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadAndConvert(t *testing.T, path string) *openapi3.T {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	doc3, err := convertV2ToV3(data)
	require.NoError(t, err)
	return doc3
}

func TestPartition_SplitsByTag(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	public, internal, err := partition(doc)
	require.NoError(t, err)

	require.NotNil(t, public.Paths.Value("/assets"), "public spec should contain /assets")
	require.Nil(t, internal.Paths.Value("/assets"), "internal spec should not contain /assets")

	assert.NotContains(t, public.Paths.Value("/assets").Get.Tags, "public",
		"public/internal discriminator tags must be stripped from operations")
	assert.NotContains(t, public.Paths.Value("/assets").Get.Tags, "internal")
	assert.Contains(t, public.Paths.Value("/assets").Get.Tags, "assets",
		"resource tag must be preserved for Redoc grouping")
}

func TestPartition_FailsOnUntaggedOperation(t *testing.T) {
	doc := loadAndConvert(t, "testdata/untagged-v2.json")
	_, _, err := partition(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/assets", "error should name the offending path")
	assert.Contains(t, err.Error(), "GET", "error should name the offending method")
}

func TestPartition_FailsOnBothTags(t *testing.T) {
	doc := loadAndConvert(t, "testdata/both-tags-v2.json")
	_, _, err := partition(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both \"public\" and \"internal\"")
}

func TestPartition_PrunesUnreferencedPublicSchemas(t *testing.T) {
	desc := "OK"
	publicResp := func(ref string) *openapi3.ResponseRef {
		return &openapi3.ResponseRef{Value: &openapi3.Response{
			Description: &desc,
			Content: openapi3.Content{"application/json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{Ref: ref},
			}},
		}}
	}

	// UsedPublic is referenced by a public operation.
	// UsedNested is transitively reachable via UsedPublic.properties.
	// InternalOnly is referenced only by an internal operation.
	// Orphan is referenced by nothing.
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"UsedPublic": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Properties: openapi3.Schemas{
						"nested": &openapi3.SchemaRef{Ref: "#/components/schemas/UsedNested"},
					},
				}},
				"UsedNested":   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
				"InternalOnly": &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
				"Orphan":       &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
			},
		},
	}

	pubResps := openapi3.NewResponsesWithCapacity(1)
	pubResps.Set("200", publicResp("#/components/schemas/UsedPublic"))
	doc.Paths.Set("/public", &openapi3.PathItem{Get: &openapi3.Operation{
		Tags: []string{"public", "res"}, Responses: pubResps,
	}})

	intResps := openapi3.NewResponsesWithCapacity(1)
	intResps.Set("200", publicResp("#/components/schemas/InternalOnly"))
	doc.Paths.Set("/internal", &openapi3.PathItem{Get: &openapi3.Operation{
		Tags: []string{"internal", "res"}, Responses: intResps,
	}})

	public, internal, err := partition(doc)
	require.NoError(t, err)

	require.NotNil(t, public.Components)
	pubSchemas := public.Components.Schemas
	assert.Contains(t, pubSchemas, "UsedPublic", "schema referenced by public operation must be kept")
	assert.Contains(t, pubSchemas, "UsedNested", "transitively-referenced schema must be kept")
	assert.NotContains(t, pubSchemas, "InternalOnly", "schema referenced only by internal operations must be pruned from public")
	assert.NotContains(t, pubSchemas, "Orphan", "unreferenced schema must be pruned from public")

	// Internal spec must be untouched by public pruning.
	require.NotNil(t, internal.Components)
	intSchemas := internal.Components.Schemas
	assert.Contains(t, intSchemas, "InternalOnly")
	assert.Contains(t, intSchemas, "UsedPublic")
	assert.Contains(t, intSchemas, "Orphan")
}
