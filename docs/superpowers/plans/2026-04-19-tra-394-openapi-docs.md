# TRA-394 — OpenAPI 3.0 spec generation + Redoc docs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert swaggo-annotated handlers into a partitioned OpenAPI 3.0 spec (public customer contract + internal Swagger UI), publish Redoc + Postman + raw spec to `docs.trakrf.id/api` via a cross-repo PR pipeline, and enforce drift detection in CI.

**Architecture:** One annotated source → `swag init` produces OpenAPI 2.0 → new in-repo Go tool `apispec` converts to 3.0 and partitions by tag (`public` vs `internal`) → public spec is committed, internal is embedded into the binary and served behind session auth → CI opens cross-repo PR to `trakrf/docs` on main-merge → Docusaurus+redocusaurus renders Redoc at `docs.trakrf.id/api`.

**Tech Stack:** Go 1.25, `github.com/getkin/kin-openapi` (2.0→3.0 conversion), `gopkg.in/yaml.v3`, swaggo 1.16.6, Docusaurus 3.9, `redocusaurus@^2`, `redoc@^2`, `@redocly/cli`, `openapi-to-postmanv2`, GitHub Actions.

**Design reference:** `docs/superpowers/specs/2026-04-19-tra394-openapi-docs-design.md`.

---

## File Structure

**New files in `platform`:**

- `backend/internal/tools/apispec/main.go` — CLI entry point
- `backend/internal/tools/apispec/convert.go` — 2.0 → 3.0 conversion wrapper
- `backend/internal/tools/apispec/partition.go` — tag validation + partition into public/internal
- `backend/internal/tools/apispec/postprocess.go` — security scheme, info, servers rewriting
- `backend/internal/tools/apispec/emit.go` — JSON + YAML writers
- `backend/internal/tools/apispec/convert_test.go` + `partition_test.go` + `postprocess_test.go` + `emit_test.go`
- `backend/internal/tools/apispec/testdata/` — fixture 2.0 specs (tiny, hand-written)
- `backend/internal/handlers/swaggerspec/swaggerspec.go` — embeds internal spec, serves JSON/YAML
- `docs/api/openapi.public.json` + `.yaml` — committed public spec (first generated in Task 14)
- `docs/api/trakrf-api.postman_collection.json` — committed Postman collection
- `.github/workflows/api-spec.yml` — PR drift check + Redocly lint
- `.github/workflows/publish-api-docs.yml` — main-merge cross-repo PR publisher

**Modified files in `platform`:**

- `backend/main.go` — add swag `@securityDefinitions.apikey APIKey` header block
- `backend/justfile` — replace `swagger` recipe with `api-spec`, extend `build` and `validate`
- `backend/internal/cmd/serve/router.go` — move `/swagger/*` behind `middleware.Auth`; replace `_ "backend/docs"` import with `swaggerspec` serve handlers
- `backend/internal/handlers/*/` — add `@Tags <resource>,public` or `@Tags <resource>,internal`; for public: `@Security APIKey`, `@Failure` lines per error catalog, enum annotations
- `backend/go.mod` — add `github.com/getkin/kin-openapi`
- `.gitignore` — add `backend/internal/handlers/swaggerspec/openapi.internal.*`

**New files in `trakrf-docs` (separate repo):**

- `docs/api/postman.mdx` — Postman download + raw spec URLs
- Modifications to `docusaurus.config.ts`, `sidebars.ts`, `package.json`, `static/api/` (spec files mirrored by cross-repo PR)

---

## Task 1: Declare the APIKey security scheme in `main.go`

**Files:**
- Modify: `backend/main.go:1-14` (add swag header block)

**Why first:** The `apispec` tool's post-processing needs `securityDefinitions.APIKey` present in the input. Adding it early means every subsequent `swag init` run emits the scheme, and tool development can proceed against realistic input.

- [ ] **Step 1: Add the swag general-info block above `package main`**

Open `backend/main.go` and insert this block as a top-of-file doc comment immediately before `package main` (swag parses general-info from the first Go file passed via `-g`):

```go
// @title TrakRF API
// @version v1
// @description TrakRF public REST API. See docs.trakrf.id/api for the customer-facing reference.
// @contact.name TrakRF Support
// @contact.email support@trakrf.id
// @license.name Business Source License 1.1
// @license.url https://github.com/trakrf/platform/blob/main/LICENSE
// @host trakrf.id
// @BasePath /api/v1
// @schemes https
// @securityDefinitions.apikey APIKey
// @in header
// @name Authorization
// @description TrakRF API key (JWT). Format: "Bearer <jwt>". Create in Settings → API Keys.
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Session JWT for internal endpoints (platform frontend uses this).
package main
```

Both schemes coexist: `APIKey` applies to `public`-tagged operations, `BearerAuth` applies to existing `internal`-tagged operations. The `apispec` post-processor will rewrite `APIKey` to `type: http; scheme: bearer; bearerFormat: JWT` for OpenAPI 3.0 compliance.

- [ ] **Step 2: Regenerate the swag docs to verify the block parses**

```bash
cd backend && swag init -g main.go --parseDependency --parseInternal -o docs
```

Expected: no errors; `backend/docs/swagger.json` contains a `securityDefinitions` map with both `APIKey` and `BearerAuth` entries. Inspect:

```bash
python3 -c "import json; print(list(json.load(open('backend/docs/swagger.json'))['securityDefinitions'].keys()))"
```

Expected output: `['APIKey', 'BearerAuth']`.

- [ ] **Step 3: Commit**

```bash
git add backend/main.go
git commit -m "feat(tra-394): declare APIKey + BearerAuth security schemes in swag header"
```

---

## Task 2: Scaffold the `apispec` Go tool

**Files:**
- Create: `backend/internal/tools/apispec/main.go`
- Create: `backend/internal/tools/apispec/testdata/minimal-v2.json`
- Modify: `backend/go.mod` (add `github.com/getkin/kin-openapi`)

- [ ] **Step 1: Add the dependency**

```bash
cd backend && go get github.com/getkin/kin-openapi@latest
cd backend && go mod tidy
```

Expected: `github.com/getkin/kin-openapi` appears in `backend/go.mod` under `require`.

- [ ] **Step 2: Create a minimal 2.0 fixture**

Create `backend/internal/tools/apispec/testdata/minimal-v2.json` with exactly one operation tagged `public,assets`:

```json
{
  "swagger": "2.0",
  "info": { "title": "Test", "version": "v1" },
  "host": "example.com",
  "basePath": "/api/v1",
  "schemes": ["https"],
  "securityDefinitions": {
    "APIKey": {
      "type": "apiKey",
      "in": "header",
      "name": "Authorization",
      "description": "TrakRF API key (JWT)."
    }
  },
  "paths": {
    "/assets": {
      "get": {
        "summary": "List assets",
        "description": "Paginated list of assets.",
        "tags": ["assets", "public"],
        "security": [{"APIKey": []}],
        "responses": {
          "200": { "description": "OK", "schema": { "type": "object" } }
        }
      }
    }
  }
}
```

- [ ] **Step 3: Create the CLI skeleton**

