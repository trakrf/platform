//go:build integration
// +build integration

// Package storage_test: RLS sentinel storage-method level tests.
//
// This file is a stub that will be fully implemented in Task 8, after Tasks
// 3-7 migrate the storage method signatures to accept an orgID parameter.
//
// The methods slated for signature change are:
//
//   - GetAssetByID(ctx, orgID int, id *int)
//   - GetIdentifiersByAssetID(ctx, orgID, assetID int)
//   - GetIdentifiersByLocationID(ctx, orgID, locationID int)
//   - GetIdentifierByID(ctx, orgID, identifierID int)
//
// These tests verify that the storage layer — not just raw pool SQL — properly
// wraps all six RLS-protected tables in WithOrgTx so that a sentinel-pool
// Storage instance returns correct data.
//
// Remove t.Skip and implement tests when Task 8 is complete.
package storage_test

import (
	"testing"
)

// TestRLS_SentinelMode_StorageMethods exercises post-refactor storage method
// signatures under a sentinel pool (current_org_id = '0') to prove WithOrgTx
// wrapping is in place for every RLS-protected table.
//
// Skipped until Tasks 3-7 migrate method signatures (tracked in TRA-455).
func TestRLS_SentinelMode_StorageMethods(t *testing.T) {
	t.Skip("pending Tasks 3-7 signature migration (TRA-455): " +
		"GetAssetByID, GetIdentifiersByAssetID, GetIdentifiersByLocationID, " +
		"GetIdentifierByID need orgID param before this test can run")

	// TODO Task 8: set up sentinel pool using newSentinelPool (defined in
	// rls_sentinel_raw_test.go) and call each post-refactor storage method.
	//
	// Per-file representative calls to implement:
	//
	//   assets.go:
	//     created, _ := store.CreateAsset(ctx, asset.Asset{OrgID: realOrgID, ...})
	//     got, err := store.GetAssetByID(ctx, realOrgID, &created.ID)
	//     require.NoError(t, err)
	//     require.NotNil(t, got)
	//
	//   identifiers.go:
	//     ident, _ := store.AddIdentifierToAsset(ctx, realOrgID, assetID, req)
	//     idents, err := store.GetIdentifiersByAssetID(ctx, realOrgID, assetID)
	//     require.NoError(t, err)
	//     require.Len(t, idents, 1)
	//
	//   locations.go:
	//     loc, _ := store.CreateLocation(ctx, location.Location{OrgID: realOrgID, ...})
	//     locs, err := store.ListAllLocations(ctx, realOrgID, 50, 0)
	//     require.NoError(t, err)
	//     require.NotEmpty(t, locs)
	//
	//   bulk_import_jobs.go:
	//     job, err := store.CreateBulkImportJob(ctx, realOrgID, 10)
	//     require.NoError(t, err)
	//     require.NotNil(t, job)
	//
	//   reports.go:
	//     items, err := store.ListCurrentLocations(ctx, realOrgID, report.CurrentLocationFilter{Limit: 10})
	//     require.NoError(t, err)
	//
	//   inventory.go:
	//     result, err := store.SaveInventoryScans(ctx, realOrgID, storage.SaveInventoryRequest{...})
	//     require.NoError(t, err)
	//     require.NotNil(t, result)
}
