package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInventoryErrorTypes(t *testing.T) {
	t.Run("location access denied error", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:     "location",
			OrgID:      123,
			LocationID: 456,
		}
		assert.Contains(t, err.Error(), "location not found or access denied")
		// Surrogate IDs must NOT appear in the user-visible error message.
		assert.NotContains(t, err.Error(), "org_id=123")
		assert.NotContains(t, err.Error(), "location_id=456")
		// The struct still carries them for structured log use.
		assert.Equal(t, 123, err.OrgID)
		assert.Equal(t, 456, err.LocationID)
		assert.True(t, err.IsAccessDenied())
	})

	t.Run("asset access denied error", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:     "assets",
			OrgID:      123,
			AssetIDs:   []int{1, 2, 3},
			ValidCount: 2,
			TotalCount: 3,
		}
		assert.Contains(t, err.Error(), "assets not found or access denied")
		// Surrogate IDs must NOT appear in the user-visible error message.
		assert.NotContains(t, err.Error(), "org_id=123")
		assert.NotContains(t, err.Error(), "valid=2/3")
		// The struct still carries them for structured log use.
		assert.Equal(t, 123, err.OrgID)
		assert.Equal(t, 2, err.ValidCount)
		assert.Equal(t, 3, err.TotalCount)
		assert.True(t, err.IsAccessDenied())
	})
}
