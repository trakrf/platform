package asset

import (
	"encoding/json"
	"testing"
	"time"

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
				ExternalKey: "FORK-007",
				Name:        "Forklift 7",
				Metadata:    nil,
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
				ExternalKey: "FORK-007",
				Name:        "Forklift 7",
				Metadata:    map[string]any{"color": "red"},
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

// TRA-610 / BB18 §1.8: description is always emitted (null when unset).
// Supersedes the TRA-547 §2.2 omit-when-empty test.
func TestPublicAssetView_DescriptionAlwaysEmittedNullWhenEmpty(t *testing.T) {
	v := ToPublicAssetView(AssetWithLocation{
		AssetView: AssetView{
			Asset: Asset{ExternalKey: "A1", Name: "n"},
		},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	desc, present := parsed["description"]
	assert.True(t, present, "description must always be present (TRA-610)")
	assert.Nil(t, desc, "description must be JSON null when empty (TRA-610)")
}

func TestPublicAssetView_DescriptionEmittedWhenPopulated(t *testing.T) {
	v := ToPublicAssetView(AssetWithLocation{
		AssetView: AssetView{
			Asset: Asset{ExternalKey: "A1", Name: "n", Description: "the description"},
		},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, "the description", parsed["description"])
}

// TRA-610 / BB18 §1.8: valid_to is always emitted (null when nil).
// Supersedes the TRA-547 §2.2 omit-when-nil test.
func TestPublicAssetView_ValidToAlwaysEmittedNullWhenNil(t *testing.T) {
	v := PublicAssetView{ExternalKey: "A1", Name: "n"}

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	vt, present := parsed["valid_to"]
	assert.True(t, present, "valid_to must always be present (TRA-610)")
	assert.Nil(t, vt, "valid_to must be JSON null when nil (TRA-610)")
}

func TestPublicAssetView_ValidToEmittedWhenPopulated(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := PublicAssetView{ExternalKey: "A1", Name: "n", ValidTo: &when}

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, "2026-01-01T00:00:00Z", parsed["valid_to"])
}
