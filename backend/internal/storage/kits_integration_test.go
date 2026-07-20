//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/kit"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func preCreateAssetWithTag(t *testing.T, db *testutil.TestDB, orgID int, name, epc string) int {
	t.Helper()
	tagType := "rfid"
	req := asset.CreateAssetWithTagsRequest{
		CreateAssetRequest: asset.CreateAssetRequest{OrgID: orgID, Name: name},
		Tags:               []shared.TagRequest{{TagType: &tagType, Value: epc}},
	}
	view, err := db.Store.CreateAssetWithTags(context.Background(), req)
	if err != nil {
		t.Fatalf("pre-create asset failed: %v", err)
	}
	return view.ID
}

func commissionFixture(label string) kit.CommissionRequest {
	role1, role2 := "coupon", "tote"
	return kit.CommissionRequest{
		Label: label,
		Members: []kit.CommissionMemberRequest{
			{EPC: label + "AA01", Role: &role1},
			{EPC: label + "AA02", Role: &role2},
		},
	}
}

func TestKits_CommissionVerifyRoundTrip(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	// Commission with two fresh EPCs: both assets auto-created with default names.
	created, err := db.Store.CommissionKit(ctx, orgID, commissionFixture("1184015"))
	if err != nil {
		t.Fatalf("commission failed: %v", err)
	}
	if created.ID == 0 || created.Label != "1184015" || created.Status != kit.StatusActive {
		t.Fatalf("unexpected kit: %+v", created)
	}
	if len(created.Members) != 2 {
		t.Fatalf("expected 2 members, got %+v", created.Members)
	}
	if created.Members[0].Name != "1184015 coupon" || created.Members[1].Name != "1184015 tote" {
		t.Errorf("auto-created names must default to '{label} {role}': %+v", created.Members)
	}
	if len(created.Members[0].EPCs) != 1 || created.Members[0].EPCs[0] != "1184015AA01" {
		t.Errorf("member epcs must round-trip: %+v", created.Members[0])
	}
	if created.LatestVerification != nil {
		t.Errorf("fresh kit must have nil latest_verification")
	}

	// Verify with both EPCs: complete, one kit_verifications row persisted.
	resp, err := db.Store.VerifyKits(ctx, orgID, []string{"1184015AA01", "1184015AA02"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if len(resp.Kits) != 1 || resp.Kits[0].Result != kit.ResultComplete {
		t.Fatalf("expected complete kit, got %+v", resp.Kits)
	}
	if len(resp.Unexpected) != 0 || len(resp.UnknownEPCs) != 0 {
		t.Errorf("clean verify must have no unexpected/unknown: %+v", resp)
	}

	// Verify with one EPC + one unknown: incomplete, missing carries epcs.
	resp, err = db.Store.VerifyKits(ctx, orgID, []string{"1184015AA01", "FFFF00"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if len(resp.Kits) != 1 || resp.Kits[0].Result != kit.ResultIncomplete {
		t.Fatalf("expected incomplete, got %+v", resp.Kits)
	}
	if len(resp.Kits[0].Missing) != 1 || len(resp.Kits[0].Missing[0].EPCs) != 1 ||
		resp.Kits[0].Missing[0].EPCs[0] != "1184015AA02" {
		t.Errorf("missing[].epcs must list the absent member's tags: %+v", resp.Kits[0].Missing)
	}
	if len(resp.UnknownEPCs) != 1 || resp.UnknownEPCs[0] != "FFFF00" {
		t.Errorf("unknown epcs: %+v", resp.UnknownEPCs)
	}

	// Two verifications persisted; latest is the incomplete one.
	detail, err := db.Store.GetKitByID(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("get kit failed: %v", err)
	}
	if detail == nil {
		t.Fatal("kit not found")
	}
	if detail.LatestVerification == nil || detail.LatestVerification.Result != kit.ResultIncomplete {
		t.Errorf("latest verification must be the incomplete run: %+v", detail.LatestVerification)
	}

	var verifCount int
	if err := db.AdminPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trakrf.kit_verifications WHERE org_id = $1", orgID).Scan(&verifCount); err != nil {
		t.Fatalf("count verifications: %v", err)
	}
	if verifCount != 2 {
		t.Errorf("expected 2 persisted verifications, got %d", verifCount)
	}
}

func TestKits_OneActiveKitConflict(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	if _, err := db.Store.CommissionKit(ctx, orgID, commissionFixture("1184015")); err != nil {
		t.Fatalf("first commission failed: %v", err)
	}

	// Second kit reusing the first kit's coupon EPC must 409 naming the kit label.
	role := "coupon"
	req := kit.CommissionRequest{
		Label: "1184099",
		Members: []kit.CommissionMemberRequest{
			{EPC: "1184015AA01", Role: &role}, // already active in 1184015
			{EPC: "1184099AA02"},
		},
	}
	_, err := db.Store.CommissionKit(ctx, orgID, req)
	var conflict *kit.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
	if conflict.KitLabel != "1184015" {
		t.Errorf("conflict must name the owning kit label, got %q", conflict.KitLabel)
	}

	// The failed commission must not have left a kit or members behind.
	var kitCount int
	if err := db.AdminPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trakrf.kits WHERE org_id = $1", orgID).Scan(&kitCount); err != nil {
		t.Fatalf("count kits: %v", err)
	}
	if kitCount != 1 {
		t.Errorf("failed commission must be atomic; expected 1 kit, got %d", kitCount)
	}
}

func TestKits_DuplicateMemberEPCRejected(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	req := kit.CommissionRequest{
		Label: "1184015",
		Members: []kit.CommissionMemberRequest{
			{EPC: "1184015AA01"},
			{EPC: "1184015AA01"},
		},
	}
	_, err := db.Store.CommissionKit(ctx, orgID, req)
	var vErr *kit.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected ValidationError for duplicate member EPCs, got %v", err)
	}
}

func TestKits_CrossKitVerifyAndLookup(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	kitA, err := db.Store.CommissionKit(ctx, orgID, commissionFixture("1184015"))
	if err != nil {
		t.Fatalf("commission A failed: %v", err)
	}
	kitB, err := db.Store.CommissionKit(ctx, orgID, commissionFixture("1184099"))
	if err != nil {
		t.Fatalf("commission B failed: %v", err)
	}

	// Full kit A + stray coupon from kit B.
	resp, err := db.Store.VerifyKits(ctx, orgID, []string{"1184015AA01", "1184015AA02", "1184099AA01"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if len(resp.Kits) != 2 {
		t.Fatalf("expected both touched kits, got %+v", resp.Kits)
	}
	found := false
	for _, u := range resp.Unexpected {
		if u.BelongsToKitID == kitB.ID {
			found = true
			if u.BelongsToKitLabel != "1184099" || u.EPC != "1184099AA01" {
				t.Errorf("stray annotation wrong: %+v", u)
			}
		}
	}
	if !found {
		t.Errorf("stray kit-B coupon must be reported unexpected: %+v", resp.Unexpected)
	}

	// Lookup by label substring.
	list, err := db.Store.ListKits(ctx, orgID, "84015", "")
	if err != nil {
		t.Fatalf("list by query failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != kitA.ID || list[0].MemberCount != 2 {
		t.Errorf("label search must find kit A with member_count 2: %+v", list)
	}
	if list[0].LatestVerification == nil {
		t.Errorf("list must include latest verification result: %+v", list[0])
	}

	// Lookup by member EPC — leading zeros and case must not matter (normalized match).
	list, err = db.Store.ListKits(ctx, orgID, "", "001184099aa02")
	if err != nil {
		t.Fatalf("list by member_epc failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != kitB.ID {
		t.Errorf("member_epc search must find kit B: %+v", list)
	}

	// Unfiltered list returns both.
	list, err = db.Store.ListKits(ctx, orgID, "", "")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 kits, got %+v", list)
	}

	// GetKitByID for a missing id returns nil, nil.
	missing, err := db.Store.GetKitByID(ctx, orgID, 999999999)
	if err != nil || missing != nil {
		t.Errorf("missing kit must be nil,nil; got %+v, %v", missing, err)
	}
}

func TestKits_CommissionResolvesExistingAsset(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	// Pre-create an asset with an rfid tag; commission must resolve, not duplicate.
	existing := preCreateAssetWithTag(t, db, orgID, "Existing Coupon", "CAFE01")

	role := "coupon"
	req := kit.CommissionRequest{
		Label: "1184020",
		Members: []kit.CommissionMemberRequest{
			{EPC: "cafe01", Role: &role}, // case-insensitive resolution to existing
			{EPC: "1184020AA02"},
		},
	}
	created, err := db.Store.CommissionKit(ctx, orgID, req)
	if err != nil {
		t.Fatalf("commission failed: %v", err)
	}
	var existingMember *kit.Member
	for i := range created.Members {
		if created.Members[i].AssetID == existing {
			existingMember = &created.Members[i]
		}
	}
	if existingMember == nil {
		t.Fatalf("existing asset must be resolved, not re-created: %+v", created.Members)
	}
	if existingMember.Name != "Existing Coupon" {
		t.Errorf("existing asset name must be preserved: %+v", existingMember)
	}

	var assetCount int
	if err := db.AdminPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trakrf.assets WHERE org_id = $1", orgID).Scan(&assetCount); err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 2 {
		t.Errorf("expected 2 assets (1 existing + 1 auto-created), got %d", assetCount)
	}
}

func TestKits_MetadataRoundTrip(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	ctx := context.Background()

	// Commission with the Howmet QA fields (TRA-1033 slide alignment).
	req := commissionFixture("1184015")
	req.Metadata = map[string]string{"part": "PN-778", "heat": "H-42", "vendor": "Acme"}
	created, err := db.Store.CommissionKit(ctx, orgID, req)
	if err != nil {
		t.Fatalf("commission with metadata failed: %v", err)
	}
	if created.Metadata["part"] != "PN-778" || created.Metadata["vendor"] != "Acme" {
		t.Errorf("commission response metadata mismatch: %+v", created.Metadata)
	}

	got, err := db.Store.GetKitByID(ctx, orgID, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get kit failed: %v", err)
	}
	if got.Metadata["heat"] != "H-42" {
		t.Errorf("get kit metadata mismatch: %+v", got.Metadata)
	}

	// The dock check carries the QA fields so scanning either tag retrieves
	// the full record.
	resp, err := db.Store.VerifyKits(ctx, orgID, []string{"1184015AA01"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if len(resp.Kits) != 1 || resp.Kits[0].Metadata["part"] != "PN-778" {
		t.Errorf("verify kit metadata mismatch: %+v", resp.Kits)
	}

	// A kit commissioned without metadata round-trips an empty object.
	bare, err := db.Store.CommissionKit(ctx, orgID, commissionFixture("1184099"))
	if err != nil {
		t.Fatalf("bare commission failed: %v", err)
	}
	if bare.Metadata == nil || len(bare.Metadata) != 0 {
		t.Errorf("bare kit metadata must be empty map, got %+v", bare.Metadata)
	}
}
