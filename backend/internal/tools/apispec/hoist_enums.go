package main

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// inlineEnumExtraction names a top-level enum schema to hoist out of
// one or more inline-enum sites.
//
// Why: openapi-generator's Go target emits a sibling-collision compile
// error ("undefined: TAG_TYPE") when two sibling schemas declare inline
// string enums with the same property name (Tag.tag_type +
// TagRequest.tag_type). The TRA-691 codegen smoke gate caught it.
// Lifting each inline enum to a named top-level schema and replacing
// the inline definition with a $ref makes the generated Go module
// compile and matches the convention expected by other generators
// (Python's openapi-generator and openapi-typescript both emit cleaner
// types from $ref'd enums than from sibling inline enums).
//
// Codegen consequence: client callers receive a named enum type
// (TagType, ErrorType, FieldErrorCode) instead of an inline string
// enum. These names are part of the generated SDK surface — adding new
// entries here is API-additive; renaming an existing target name is
// API-breaking.
type inlineEnumExtraction struct {
	// Target is the new component schema name in components.schemas.
	Target string
	// Sources lists each inline-enum site to lift into Target.
	Sources []enumSource
	// Description, when non-empty, overwrites the description on the
	// hoisted schema. Generators emit this as a docstring on the typed
	// enum, so every named-enum schema should carry one — leaving a
	// generated SDK type undocumented is worse than the original inline
	// string surface that at least let callers see the JSON example.
	Description string
}

// enumSource locates an inline enum within a known parent schema by
// pre-rename name (postprocessing runs before renamePublicSpec so the
// dotted Go-package-prefixed names are still in effect here).
type enumSource struct {
	Schema   string
	Property []string // path from the parent schema; e.g. {"error", "type"} for ErrorResponse.error.type
}

// inlineEnumExtractions enumerates every inline enum currently emitted
// by swag that needs hoisting to a named top-level schema. shared.Tag
// and shared.TagRequest used to surface their tag_type enum here, but
// splitTagPolymorphism (TRA-714) runs first and rewrites each parent
// into a discriminated union; the post-split subtypes (RfidTag /
// BleTag / BarcodeTag and the request equivalents) each carry their
// own single-value tag_type enum inline, which is the idiomatic
// discriminator shape and produces distinct generated constant names
// per subtype so the Go-codegen sibling collision that drove the
// original hoist (TRA-691) does not apply.
var inlineEnumExtractions = []inlineEnumExtraction{
	{
		Target:      "ErrorType",
		Description: "Machine-readable error envelope discriminator. Pairs with `title` and `detail` to drive client-side branching on a stable token (per RFC 9457).",
		Sources: []enumSource{
			{Schema: "errors.ErrorResponse", Property: []string{"error", "type"}},
		},
	},
	{
		Target:      "FieldErrorCode",
		Description: "Machine-readable field-level validation error code. Identifies which constraint a request payload violated; pairs with the field path and human-readable message in a FieldError entry.",
		Sources: []enumSource{
			{Schema: "errors.FieldError", Property: []string{"code"}},
		},
	},
}

// hoistInlineEnums lifts every site listed in inlineEnumExtractions
// into a named top-level schema and replaces the inline definitions
// with bare $ref. `nullable` and `default` from any source are merged
// onto the canonical (hoisted) schema rather than left as siblings of
// the $ref — siblings of $ref/allOf trip OpenAPI 3.0 strict readers
// (Pydantic-strict Python codegen emits a serialization warning on
// every model use; TRA-712 / BB33 F6).
//
// Tradeoff: nullable/default declared on the shared schema applies to
// every site that refs it. For TagType this means Tag.tag_type — which
// the server never returns null and treats as required — admits null
// per the spec. Documentation looseness is acceptable pre-launch; the
// alternative is a separate per-site schema, which re-introduces the
// enum-constant collision in Go codegen that the hoist exists to
// prevent (TRA-691).
//
// Must run before renamePublicSpec so the source schema names (which
// still carry their dotted Go-package prefix) resolve.
func hoistInlineEnums(doc *openapi3.T) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil
	}
	for _, ext := range inlineEnumExtractions {
		if err := hoistOneEnum(doc, ext); err != nil {
			return err
		}
	}
	return nil
}

