package asset

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC10: POST /assets and GET /assets must agree on the metadata shape.
// When no metadata is supplied, the public view must serialize "metadata": {}
// (not omit the field), matching the GET list shape.
func TestToPublicAssetView_NilMetadataSerializesAsEmptyObject(t *testing.T) {
	in := AssetWithLocation{
		AssetView: AssetView{
			Asset: Asset{
				Identifier: "FORK-007",
				Name:       "Forklift 7",
				Metadata:   nil,
			},
		},
	}

	got := ToPublicAssetView(in)

	data, err := json.Marshal(got)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	raw, present := parsed["metadata"]
	assert.True(t, present, "metadata must be present in JSON output, not omitted")
	assert.Equal(t, map[string]any{}, raw, "metadata must serialize as empty object when nil")
}

func TestToPublicAssetView_PopulatedMetadataPreserved(t *testing.T) {
	in := AssetWithLocation{
		AssetView: AssetView{
			Asset: Asset{
				Identifier: "FORK-007",
				Name:       "Forklift 7",
				Metadata:   map[string]any{"color": "red"},
			},
		},
	}

	got := ToPublicAssetView(in)

	data, err := json.Marshal(got)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, map[string]any{"color": "red"}, parsed["metadata"])
}
