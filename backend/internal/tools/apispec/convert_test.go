package main

import (
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertV2ToV3_ReadsAndConverts(t *testing.T) {
	data, err := os.ReadFile("testdata/minimal-v2.json")
	require.NoError(t, err)

	doc3, err := convertV2ToV3(data)
	require.NoError(t, err)

	require.True(t, doc3.OpenAPI == "3.0.0" || doc3.OpenAPI == "3.0.3", "expected OpenAPI 3.0.x, got %s", doc3.OpenAPI)
	require.NotNil(t, doc3.Paths)
	assetsPath := doc3.Paths.Value("/assets")
	require.NotNil(t, assetsPath, "expected /assets path in converted doc")
	require.NotNil(t, assetsPath.Get, "expected GET operation on /assets")
	require.Equal(t, []string{"assets", "public"}, assetsPath.Get.Tags)
}

// TRA-626 §S1: kin-openapi's openapi2conv.ToV3 drops Swagger 2.0 collectionFormat,
// which loses the multi-vs-csv distinction codegen needs to render repeatable
// filters as `string[]` / `number[]` instead of scalar. preserveCollectionFormat
// rehydrates Style/Explode on the v3 parameter from the v2 collectionFormat.
func TestConvertV2ToV3_PreservesCollectionFormat(t *testing.T) {
	data, err := os.ReadFile("testdata/collection-formats-v2.json")
	require.NoError(t, err)

	doc3, err := convertV2ToV3(data)
	require.NoError(t, err)

	op := doc3.Paths.Value("/things").Get
	require.NotNil(t, op)

	params := map[string]*openapi3.Parameter{}
	for _, ref := range op.Parameters {
		params[ref.Value.Name] = ref.Value
	}

	assertExplode := func(t *testing.T, name, wantStyle string, wantExplode bool) {
		t.Helper()
		p, ok := params[name]
		require.True(t, ok, "param %q missing", name)
		assert.Equal(t, wantStyle, p.Style, "param %q style", name)
		require.NotNil(t, p.Explode, "param %q explode unset", name)
		assert.Equal(t, wantExplode, *p.Explode, "param %q explode", name)
	}

	assertExplode(t, "external_key", "form", true)
	assertExplode(t, "id", "form", true)
	assertExplode(t, "sort", "form", false)
	assertExplode(t, "tags_ssv", "spaceDelimited", false)
	assertExplode(t, "tags_pipes", "pipeDelimited", false)

	scalar, ok := params["scalar_no_format"]
	require.True(t, ok)
	assert.Empty(t, scalar.Style, "scalar param style should be left untouched")
	assert.Nil(t, scalar.Explode, "scalar param explode should be left untouched")
}
