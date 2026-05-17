package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenamePublicSpec_RenamesSchemaKeysAndRefs covers TRA-660 / BB25 C1:
// schemas under dotted Go-package prefixes (asset.PublicAssetView,
// errors.ErrorResponse) get renamed to clean PascalCase, and every $ref
// in the document is rewritten to point at the new key.
func TestRenamePublicSpec_RenamesSchemaKeysAndRefs(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
		}},
		"errors.ErrorResponse": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
		}},
		"asset.GetAssetResponse": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: openapi3.Schemas{
				"data": &openapi3.SchemaRef{Ref: "#/components/schemas/asset.PublicAssetView"},
			},
		}},
	})

	require.NoError(t, renamePublicSpec(doc))

	assert.Contains(t, doc.Components.Schemas, "AssetView")
	assert.Contains(t, doc.Components.Schemas, "ErrorResponse")
	assert.Contains(t, doc.Components.Schemas, "GetAssetResponse")
	assert.NotContains(t, doc.Components.Schemas, "asset.PublicAssetView")
	assert.NotContains(t, doc.Components.Schemas, "errors.ErrorResponse")
	assert.NotContains(t, doc.Components.Schemas, "asset.GetAssetResponse")

	dataRef := doc.Components.Schemas["GetAssetResponse"].Value.Properties["data"]
	assert.Equal(t, "#/components/schemas/AssetView", dataRef.Ref,
		"$ref inside renamed schema must point at the renamed target")
}

// TestRenamePublicSpec_RenamesOperationIds verifies dotted operationIds
// from swag (assets.create, locations.tags.add, reports.asset-locations)
// are rewritten to camelCase verbResource form.
func TestRenamePublicSpec_RenamesOperationIds(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{})
	doc.Paths.Set("/api/v1/assets", &openapi3.PathItem{
		Get:  &openapi3.Operation{OperationID: "assets.list"},
		Post: &openapi3.Operation{OperationID: "assets.create"},
	})
	doc.Paths.Set("/api/v1/locations/{id}/tags", &openapi3.PathItem{
		Post: &openapi3.Operation{OperationID: "locations.tags.add"},
	})
	doc.Paths.Set("/api/v1/reports/asset-locations", &openapi3.PathItem{
		Get: &openapi3.Operation{OperationID: "reports.asset-locations"},
	})

	require.NoError(t, renamePublicSpec(doc))

	assert.Equal(t, "listAssets", doc.Paths.Find("/api/v1/assets").Get.OperationID)
	assert.Equal(t, "createAsset", doc.Paths.Find("/api/v1/assets").Post.OperationID)
	assert.Equal(t, "addLocationTag", doc.Paths.Find("/api/v1/locations/{id}/tags").Post.OperationID)
	assert.Equal(t, "listAssetLocations", doc.Paths.Find("/api/v1/reports/asset-locations").Get.OperationID)
}

// TestRenamePublicSpec_AddsTopLevelTagDescriptions verifies the top-level
// tags array is populated with descriptions for each resource grouping.
// Tag names already on operations are not duplicated in the top-level
// list; the pass only adds the description metadata.
func TestRenamePublicSpec_AddsTopLevelTagDescriptions(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{})
	require.NoError(t, renamePublicSpec(doc))

	names := map[string]string{}
	for _, tag := range doc.Tags {
		require.NotNil(t, tag)
		names[tag.Name] = tag.Description
	}
	assert.Contains(t, names, "assets")
	assert.Contains(t, names, "locations")
	assert.Contains(t, names, "orgs")
	assert.Contains(t, names, "reports")
	assert.NotEmpty(t, names["assets"], "assets tag must carry a description")
}

