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
