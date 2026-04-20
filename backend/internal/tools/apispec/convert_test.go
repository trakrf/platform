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

	require.True(t, doc3.OpenAPI == "3.0.0" || doc3.OpenAPI == "3.0.3", "expected OpenAPI 3.0.x, got %s", doc3.OpenAPI)
	require.NotNil(t, doc3.Paths)
	assetsPath := doc3.Paths.Value("/assets")
	require.NotNil(t, assetsPath, "expected /assets path in converted doc")
	require.NotNil(t, assetsPath.Get, "expected GET operation on /assets")
	require.Equal(t, []string{"assets", "public"}, assetsPath.Get.Tags)
}
