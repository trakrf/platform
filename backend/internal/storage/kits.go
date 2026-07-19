package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/kit"
)

// resolveEPCsQuery maps each input EPC (scan order preserved) to an asset via
// the ingest-path normalization scheme (tags.normalized_value, TRA-944).
// Unresolved EPCs come back with asset_id 0.
const resolveEPCsQuery = `
	SELECT input.epc, COALESCE(t.asset_id, 0)
	FROM unnest($2::text[]) WITH ORDINALITY AS input(epc, ord)
	LEFT JOIN LATERAL (
		SELECT asset_id FROM trakrf.tags
		WHERE org_id = $1
		  AND normalized_value = trakrf.normalize_tag_value(input.epc)
		  AND asset_id IS NOT NULL
		  AND deleted_at IS NULL
		LIMIT 1
	) t ON true
	ORDER BY input.ord
`

const activeMembershipsQuery = `
	SELECT km.asset_id, km.kit_id, k.label
	FROM trakrf.kit_members km
	JOIN trakrf.kits k ON k.id = km.kit_id
	WHERE k.org_id = $1
	  AND km.asset_id = ANY($2)
	  AND km.removed_at IS NULL
	  AND k.status = 'active'
`

// kitRosterQuery loads the active roster of a set of kits with display fields
// and each member's active rfid tag values (for Locate mode).
const kitRosterQuery = `
	SELECT km.kit_id, km.asset_id, km.role, a.name,
	       COALESCE(array_agg(t.value ORDER BY t.id) FILTER (WHERE t.value IS NOT NULL), '{}')
	FROM trakrf.kit_members km
	JOIN trakrf.assets a ON a.id = km.asset_id
	LEFT JOIN trakrf.tags t ON t.asset_id = km.asset_id
	     AND t.deleted_at IS NULL AND t.is_active AND t.type = 'rfid'
	WHERE km.kit_id = ANY($1)
	  AND km.removed_at IS NULL
	GROUP BY km.kit_id, km.id, km.asset_id, km.role, a.name
	ORDER BY km.kit_id, km.added_at, km.id
`

func resolveEPCs(ctx context.Context, tx pgx.Tx, orgID int, epcs []string) ([]scannedEPC, error) {
	rows, err := tx.Query(ctx, resolveEPCsQuery, orgID, epcs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve epcs: %w", err)
	}
	defer rows.Close()

	scans := make([]scannedEPC, 0, len(epcs))
	for rows.Next() {
		var s scannedEPC
		if err := rows.Scan(&s.EPC, &s.AssetID); err != nil {
			return nil, fmt.Errorf("failed to scan epc resolution: %w", err)
		}
		scans = append(scans, s)
	}
	return scans, rows.Err()
}

