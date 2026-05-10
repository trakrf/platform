package main

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// publicSchemaRenames maps the dotted Go-package-prefixed schema names
// emitted by swag (after consolidateSchemaNamespaces folds the splits)
// to clean PascalCase identifiers for the customer-facing public spec
// (TRA-660 / BB25 C1).
//
// Rules:
//
//  1. Drop the Go-package prefix (asset., location., org., report.,
//     errors., shared.) — codegen tools that flatten "." into a legal
//     identifier emit doubled prefixes (AssetPublicAssetView, etc.).
//  2. Drop the redundant "Public" qualifier — the spec is the public
//     surface; the Go-side distinction between PublicAssetView and any
//     internal asset model has no meaning to a generated SDK.
//  3. Where a clean name collides across packages, keep a resource
//     prefix in the verb-noun form already used for adjacent schemas
//     (asset.AddTagResponse / location.AddTagResponse → AddAssetTag /
//     AddLocationTag, matching CreateAssetResponse / GetLocationResponse).
//
// The pass runs ONLY on the public spec — the internal spec keeps its
// dotted names since no external SDKs are generated from it. The list
// is collision-checked at runtime (see renamePublicSpec).
//
// CreateAssetWithTagsRequest / CreateLocationWithTagsRequest keep the
// "WithTags" suffix: the Go struct shape allows tags inline on create,
// and the suffix carries that semantic. Stripping it would conflate the
// schema with a hypothetical tagless create that doesn't exist.
var publicSchemaRenames = map[string]string{
	// asset
	"asset.AddTagResponse":             "AddAssetTagResponse",
	"asset.CreateAssetResponse":        "CreateAssetResponse",
	"asset.CreateAssetWithTagsRequest": "CreateAssetWithTagsRequest",
	"asset.GetAssetResponse":           "GetAssetResponse",
	"asset.ListAssetsResponse":         "ListAssetsResponse",
	"asset.PublicAssetView":            "AssetView",
	"asset.UpdateAssetRequest":         "UpdateAssetRequest",
	"asset.UpdateAssetResponse":        "UpdateAssetResponse",

	// location
	"location.AddTagResponse":                "AddLocationTagResponse",
	"location.CreateLocationResponse":        "CreateLocationResponse",
	"location.CreateLocationWithTagsRequest": "CreateLocationWithTagsRequest",
	"location.GetLocationResponse":           "GetLocationResponse",
	"location.ListAncestorsResponse":         "ListLocationAncestorsResponse",
	"location.ListChildrenResponse":          "ListLocationChildrenResponse",
	"location.ListDescendantsResponse":       "ListLocationDescendantsResponse",
	"location.ListLocationsResponse":         "ListLocationsResponse",
	"location.PublicLocationView":            "LocationView",
	"location.UpdateLocationRequest":         "UpdateLocationRequest",
	"location.UpdateLocationResponse":        "UpdateLocationResponse",

	// org
	"org.GetOrgMeResponse": "GetCurrentOrgResponse",
	"org.OrgMeView":        "OrgView",

	// report
	"report.AssetHistoryResponse":         "AssetHistoryResponse",
	"report.ListCurrentLocationsResponse": "AssetLocationsResponse",
	"report.PublicAssetHistoryItem":       "AssetHistoryItem",
	"report.PublicCurrentLocationItem":    "AssetLocationItem",

	// errors
	"errors.ErrorResponse": "ErrorResponse",
	"errors.FieldError":    "FieldError",

	// shared
	"shared.Tag":        "Tag",
	"shared.TagRequest": "TagRequest",
}

// publicOperationIdRenames maps the dotted operationIds swag emits
// (assets.create, locations.tags.add, reports.asset-locations) to
// camelCase verbResource form (createAsset, addLocationTag,
// getAssetLocations) for the public spec (TRA-660 / BB25 C1). Codegen
// tools derive method names from operationId, so this is what
// determines whether an SDK call reads `client.create_asset()` or
// `client.assets_create()`.
var publicOperationIdRenames = map[string]string{
	"assets.list":             "listAssets",
	"assets.create":           "createAsset",
	"assets.delete":           "deleteAsset",
	"assets.get":              "getAsset",
	"assets.update":           "updateAsset",
	"assets.history":          "getAssetHistory",
	"assets.tags.add":         "addAssetTag",
	"assets.tags.remove":      "removeAssetTag",
	"locations.list":          "listLocations",
	"locations.create":        "createLocation",
	"locations.delete":        "deleteLocation",
	"locations.get":           "getLocation",
	"locations.update":        "updateLocation",
	"locations.ancestors":     "listLocationAncestors",
	"locations.children":      "listLocationChildren",
	"locations.descendants":   "listLocationDescendants",
	"locations.tags.add":      "addLocationTag",
	"locations.tags.remove":   "removeLocationTag",
	"orgs.me":                 "getCurrentOrg",
	"reports.asset-locations": "getAssetLocations",
}

