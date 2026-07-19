package storage

import (
	"testing"

	"github.com/trakrf/platform/backend/internal/models/kit"
)

// Fixture: kit 10 "1184015" = assets 1 (coupon) + 2 (tote);
// kit 20 "1184099" = assets 3 (coupon) + 4 (tote).
func kitFixture() ([]kitMembership, []rosterMember) {
	memberships := []kitMembership{
		{AssetID: 1, KitID: 10, KitLabel: "1184015"},
		{AssetID: 2, KitID: 10, KitLabel: "1184015"},
		{AssetID: 3, KitID: 20, KitLabel: "1184099"},
		{AssetID: 4, KitID: 20, KitLabel: "1184099"},
	}
	roster := []rosterMember{
		{KitID: 10, AssetID: 1, Role: strPtr("coupon"), Name: "1184015 coupon", EPCs: []string{"AAA1"}},
		{KitID: 10, AssetID: 2, Role: strPtr("tote"), Name: "1184015 tote", EPCs: []string{"AAA2"}},
		{KitID: 20, AssetID: 3, Role: strPtr("coupon"), Name: "1184099 coupon", EPCs: []string{"BBB3"}},
		{KitID: 20, AssetID: 4, Role: strPtr("tote"), Name: "1184099 tote", EPCs: []string{"BBB4"}},
	}
	return memberships, roster
}

func TestClassifyVerification_CompleteSingleKit(t *testing.T) {
	memberships, roster := kitFixture()
	scans := []scannedEPC{{EPC: "AAA1", AssetID: 1}, {EPC: "AAA2", AssetID: 2}}

	resp, perKitUnexpected := classifyVerification(scans, memberships, roster)

	if len(resp.Kits) != 1 {
		t.Fatalf("expected 1 kit, got %d", len(resp.Kits))
	}
	k := resp.Kits[0]
	if k.KitID != 10 || k.Label != "1184015" || k.Result != kit.ResultComplete {
		t.Errorf("unexpected kit result: %+v", k)
	}
	if len(k.Seen) != 2 || len(k.Missing) != 0 {
		t.Errorf("expected 2 seen / 0 missing, got %d/%d", len(k.Seen), len(k.Missing))
	}
	if k.Seen[0].AssetID != 1 || k.Seen[0].Name != "1184015 coupon" || *k.Seen[0].Role != "coupon" {
		t.Errorf("unexpected seen[0]: %+v", k.Seen[0])
	}
	if len(resp.Unexpected) != 0 {
		t.Errorf("single-kit scan must have empty unexpected, got %+v", resp.Unexpected)
	}
	if resp.Unexpected == nil || resp.UnknownEPCs == nil || resp.Kits == nil {
		t.Error("response slices must be non-nil")
	}
	if len(perKitUnexpected[10]) != 0 {
		t.Errorf("per-kit unexpected must be empty, got %v", perKitUnexpected[10])
	}
}

func TestClassifyVerification_IncompleteSingleKit(t *testing.T) {
	memberships, roster := kitFixture()
	scans := []scannedEPC{{EPC: "AAA1", AssetID: 1}}

	resp, _ := classifyVerification(scans, memberships, roster)

	if len(resp.Kits) != 1 {
		t.Fatalf("expected 1 kit, got %d", len(resp.Kits))
	}
	k := resp.Kits[0]
	if k.Result != kit.ResultIncomplete {
		t.Errorf("expected incomplete, got %s", k.Result)
	}
	if len(k.Missing) != 1 || k.Missing[0].AssetID != 2 {
		t.Fatalf("expected asset 2 missing, got %+v", k.Missing)
	}
	if len(k.Missing[0].EPCs) != 1 || k.Missing[0].EPCs[0] != "AAA2" {
		t.Errorf("missing[].epcs must carry the member's tag values for Locate, got %v", k.Missing[0].EPCs)
	}
}