func loadActiveMemberships(ctx context.Context, tx pgx.Tx, orgID int, assetIDs []int) ([]kitMembership, error) {
	rows, err := tx.Query(ctx, activeMembershipsQuery, orgID, assetIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load kit memberships: %w", err)
	}
	defer rows.Close()

	memberships := []kitMembership{}
	for rows.Next() {
		var m kitMembership
		if err := rows.Scan(&m.AssetID, &m.KitID, &m.KitLabel); err != nil {
			return nil, fmt.Errorf("failed to scan kit membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

func loadKitRoster(ctx context.Context, tx pgx.Tx, kitIDs []int) ([]rosterMember, error) {
	rows, err := tx.Query(ctx, kitRosterQuery, kitIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load kit roster: %w", err)
	}
	defer rows.Close()

	roster := []rosterMember{}
	for rows.Next() {
		var r rosterMember
		if err := rows.Scan(&r.KitID, &r.AssetID, &r.Role, &r.Name, &r.EPCs); err != nil {
			return nil, fmt.Errorf("failed to scan kit roster: %w", err)
		}
		roster = append(roster, r)
	}
	return roster, rows.Err()
}

// loadKit assembles the full kit payload (members + latest verification)
// inside an existing org transaction. Returns nil when the kit doesn't exist.
func loadKit(ctx context.Context, tx pgx.Tx, orgID, kitID int) (*kit.Kit, error) {
	k := kit.Kit{Members: []kit.Member{}}
	err := tx.QueryRow(ctx,
		`SELECT id, label, status, created_at, updated_at FROM trakrf.kits WHERE org_id = $1 AND id = $2`,
		orgID, kitID,
	).Scan(&k.ID, &k.Label, &k.Status, &k.CreatedAt, &k.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load kit: %w", err)
	}

	roster, err := loadKitRoster(ctx, tx, []int{kitID})
	if err != nil {
		return nil, err
	}
	for _, r := range roster {
		epcs := r.EPCs
		if epcs == nil {
			epcs = []string{}
		}
		k.Members = append(k.Members, kit.Member{AssetID: r.AssetID, Role: r.Role, Name: r.Name, EPCs: epcs})
	}

	var summary kit.VerificationSummary
	err = tx.QueryRow(ctx,
		`SELECT result, verified_at FROM trakrf.kit_verifications
		 WHERE kit_id = $1 ORDER BY verified_at DESC, id DESC LIMIT 1`,
		kitID,
	).Scan(&summary.Result, &summary.VerifiedAt)
	if err == nil {
		k.LatestVerification = &summary
	} else if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to load latest verification: %w", err)
	}

	return &k, nil
}

// CommissionKit creates a kit and its create-time-fixed membership in one
// transaction (TRA-1032). Unknown EPCs auto-create a minimal asset + rfid tag
// via the create_asset_with_tags stored procedure; any member already active
// in another active kit aborts the whole commission with kit.ConflictError.
func (s *Storage) CommissionKit(ctx context.Context, orgID int, req kit.CommissionRequest) (*kit.Kit, error) {
	var result *kit.Kit
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		epcs := make([]string, len(req.Members))
		rawSeen := map[string]bool{}
		for i, m := range req.Members {
			if rawSeen[m.EPC] {
				return &kit.ValidationError{Detail: fmt.Sprintf("duplicate member epc %q", m.EPC)}
			}
			rawSeen[m.EPC] = true
			epcs[i] = m.EPC
		}

		scans, err := resolveEPCs(ctx, tx, orgID, epcs)
		if err != nil {
			return err
		}

		// Two distinct EPC strings can normalize to the same tag/asset
		// (case, leading zeros) — that's still one physical member twice.
		assetIDs := make([]int, len(req.Members))
		assetSeen := map[int]string{}
		nextSeq := 0
		for i, scan := range scans {
			assetID := scan.AssetID
			if assetID == 0 {
				name := memberName(req.Label, req.Members[i], i)
				if nextSeq == 0 {
					if err := tx.QueryRow(ctx, `
						SELECT COALESCE(MAX(CAST(SUBSTRING(external_key FROM 'ASSET-([0-9]+)') AS INT)), 0) + 1
						FROM trakrf.assets
						WHERE org_id = $1 AND external_key ~ '^ASSET-[0-9]+$' AND deleted_at IS NULL`,
						orgID,
					).Scan(&nextSeq); err != nil {
						return fmt.Errorf("failed to derive next asset sequence: %w", err)
					}
				}
				assetID, err = createMinimalAsset(ctx, tx, orgID, GenerateAssetExternalKey(nextSeq), name, scan.EPC)
				if err != nil {
					return err
				}
				nextSeq++
			}
			if prev, dup := assetSeen[assetID]; dup {
				return &kit.ValidationError{Detail: fmt.Sprintf("member epcs %q and %q resolve to the same asset", prev, scan.EPC)}
			}
			assetSeen[assetID] = scan.EPC
			assetIDs[i] = assetID
		}

		// One-active-kit-per-asset invariant, app-level (see migration 000029).
		var conflictAsset, conflictKit string
		err = tx.QueryRow(ctx, `
			SELECT a.name, k.label
			FROM trakrf.kit_members km
			JOIN trakrf.kits k ON k.id = km.kit_id
			JOIN trakrf.assets a ON a.id = km.asset_id
			WHERE km.asset_id = ANY($1) AND km.removed_at IS NULL AND k.status = 'active'
			LIMIT 1`,
			assetIDs,
		).Scan(&conflictAsset, &conflictKit)
		if err == nil {
			return &kit.ConflictError{AssetName: conflictAsset, KitLabel: conflictKit}
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("failed to check kit membership conflicts: %w", err)
		}

		var kitID int
		if err := tx.QueryRow(ctx,
			`INSERT INTO trakrf.kits (org_id, label) VALUES ($1, $2) RETURNING id`,
			orgID, req.Label,
		).Scan(&kitID); err != nil {
			return fmt.Errorf("failed to create kit: %w", err)
		}

		roles := make([]*string, len(req.Members))
		for i, m := range req.Members {
			roles[i] = m.Role
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO trakrf.kit_members (org_id, kit_id, asset_id, role)
			SELECT $1, $2, u.asset_id, u.role
			FROM unnest($3::bigint[], $4::text[]) WITH ORDINALITY AS u(asset_id, role, ord)
			ORDER BY u.ord`,
			orgID, kitID, assetIDs, roles,
		); err != nil {
			return fmt.Errorf("failed to add kit members: %w", err)
		}

		result, err = loadKit(ctx, tx, orgID, kitID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func memberName(label string, m kit.CommissionMemberRequest, idx int) string {
	if m.Name != nil && *m.Name != "" {
		return *m.Name
	}
	if m.Role != nil && *m.Role != "" {
		return fmt.Sprintf("%s %s", label, *m.Role)
	}
	return fmt.Sprintf("%s item %d", label, idx+1)
}

func createMinimalAsset(ctx context.Context, tx pgx.Tx, orgID int, externalKey, name, epc string) (int, error) {
	tagsJSON, err := json.Marshal([]map[string]string{{"type": "rfid", "value": epc}})
	if err != nil {
		return 0, fmt.Errorf("failed to serialize tag: %w", err)
	}
	var assetID int
	var tagIDs []int
	err = tx.QueryRow(ctx,
		`SELECT * FROM trakrf.create_asset_with_tags($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		orgID, externalKey, name, "", time.Now().UTC(), nil, true, map[string]any{}, tagsJSON,
	).Scan(&assetID, &tagIDs)
	if err != nil {
		return 0, parseAssetWithTagsError(err, externalKey)
	}
	return assetID, nil
}

// VerifyKits is the dock check (TRA-1032): resolve the scan session's EPCs,
// compute per-kit seen/missing plus cross-kit unexpected, persist one
// kit_verifications row per touched kit. Read + append only; never mutates
// kit state.
func (s *Storage) VerifyKits(ctx context.Context, orgID int, epcs []string) (*kit.VerifyResponse, error) {
	var resp kit.VerifyResponse
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		scans, err := resolveEPCs(ctx, tx, orgID, epcs)
		if err != nil {
			return err
		}

		assetIDs := []int{}
		for _, sc := range scans {
			if sc.AssetID != 0 {
				assetIDs = append(assetIDs, sc.AssetID)
			}
		}

		memberships := []kitMembership{}
		if len(assetIDs) > 0 {
			if memberships, err = loadActiveMemberships(ctx, tx, orgID, assetIDs); err != nil {
				return err
			}
		}

		kitIDSet := map[int]bool{}
		kitIDs := []int{}
		for _, m := range memberships {
			if !kitIDSet[m.KitID] {
				kitIDSet[m.KitID] = true
				kitIDs = append(kitIDs, m.KitID)
			}
		}

		roster := []rosterMember{}
		if len(kitIDs) > 0 {
			if roster, err = loadKitRoster(ctx, tx, kitIDs); err != nil {
				return err
			}
		}

		var perKitUnexpected map[int][]int
		resp, perKitUnexpected = classifyVerification(scans, memberships, roster)

		for _, kr := range resp.Kits {
			seen := make([]int, len(kr.Seen))
			for i, m := range kr.Seen {
				seen[i] = m.AssetID
			}
			missing := make([]int, len(kr.Missing))
			for i, m := range kr.Missing {
				missing[i] = m.AssetID
			}
			unexpected := perKitUnexpected[kr.KitID]
			if unexpected == nil {
				unexpected = []int{}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO trakrf.kit_verifications
					(org_id, kit_id, result, seen_asset_ids, missing_asset_ids, unexpected_asset_ids)
				VALUES ($1, $2, $3, $4::bigint[], $5::bigint[], $6::bigint[])`,
				orgID, kr.KitID, kr.Result, seen, missing, unexpected,
			); err != nil {
				return fmt.Errorf("failed to persist kit verification: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListKits returns kits with member counts and latest verification, filtered
// by label substring and/or member EPC (normalized match).
func (s *Storage) ListKits(ctx context.Context, orgID int, query, memberEPC string) ([]kit.KitSummary, error) {
	kits := []kit.KitSummary{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT k.id, k.label, k.status, k.created_at,
			       (SELECT COUNT(*) FROM trakrf.kit_members km
			        WHERE km.kit_id = k.id AND km.removed_at IS NULL) AS member_count,
			       v.result, v.verified_at
			FROM trakrf.kits k
			LEFT JOIN LATERAL (
				SELECT result, verified_at FROM trakrf.kit_verifications kv
				WHERE kv.kit_id = k.id ORDER BY verified_at DESC, id DESC LIMIT 1
			) v ON true
			WHERE k.org_id = $1
			  AND ($2 = '' OR k.label ILIKE '%' || $2 || '%')
			  AND ($3 = '' OR EXISTS (
				SELECT 1 FROM trakrf.kit_members km2
				JOIN trakrf.tags t ON t.asset_id = km2.asset_id AND t.deleted_at IS NULL
				WHERE km2.kit_id = k.id AND km2.removed_at IS NULL
				  AND t.normalized_value = trakrf.normalize_tag_value($3)))
			ORDER BY k.created_at DESC, k.id DESC
			LIMIT 200`,
			orgID, query, memberEPC,
		)
		if err != nil {
			return fmt.Errorf("failed to list kits: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var ks kit.KitSummary
			var result *string
			var verifiedAt *time.Time
			if err := rows.Scan(&ks.ID, &ks.Label, &ks.Status, &ks.CreatedAt, &ks.MemberCount, &result, &verifiedAt); err != nil {
				return fmt.Errorf("failed to scan kit summary: %w", err)
			}
			if result != nil && verifiedAt != nil {
				ks.LatestVerification = &kit.VerificationSummary{Result: *result, VerifiedAt: *verifiedAt}
			}
			kits = append(kits, ks)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return kits, nil
}

// GetKitByID returns the kit with members and latest verification, or nil
// when it doesn't exist in the org.
func (s *Storage) GetKitByID(ctx context.Context, orgID, kitID int) (*kit.Kit, error) {
	var result *kit.Kit
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		result, err = loadKit(ctx, tx, orgID, kitID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
