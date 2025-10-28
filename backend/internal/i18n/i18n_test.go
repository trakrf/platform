package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestT_SimpleKey(t *testing.T) {
	result := T("auth.login.invalid_credentials")
	assert.Equal(t, "Invalid email or password", result)
}

func TestT_WithInterpolation(t *testing.T) {
	result := T("common.invalid_id", map[string]interface{}{
		"resource": "Asset",
	})
	assert.Equal(t, "Invalid Asset ID", result)
}

func TestT_MultipleInterpolations(t *testing.T) {
	result := T("bulk_import.validation.file_too_large", map[string]interface{}{
		"size": 6000000,
		"max":  5242880,
	})
	assert.Equal(t, "file too large: 6000000 bytes (max 5242880 bytes / 5MB)", result)
}

func TestT_NestedKey(t *testing.T) {
	result := T("assets.create.invalid_json")
	assert.Equal(t, "Invalid JSON", result)
}

func TestT_MissingKey(t *testing.T) {
	result := T("non.existent.key")
	assert.Equal(t, "non.existent.key", result)
}

func TestT_NoParams(t *testing.T) {
	result := T("auth.signup.email_exists")
	assert.Equal(t, "Email already exists", result)
}
