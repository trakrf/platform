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
