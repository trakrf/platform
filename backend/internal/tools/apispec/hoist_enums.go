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
}

// enumSource locates an inline enum within a known parent schema by
// pre-rename name (postprocessing runs before renamePublicSpec so the
// dotted Go-package-prefixed names are still in effect here).
type enumSource struct {
	Schema   string
	Property []string // path from the parent schema; e.g. {"error", "type"} for ErrorResponse.error.type
}

// inlineEnumExtractions enumerates every inline enum currently emitted
// by swag. Audit (2026-05-13) found only these four sites; a top-level
// audit on every spec regeneration is unnecessary as long as
// hoistInlineEnums errors when a configured Source does not contain an
// enum.
var inlineEnumExtractions = []inlineEnumExtraction{
	{
		Target: "TagType",
		Sources: []enumSource{
			{Schema: "shared.Tag", Property: []string{"tag_type"}},
			{Schema: "shared.TagRequest", Property: []string{"tag_type"}},
		},
	},
	{
		Target: "ErrorType",
		Sources: []enumSource{
			{Schema: "errors.ErrorResponse", Property: []string{"error", "type"}},
		},
	},
	{
		Target: "FieldErrorCode",
		Sources: []enumSource{
			{Schema: "errors.FieldError", Property: []string{"code"}},
		},
	},
}

// hoistInlineEnums lifts every site listed in inlineEnumExtractions
// into a named top-level schema and replaces the inline definitions
// with $ref. When a source carries `nullable` or `default` (which
// cannot live next to a bare $ref in OpenAPI 3.0), the replacement is
// wrapped in an allOf so the constraint is preserved on the property.
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
				break
			}
			curRef = next
		}
	}

	if canonical == nil || len(sites) == 0 {
		return fmt.Errorf("apispec: hoistInlineEnums: no sites resolved for target %s", ext.Target)
	}

	doc.Components.Schemas[ext.Target] = &openapi3.SchemaRef{Value: canonical}
	refPath := "#/components/schemas/" + ext.Target

	for _, s := range sites {
		old := s.ref.Value
		needsWrapper := old.Nullable || old.Default != nil
		if needsWrapper {
			// OpenAPI 3.0 requires `type` to be present alongside `nullable`
			// (openapi-typescript's redocly validator enforces this strictly;
			// other generators do not, but they don't reject the redundancy
			// either). Copy the underlying type onto the wrapper so the spec
			// validates cleanly across all three codegen targets.
			wrapped := &openapi3.Schema{
				Type:     canonical.Type,
				AllOf:    openapi3.SchemaRefs{{Ref: refPath}},
				Nullable: old.Nullable,
				Default:  old.Default,
			}
			s.owner.Properties[s.key] = &openapi3.SchemaRef{Value: wrapped}
		} else {
			s.owner.Properties[s.key] = &openapi3.SchemaRef{Ref: refPath}
		}
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
