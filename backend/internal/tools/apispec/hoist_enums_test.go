package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringSchema returns a small string-enum schema for fixture use.
func stringSchema(enum []any, nullable bool, deflt any) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:     &openapi3.Types{openapi3.TypeString},
		Enum:     enum,
		Nullable: nullable,
		Default:  deflt,
	}}
}

// TestHoistInlineEnums_LiftsDuplicateSiblingEnum is the TRA-691 regression
// guard: two sibling schemas declare an inline enum on the same property
// name, which makes openapi-generator's Go target emit a collision
// compile error. After hoisting, every site becomes a bare $ref to the
// named component; nullable/default carried by any source are merged
// onto the canonical schema (TRA-712 / BB33 F6 — keeping them as
// siblings of $ref/allOf produces the OAS-3.0 allOf-with-siblings form
// that Pydantic-strict Python codegen rejects).
func TestHoistInlineEnums_LiftsDuplicateSiblingEnum(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"shared.Tag": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"tag_type": stringSchema([]any{"rfid", "ble", "barcode"}, false, nil),
					},
				}},
				"shared.TagRequest": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"tag_type": stringSchema([]any{"rfid", "ble", "barcode"}, true, "rfid"),
					},
				}},
			},
		},
	}

	saved := inlineEnumExtractions
	inlineEnumExtractions = []inlineEnumExtraction{{
		Target: "TagType",
		Sources: []enumSource{
			{Schema: "shared.Tag", Property: []string{"tag_type"}},
			{Schema: "shared.TagRequest", Property: []string{"tag_type"}},
		},
	}}
	t.Cleanup(func() { inlineEnumExtractions = saved })

	require.NoError(t, hoistInlineEnums(doc))

	tagType := doc.Components.Schemas["TagType"]
	require.NotNil(t, tagType, "named TagType schema must be created")
	require.NotNil(t, tagType.Value)
	assert.True(t, tagType.Value.Type.Is(openapi3.TypeString))
	assert.Equal(t, []any{"rfid", "ble", "barcode"}, tagType.Value.Enum)
	// nullable/default from the TagRequest source are merged onto the
	// canonical so the call sites stay sibling-free.
	assert.True(t, tagType.Value.Nullable, "canonical inherits nullable from any source")
	assert.Equal(t, "rfid", tagType.Value.Default, "canonical inherits default from any source")

	tagSite := doc.Components.Schemas["shared.Tag"].Value.Properties["tag_type"]
	assert.Equal(t, "#/components/schemas/TagType", tagSite.Ref, "bare $ref at the call site")
	assert.Nil(t, tagSite.Value, "bare $ref must not carry an inline value")

	reqSite := doc.Components.Schemas["shared.TagRequest"].Value.Properties["tag_type"]
	assert.Equal(t, "#/components/schemas/TagType", reqSite.Ref, "bare $ref at the call site")
	assert.Nil(t, reqSite.Value, "bare $ref must not carry sibling nullable/default/type")
}

// TestHoistInlineEnums_NestedPath walks into nested property paths
// (ErrorResponse.error.type) for hoisting deeply nested inline enums.
func TestHoistInlineEnums_NestedPath(t *testing.T) {
	errorObj := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{openapi3.TypeObject},
		Properties: openapi3.Schemas{
			"type": stringSchema([]any{"validation_error", "bad_request"}, false, nil),
		},
	}}
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"errors.ErrorResponse": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"error": errorObj,
					},
				}},
			},
		},
	}

	saved := inlineEnumExtractions
	inlineEnumExtractions = []inlineEnumExtraction{{
		Target: "ErrorType",
		Sources: []enumSource{
			{Schema: "errors.ErrorResponse", Property: []string{"error", "type"}},
		},
	}}
	t.Cleanup(func() { inlineEnumExtractions = saved })

	require.NoError(t, hoistInlineEnums(doc))

	require.NotNil(t, doc.Components.Schemas["ErrorType"])
	nested := doc.Components.Schemas["errors.ErrorResponse"].Value.
		Properties["error"].Value.
		Properties["type"]
	assert.Equal(t, "#/components/schemas/ErrorType", nested.Ref)
}

// TestHoistInlineEnums_RejectsDivergentEnumValues is the rename-drift
// guard: if two sources for the same target carry different enum values,
// fail the build rather than silently picking one set.
func TestHoistInlineEnums_RejectsDivergentEnumValues(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"shared.Tag": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"tag_type": stringSchema([]any{"rfid", "ble"}, false, nil),
					},
				}},
				"shared.TagRequest": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"tag_type": stringSchema([]any{"rfid", "ble", "barcode"}, false, nil),
					},
				}},
			},
		},
	}

	saved := inlineEnumExtractions
	inlineEnumExtractions = []inlineEnumExtraction{{
		Target: "TagType",
		Sources: []enumSource{
			{Schema: "shared.Tag", Property: []string{"tag_type"}},
			{Schema: "shared.TagRequest", Property: []string{"tag_type"}},
		},
	}}
	t.Cleanup(func() { inlineEnumExtractions = saved })

	err := hoistInlineEnums(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diverge")
}

// TestHoistInlineEnums_ErrorsOnUnknownSource catches stale config that
// references a renamed source schema. Same lenient-vs-strict trade-off
// as markRequiredFields — drift breaks the build.
func TestHoistInlineEnums_ErrorsOnUnknownSource(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"other.Schema": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}},
			},
		},
	}

	saved := inlineEnumExtractions
	inlineEnumExtractions = []inlineEnumExtraction{{
		Target: "TagType",
		Sources: []enumSource{
			{Schema: "shared.Tag", Property: []string{"tag_type"}},
		},
	}}
	t.Cleanup(func() { inlineEnumExtractions = saved })

	err := hoistInlineEnums(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shared.Tag")
}
