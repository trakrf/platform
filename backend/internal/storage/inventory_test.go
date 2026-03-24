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
		assert.Contains(t, err.Error(), "org_id=123")
		assert.Contains(t, err.Error(), "location_id=456")
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
		assert.Contains(t, err.Error(), "org_id=123")
		assert.Contains(t, err.Error(), "valid=2/3")
		assert.True(t, err.IsAccessDenied())
	})
}
