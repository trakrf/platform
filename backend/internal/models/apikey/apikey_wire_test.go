package apikey

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-547 §2.1: APIKeyCreateResponse omits expires_at when unset.
func TestAPIKeyCreateResponse_ExpiresAtAbsentWhenNil(t *testing.T) {
	resp := APIKeyCreateResponse{
		ClientID: "j", ClientSecret: "trakrf_secret", Name: "n", Scopes: []string{"s"},
		// ExpiresAt left nil
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["expires_at"]
	assert.False(t, present, "expires_at must be omitted when nil per TRA-547 §2.1")
}

// TRA-547 §2.1: APIKeyListItem omits expires_at when unset.
func TestAPIKeyListItem_ExpiresAtAbsentWhenNil(t *testing.T) {
	item := APIKeyListItem{
		JTI: "j", Name: "n", Scopes: []string{"s"},
		// ExpiresAt left nil
	}
	data, err := json.Marshal(item)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["expires_at"]
	assert.False(t, present, "expires_at must be omitted when nil per TRA-547 §2.1")
}
