package location

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-547 §2.2: PublicLocationView.valid_to is omitted when nil.
func TestPublicLocationView_ValidToAbsentWhenNil(t *testing.T) {
	v := PublicLocationView{
		ExternalKey: "L1",
		Name:        "n",
	}
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["valid_to"]
	assert.False(t, present, "valid_to must be omitted when nil per TRA-547 §2.2")
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
