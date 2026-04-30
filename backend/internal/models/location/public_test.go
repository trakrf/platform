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
		Identifier: "L1",
		Name:       "n",
	}
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["valid_to"]
	assert.False(t, present, "valid_to must be omitted when nil per TRA-547 §2.2")
}

// TRA-547 §2.2: PublicLocationView.parent is omitted when nil.
func TestPublicLocationView_ParentAbsentWhenNil(t *testing.T) {
	v := PublicLocationView{
		Identifier: "L1",
		Name:       "n",
	}
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["parent"]
	assert.False(t, present, "parent must be omitted when nil per TRA-547 §2.2")
}