// publicTagDescriptions adds top-level tag definitions to the public
// spec. swag emits per-operation `tags: [name]` arrays with the bare
// resource name; without a top-level tags definition the rendered docs
// have no description text under each tag heading. Tag names match
// what's already on operations — the only addition is the description
// metadata.
var publicTagDescriptions = []*openapi3.Tag{
	{Name: "assets", Description: "Asset CRUD, history, and tag membership."},
	{Name: "locations", Description: "Location CRUD, hierarchy traversal, and tag membership."},
	{Name: "orgs", Description: "Caller's organization context."},
	{Name: "reports", Description: "Cross-cutting reporting endpoints (asset locations, etc.)."},
}

// renamePublicSpec applies publicSchemaRenames + publicOperationIdRenames
// to the document and adds top-level tag descriptions. Runs LAST in
// postprocessPublic so every preceding pass continues to use the dotted
// schema names already encoded in requiredFields, nullableFields,
// readOnlyFields, externalKeyPatternFields, and publicResponseSchemas.
//
// Collision guard: errors loudly if any rename target is already taken
// in the schema map by a different schema. Errors loudly if two source
// schemas would collide on the same target. Either case indicates the
// rename map is out of sync with the spec — better to fail the build
// than silently overwrite.
func renamePublicSpec(doc *openapi3.T) error {
	if doc.Components == nil {
		return nil
	}
	if err := renamePublicSchemas(doc); err != nil {
		return err
	}
	renamePublicOperationIds(doc)
	addPublicTagDescriptions(doc)
	return nil
}

// renamePublicSchemas walks doc.Components.Schemas, renames keys per
// publicSchemaRenames, and rewrites every $ref in the document
// accordingly. Reuses rewriteSchemaRefs to walk paths, components,
// nested schemas, etc. — same traversal as consolidateSchemaNamespaces.
func renamePublicSchemas(doc *openapi3.T) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil
	}
	schemas := doc.Components.Schemas

	applied := map[string]string{}
	targetSources := map[string]string{}

	for oldName, newName := range publicSchemaRenames {
		if _, present := schemas[oldName]; !present {
			continue
		}
		if existing, ok := schemas[newName]; ok && existing != schemas[oldName] {
			return fmt.Errorf("apispec: rename %s → %s collides with existing schema", oldName, newName)
		}
		if prior, taken := targetSources[newName]; taken {
			return fmt.Errorf("apispec: schemas %s and %s both rename to %s", prior, oldName, newName)
		}
		targetSources[newName] = oldName
		applied[oldName] = newName
	}

	if len(applied) == 0 {
		return nil
	}

	for oldName, newName := range applied {
		if oldName == newName {
			continue
		}
		schemas[newName] = schemas[oldName]
		delete(schemas, oldName)
	}

	rewriteSchemaRefs(doc, applied)
	return nil
}

// renamePublicOperationIds walks every operation, looks up the current
// operationId in publicOperationIdRenames, and rewrites it. Operations
// not in the map are left untouched.
func renamePublicOperationIds(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			if newID, ok := publicOperationIdRenames[op.OperationID]; ok {
				op.OperationID = newID
			}
		}
	}
}

// addPublicTagDescriptions populates doc.Tags from publicTagDescriptions
// without disturbing any tags already defined. Idempotent: a name
// already present on doc.Tags is left alone.
func addPublicTagDescriptions(doc *openapi3.T) {
	existing := map[string]bool{}
	for _, t := range doc.Tags {
		if t != nil {
			existing[t.Name] = true
		}
	}
	for _, t := range publicTagDescriptions {
		if existing[t.Name] {
			continue
		}
		doc.Tags = append(doc.Tags, t)
	}
}

