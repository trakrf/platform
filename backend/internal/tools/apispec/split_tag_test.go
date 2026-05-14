package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTagFixture returns a minimal doc carrying the post-field-marking
// shape of shared.Tag and shared.TagRequest as splitTagPolymorphism
// expects to see them.
func buildTagFixture() *openapi3.T {
	stringType := &openapi3.Types{openapi3.TypeString}
	intType := &openapi3.Types{openapi3.TypeInteger}
	f := false
	return &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"shared.Tag": {Value: &openapi3.Schema{
					Type:        &openapi3.Types{openapi3.TypeObject},
					Description: "tag schema description",
					Properties: openapi3.Schemas{
						"id": {Value: &openapi3.Schema{
							Type:     intType,
							Format:   "int32",
							ReadOnly: true,
						}},
						"tag_type": {Value: &openapi3.Schema{
							Type: stringType,
							Enum: []any{"rfid", "ble", "barcode"},
						}},
						"value": {Value: &openapi3.Schema{
							Type:      stringType,
							MinLength: 1,
							MaxLength: ptr64(255),
							Pattern:   "^[^\\x00-\\x08\\x0B\\x0C\\x0E-\\x1F\\x7F]*$",
						}},
					},
					Required: []string{"id", "tag_type", "value"},
				}},
				"shared.TagRequest": {Value: &openapi3.Schema{
					Type:                 &openapi3.Types{openapi3.TypeObject},
					Description:          "tag request schema description",
					AdditionalProperties: openapi3.AdditionalProperties{Has: &f},
					Properties: openapi3.Schemas{
						"tag_type": {Value: &openapi3.Schema{
							Type:     stringType,
							Enum:     []any{"rfid", "ble", "barcode"},
							Nullable: true,
							Default:  "rfid",
						}},
						"value": {Value: &openapi3.Schema{
							Type:      stringType,
							MinLength: 1,
							MaxLength: ptr64(255),
							Pattern:   "^[^\\x00-\\x08\\x0B\\x0C\\x0E-\\x1F\\x7F]*$",
						}},
					},
					Required: []string{"value"},
				}},
			},
		},
	}
}

func ptr64(v uint64) *uint64 { return &v }

// TestSplitTagPolymorphism_RewritesParentAsOneOfUnion verifies the core
// invariant: shared.Tag becomes a oneOf of three subtypes with a
// discriminator on tag_type. Wire shape of the parent (id/tag_type/value)
// is preserved across each subtype.
func TestSplitTagPolymorphism_RewritesParentAsOneOfUnion(t *testing.T) {
	doc := buildTagFixture()
	require.NoError(t, splitTagPolymorphism(doc))

	parent := doc.Components.Schemas["shared.Tag"]
	require.NotNil(t, parent)
	require.NotNil(t, parent.Value)
	assert.Empty(t, parent.Value.Properties, "union parent should not declare properties")
	assert.Equal(t, "tag schema description", parent.Value.Description, "union inherits description")

	require.Len(t, parent.Value.OneOf, 3)
	gotRefs := make([]string, 0, 3)
	for _, r := range parent.Value.OneOf {
		gotRefs = append(gotRefs, r.Ref)
	}
	assert.ElementsMatch(t, []string{
		"#/components/schemas/shared.RfidTag",
		"#/components/schemas/shared.BleTag",
		"#/components/schemas/shared.BarcodeTag",
	}, gotRefs)

	require.NotNil(t, parent.Value.Discriminator)
	assert.Equal(t, "tag_type", parent.Value.Discriminator.PropertyName)
	require.Len(t, parent.Value.Discriminator.Mapping, 3)
	assert.Equal(t, "#/components/schemas/shared.RfidTag", parent.Value.Discriminator.Mapping["rfid"].Ref)
	assert.Equal(t, "#/components/schemas/shared.BleTag", parent.Value.Discriminator.Mapping["ble"].Ref)
	assert.Equal(t, "#/components/schemas/shared.BarcodeTag", parent.Value.Discriminator.Mapping["barcode"].Ref)
}

// TestSplitTagPolymorphism_SubtypesCarryFlatShape verifies each subtype
// carries id/tag_type/value with the discriminator value pinned and the
// parent's printable-string pattern intact on value.
func TestSplitTagPolymorphism_SubtypesCarryFlatShape(t *testing.T) {
	doc := buildTagFixture()
	require.NoError(t, splitTagPolymorphism(doc))

	for kind, name := range map[string]string{
		"rfid":    "shared.RfidTag",
		"ble":     "shared.BleTag",
		"barcode": "shared.BarcodeTag",
	} {
		sub := doc.Components.Schemas[name]
		require.NotNil(t, sub, "subtype %q must exist", name)
		require.NotNil(t, sub.Value)
		require.NotNil(t, sub.Value.Type)
		assert.True(t, sub.Value.Type.Is(openapi3.TypeObject))
		assert.ElementsMatch(t, []string{"id", "tag_type", "value"}, sub.Value.Required)

		tagTypeProp := sub.Value.Properties["tag_type"]
		require.NotNil(t, tagTypeProp)
		require.NotNil(t, tagTypeProp.Value)
		assert.Equal(t, []any{kind}, tagTypeProp.Value.Enum, "subtype tag_type must be locked to its kind")
		assert.True(t, tagTypeProp.Value.Type.Is(openapi3.TypeString))

		valueProp := sub.Value.Properties["value"]
		require.NotNil(t, valueProp)
		require.NotNil(t, valueProp.Value)
		assert.Equal(t, "^[^\\x00-\\x08\\x0B\\x0C\\x0E-\\x1F\\x7F]*$", valueProp.Value.Pattern,
			"value pattern must carry through to each subtype")

		idProp := sub.Value.Properties["id"]
		require.NotNil(t, idProp)
		require.NotNil(t, idProp.Value)
		assert.True(t, idProp.Value.ReadOnly, "id readOnly must carry through to each subtype")
	}
}

