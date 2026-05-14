package main

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// tagKinds is the discriminator value ordering used when emitting Tag
// subtypes. Order matters only for yaml stability (oneOf list and
// discriminator.mapping iteration); the protocol is order-insensitive.
var tagKinds = []string{"rfid", "ble", "barcode"}

// tagSubtypeNamesPreRename maps each parent schema (pre-rename, swag-
// emitted dotted form) to the (rfid, ble, barcode) subtype names this
// pass will create.
var tagSubtypeNamesPreRename = map[string]map[string]string{
	"shared.Tag": {
		"rfid":    "shared.RfidTag",
		"ble":     "shared.BleTag",
		"barcode": "shared.BarcodeTag",
	},
	"shared.TagRequest": {
		"rfid":    "shared.RfidTagRequest",
		"ble":     "shared.BleTagRequest",
		"barcode": "shared.BarcodeTagRequest",
	},
}

// splitTagPolymorphism rewrites shared.Tag and shared.TagRequest from
// single schemas into discriminated unions over three named subtypes
// (rfid, ble, barcode). The wire format does not change — every subtype
// keeps the existing field names (id/tag_type/value) and server-side
// Go validation is universal. The split exists so generated clients
// surface RfidTag / BleTag / BarcodeTag (and the request equivalents)
// as named types (TRA-714 / BB33 C1).
//
// Pass ordering:
//
//  1. Runs AFTER markRequiredFields, markReadOnlyFields, markNullableFields,
//     markPrintableStringFields, and closeWriteSchemasToUnknownFields so
//     that each property's attributes are populated on the parent before
//     the split copies them onto subtypes.
//  2. Runs AFTER annotateTagPolymorphism so the schema-level description
//     is in place on the parent (the union inherits it).
//  3. Runs BEFORE hoistInlineEnums — which is told to skip the Tag /
//     TagRequest sites since after the split each subtype carries its
//     own single-value tag_type inline and there is no longer a shared
//     multi-value enum to hoist.
//  4. Runs BEFORE renamePublicSpec — publicSchemaRenames is extended to
//     name each subtype, and rewriteSchemaRefs covers discriminator
//     mapping refs so the mapping entries follow the rename.
//
// Discriminator semantics: OpenAPI requires the discriminator property
// be present in the payload for the type to be unambiguous, so each
// subtype lists tag_type as required even though shared.TagRequest's
// parent has it optional with a server-side rfid default. The server
// still accepts payloads without tag_type and defaults to rfid (wire-
// compatible for any client that omits the field) — this is a spec
// tightening, not a wire-format change.
func splitTagPolymorphism(doc *openapi3.T) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil
	}
	for parentName, subtypeNames := range tagSubtypeNamesPreRename {
		if err := splitOneTagSchema(doc, parentName, subtypeNames); err != nil {
			return err
		}
	}
	return nil
}

func splitOneTagSchema(doc *openapi3.T, parentName string, subtypeNames map[string]string) error {
	parentRef := doc.Components.Schemas[parentName]
	if parentRef == nil || parentRef.Value == nil {
		// Lenient skip mirrors annotateTagPolymorphism: minimal in-memory
		// test fixtures that exercise other postprocess passes need not
		// carry a Tag schema. Real swag emissions always include it; the
		// adjacent passes that depend on Tag (markRequiredFields,
		// markReadOnlyFields, etc.) would already have errored upstream
		// if it were missing.
		return nil
	}
	for _, kind := range tagKinds {
		name := subtypeNames[kind]
		if _, exists := doc.Components.Schemas[name]; exists {
			return fmt.Errorf("apispec: splitTagPolymorphism: subtype schema %q already exists", name)
		}
	}
	src := parentRef.Value
	if _, ok := src.Properties["tag_type"]; !ok {
		return fmt.Errorf("apispec: splitTagPolymorphism: %q has no tag_type property", parentName)
	}
	if _, ok := src.Properties["value"]; !ok {
		return fmt.Errorf("apispec: splitTagPolymorphism: %q has no value property", parentName)
	}

	for _, kind := range tagKinds {
		sub := buildTagSubtype(src, kind)
		doc.Components.Schemas[subtypeNames[kind]] = &openapi3.SchemaRef{Value: sub}
	}

	mapping := openapi3.StringMap[openapi3.MappingRef]{}
	oneOf := make(openapi3.SchemaRefs, 0, len(tagKinds))
	for _, kind := range tagKinds {
		ref := schemaRefPrefix + subtypeNames[kind]
		oneOf = append(oneOf, &openapi3.SchemaRef{Ref: ref})
		mapping[kind] = openapi3.MappingRef{Ref: ref}
	}

	// Replace the map entry with a fresh SchemaRef instead of mutating
	// parentRef.Value in place. partition() shares the Components.Schemas
	// pointer values between the public and internal docs; mutating the
	// existing SchemaRef would leak the union into the internal spec
	// where markRequiredFields still expects the original flat shape.
	doc.Components.Schemas[parentName] = &openapi3.SchemaRef{Value: &openapi3.Schema{
		Description: src.Description,
		OneOf:       oneOf,
		Discriminator: &openapi3.Discriminator{
			PropertyName: "tag_type",
			Mapping:      mapping,
		},
	}}
	return nil
}

// buildTagSubtype builds a single subtype schema for the given kind by
// copying the parent's non-tag_type properties verbatim (carrying any
// pattern / readOnly / format / nullable already set by earlier passes)
// and replacing tag_type with a fresh single-value enum keyed to this
// kind. tag_type and value are always required on the subtype; id is
// included in required only when present on the parent (omitted for
// the request schema).
func buildTagSubtype(src *openapi3.Schema, kind string) *openapi3.Schema {
	props := openapi3.Schemas{}
	for k, v := range src.Properties {
		if k == "tag_type" {
			continue
		}
		props[k] = v
	}
	props["tag_type"] = &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:        &openapi3.Types{openapi3.TypeString},
		Enum:        []any{kind},
		Description: tagTypeFieldDescription,
	}}

	required := make([]string, 0, 3)
	if _, hasID := src.Properties["id"]; hasID {
		required = append(required, "id")
	}
	required = append(required, "tag_type", "value")

	return &openapi3.Schema{
		Type:                 &openapi3.Types{openapi3.TypeObject},
		Description:          src.Description,
		Properties:           props,
		Required:             required,
		AdditionalProperties: src.AdditionalProperties,
	}
}