Create `backend/internal/tools/apispec/main.go`:

```go
// Package main is the apispec CLI tool. It converts swaggo-generated
// OpenAPI 2.0 specs into partitioned OpenAPI 3.0 specs: one public spec
// (operations tagged "public") and one internal spec (operations tagged
// "internal"), with post-processing to match TRA-392's documented contract.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		inPath       = flag.String("in", "", "Path to swagger 2.0 JSON (required)")
		publicOut    = flag.String("public-out", "", "Output path prefix for public spec (required, writes .json and .yaml)")
		internalOut  = flag.String("internal-out", "", "Output path prefix for internal spec (required, writes .json and .yaml)")
	)
	flag.Parse()

	if *inPath == "" || *publicOut == "" || *internalOut == "" {
		fmt.Fprintln(os.Stderr, "usage: apispec --in <path> --public-out <prefix> --internal-out <prefix>")
		os.Exit(2)
	}

	if err := run(*inPath, *publicOut, *internalOut); err != nil {
		fmt.Fprintf(os.Stderr, "apispec: %v\n", err)
		os.Exit(1)
	}
}

func run(inPath, publicOut, internalOut string) error {
	return fmt.Errorf("not implemented")
}
```

- [ ] **Step 4: Verify it builds**

```bash
cd backend && go build ./internal/tools/apispec/
```

Expected: no compile errors. The binary exits with "not implemented" when invoked; that's intentional for now.

