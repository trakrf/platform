package storage

import (
	"github.com/trakrf/platform/backend/internal/models/kit"
)

// scannedEPC is one input EPC after resolution; AssetID 0 means unknown.
type scannedEPC struct {
	EPC     string
	AssetID int
}

// kitMembership is an active-kit membership of a scanned asset.
type kitMembership struct {
	AssetID     int
	KitID       int
	KitLabel    string
	KitMetadata map[string]string
}

// rosterMember is one active member of a touched kit, with display fields.
type rosterMember struct {
	KitID   int
	AssetID int
	Role    *string
	Name    string
	EPCs    []string
}

// classifyVerification is the pure core of the dock check (TRA-1032). For every
// kit with >=1 scanned active member it computes seen/missing; the top-level
// unexpected list is the deduped union of each touched kit's "scanned members
// of a different kit" set, each entry annotated with its OWN kit so the
// frontend can anchor-filter. With a single touched kit that union is empty.
// The second return value is the precise per-kit unexpected asset-id set,
// persisted to kit_verifications.
func classifyVerification(scans []scannedEPC, memberships []kitMembership, roster []rosterMember) (kit.VerifyResponse, map[int][]int) {
	resp := kit.VerifyResponse{
		Kits:        []kit.VerifyKitResult{},
		Unexpected:  []kit.VerifyUnexpected{},
		UnknownEPCs: []string{},
	}

	// Dedup scanned assets preserving scan order; first EPC per asset wins for
	// the unexpected annotation, but every distinct matching EPC is kept so
	// seen members can list their scanned tag values (TRA-1033).
	seenAssetOrder := []int{}
	firstEPC := map[int]string{}
	matchedEPCs := map[int][]string{}
	matchedEPCSeen := map[int]map[string]bool{}
	unknownSeen := map[string]bool{}
	for _, s := range scans {
		if s.AssetID == 0 {
			if !unknownSeen[s.EPC] {
				unknownSeen[s.EPC] = true
				resp.UnknownEPCs = append(resp.UnknownEPCs, s.EPC)
			}
			continue
		}
		if _, ok := firstEPC[s.AssetID]; !ok {
			firstEPC[s.AssetID] = s.EPC
			seenAssetOrder = append(seenAssetOrder, s.AssetID)
		}
		if matchedEPCSeen[s.AssetID] == nil {
			matchedEPCSeen[s.AssetID] = map[string]bool{}
		}
		if !matchedEPCSeen[s.AssetID][s.EPC] {
			matchedEPCSeen[s.AssetID][s.EPC] = true
			matchedEPCs[s.AssetID] = append(matchedEPCs[s.AssetID], s.EPC)
		}
	}

	memberKit := map[int]kitMembership{}
	for _, m := range memberships {
		memberKit[m.AssetID] = m
	}

	rosterByKit := map[int][]rosterMember{}
	rosterInfo := map[int]rosterMember{}
	for _, r := range roster {
		rosterByKit[r.KitID] = append(rosterByKit[r.KitID], r)
		rosterInfo[r.AssetID] = r
	}

	// Touched kits ordered by first-seen scan order; scanned kit-member assets
	// in scan order.
	touchedOrder := []int{}
	touched := map[int]bool{}
	scannedMembers := []int{}
	scannedSet := map[int]bool{}
	for _, assetID := range seenAssetOrder {
		m, ok := memberKit[assetID]
		if !ok {
			continue // known asset, not in any active kit: ambient read, omitted
		}
		scannedMembers = append(scannedMembers, assetID)
		scannedSet[assetID] = true
		if !touched[m.KitID] {
			touched[m.KitID] = true
			touchedOrder = append(touchedOrder, m.KitID)
		}
	}

	perKitUnexpected := map[int][]int{}
	for _, kitID := range touchedOrder {
		var label string
		seen := []kit.VerifySeenMember{}
		missing := []kit.VerifyMissingMember{}
		for _, r := range rosterByKit[kitID] {
			if scannedSet[r.AssetID] {
				seen = append(seen, kit.VerifySeenMember{
					AssetID: r.AssetID, Role: r.Role, Name: r.Name,
					EPCs: matchedEPCs[r.AssetID],
				})
			} else {
				epcs := r.EPCs
				if epcs == nil {
					epcs = []string{}
				}
				missing = append(missing, kit.VerifyMissingMember{AssetID: r.AssetID, Role: r.Role, Name: r.Name, EPCs: epcs})
			}
		}
		unexpected := []int{}
		for _, assetID := range scannedMembers {
			if memberKit[assetID].KitID != kitID {
				unexpected = append(unexpected, assetID)
			}
		}
		perKitUnexpected[kitID] = unexpected

		// Every touched kit has >=1 scanned member, so its label is present in
		// the memberships of its own scanned assets; fall back to roster lookup
		// is unnecessary but label comes from membership records.
		metadata := map[string]string{}
		for _, assetID := range scannedMembers {
			if m := memberKit[assetID]; m.KitID == kitID {
				label = m.KitLabel
				if m.KitMetadata != nil {
					metadata = m.KitMetadata
				}
				break
			}
		}

		result := kit.ResultComplete
		if len(missing) > 0 {
			result = kit.ResultIncomplete
		}
		resp.Kits = append(resp.Kits, kit.VerifyKitResult{
			KitID:    kitID,
			Label:    label,
			Result:   result,
			Metadata: metadata,
			Seen:     seen,
			Missing:  missing,
		})
	}

	// Top-level unexpected: deduped union of per-kit sets == all scanned
	// members when >=2 kits are touched, empty otherwise.
	if len(touchedOrder) > 1 {
		for _, assetID := range scannedMembers {
			m := memberKit[assetID]
			resp.Unexpected = append(resp.Unexpected, kit.VerifyUnexpected{
				AssetID:           assetID,
				EPC:               firstEPC[assetID],
				Name:              rosterInfo[assetID].Name,
				BelongsToKitID:    m.KitID,
				BelongsToKitLabel: m.KitLabel,
			})
		}
	}

	return resp, perKitUnexpected
}
