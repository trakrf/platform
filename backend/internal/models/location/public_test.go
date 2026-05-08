package location

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-610 / BB18 §1.8: description, valid_to, and updated_at are always
// emitted (null when unset). Supersedes the TRA-547 §2.2 omit-when-nil
// behavior — generated clients in stricter languages need a stable
// shape rather than the three-state required-vs-omitted-vs-null mess.
func TestPublicLocationView_OptionalFieldsAlwaysEmittedNullWhenUnset(t *testing.T) {
	v := ToPublicLocationView(LocationWithParent{
		LocationView: LocationView{Location: Location{ExternalKey: "L1", Name: "n"}},
	})

	data, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	for _, field := range []string{"description", "valid_to", "updated_at"} {
		raw, present := parsed[field]
		assert.True(t, present, "%s must always be present (TRA-610)", field)
		assert.Nil(t, raw, "%s must be JSON null when unset (TRA-610)", field)
	}
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