// TestRenamePublicSpec_TopLevelTagsIdempotent verifies that an already-set
// tag with the same name is left alone; the pass appends only missing
// names, so a re-run does not duplicate or overwrite custom descriptions.
func TestRenamePublicSpec_TopLevelTagsIdempotent(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{})
	doc.Tags = openapi3.Tags{
		{Name: "assets", Description: "custom-by-test"},
	}
	require.NoError(t, renamePublicSpec(doc))

	custom := 0
	for _, tag := range doc.Tags {
		if tag != nil && tag.Name == "assets" && tag.Description == "custom-by-test" {
			custom++
		}
	}
	assert.Equal(t, 1, custom, "existing assets tag must be preserved exactly once")
}

// TestRenamePublicSpec_CollisionInTargetErrors verifies the safety guard:
// if two source schemas would rename to the same target name, the rename
// fails loudly instead of silently overwriting one of the sources.
func TestRenamePublicSpec_CollisionInTargetErrors(t *testing.T) {
	defer func(orig map[string]string) { publicSchemaRenames = orig }(publicSchemaRenames)
	publicSchemaRenames = map[string]string{
		"src.A": "Target",
		"src.B": "Target",
	}
	doc := docWithSchemas(openapi3.Schemas{
		"src.A": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
		"src.B": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
	})
	err := renamePublicSpec(doc)
	require.Error(t, err, "two sources renaming to the same target must error")
	assert.Contains(t, err.Error(), "Target")
}

// TestRenamePublicSpec_PreExistingTargetErrors verifies that if a target
// name already exists in the schema map (a hand-written schema with the
// new name shape), the rename fails loudly to avoid silent overwrite.
func TestRenamePublicSpec_PreExistingTargetErrors(t *testing.T) {
	defer func(orig map[string]string) { publicSchemaRenames = orig }(publicSchemaRenames)
	publicSchemaRenames = map[string]string{"src.X": "ExistingTarget"}
	doc := docWithSchemas(openapi3.Schemas{
		"src.X":          &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
		"ExistingTarget": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
	})
	err := renamePublicSpec(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ExistingTarget")
}

// TestRenamePublicSpec_LeavesUnmappedSchemasUntouched verifies that
// schemas not in the rename map are left as-is.
func TestRenamePublicSpec_LeavesUnmappedSchemasUntouched(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{
		"asset.PublicAssetView": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
		"unrelated.Thing":       &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
	})
	require.NoError(t, renamePublicSpec(doc))
	assert.Contains(t, doc.Components.Schemas, "AssetView", "mapped schema renamed")
	assert.Contains(t, doc.Components.Schemas, "unrelated.Thing", "unmapped schema preserved as-is")
}

// TestRenamePublicSpec_NoMappedSchemasNoOp verifies the pass does nothing
// destructive when none of the schemas are in the rename map.
func TestRenamePublicSpec_NoMappedSchemasNoOp(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{
		"unrelated.Thing": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
	})
	require.NoError(t, renamePublicSpec(doc))
	assert.Contains(t, doc.Components.Schemas, "unrelated.Thing")
	assert.Len(t, doc.Components.Schemas, 1)
}

// TestRenamePublicSpec_RewritesInjectedResponseRefs verifies that the
// rename pass also rewrites $refs inside Components.Responses — the
// MethodNotAllowed and Gone responses are injected by sibling passes
// with a $ref to errors.ErrorResponse, and the rename must follow.
func TestRenamePublicSpec_RewritesInjectedResponseRefs(t *testing.T) {
	doc := docWithSchemas(openapi3.Schemas{
		"errors.ErrorResponse": &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
		}},
	})
	desc := "Method not allowed"
	doc.Components.Responses = openapi3.ResponseBodies{
		"MethodNotAllowed": &openapi3.ResponseRef{Value: &openapi3.Response{
			Description: &desc,
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/errors.ErrorResponse"},
				},
			},
		}},
	}

	require.NoError(t, renamePublicSpec(doc))

	media := doc.Components.Responses["MethodNotAllowed"].Value.Content["application/json"]
	assert.Equal(t, "#/components/schemas/ErrorResponse", media.Schema.Ref,
		"$ref inside an injected response must be rewritten by the rename pass")
}
