package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	tagPublic   = "public"
	tagInternal = "internal"
)

// partition splits doc into a public spec (operations tagged "public") and
// an internal spec (operations tagged "internal"). Each operation must have
// exactly one of the two discriminator tags; the discriminator is stripped
// from the output so Redoc's tag-based grouping uses only resource tags.
func partition(doc *openapi3.T) (public, internal *openapi3.T, err error) {
	public = cloneDocShell(doc)
	internal = cloneDocShell(doc)

	var violations []string
	for path, item := range doc.Paths.Map() {
		publicItem := &openapi3.PathItem{}
		internalItem := &openapi3.PathItem{}
		hasPublic, hasInternal := false, false

		for method, op := range item.Operations() {
			isPublic := slices.Contains(op.Tags, tagPublic)
			isInternal := slices.Contains(op.Tags, tagInternal)
			switch {
			case isPublic && isInternal:
				violations = append(violations, fmt.Sprintf("%s %s: has both \"public\" and \"internal\" tags (must have exactly one)", method, path))
			case !isPublic && !isInternal:
				violations = append(violations, fmt.Sprintf("%s %s: missing \"public\" or \"internal\" tag", method, path))
			case isPublic:
				publicItem.SetOperation(method, stripDiscriminator(op))
				hasPublic = true
			case isInternal:
				internalItem.SetOperation(method, stripDiscriminator(op))
				hasInternal = true
			}
		}
		if hasPublic {
			public.Paths.Set(path, publicItem)
		}
		if hasInternal {
			internal.Paths.Set(path, internalItem)
		}
	}

	if len(violations) > 0 {
		return nil, nil, fmt.Errorf("tag validation failed:\n  %s", strings.Join(violations, "\n  "))
	}

	prunePublicSchemas(public)
	return public, internal, nil
}

const schemaRefPrefix = "#/components/schemas/"

// prunePublicSchemas drops schemas from public.Components.Schemas that are
// not reachable from any public operation. swaggo emits a single Components
// map covering every handler in the binary; after operation partitioning,
// internal-only types remain in the public spec as orphans and leak in the
// published docs. We walk the public paths, collect every schema ref
// transitively, and replace Schemas with the reachable subset. A fresh
// Components struct is allocated so the internal spec keeps its full map.
func prunePublicSchemas(public *openapi3.T) {
	if public.Components == nil || public.Components.Schemas == nil {
		return
	}
	schemas := public.Components.Schemas
	reachable := make(map[string]bool, len(schemas))

	var visit func(ref *openapi3.SchemaRef)
	visit = func(ref *openapi3.SchemaRef) {
		if ref == nil {
			return
		}
		if name, ok := strings.CutPrefix(ref.Ref, schemaRefPrefix); ok {
			if reachable[name] {
				return
			}
			reachable[name] = true
			if target, found := schemas[name]; found {
				visit(target)
			}
		}
		s := ref.Value
		if s == nil {
			return
		}
		for _, r := range s.OneOf {
			visit(r)
		}
		for _, r := range s.AnyOf {
			visit(r)
		}
		for _, r := range s.AllOf {
			visit(r)
		}
		visit(s.Not)
		visit(s.Items)
		for _, r := range s.Properties {
			visit(r)
		}
		if s.AdditionalProperties.Schema != nil {
			visit(s.AdditionalProperties.Schema)
		}
	}

	for _, item := range public.Paths.Map() {
		for _, op := range item.Operations() {
			for _, p := range op.Parameters {
				if p == nil || p.Value == nil {
					continue
				}
				visit(p.Value.Schema)
				for _, mt := range p.Value.Content {
					if mt != nil {
						visit(mt.Schema)
					}
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, mt := range op.RequestBody.Value.Content {
					if mt != nil {
						visit(mt.Schema)
					}
				}
			}
			if op.Responses == nil {
				continue
			}
			for _, resp := range op.Responses.Map() {
				if resp == nil || resp.Value == nil {
					continue
				}
				for _, h := range resp.Value.Headers {
					if h != nil && h.Value != nil {
						visit(h.Value.Schema)
					}
				}
				for _, mt := range resp.Value.Content {
					if mt != nil {
						visit(mt.Schema)
					}
				}
			}
		}
	}

	pruned := make(openapi3.Schemas, len(reachable))
	for name, ref := range schemas {
		if reachable[name] {
			pruned[name] = ref
		}
	}
	comps := *public.Components
	comps.Schemas = pruned
	public.Components = &comps
}

// cloneDocShell returns a copy of doc with an empty Paths map and an
// independently-owned Info pointer, so postprocessPublic and postprocessInternal
// don't clobber each other's Info.Title. Components and securitySchemes remain
// shared; postprocessing replaces leaf values (including the APIKey scheme ref),
// not the containers.
func cloneDocShell(doc *openapi3.T) *openapi3.T {
	shell := *doc
	shell.Paths = openapi3.NewPaths()
	if doc.Info != nil {
		info := *doc.Info
		shell.Info = &info
	}
	shell.Servers = nil
	return &shell
}

// stripDiscriminator returns a copy of op with the "public"/"internal" tags
// removed from its Tags slice. Resource tags are preserved.
func stripDiscriminator(op *openapi3.Operation) *openapi3.Operation {
	out := *op
	out.Tags = slices.DeleteFunc(slices.Clone(op.Tags), func(t string) bool {
		return t == tagPublic || t == tagInternal
	})
	return &out
}