- [ ] **Step 5: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/tools/apispec/
git commit -m "feat(tra-394): scaffold apispec CLI tool"
```

---

## Task 3: Implement 2.0 → 3.0 conversion (TDD)

**Files:**
- Create: `backend/internal/tools/apispec/convert.go`
- Create: `backend/internal/tools/apispec/convert_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/tools/apispec/convert_test.go`:

```go
package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertV2ToV3_ReadsAndConverts(t *testing.T) {
	data, err := os.ReadFile("testdata/minimal-v2.json")
	require.NoError(t, err)

	doc3, err := convertV2ToV3(data)
	require.NoError(t, err)

	require.Equal(t, "3.0.0", doc3.OpenAPI)
	require.NotNil(t, doc3.Paths)
	assetsPath := doc3.Paths.Value("/assets")
	require.NotNil(t, assetsPath, "expected /assets path in converted doc")
	require.NotNil(t, assetsPath.Get, "expected GET operation on /assets")
	require.Equal(t, []string{"assets", "public"}, assetsPath.Get.Tags)
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestConvertV2ToV3_ReadsAndConverts -v
```

Expected: FAIL — `convertV2ToV3` is undefined.

- [ ] **Step 3: Implement `convertV2ToV3`**

Create `backend/internal/tools/apispec/convert.go`:

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
)

// convertV2ToV3 reads swaggo-emitted OpenAPI 2.0 JSON and returns the
// equivalent 3.0 document.
func convertV2ToV3(data []byte) (*openapi3.T, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err != nil {
		return nil, fmt.Errorf("parse v2 JSON: %w", err)
	}
	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert v2 → v3: %w", err)
	}
	return doc3, nil
}
```

- [ ] **Step 4: Run to confirm it passes**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestConvertV2ToV3_ReadsAndConverts -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/tools/apispec/convert.go backend/internal/tools/apispec/convert_test.go
git commit -m "feat(tra-394): apispec v2→v3 conversion"
```

---

## Task 4: Implement tag partition + validation (TDD)

**Files:**
- Create: `backend/internal/tools/apispec/partition.go`
- Create: `backend/internal/tools/apispec/partition_test.go`
- Create: `backend/internal/tools/apispec/testdata/untagged-v2.json`
- Create: `backend/internal/tools/apispec/testdata/both-tags-v2.json`

- [ ] **Step 1: Add two more fixtures**

Create `backend/internal/tools/apispec/testdata/untagged-v2.json` — same as `minimal-v2.json` but remove `"public"` from the tags array (leaving only `["assets"]`).

Create `backend/internal/tools/apispec/testdata/both-tags-v2.json` — same as `minimal-v2.json` but tags is `["assets", "public", "internal"]`.

- [ ] **Step 2: Write failing tests**

Create `backend/internal/tools/apispec/partition_test.go`:

```go
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
```

- [ ] **Step 3: Run to confirm they fail**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestPartition -v
```

Expected: FAIL — `partition` is undefined.

- [ ] **Step 4: Implement `partition`**

Create `backend/internal/tools/apispec/partition.go`:

```go
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

// cloneDocShell returns a copy of doc with an empty Paths map. Info, components,
// security schemes, and servers are shared (shallow copy) — post-processing
// step rewrites those separately for public vs internal.
func cloneDocShell(doc *openapi3.T) *openapi3.T {
	shell := *doc
	shell.Paths = openapi3.NewPaths()
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
```

- [ ] **Step 5: Run to confirm they pass**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestPartition -v
```

Expected: PASS on all three `TestPartition_*` tests.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/tools/apispec/partition.go backend/internal/tools/apispec/partition_test.go backend/internal/tools/apispec/convert.go backend/internal/tools/apispec/testdata/
git commit -m "feat(tra-394): apispec tag partition + validation"
```

---

## Task 5: Implement post-processing (security scheme, info, servers) (TDD)

**Files:**
- Create: `backend/internal/tools/apispec/postprocess.go`
- Create: `backend/internal/tools/apispec/postprocess_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/tools/apispec/postprocess_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostprocess_RewritesAPIKeySchemeToHTTPBearer(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	require.NotNil(t, doc.Components)
	scheme := doc.Components.SecuritySchemes["APIKey"]
	require.NotNil(t, scheme, "APIKey scheme must be present")
	require.NotNil(t, scheme.Value)

	assert.Equal(t, "http", scheme.Value.Type)
	assert.Equal(t, "bearer", scheme.Value.Scheme)
	assert.Equal(t, "JWT", scheme.Value.BearerFormat)
	assert.Contains(t, scheme.Value.Description, "TrakRF API key")
}

func TestPostprocess_SetsPublicInfoAndServers(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	assert.Equal(t, "TrakRF API", doc.Info.Title)
	assert.Equal(t, "v1", doc.Info.Version)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "https://trakrf.id", doc.Servers[0].URL)
	assert.Equal(t, "Production", doc.Servers[0].Description)
}

func TestPostprocess_SetsInternalInfoAndServers(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessInternal(doc)

	assert.Equal(t, "TrakRF Internal API — not for customer use", doc.Info.Title)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "http://localhost:8080", doc.Servers[0].URL)
	assert.Equal(t, "Local development", doc.Servers[0].Description)
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestPostprocess -v
```

Expected: FAIL — `postprocessPublic` / `postprocessInternal` undefined.

- [ ] **Step 3: Implement post-processing**

Create `backend/internal/tools/apispec/postprocess.go`:

```go
package main

import "github.com/getkin/kin-openapi/openapi3"

// postprocessPublic rewrites the doc for customer-facing publication:
// converts the APIKey security scheme from swaggo's 2.0 "apiKey" form
// to 3.0 HTTP-Bearer with bearerFormat JWT, and sets the customer-facing
// info and server URLs.
func postprocessPublic(doc *openapi3.T) {
	rewriteAPIKeyScheme(doc)
	doc.Info.Title = "TrakRF API"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "https://trakrf.id", Description: "Production"},
	}
}

// postprocessInternal is the same but labels the doc as internal and
// uses a local development server URL. The APIKey scheme rewrite is
// applied here too so both specs share a consistent 3.0 surface.
func postprocessInternal(doc *openapi3.T) {
	rewriteAPIKeyScheme(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
}

func rewriteAPIKeyScheme(doc *openapi3.T) {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	ref := doc.Components.SecuritySchemes["APIKey"]
	if ref == nil || ref.Value == nil {
		return
	}
	desc := ref.Value.Description
	ref.Value = &openapi3.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  desc,
	}
}
```

- [ ] **Step 4: Run to confirm they pass**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestPostprocess -v
```

Expected: PASS on all three `TestPostprocess_*` tests.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go backend/internal/tools/apispec/postprocess_test.go
git commit -m "feat(tra-394): apispec post-processing (security scheme, info, servers)"
```

---

## Task 6: Implement JSON + YAML emission (TDD)

**Files:**
- Create: `backend/internal/tools/apispec/emit.go`
- Create: `backend/internal/tools/apispec/emit_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/tools/apispec/emit_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEmit_WritesJSONAndYAML(t *testing.T) {
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	postprocessPublic(doc)

	dir := t.TempDir()
	prefix := filepath.Join(dir, "openapi.public")

	err := emit(doc, prefix)
	require.NoError(t, err)

	jsonBytes, err := os.ReadFile(prefix + ".json")
	require.NoError(t, err)
	var parsedJSON map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsedJSON))
	assert.Equal(t, "3.0.0", parsedJSON["openapi"])

	yamlBytes, err := os.ReadFile(prefix + ".yaml")
	require.NoError(t, err)
	var parsedYAML map[string]any
	require.NoError(t, yaml.Unmarshal(yamlBytes, &parsedYAML))
	assert.Equal(t, "3.0.0", parsedYAML["openapi"])
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestEmit -v
```

Expected: FAIL — `emit` undefined.

- [ ] **Step 3: Implement `emit`**

Create `backend/internal/tools/apispec/emit.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// emit writes doc to <prefix>.json and <prefix>.yaml. The JSON is
// pretty-printed with two-space indentation; the YAML uses the default
// gopkg.in/yaml.v3 encoding with explicit document start omitted.
func emit(doc *openapi3.T, prefix string) error {
	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := os.WriteFile(prefix+".json", jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write %s.json: %w", prefix, err)
	}

	// Round-trip through JSON so kin-openapi's JSON-tag-driven serialization
	// is used (openapi3.T is JSON-first; direct yaml.Marshal drops fields).
	var asMap map[string]any
	if err := json.Unmarshal(jsonBytes, &asMap); err != nil {
		return fmt.Errorf("round-trip JSON→map: %w", err)
	}
	yamlBytes, err := yaml.Marshal(asMap)
	if err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}
	if err := os.WriteFile(prefix+".yaml", yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write %s.yaml: %w", prefix, err)
	}
	return nil
}
```

- [ ] **Step 4: Run to confirm it passes**

```bash
cd backend && go test ./internal/tools/apispec/ -run TestEmit -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/tools/apispec/emit.go backend/internal/tools/apispec/emit_test.go
git commit -m "feat(tra-394): apispec JSON+YAML emission"
```

---

## Task 7: Wire the `apispec` CLI `run()` function (integration)

**Files:**
- Modify: `backend/internal/tools/apispec/main.go`

- [ ] **Step 1: Replace the stub `run` with the real pipeline**

Edit `backend/internal/tools/apispec/main.go`, replacing the `run` function body:

```go
func run(inPath, publicOut, internalOut string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}

	doc3, err := convertV2ToV3(data)
	if err != nil {
		return err
	}

	public, internal, err := partition(doc3)
	if err != nil {
		return err
	}

	postprocessPublic(public)
	postprocessInternal(internal)

	if err := emit(public, publicOut); err != nil {
		return fmt.Errorf("emit public: %w", err)
	}
	if err := emit(internal, internalOut); err != nil {
		return fmt.Errorf("emit internal: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Verify end-to-end against the fixture**

```bash
cd backend && go build -o /tmp/apispec ./internal/tools/apispec && /tmp/apispec --in internal/tools/apispec/testdata/minimal-v2.json --public-out /tmp/openapi.public --internal-out /tmp/openapi.internal
```

Expected: exit code 0. Verify outputs:

```bash
ls /tmp/openapi.public.* /tmp/openapi.internal.*
head -5 /tmp/openapi.public.yaml
```

Expected: four files exist; YAML starts with `openapi: 3.0.0`.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/tools/apispec/main.go
git commit -m "feat(tra-394): apispec CLI integration — full pipeline"
```

---

## Task 8: Add `just backend api-spec` recipe + gitignore

**Files:**
- Modify: `backend/justfile` (replace `swagger` recipe with `api-spec`)
- Modify: `.gitignore` (add embedded internal spec path)

- [ ] **Step 1: Update `.gitignore`**

Append to `.gitignore` (keep the existing `backend/docs/` line intact):

```
# Generated internal API spec (embedded into binary via go:embed; regenerated per-build)
backend/internal/handlers/swaggerspec/openapi.internal.*
```

- [ ] **Step 2: Replace the `swagger` recipe in `backend/justfile`**

Open `backend/justfile` and replace the existing `swagger` recipe (lines ~57-61, starts with `# Generate Swagger API documentation`) with:

```
# Generate OpenAPI 3.0 specs (public → docs/api/, internal → embedded into binary)
api-spec:
    @echo "📚 Generating OpenAPI 3.0 specs..."
    swag init -g main.go --parseDependency --parseInternal -o docs
    @mkdir -p internal/handlers/swaggerspec ../docs/api
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out ../docs/api/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
    @echo "✅ Public spec:   docs/api/openapi.public.{json,yaml}  (committed)"
    @echo "✅ Internal spec: backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}  (gitignored, embedded)"

# Legacy alias retained for muscle memory
swagger: api-spec
```

- [ ] **Step 3: Smoke test the recipe (expected to fail on tag validation)**

```bash
just backend api-spec
```

Expected: FAILS with tag-validation errors from `apispec` (every existing handler is missing `public` or `internal`). That's the point — the recipe works; the handlers need tagging in the next tasks. Verify the error lists several paths (e.g., `GET /api/v1/assets: missing "public" or "internal" tag`).

- [ ] **Step 4: Commit**

```bash
git add .gitignore backend/justfile
git commit -m "feat(tra-394): just backend api-spec recipe + .gitignore for embedded internal spec"
```

---

## Task 9: Tag internal-only handlers

**Files:**
- Modify: `backend/internal/handlers/users/users.go`
- Modify: `backend/internal/handlers/inventory/save.go`
- Modify: `backend/internal/handlers/lookup/lookup.go`
- Modify: `backend/internal/handlers/auth/auth.go`
- Modify: `backend/internal/handlers/health/health.go`
- Modify: `backend/internal/handlers/testhandler/*.go` (if any `@Tags` annotations exist)
- Modify: `backend/internal/handlers/assets/bulkimport.go` (the one asset handler that's internal)

**Approach:** every `@Tags X` line in these files becomes `@Tags X,internal`. If a handler has no `@Tags` line at all, add `@Tags internal`.

- [ ] **Step 1: Inventory the existing tags across internal-only handlers**

```bash
grep -rn "^// @Tags " backend/internal/handlers/users backend/internal/handlers/inventory backend/internal/handlers/lookup backend/internal/handlers/auth backend/internal/handlers/health backend/internal/handlers/testhandler backend/internal/handlers/assets/bulkimport.go
```

This lists every `@Tags` annotation line. Note the existing tag on each.

- [ ] **Step 2: Append `,internal` to each existing `@Tags` line in the files above**

For each file and each `@Tags <tag>` line, replace with `@Tags <tag>,internal`. For any operation without an existing `@Tags` line, add `// @Tags internal` before the `@Router` line.

Example transformation in `backend/internal/handlers/users/users.go`:

```
// @Tags users
```

becomes

```
// @Tags users,internal
```

Apply uniformly across all the files listed under **Files:** above.

- [ ] **Step 3: Run `just backend api-spec` and confirm internal-only handlers are clean**

```bash
just backend api-spec
```

Expected: FAILS — but the error list now only names public-destined handlers (assets, locations, reports, orgs). Internal handlers no longer appear. If any internal handler still surfaces, it was missed in Step 2.

- [ ] **Step 4: Run existing backend tests to confirm no regressions**

```bash
just backend test
```

Expected: existing test suite passes (tag changes are comment-only).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/
git commit -m "feat(tra-394): tag internal-only handlers (users, auth, health, inventory, lookup, bulkimport, testhandler)"
```

---

## Task 10: Tag public handlers + orgs/me

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (every handler EXCEPT bulkimport — those are already internal per Task 9)
- Modify: `backend/internal/handlers/locations/locations.go`
- Modify: `backend/internal/handlers/reports/current_locations.go`
- Modify: `backend/internal/handlers/reports/asset_history.go`
- Modify: `backend/internal/handlers/orgs/*.go` — `/orgs/me` operation gets `public`; all other org operations get `internal`

- [ ] **Step 1: Inventory orgs endpoints to decide per-operation**

```bash
grep -n "@Router" backend/internal/handlers/orgs/*.go
```

Note which router paths correspond to `/orgs/me` (public) vs everything else (internal).

- [ ] **Step 2: Append `,public` to `@Tags` on public-destined handlers**

In `assets/assets.go`, `locations/locations.go`, `reports/current_locations.go`, `reports/asset_history.go`: every existing `@Tags <tag>` becomes `@Tags <tag>,public`.

In `orgs/*.go`: the `/orgs/me` operation's `@Tags` gets `,public`; all other orgs operations get `,internal`.

- [ ] **Step 3: Run `just backend api-spec` and confirm it succeeds**

```bash
just backend api-spec
```

Expected: SUCCESS. Both `docs/api/openapi.public.{json,yaml}` and `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}` are written.

Inspect the public spec:

```bash
python3 -c "import json; d = json.load(open('docs/api/openapi.public.json')); print('paths:', list(d['paths'].keys()))"
```

Expected: paths include `/api/v1/assets`, `/api/v1/assets/{id}`, `/api/v1/locations`, `/api/v1/locations/{id}`, `/api/v1/locations/current`, `/api/v1/assets/{id}/history`, `/api/v1/orgs/me`. No internal paths (`/users`, `/auth`, `/inventory`, `/lookup`, `/health`, `/bulkimport`).

- [ ] **Step 4: Run existing backend tests**

```bash
just backend test
```

Expected: existing test suite passes.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/
git commit -m "feat(tra-394): tag public handlers (assets CRUD, locations CRUD, reports, orgs/me)"
```

---

## Task 11: Enhance public handler annotations — assets

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (every handler EXCEPT bulkimport)

**Why:** public handlers render in Redoc as customer-facing docs. Summaries and descriptions need customer-grade wording; every operation needs `@Security APIKey <scope>` and full `@Failure` coverage per TRA-392's error catalog.

**Scopes per TRA-392:** `assets:read` for GET operations; `assets:write` for POST/PUT/DELETE.

- [ ] **Step 1: Rewrite each handler's annotation block**

For each public `assets` operation, bring its annotations up to this template (example for `ListAssets`):

```go
// @Summary      List assets
// @Description  Returns a paginated list of assets for the current organization.
// @Description  Supports filtering by location, activity status, and full-text search.
// @Tags         assets,public
// @Accept       json
// @Produce      json
// @Param        limit       query  int     false  "Max results (default 50, max 200)"  default(50)  minimum(1)  maximum(200)
// @Param        offset      query  int     false  "Pagination offset"                   default(0)   minimum(0)
// @Param        location    query  string  false  "Filter by parent location identifier"
// @Param        is_active   query  bool    false  "Filter by active status"
// @Param        type        query  string  false  "Filter by asset type"
// @Param        q           query  string  false  "Full-text match on name, identifier, description"
// @Success      200  {object}  ListAssetsResponse
// @Failure      400  {object}  modelerrors.ErrorResponse  "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse  "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse  "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse  "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse  "internal_error"
// @Security     APIKey[assets:read]
// @Router       /assets [get]
```

Apply the same pattern to: `GetAsset` (add `404 not_found`, `@Security APIKey[assets:read]`), `CreateAsset` (add `409 conflict` + `validation_error`, `@Security APIKey[assets:write]`), `UpdateAsset` (add `404` + `409`, `@Security APIKey[assets:write]`), `DeleteAsset` (add `404`, `@Security APIKey[assets:write]`).

**Notes on every public annotation:**
- `@Router` paths lose the `/api/v1` prefix (the spec's `basePath` provides it).
- `@Security APIKey[<scope>]` — the scope is encoded in brackets; `apispec` + kin-openapi translate this correctly into OpenAPI 3.0 `security: [{APIKey: [<scope>]}]`.
- For handlers that reference surrogate IDs via `{id}`: leave `{id}` as-is for now; TRA-396 converts to `{identifier}` when it lands. (Per design doc §Annotation strategy — whichever ticket merges first drives the parameter name.)

- [ ] **Step 2: Regenerate and verify**

```bash
just backend api-spec
```

Expected: SUCCESS. Inspect an operation to confirm security and failures:

```bash
python3 -c "import json; d=json.load(open('docs/api/openapi.public.json')); op=d['paths']['/api/v1/assets']['get']; print('security:', op.get('security')); print('responses:', list(op['responses'].keys()))"
```

Expected: `security: [{'APIKey': ['assets:read']}]`; responses include `200`, `400`, `401`, `403`, `429`, `500`.

- [ ] **Step 3: Run backend tests**

```bash
just backend test
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/assets/assets.go
git commit -m "feat(tra-394): customer-grade annotations on assets handlers"
```

---

## Task 12: Enhance public handler annotations — locations, reports, orgs/me

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go`
- Modify: `backend/internal/handlers/reports/current_locations.go`
- Modify: `backend/internal/handlers/reports/asset_history.go`
- Modify: `backend/internal/handlers/orgs/*.go` (just the `/orgs/me` handler)

- [ ] **Step 1: Apply the same annotation template to `locations`**

For each `locations` CRUD operation, use the same template as Task 11 with these adjustments:

- Scopes: `locations:read` for GET; `locations:write` for POST/PUT/DELETE.
- Query params for `ListLocations`: `limit`, `offset`, `parent` (filter by parent identifier), `is_active`, `q`.
- Add `@Failure 404 not_found` on `GetLocation`, `UpdateLocation`, `DeleteLocation`.
- Add `@Failure 409 conflict` on `CreateLocation`, `UpdateLocation`.

- [ ] **Step 2: Apply to `reports/current_locations.go`**

Template for the current-locations report:

```go
// @Summary      Current asset-at-location snapshot
// @Description  Returns the current location of every active asset in the organization.
// @Description  Each item includes the asset identifier, the location identifier, and the last-seen timestamp.
// @Tags         reports,public
// @Produce      json
// @Param        limit   query  int  false  "Max results (default 50, max 200)"  default(50)  minimum(1)  maximum(200)
// @Param        offset  query  int  false  "Pagination offset"                   default(0)   minimum(0)
// @Success      200  {object}  CurrentLocationsResponse
// @Failure      401  {object}  modelerrors.ErrorResponse  "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse  "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse  "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse  "internal_error"
// @Security     APIKey[scans:read]
// @Router       /locations/current [get]
```

- [ ] **Step 3: Apply to `reports/asset_history.go`**

Template for asset history:

```go
// @Summary      Asset movement history
// @Description  Returns the movement history for a single asset — each location transition with timestamps and duration.
// @Tags         reports,public
// @Produce      json
// @Param        identifier  path   string  true   "Asset identifier (natural key)"
// @Param        from        query  string  false  "Filter: scans at or after this ISO 8601 timestamp"
// @Param        to          query  string  false  "Filter: scans at or before this ISO 8601 timestamp"
// @Param        limit       query  int     false  "Max results"                        default(50)  minimum(1)  maximum(200)
// @Param        offset      query  int     false  "Pagination offset"                   default(0)   minimum(0)
// @Success      200  {object}  AssetHistoryResponse
// @Failure      401  {object}  modelerrors.ErrorResponse  "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse  "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse  "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse  "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse  "internal_error"
// @Security     APIKey[scans:read]
// @Router       /assets/{identifier}/history [get]
```

- [ ] **Step 4: Apply to `orgs/me`**

Template for the organization-identity endpoint:

```go
// @Summary      Organization identity for this API key
// @Description  Returns basic information about the organization that owns the API key used in this request.
// @Description  Useful for connectivity checks and to verify which org a key is scoped to before making further calls.
// @Tags         orgs,public
// @Produce      json
// @Success      200  {object}  OrgMeResponse
// @Failure      401  {object}  modelerrors.ErrorResponse  "unauthorized"
// @Failure      500  {object}  modelerrors.ErrorResponse  "internal_error"
// @Security     APIKey
// @Router       /orgs/me [get]
```

Note: `/orgs/me` is explicitly excluded from rate limiting per TRA-392 §G-4, so no 429 entry. Scope is required (any valid key) but no specific scope token — `@Security APIKey` with no brackets emits an empty scope list, which TRA-392 specifies as "no scope required beyond valid key."

- [ ] **Step 5: Regenerate and verify**

```bash
just backend api-spec
```

Expected: SUCCESS. Verify `docs/api/openapi.public.json` contains these operations with the described security and failure entries.

- [ ] **Step 6: Run backend tests**

```bash
just backend test
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/locations backend/internal/handlers/reports backend/internal/handlers/orgs
git commit -m "feat(tra-394): customer-grade annotations on locations, reports, orgs/me"
```

---

## Task 13: Enum-openness annotations

**Files:**
- Modify: `backend/internal/models/asset/asset.go` (Asset.Type)
- Modify: `backend/internal/models/errors/errors.go` (ErrorType constants + ErrorResponse field)
- Modify: `backend/internal/handlers/assets/assets.go` (sort param enum on ListAssets)
- Modify: `backend/internal/handlers/locations/locations.go` (sort param enum on ListLocations)

**Goal:** apply `enum:"..."` for closed enums and `x-extensible-enum:"true"` for open enums, per the audit table in the design doc §Enum openness.

- [ ] **Step 1: Annotate `Asset.Type` as open**

In `backend/internal/models/asset/asset.go`, find the `Type` field on the public-facing `AssetView` struct (not the raw `Asset` struct). Add this swag tag to the struct field:

```go
Type string `json:"type" example:"asset" enums:"asset" extensions:"x-extensible-enum=true"`
```

The `extensions:` swag tag emits `x-extensible-enum: true` on the schema property. Description blurb is added via a `@Description` comment on the struct field if swaggo parses it; otherwise add it post-hoc in the `apispec` post-processor (see Step 5).

- [ ] **Step 2: Annotate `error.type` as open**

In `backend/internal/models/errors/errors.go`, find the `ErrorResponse.Error.Type` field (or wherever the type field is declared). Add:

```go
Type string `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error" extensions:"x-extensible-enum=true"`
```

Do the same for `error.fields[].code`:

```go
Code string `json:"code" example:"required" enums:"required,invalid_value,too_short,too_long,invalid_format,out_of_range" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 3: Annotate sort params as closed enums**

In `backend/internal/handlers/assets/assets.go` `ListAssets` annotation block, add a sort param:

```go
// @Param        sort  query  string  false  "Sort (prefix with - for DESC, comma-separated for multi-field)"  Enums(identifier,name,created_at,updated_at,-identifier,-name,-created_at,-updated_at)  default(identifier)
```

Repeat for `locations` (`path`, `identifier`, `name`, `created_at`).

- [ ] **Step 4: Regenerate and verify**

```bash
just backend api-spec
```

Verify the Asset type enum:

```bash
python3 -c "import json; d=json.load(open('docs/api/openapi.public.json')); comp=d['components']['schemas']; asset=comp.get('asset.AssetView') or comp.get('AssetView'); print('Type schema:', asset['properties']['type'])"
```

Expected: the `type` property includes `enum: ['asset']` and `x-extensible-enum: true`.

If the extension is missing (swaggo silently dropped the `extensions:` tag on that field), add an explicit annotation instead:

```go
// Type is the asset type.
// @extensions x-extensible-enum=true
Type string `json:"type" example:"asset" enums:"asset"`
```

Regenerate and re-check. Do the same for `error.type` / `error.fields[].code` if either is missing the extension.

- [ ] **Step 5: Run backend tests**

```bash
just backend test
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models backend/internal/handlers
git commit -m "feat(tra-394): enum-openness annotations per audit table"
```

---

## Task 14: Commit the initial public spec

**Files:**
- Create: `docs/api/openapi.public.json`
- Create: `docs/api/openapi.public.yaml`

- [ ] **Step 1: Regenerate fresh and stage**

```bash
just backend api-spec
git status docs/api/
```

Expected: both `openapi.public.json` and `openapi.public.yaml` appear as untracked files.

- [ ] **Step 2: Quick sanity check — OpenAPI validity**

```bash
pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml --extends=recommended
```

Expected: either clean, or a small number of `warn`/`error` items describing missing `operationId`, `description`, etc. Fix the errors in the relevant handler annotation (Task 11/12), regenerate, re-lint. Do not proceed until `redocly lint` exits 0.

- [ ] **Step 3: Commit**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "feat(tra-394): initial committed public OpenAPI 3.0 spec"
```

---

## Task 15: Generate and commit the initial Postman collection

**Files:**
- Create: `docs/api/trakrf-api.postman_collection.json`

- [ ] **Step 1: Generate the collection**

```bash
pnpm dlx openapi-to-postmanv2 \
    -s docs/api/openapi.public.json \
    -o docs/api/trakrf-api.postman_collection.json \
    -p -O folderStrategy=Paths
```

Expected: file is created; `pnpm dlx` may prompt to install on first run — accept.

- [ ] **Step 2: Sanity-check the collection**

```bash
python3 -c "import json; c=json.load(open('docs/api/trakrf-api.postman_collection.json')); print('name:', c['info']['name']); print('folders:', [i['name'] for i in c['item']])"
```

Expected: one item per path group (e.g., `/assets`, `/locations`, `/orgs`).

- [ ] **Step 3: Commit**

```bash
git add docs/api/trakrf-api.postman_collection.json
git commit -m "feat(tra-394): initial committed Postman collection"
```

---

## Task 16: Embed the internal spec + create the `swaggerspec` serve handlers

**Files:**
- Create: `backend/internal/handlers/swaggerspec/swaggerspec.go`

- [ ] **Step 1: Write the package**

Create `backend/internal/handlers/swaggerspec/swaggerspec.go`:

```go
// Package swaggerspec embeds the internal OpenAPI 3.0 spec generated by
// the apispec tool and serves it over HTTP for the internal Swagger UI.
// The spec is regenerated on every build; see just backend api-spec.
package swaggerspec

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.internal.json
var internalJSON []byte

//go:embed openapi.internal.yaml
var internalYAML []byte

// ServeJSON writes the embedded internal OpenAPI spec as JSON.
func ServeJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(internalJSON)
}

// ServeYAML writes the embedded internal OpenAPI spec as YAML.
func ServeYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(internalYAML)
}
```

- [ ] **Step 2: Verify it builds**

```bash
cd backend && go build ./internal/handlers/swaggerspec/
```

Expected: SUCCESS (the `openapi.internal.{json,yaml}` files already exist in the package directory from Task 10's `just backend api-spec` run).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/swaggerspec/
git commit -m "feat(tra-394): swaggerspec package — embeds internal spec + HTTP handlers"
```

---

## Task 17: Move `/swagger/*` behind `middleware.Auth`

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`

- [ ] **Step 1: Edit imports**

In `backend/internal/cmd/serve/router.go`:

- Remove line 16: `_ "github.com/trakrf/platform/backend/docs"` (no longer needed — httpSwagger now reads spec from URL)
- Add import: `"github.com/trakrf/platform/backend/internal/handlers/swaggerspec"`

- [ ] **Step 2: Remove the unauthenticated swagger route**

Delete line 63: `r.Get("/swagger/*", httpSwagger.WrapHandler)`

- [ ] **Step 3: Add the authenticated swagger routes inside the existing `middleware.Auth` group**

In the existing `r.Group` block (around line 71–83), add after the last existing handler registration:

```go
r.Get("/swagger/openapi.internal.json", swaggerspec.ServeJSON)
r.Get("/swagger/openapi.internal.yaml", swaggerspec.ServeYAML)
r.Get("/swagger/*", httpSwagger.Handler(
    httpSwagger.URL("/swagger/openapi.internal.json"),
))
```

- [ ] **Step 4: Verify it builds**

```bash
cd backend && go build ./...
```

Expected: SUCCESS.

- [ ] **Step 5: Smoke test — unauthenticated request gets 401**

```bash
just backend smoke-test
```

The smoke test hits `/healthz` (not `/swagger/*`), so it continues to pass. Manually verify in a separate terminal while the dev server is running (after `just backend dev`):

```bash
curl -i http://localhost:8080/swagger/index.html
```

Expected: 401 Unauthorized (middleware.Auth rejects the unauthenticated request).

- [ ] **Step 6: Run backend tests**

```bash
just backend test
```

Expected: PASS. If any router/handler test hard-codes the old unauthenticated `/swagger/*` path, update it to expect 401 without credentials or to expect 200 with a mock auth token.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/cmd/serve/router.go
git commit -m "feat(tra-394): auth-gate /swagger/* + serve embedded internal spec"
```

---

## Task 18: Extend `just backend build` and `just backend validate`

**Files:**
- Modify: `backend/justfile`

- [ ] **Step 1: Make `build` depend on `api-spec`**

Edit `backend/justfile`. Change the `build` recipe from:

```
# Build backend binary with version injection
build:
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .
```

to:

```
# Build backend binary with version injection (regenerates internal spec first)
build: api-spec
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .
```

- [ ] **Step 2: Extend `validate` to include `api-spec` and `redocly lint`**

Add a new recipe before `validate`:

```
# Lint the public OpenAPI spec with Redocly CLI (requires pnpm)
api-lint:
    pnpm dlx @redocly/cli lint ../docs/api/openapi.public.yaml --extends=recommended
```

Then change `validate`:

```
# Run all backend validation checks
validate: api-spec api-lint lint test build smoke-test
```

- [ ] **Step 3: Test the full validate chain**

```bash
just backend validate
```

Expected: every step succeeds (api-spec regenerates → lint passes → build runs → smoke-test passes).

- [ ] **Step 4: Commit**

```bash
git add backend/justfile
git commit -m "feat(tra-394): extend just backend build/validate with api-spec + redocly lint"
```

---

## Task 19: `api-spec.yml` CI workflow (PR drift check + lint)

**Files:**
- Create: `.github/workflows/api-spec.yml`

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/api-spec.yml`:

```yaml
name: API Spec

on:
  pull_request:
    paths:
      - 'backend/**'
      - 'docs/api/**'
      - '.github/workflows/api-spec.yml'
  push:
    branches: [main]

concurrency:
  group: api-spec-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

permissions:
  contents: read

jobs:
  api-spec:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true
          cache-dependency-path: backend/go.sum

      - name: Set up pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'pnpm'
          cache-dependency-path: pnpm-lock.yaml

      - name: Install just
        uses: extractions/setup-just@v2

      - name: Install swag CLI
        run: go install github.com/swaggo/swag/cmd/swag@latest

      - name: Regenerate public + internal specs
        run: just backend api-spec

      - name: Regenerate Postman collection
        run: |
          pnpm dlx openapi-to-postmanv2 \
            -s docs/api/openapi.public.json \
            -o docs/api/trakrf-api.postman_collection.json \
            -p -O folderStrategy=Paths

      - name: Drift check — committed public spec must match regeneration
        run: |
          if ! git diff --exit-code docs/api/; then
            echo "::error::docs/api/ is stale. Run 'just backend api-spec' locally, regenerate the Postman collection, and commit the result."
            exit 1
          fi

      - name: Redocly lint
        run: pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml --extends=recommended

      - name: Upload specs as artifact
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: openapi-specs
          path: |
            docs/api/openapi.public.json
            docs/api/openapi.public.yaml
            docs/api/trakrf-api.postman_collection.json
          retention-days: 7
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/api-spec.yml
git commit -m "feat(tra-394): api-spec CI workflow — drift check + redocly lint"
```

---

## Task 20: `publish-api-docs.yml` CI workflow (cross-repo publish)

**Files:**
- Create: `.github/workflows/publish-api-docs.yml`

**Prerequisite (manual):** create a GitHub Personal Access Token (fine-grained) with `Contents: Read & Write` and `Pull requests: Read & Write` scopes on `trakrf/docs`. Store it as secret `TRAKRF_DOCS_PAT` on `trakrf/platform`. Document this in the workflow's header comment so the next maintainer knows how to rotate.

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/publish-api-docs.yml`:

```yaml
# Publishes the OpenAPI spec + Postman collection from platform main to
# trakrf/docs via a cross-repo PR. Requires repo secret TRAKRF_DOCS_PAT —
# a fine-grained PAT scoped to trakrf/docs with Contents (read/write) and
# Pull requests (read/write) permissions.
name: Publish API Docs

on:
  push:
    branches: [main]
    paths:
      - 'docs/api/**'
      - '.github/workflows/publish-api-docs.yml'
  workflow_dispatch:

permissions:
  contents: read

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout platform
        uses: actions/checkout@v4
        with:
          path: platform

      - name: Get short SHA
        id: sha
        run: echo "sha=$(git -C platform rev-parse --short HEAD)" >> "$GITHUB_OUTPUT"

      - name: Checkout trakrf-docs
        uses: actions/checkout@v4
        with:
          repository: trakrf/docs
          token: ${{ secrets.TRAKRF_DOCS_PAT }}
          path: docs-repo

      - name: Copy spec + collection into docs-repo/static/api/
        run: |
          mkdir -p docs-repo/static/api
          cp platform/docs/api/openapi.public.json docs-repo/static/api/
          cp platform/docs/api/openapi.public.yaml docs-repo/static/api/
          cp platform/docs/api/trakrf-api.postman_collection.json docs-repo/static/api/

      - name: Open PR against trakrf-docs
        working-directory: docs-repo
        env:
          GH_TOKEN: ${{ secrets.TRAKRF_DOCS_PAT }}
          PLATFORM_SHA: ${{ steps.sha.outputs.sha }}
        run: |
          BRANCH="sync/platform-${PLATFORM_SHA}"
          git config user.name "trakrf-bot"
          git config user.email "bot@trakrf.id"
          git checkout -B "$BRANCH"
          if git diff --quiet static/api/; then
            echo "No spec changes to publish — exiting cleanly."
            exit 0
          fi
          git add static/api/
          git commit -m "chore(api): sync spec from platform@${PLATFORM_SHA}"
          git push -u origin "$BRANCH" --force
          gh pr create \
            --title "chore(api): sync spec from platform@${PLATFORM_SHA}" \
            --body "Automated spec sync from trakrf/platform main (commit ${PLATFORM_SHA}).

          Preview this PR to review the rendered Redoc before merging." \
            --assignee miks2u || echo "PR may already exist; that's ok."
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/publish-api-docs.yml
git commit -m "feat(tra-394): publish-api-docs CI workflow — cross-repo PR to trakrf/docs"
```

---

## Task 21: trakrf-docs — install redocusaurus + configure `/api` route

**Repo:** `trakrf/docs` (separate repo, local checkout at `/home/mike/trakrf-docs`)

**Prerequisite:** create a feature branch in the trakrf-docs repo:

```bash
cd /home/mike/trakrf-docs && git checkout -b feature/tra-394-redocusaurus
```

**Files:**
- Modify: `package.json` (add `redocusaurus`, `redoc`, `@docusaurus/plugin-client-redirects`)
- Modify: `docusaurus.config.ts` (add preset + redirects)
- Create (initially empty dir): `static/api/`

- [ ] **Step 1: Install dependencies**

```bash
cd /home/mike/trakrf-docs && pnpm add redocusaurus redoc @docusaurus/plugin-client-redirects
```

Expected: `redocusaurus`, `redoc`, and `@docusaurus/plugin-client-redirects` appear in `package.json` under `dependencies`.

- [ ] **Step 2: Add the redocusaurus preset to `docusaurus.config.ts`**

Edit the `presets` array in `docusaurus.config.ts` to add redocusaurus alongside the existing `classic` preset:

```ts
presets: [
  [
    "classic",
    {
      docs: {
        sidebarPath: "./sidebars.ts",
        editUrl: "https://github.com/trakrf/docs/edit/main/",
      },
      blog: false,
      theme: {
        customCss: "./src/css/custom.css",
      },
    } satisfies Preset.Options,
  ],
  [
    "redocusaurus",
    {
      specs: [
        {
          id: "trakrf-api",
          spec: "static/api/openapi.public.yaml",
          route: "/api",
        },
      ],
      theme: {
        primaryColor: "var(--ifm-color-primary)",
      },
    },
  ],
],
```

- [ ] **Step 3: Add the client-redirects plugin for short-form spec URLs**

Add (or create) a `plugins` array in `docusaurus.config.ts`:

```ts
plugins: [
  [
    "@docusaurus/plugin-client-redirects",
    {
      redirects: [
        { from: "/api/openapi.json", to: "/api/openapi.public.json" },
        { from: "/api/openapi.yaml", to: "/api/openapi.public.yaml" },
      ],
    },
  ],
],
```

- [ ] **Step 4: Add an `API Reference` navbar entry**

In the `themeConfig.navbar.items` array, add after the existing `API` sidebar item:

```ts
{ to: "/api", label: "API Reference", position: "left" },
```

- [ ] **Step 5: Seed `static/api/` with a placeholder spec for local dev**

Docusaurus will fail to build without a spec at the configured path. Copy the current committed spec from the platform repo:

```bash
mkdir -p /home/mike/trakrf-docs/static/api
cp /home/mike/platform/.worktrees/tra-394-openapi-spec/docs/api/openapi.public.yaml /home/mike/trakrf-docs/static/api/
cp /home/mike/platform/.worktrees/tra-394-openapi-spec/docs/api/openapi.public.json /home/mike/trakrf-docs/static/api/
cp /home/mike/platform/.worktrees/tra-394-openapi-spec/docs/api/trakrf-api.postman_collection.json /home/mike/trakrf-docs/static/api/
```

The cross-repo CI workflow (Task 20) will replace these on every platform main-merge; this initial copy just lets the first build succeed.

- [ ] **Step 6: Build locally and verify**

```bash
cd /home/mike/trakrf-docs && pnpm build
```

Expected: build succeeds. No broken links reported.

Start the dev server:

```bash
cd /home/mike/trakrf-docs && pnpm dev
```

Visit `http://localhost:3000/api` in a browser. Expected: Redoc renders the TrakRF API reference.

- [ ] **Step 7: Commit**

```bash
cd /home/mike/trakrf-docs && git add package.json pnpm-lock.yaml docusaurus.config.ts static/api/
git commit -m "feat(tra-394): redocusaurus + /api route + seed spec"
```

---

## Task 22: trakrf-docs — Postman MDX page + sidebar integration

**Repo:** `trakrf/docs`

**Files:**
- Create: `docs/api/postman.mdx`
- Modify: `sidebars.ts` (add the postman page to the API sidebar)

- [ ] **Step 1: Create `docs/api/postman.mdx`**

```mdx
---
title: Postman collection
sidebar_position: 10
---

# Postman collection

A Postman collection for the TrakRF API is available for download. It's
regenerated from the OpenAPI spec on every platform release, so it always
reflects the current v1 contract.

## Download

- [Postman collection (JSON)](/api/trakrf-api.postman_collection.json)

## Raw OpenAPI spec

If you'd rather use a codegen tool, the raw spec is available at:

- [`/api/openapi.json`](/api/openapi.json) (JSON)
- [`/api/openapi.yaml`](/api/openapi.yaml) (YAML)

The rendered interactive reference lives at [`/api`](/api).

## Importing into Postman

1. Open Postman.
2. Click **Import** → **File**.
3. Select the downloaded `trakrf-api.postman_collection.json`.
4. In the collection variables, set:
    - `baseUrl` to `https://trakrf.id/api/v1`.
    - `apiKey` to your API key (create one in **Settings → API Keys** on trakrf.id).
5. The collection's **Authorization** is preconfigured as a Bearer token referencing `{{apiKey}}`.
```

- [ ] **Step 2: Add the page to the API sidebar**

Edit `sidebars.ts`. Find the `apiSidebar` entry and add an entry for the new page. If the sidebar is auto-generated (`'autogenerated'` type), Docusaurus will pick up `docs/api/postman.mdx` automatically and no further edit is needed.

If it's manual, add:

```ts
apiSidebar: [
  // ...existing entries
  'api/postman',
],
```

- [ ] **Step 3: Build and preview**

```bash
cd /home/mike/trakrf-docs && pnpm build && pnpm serve
```

Visit `http://localhost:3000/docs/api/postman`. Expected: the page renders with a working download link and working links to `/api`, `/api/openapi.json`, `/api/openapi.yaml`.

- [ ] **Step 4: Commit**

```bash
cd /home/mike/trakrf-docs && git add docs/api/postman.mdx sidebars.ts
git commit -m "feat(tra-394): Postman MDX page + API sidebar entry"
```

---

## Task 23: Open PRs in both repos

- [ ] **Step 1: Push the platform branch and open PR**

```bash
# From /home/mike/platform/.worktrees/tra-394-openapi-spec
git push -u origin feature/tra-394-openapi-spec
gh pr create --title "feat(tra-394): OpenAPI spec generation + Redoc docs" --body "$(cat <<'EOF'
## Summary
- New in-repo Go tool `apispec` converts swaggo 2.0 → OpenAPI 3.0, partitions by `public`/`internal` tag, post-processes security scheme + info + servers
- Committed public spec at `docs/api/openapi.public.{json,yaml}` + Postman collection
- Internal spec embedded into binary, served at auth-gated `/swagger/*`
- CI workflows: PR drift check + Redocly lint; main-merge cross-repo PR to trakrf/docs
- Enum-openness audit applied per design doc

Design: `docs/superpowers/specs/2026-04-19-tra394-openapi-docs-design.md`
Plan: `docs/superpowers/plans/2026-04-19-tra-394-openapi-docs.md`

## Test plan
- [ ] `just backend validate` passes locally (api-spec + redocly lint + lint + test + build + smoke-test)
- [ ] `/swagger/*` returns 401 unauthenticated, 200 authenticated
- [ ] `docs/api/openapi.public.{json,yaml}` is current
- [ ] CI `api-spec.yml` drift check passes

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 2: Push the trakrf-docs branch and open PR**

```bash
cd /home/mike/trakrf-docs && git push -u origin feature/tra-394-redocusaurus
gh pr create --title "feat(tra-394): redocusaurus + /api route + Postman page" --body "$(cat <<'EOF'
## Summary
- Adds `redocusaurus@^2`, `redoc@^2`, `@docusaurus/plugin-client-redirects`
- Renders Redoc at `/api` from `static/api/openapi.public.yaml`
- Redirects `/api/openapi.{json,yaml}` → `/api/openapi.public.{json,yaml}` (per TRA-392 promised URLs)
- Navbar entry `API Reference` → `/api`
- Postman collection MDX page with download + import instructions
- Seed spec copied from trakrf/platform; future updates arrive via automated cross-repo PR from platform main

Related: trakrf/platform TRA-394

## Test plan
- [ ] `pnpm build` succeeds
- [ ] `/api` renders Redoc correctly
- [ ] `/api/openapi.json` and `/api/openapi.yaml` resolve via redirects
- [ ] `/api/trakrf-api.postman_collection.json` downloads successfully
- [ ] Postman MDX page renders with working links

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Manual verification before merge**

Preview-deploy both PRs. Verify:

- Platform preview: `app.preview.trakrf.id/swagger/*` (authenticated) renders the internal spec.
- trakrf-docs preview (wherever it deploys to): `/api` renders the current committed spec via Redoc.

---

## Self-Review

### Spec coverage

Every section of the design doc is covered by at least one task:

- §Architecture → Tasks 2-7 (apispec tool) + Task 8 (recipe)
- §Annotation strategy (tagging) → Tasks 9-10
- §Annotation strategy (public enhancements) → Tasks 11-12
- §Annotation strategy (security scheme) → Task 1
- §Build pipeline → Tasks 7-8, 18
- §trakrf-docs integration → Tasks 21-22
- §Internal /swagger/* changes → Tasks 16-17
- §Drift detection & validation → Tasks 18-19
- §Enum openness → Task 13
- §Non-goals → not implemented (correctly)
- §Acceptance criteria → Task 23 test plan

### Placeholder scan

No TODO/TBD. Every code block contains the actual code to paste. Command expected outputs are specified. Task 13 Step 4 includes a conditional fallback — concrete swag annotations to apply if the struct-tag form silently drops the extension.

### Type consistency

- `convertV2ToV3`, `partition`, `postprocessPublic`, `postprocessInternal`, `emit`, `run` — all function names consistent between where they're defined and where they're called.
- Struct field tags (`extensions:"x-extensible-enum=true"`) use swaggo's documented format.
- CLI flags (`--in`, `--public-out`, `--internal-out`) match between Task 2 scaffold, Task 7 wiring, and Task 8 recipe.
- File paths consistent throughout (the embed path in Task 16 matches the `--internal-out` target in Task 8).
