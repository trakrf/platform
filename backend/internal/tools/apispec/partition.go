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
	return public, internal, nil
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
