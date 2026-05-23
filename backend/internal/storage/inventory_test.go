package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedupInts(t *testing.T) {
	t.Run("preserves first-seen order, drops repeats", func(t *testing.T) {
		got := dedupInts([]int{3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5})
		assert.Equal(t, []int{3, 1, 4, 5, 9, 2, 6}, got)
	})
	t.Run("returns empty for empty", func(t *testing.T) {
		got := dedupInts([]int{})
		assert.Empty(t, got)
	})
	t.Run("no-op when already unique", func(t *testing.T) {
		got := dedupInts([]int{1, 2, 3, 4})
		assert.Equal(t, []int{1, 2, 3, 4}, got)
	})
	t.Run("collapses an all-duplicate run to one", func(t *testing.T) {
		// SaveInventoryScans depends on this: multi-tag-per-asset scans (e.g.
		// two RFID tags both bound to ASSET-0020) must collapse to a single
		// row so the validation count matches input length. (TRA-812)
		got := dedupInts([]int{42, 42, 42})
		assert.Equal(t, []int{42}, got)
	})
}

func TestInventoryErrorTypes(t *testing.T) {
	t.Run("location access denied error", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:     "location",
			OrgID:      123,
			LocationID: 456,
		}
		assert.Contains(t, err.Error(), "location not found or access denied")
		// Sanitize check: integer surrogate IDs must not appear in Error() output.
		assert.NotContains(t, err.Error(), "org_id=123",
			"InventoryAccessError.Error() must not leak org_id surrogate")
		assert.NotContains(t, err.Error(), "location_id=456",
			"InventoryAccessError.Error() must not leak location_id surrogate")
		assert.True(t, err.IsAccessDenied())
	})

	t.Run("asset access denied error names a real cause and stays generic on the wire (TRA-812)", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:              "assets",
			OrgID:               123,
			AssetIDs:            []int{1, 2, 3},
			ValidCount:          1,
			TotalCount:          3,
			MissingAssetIDs:     []int{2},
			SoftDeletedAssetIDs: []int{3},
		}
		// Message counts the failure but does not claim "org mismatch" the way
		// the prior wording did — that was wrong for three of the four
		// failure modes (duplicates, soft-deleted, missing, only the last
		// being genuine cross-org).
		assert.Equal(t, "2 of 3 assets are unavailable; refresh and try again", err.Error())
		// Sanitize check: per-bucket IDs and surrogate org/asset IDs must
		// never appear in the user-facing string. They go through the handler
		// log via the typed fields instead.
		assert.NotContains(t, err.Error(), "org_id")
		assert.NotContains(t, err.Error(), "asset_id")
		assert.NotContains(t, err.Error(), "missing")
		assert.NotContains(t, err.Error(), "cross_org")
		assert.NotContains(t, err.Error(), "soft")
		assert.True(t, err.IsAccessDenied())
	})
}