// TestSplitTagPolymorphism_RequestSubtypesRequireDiscriminator covers the
// request side: tag_type becomes required on each subtype even though the
// parent shared.TagRequest only required value. additionalProperties:
// false is propagated to each subtype to keep DisallowUnknownFields parity.
func TestSplitTagPolymorphism_RequestSubtypesRequireDiscriminator(t *testing.T) {
	doc := buildTagFixture()
	require.NoError(t, splitTagPolymorphism(doc))

	for _, name := range []string{"shared.RfidTagRequest", "shared.BleTagRequest", "shared.BarcodeTagRequest"} {
		sub := doc.Components.Schemas[name]
		require.NotNil(t, sub, "subtype %q must exist", name)
		require.NotNil(t, sub.Value)
		assert.ElementsMatch(t, []string{"tag_type", "value"}, sub.Value.Required,
			"request subtype must require both discriminator and value")
		_, hasID := sub.Value.Properties["id"]
		assert.False(t, hasID, "request subtype must not surface id")
		require.NotNil(t, sub.Value.AdditionalProperties.Has)
		assert.False(t, *sub.Value.AdditionalProperties.Has,
			"request subtype must inherit additionalProperties: false")
	}
}

// TestSplitTagPolymorphism_IsAtomicOnMissingProperty verifies the split
// errors loudly when the parent lacks the expected wire fields rather
// than producing a malformed union.
func TestSplitTagPolymorphism_IsAtomicOnMissingProperty(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "v1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"shared.Tag": {Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"id": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
					},
				}},
			},
		},
	}
	err := splitTagPolymorphism(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag_type")
}

// TestSplitTagPolymorphism_RejectsSubtypeCollision catches the
// double-run / stale-config case where a target subtype name is
// already present.
func TestSplitTagPolymorphism_RejectsSubtypeCollision(t *testing.T) {
	doc := buildTagFixture()
	doc.Components.Schemas["shared.RfidTag"] = &openapi3.SchemaRef{Value: &openapi3.Schema{}}
	err := splitTagPolymorphism(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestRewriteSchemaRefs_RewritesDiscriminatorMapping is the rename guard:
// publicSchemaRenames must follow through into discriminator mapping refs,
// not just $ref strings. Without this, mapping entries point to the
// pre-rename subtype names and codegen breaks.
func TestRewriteSchemaRefs_RewritesDiscriminatorMapping(t *testing.T) {
	doc := buildTagFixture()
	require.NoError(t, splitTagPolymorphism(doc))

	doc.Components.Schemas["shared.RfidTag"] = doc.Components.Schemas["shared.RfidTag"]
	doc.Components.Schemas["RfidTag"] = doc.Components.Schemas["shared.RfidTag"]
	doc.Components.Schemas["BleTag"] = doc.Components.Schemas["shared.BleTag"]
	doc.Components.Schemas["BarcodeTag"] = doc.Components.Schemas["shared.BarcodeTag"]
	delete(doc.Components.Schemas, "shared.RfidTag")
	delete(doc.Components.Schemas, "shared.BleTag")
	delete(doc.Components.Schemas, "shared.BarcodeTag")

	renames := map[string]string{
		"shared.RfidTag":    "RfidTag",
		"shared.BleTag":     "BleTag",
		"shared.BarcodeTag": "BarcodeTag",
	}
	rewriteSchemaRefs(doc, renames)

	parent := doc.Components.Schemas["shared.Tag"]
	require.NotNil(t, parent)
	require.NotNil(t, parent.Value)
	require.NotNil(t, parent.Value.Discriminator)

	assert.Equal(t, "#/components/schemas/RfidTag", parent.Value.Discriminator.Mapping["rfid"].Ref,
		"discriminator mapping must follow the rename")
	assert.Equal(t, "#/components/schemas/BleTag", parent.Value.Discriminator.Mapping["ble"].Ref)
	assert.Equal(t, "#/components/schemas/BarcodeTag", parent.Value.Discriminator.Mapping["barcode"].Ref)

	// The oneOf refs must also follow.
	gotOneOf := make([]string, 0, 3)
	for _, r := range parent.Value.OneOf {
		gotOneOf = append(gotOneOf, r.Ref)
	}
	assert.ElementsMatch(t, []string{
		"#/components/schemas/RfidTag",
		"#/components/schemas/BleTag",
		"#/components/schemas/BarcodeTag",
	}, gotOneOf)
}