type enumSite struct {
	owner *openapi3.Schema
	key   string
	ref   *openapi3.SchemaRef
}

func hoistOneEnum(doc *openapi3.T, ext inlineEnumExtraction) error {
	if _, exists := doc.Components.Schemas[ext.Target]; exists {
		return fmt.Errorf("apispec: hoistInlineEnums: target schema %q already exists", ext.Target)
	}

	var canonical *openapi3.Schema
	sites := make([]enumSite, 0, len(ext.Sources))

	for _, src := range ext.Sources {
		schemaRef, ok := doc.Components.Schemas[src.Schema]
		if !ok || schemaRef == nil || schemaRef.Value == nil {
			return fmt.Errorf("apispec: hoistInlineEnums: source schema %q not found (target %s)", src.Schema, ext.Target)
		}
		if len(src.Property) == 0 {
			return fmt.Errorf("apispec: hoistInlineEnums: empty property path for target %s", ext.Target)
		}

		curRef := schemaRef
		for i, key := range src.Property {
			if curRef.Value == nil || curRef.Value.Properties == nil {
				return fmt.Errorf("apispec: hoistInlineEnums: cannot descend %s.%v at %q", src.Schema, src.Property, key)
			}
			next, ok := curRef.Value.Properties[key]
			if !ok || next == nil {
				return fmt.Errorf("apispec: hoistInlineEnums: property %q missing in %s.%v", key, src.Schema, src.Property)
			}
			if i == len(src.Property)-1 {
				if next.Value == nil || len(next.Value.Enum) == 0 {
					return fmt.Errorf("apispec: hoistInlineEnums: %s.%v is not an inline enum", src.Schema, src.Property)
				}
				sites = append(sites, enumSite{owner: curRef.Value, key: key, ref: next})
				if canonical == nil {
					canonical = cloneEnumSchema(next.Value)
				} else if !enumValuesEqual(canonical.Enum, next.Value.Enum) {
					return fmt.Errorf("apispec: hoistInlineEnums: enum values diverge across sources for target %s", ext.Target)
				}
				// Merge nullable/default from this source onto canonical so
				// the constraint lives inside the referenced schema instead
				// of becoming a $ref sibling at the call site.
				if next.Value.Nullable {
					canonical.Nullable = true
				}
				if next.Value.Default != nil && canonical.Default == nil {
					canonical.Default = next.Value.Default
				}
				break
			}
			curRef = next
		}
	}

	if canonical == nil || len(sites) == 0 {
		return fmt.Errorf("apispec: hoistInlineEnums: no sites resolved for target %s", ext.Target)
	}

	if ext.Description != "" {
		canonical.Description = ext.Description
	}
	doc.Components.Schemas[ext.Target] = &openapi3.SchemaRef{Value: canonical}
	refPath := "#/components/schemas/" + ext.Target

	for _, s := range sites {
		s.owner.Properties[s.key] = &openapi3.SchemaRef{Ref: refPath}
	}
	return nil
}

func cloneEnumSchema(s *openapi3.Schema) *openapi3.Schema {
	out := &openapi3.Schema{
		Type:        s.Type,
		Description: s.Description,
		Example:     s.Example,
	}
	if len(s.Enum) > 0 {
		out.Enum = append([]any{}, s.Enum...)
	}
	if len(s.Extensions) > 0 {
		out.Extensions = make(map[string]any, len(s.Extensions))
		for k, v := range s.Extensions {
			out.Extensions[k] = v
		}
	}
	return out
}

func enumValuesEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