func TestClassifyVerification_CrossKitUnexpected(t *testing.T) {
	memberships, roster := kitFixture()
	// Full kit 10 plus a stray coupon from kit 20 in the tote.
	scans := []scannedEPC{
		{EPC: "AAA1", AssetID: 1},
		{EPC: "AAA2", AssetID: 2},
		{EPC: "BBB3", AssetID: 3},
	}

	resp, perKitUnexpected := classifyVerification(scans, memberships, roster)

	if len(resp.Kits) != 2 {
		t.Fatalf("expected both touched kits reported, got %d", len(resp.Kits))
	}
	// Ordered by first-touched scan order: kit 10 then kit 20.
	if resp.Kits[0].KitID != 10 || resp.Kits[1].KitID != 20 {
		t.Errorf("kits must be ordered by first-seen scan order: %+v", resp.Kits)
	}
	if resp.Kits[0].Result != kit.ResultComplete || resp.Kits[1].Result != kit.ResultIncomplete {
		t.Errorf("expected kit10 complete, kit20 incomplete: %+v", resp.Kits)
	}
	// >=2 kits touched: every seen member is unexpected from the other kit's
	// perspective; each entry annotated with its OWN kit so the frontend can
	// anchor-filter (exclude belongs_to == displayed kit).
	if len(resp.Unexpected) != 3 {
		t.Fatalf("expected 3 unexpected entries, got %+v", resp.Unexpected)
	}
	stray := resp.Unexpected[2]
	if stray.AssetID != 3 || stray.EPC != "BBB3" || stray.Name != "1184099 coupon" ||
		stray.BelongsToKitID != 20 || stray.BelongsToKitLabel != "1184099" {
		t.Errorf("unexpected stray annotation: %+v", stray)
	}
	if resp.Unexpected[0].BelongsToKitID != 10 || resp.Unexpected[1].BelongsToKitID != 10 {
		t.Errorf("kit10 members must be annotated with kit10: %+v", resp.Unexpected[:2])
	}
	// Persisted per-kit sets are the precise "members of OTHER kits" view.
	if got := perKitUnexpected[10]; len(got) != 1 || got[0] != 3 {
		t.Errorf("kit10 unexpected must be [3], got %v", got)
	}
	if got := perKitUnexpected[20]; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("kit20 unexpected must be [1 2], got %v", got)
	}
}

func TestClassifyVerification_UnknownEPCs(t *testing.T) {
	memberships, roster := kitFixture()
	scans := []scannedEPC{
		{EPC: "AAA1", AssetID: 1},
		{EPC: "ZZZZ", AssetID: 0},
		{EPC: "YYYY", AssetID: 0},
		{EPC: "ZZZZ", AssetID: 0}, // duplicate read
	}

	resp, _ := classifyVerification(scans, memberships, roster)

	if len(resp.UnknownEPCs) != 2 || resp.UnknownEPCs[0] != "ZZZZ" || resp.UnknownEPCs[1] != "YYYY" {
		t.Errorf("unknown epcs must dedupe preserving order, got %v", resp.UnknownEPCs)
	}
}

func TestClassifyVerification_Mixed(t *testing.T) {
	memberships, roster := kitFixture()
	// Kit 10 partially seen, stray kit-20 coupon, an asset in no kit (id 9),
	// and an unknown EPC.
	scans := []scannedEPC{
		{EPC: "AAA1", AssetID: 1},
		{EPC: "BBB3", AssetID: 3},
		{EPC: "CCC9", AssetID: 9},
		{EPC: "ZZZZ", AssetID: 0},
	}

	resp, perKitUnexpected := classifyVerification(scans, memberships, roster)

	if len(resp.Kits) != 2 {
		t.Fatalf("expected 2 kits, got %+v", resp.Kits)
	}
	if resp.Kits[0].Result != kit.ResultIncomplete || resp.Kits[1].Result != kit.ResultIncomplete {
		t.Errorf("both kits incomplete: %+v", resp.Kits)
	}
	// Asset 9 (no kit) appears nowhere.
	for _, u := range resp.Unexpected {
		if u.AssetID == 9 {
			t.Errorf("no-kit asset must not appear in unexpected: %+v", u)
		}
	}
	if len(resp.Unexpected) != 2 {
		t.Errorf("expected 2 unexpected (assets 1,3), got %+v", resp.Unexpected)
	}
	if len(resp.UnknownEPCs) != 1 || resp.UnknownEPCs[0] != "ZZZZ" {
		t.Errorf("unknown epcs: %v", resp.UnknownEPCs)
	}
	if got := perKitUnexpected[10]; len(got) != 1 || got[0] != 3 {
		t.Errorf("kit10 unexpected must be [3], got %v", got)
	}
}

func TestClassifyVerification_DuplicateEPCsSameAsset(t *testing.T) {
	memberships, roster := kitFixture()
	// Asset 1 carries two tags; both read. Asset must count once.
	scans := []scannedEPC{
		{EPC: "AAA1", AssetID: 1},
		{EPC: "AAA1B", AssetID: 1},
		{EPC: "BBB3", AssetID: 3},
	}

	resp, _ := classifyVerification(scans, memberships, roster)

	if len(resp.Kits[0].Seen) != 1 {
		t.Errorf("asset scanned via two tags must appear once in seen: %+v", resp.Kits[0].Seen)
	}
	// First-scanned EPC wins for the unexpected annotation.
	for _, u := range resp.Unexpected {
		if u.AssetID == 1 && u.EPC != "AAA1" {
			t.Errorf("expected first EPC AAA1 for asset 1, got %s", u.EPC)
		}
	}
}
