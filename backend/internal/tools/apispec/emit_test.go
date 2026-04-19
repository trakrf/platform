package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	require.True(t, strings.HasPrefix(parsedJSON["openapi"].(string), "3.0."), "expected OpenAPI 3.0.x, got %v", parsedJSON["openapi"])

	yamlBytes, err := os.ReadFile(prefix + ".yaml")
	require.NoError(t, err)
	var parsedYAML map[string]any
	require.NoError(t, yaml.Unmarshal(yamlBytes, &parsedYAML))
	assert.Equal(t, parsedJSON["openapi"], parsedYAML["openapi"], "JSON and YAML should report the same OpenAPI version")
}
