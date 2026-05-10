package location

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-610 / BB18 §1.8: description and valid_to are always emitted (null
// when unset). Supersedes the TRA-547 §2.2 omit-when-nil behavior —
// generated clients in stricter languages need a stable shape rather than
// the three-state required-vs-omitted-vs-null mess.
//
// updated_at was nullable on this view through TRA-610 but TRA-649 / BB23
// S2 made it non-nullable: the DB column is NOT NULL with default
// CURRENT_TIMESTAMP, so an emitted null was never reachable from real
// data and forced gratuitous null-checks in the generated client. See
// TestPublicLocationView_UpdatedAtNonNullable below.
func TestPublicLocationView_OptionalFieldsAlwaysEmittedNullWhenUnset(t *testing.T) {
	v := ToPublicLocationView(LocationWithParent{
		LocationView: LocationView{Location: Location{ExternalKey: "L1", Name: "n"}},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	for _, field := range []string{"description", "valid_to"} {
		raw, present := parsed[field]
		assert.True(t, present, "%s must always be present (TRA-610)", field)
		assert.Nil(t, raw, "%s must be JSON null when unset (TRA-610)", field)
	}
}

// TRA-649 / BB23 S2: PublicLocationView.updated_at is non-nullable to
// match PublicAssetView. The pointer source field can be nil during
// constructor calls; ToPublicLocationView dereferences to the zero time
// in that case so the JSON output remains a string. The locations table
// schema (NOT NULL DEFAULT CURRENT_TIMESTAMP) makes the zero-time path
// unreachable from real data.
func TestPublicLocationView_UpdatedAtNonNullable(t *testing.T) {
	v := ToPublicLocationView(LocationWithParent{
		LocationView: LocationView{Location: Location{ExternalKey: "L1", Name: "n"}},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	raw, present := parsed["updated_at"]
	assert.True(t, present, "updated_at must always be present")
	assert.IsType(t, "", raw, "updated_at must be a JSON string, never null")
}

func TestPublicLocationView_OptionalFieldsEmittedWhenPopulated(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := ToPublicLocationView(LocationWithParent{
		LocationView: LocationView{
			Location: Location{
				ExternalKey: "L1",
				Name:        "n",
				Description: "the description",
				ValidTo:     &when,
				UpdatedAt:   &when,
			},
		},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, "the description", parsed["description"])
	assert.Equal(t, "2026-01-01T00:00:00Z", parsed["valid_to"])
	assert.Equal(t, "2026-01-01T00:00:00Z", parsed["updated_at"])
}

// TRA-554: PublicLocationView.parent_id and parent_external_key are nullable
// (always present, JSON null when no parent).
func TestPublicLocationView_ParentFieldsNullableNotOmitted(t *testing.T) {
	v := PublicLocationView{
		ExternalKey: "L1",
		Name:        "n",
	}
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	pid, present := parsed["parent_id"]
	assert.True(t, present, "parent_id must be present (nullable, not omitted)")
	assert.Nil(t, pid, "parent_id must be JSON null when no parent")
	pek, present := parsed["parent_external_key"]
	assert.True(t, present, "parent_external_key must be present (nullable, not omitted)")
	assert.Nil(t, pek, "parent_external_key must be JSON null when no parent")
}
