package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trakrf/platform/backend/internal/models/muster"
)

// ── Presence queries ──────────────────────────────────────────────────────────

// ListPersonPresence returns the most-recent sighting (within window) for
// each live person-asset (metadata->>'person' = 'true'). Uses DISTINCT ON
// to pick the most-recent scan per asset — same pattern as ListCurrentLocations.
// All ids use int (house style); pgx scans BIGINT into int safely on 64-bit.
func (s *Storage) ListPersonPresence(ctx context.Context, orgID int, window time.Duration) ([]muster.PersonPresence, error) {
	cutoff := time.Now().Add(-window)
	query := `
		WITH latest AS (
			SELECT DISTINCT ON (s.asset_id)
				s.asset_id,
				s.location_id,
				s.timestamp AS last_seen_at
			FROM trakrf.asset_scans s
			WHERE s.org_id = $1
			  AND s.timestamp >= $2
			ORDER BY s.asset_id, s.timestamp DESC
		)
		SELECT
			a.id         AS asset_id,
			a.name       AS label,
			l.location_id,
			l.last_seen_at
		FROM latest l
		JOIN trakrf.assets a
		  ON a.id = l.asset_id
		 AND a.org_id = $1
		 AND a.deleted_at IS NULL
		 AND a.is_active = true
		 AND a.metadata->>'person' = 'true'
		ORDER BY a.id
	`
	var out []muster.PersonPresence
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID, cutoff)
		if err != nil {
			return fmt.Errorf("ListPersonPresence query: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p muster.PersonPresence
			if err := rows.Scan(&p.AssetID, &p.Label, &p.LocationID, &p.LastSeenAt); err != nil {
				return fmt.Errorf("ListPersonPresence scan: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListZones returns all live locations for the org with MusterPoint flag set
// when metadata->>'muster_point' = 'true'. Count is always 0 (filled by engine).
func (s *Storage) ListZones(ctx context.Context, orgID int) ([]muster.ZonePresence, error) {
	query := `
		SELECT id, name, COALESCE(metadata->>'muster_point', 'false') = 'true' AS muster_point
		FROM trakrf.locations
		WHERE org_id = $1
		  AND deleted_at IS NULL
		  AND is_active = true
		ORDER BY name
	`
	var out []muster.ZonePresence
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID)
		if err != nil {
			return fmt.Errorf("ListZones query: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var z muster.ZonePresence
			if err := rows.Scan(&z.LocationID, &z.Name, &z.MusterPoint); err != nil {
				return fmt.Errorf("ListZones scan: %w", err)
			}
			out = append(out, z)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListMusterPointIDs returns the ids of all live muster-point locations for
// the org (metadata->>'muster_point' = 'true').
func (s *Storage) ListMusterPointIDs(ctx context.Context, orgID int) ([]int, error) {
	query := `
		SELECT id FROM trakrf.locations
		WHERE org_id = $1
		  AND deleted_at IS NULL
		  AND is_active = true
		  AND metadata->>'muster_point' = 'true'
		ORDER BY id
	`
	var out []int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID)
		if err != nil {
			return fmt.Errorf("ListMusterPointIDs query: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("ListMusterPointIDs scan: %w", err)
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ── Event lifecycle ───────────────────────────────────────────────────────────

// CreateMusterEvent creates an active muster event and snapshots all
// person-assets with a sighting in the last windowMinutes as entries
// (all starting with status='missing'). The presence snapshot is read
// just before the write transaction, which is acceptable "as of trigger"
// semantics for the POC. The event row and its entries are written in ONE
// transaction. Returns ErrActiveEventExists if the org already has an
// active event (partial unique index violation).
func (s *Storage) CreateMusterEvent(ctx context.Context, orgID, startedBy, windowMinutes int) (*muster.Event, error) {
	window := time.Duration(windowMinutes) * time.Minute
	presence, err := s.ListPersonPresence(ctx, orgID, window)
	if err != nil {
		return nil, fmt.Errorf("CreateMusterEvent ListPersonPresence: %w", err)
	}

	var event muster.Event
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		// Insert event row
		err := tx.QueryRow(ctx, `
			INSERT INTO trakrf.muster_events
			  (org_id, status, window_minutes, started_by)
			VALUES ($1, 'active', $2, $3)
			RETURNING id, org_id, status, started_at, ended_at, window_minutes,
			          started_by, report, metadata, created_at, updated_at`,
			orgID, windowMinutes, startedBy,
		).Scan(
			&event.ID, &event.OrgID, &event.Status,
			&event.StartedAt, &event.EndedAt, &event.WindowMinutes,
			&event.StartedBy, &event.Report, &event.Metadata,
			&event.CreatedAt, &event.UpdatedAt,
		)
		if err != nil {
			return err
		}

		// Insert entries for each person in the snapshot
		for _, p := range presence {
			var entry muster.Entry
			err := tx.QueryRow(ctx, `
				INSERT INTO trakrf.muster_event_entries
				  (org_id, muster_event_id, asset_id, label, expected_location_id, status)
				VALUES ($1, $2, $3, $4, $5, 'missing')
				RETURNING id, org_id, muster_event_id, asset_id, label,
				          expected_location_id, status, muster_location_id,
				          first_muster_seen_at, verified_by, verified_at,
				          marked_safe_by, marked_safe_at, COALESCE(marked_safe_note, ''),
				          created_at, updated_at`,
				orgID, event.ID, p.AssetID, p.Label, p.LocationID,
			).Scan(
				&entry.ID, &entry.OrgID, &entry.MusterEventID, &entry.AssetID,
				&entry.Label, &entry.ExpectedLocationID, &entry.Status,
				&entry.MusterLocationID, &entry.FirstMusterSeenAt,
				&entry.VerifiedBy, &entry.VerifiedAt,
				&entry.MarkedSafeBy, &entry.MarkedSafeAt, &entry.MarkedSafeNote,
				&entry.CreatedAt, &entry.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("insert entry for asset %d: %w", p.AssetID, err)
			}
			event.Entries = append(event.Entries, entry)
		}
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "muster_events_one_active_per_org" {
			return nil, muster.ErrActiveEventExists{}
		}
		return nil, fmt.Errorf("CreateMusterEvent: %w", err)
	}

	event.Counts = computeCounts(event.Entries)
	return &event, nil
}

// GetActiveMusterEvent returns the active event for the org (with entries and
// counts), or (nil, nil) when none exists.
func (s *Storage) GetActiveMusterEvent(ctx context.Context, orgID int) (*muster.Event, error) {
	return s.getEventByFilter(ctx, orgID, "AND me.status = 'active'")
}

// GetMusterEvent returns a muster event by id for the org (with entries and
// counts), or (nil, nil) when not found / wrong org.
func (s *Storage) GetMusterEvent(ctx context.Context, orgID int, id int) (*muster.Event, error) {
	return s.getEventByFilter(ctx, orgID, fmt.Sprintf("AND me.id = %d", id))
}

// ListMusterEvents returns all events for the org ordered by started_at DESC
// (header + counts, no entries).
func (s *Storage) ListMusterEvents(ctx context.Context, orgID int) ([]muster.Event, error) {
	query := `
		SELECT me.id, me.org_id, me.status, me.started_at, me.ended_at,
		       me.window_minutes, me.started_by, me.report, me.metadata,
		       me.created_at, me.updated_at,
		       COUNT(e.id)                                          AS expected,
		       COUNT(e.id) FILTER (WHERE e.status = 'missing')     AS missing,
		       COUNT(e.id) FILTER (WHERE e.status = 'at_muster')   AS at_muster,
		       COUNT(e.id) FILTER (WHERE e.status = 'verified')    AS verified,
		       COUNT(e.id) FILTER (WHERE e.status = 'safe_manual') AS safe_manual
		FROM trakrf.muster_events me
		LEFT JOIN trakrf.muster_event_entries e
		       ON e.muster_event_id = me.id AND e.org_id = me.org_id
		WHERE me.org_id = $1
		GROUP BY me.id
		ORDER BY me.started_at DESC
	`
	var out []muster.Event
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, orgID)
		if err != nil {
			return fmt.Errorf("ListMusterEvents query: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var ev muster.Event
			if err := scanEventRow(rows, &ev); err != nil {
				return fmt.Errorf("ListMusterEvents scan: %w", err)
			}
			out = append(out, ev)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkEntryAtMuster transitions a muster_event_entry from status='missing' to
// 'at_muster', recording the muster location and first_muster_seen_at.
// Returns the updated entry, or nil (no error) if the entry was already
// non-missing (verified/safe_manual/at_muster are all sticky — we never
// downgrade them).
func (s *Storage) MarkEntryAtMuster(ctx context.Context, orgID int, eventID int, assetID int, musterLocationID int, seenAt time.Time) (*muster.Entry, error) {
	var entry *muster.Entry
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var e muster.Entry
		err := tx.QueryRow(ctx, `
			UPDATE trakrf.muster_event_entries
			   SET status               = 'at_muster',
			       muster_location_id   = $4,
			       first_muster_seen_at = $5,
			       updated_at           = now()
			WHERE muster_event_id = $2
			  AND asset_id        = $3
			  AND org_id          = $1
			  AND status          = 'missing'
			RETURNING id, org_id, muster_event_id, asset_id, label,
			          expected_location_id, status, muster_location_id,
			          first_muster_seen_at, verified_by, verified_at,
			          marked_safe_by, marked_safe_at, COALESCE(marked_safe_note, ''),
			          created_at, updated_at`,
			orgID, eventID, assetID, musterLocationID, seenAt,
		).Scan(
			&e.ID, &e.OrgID, &e.MusterEventID, &e.AssetID,
			&e.Label, &e.ExpectedLocationID, &e.Status,
			&e.MusterLocationID, &e.FirstMusterSeenAt,
			&e.VerifiedBy, &e.VerifiedAt,
			&e.MarkedSafeBy, &e.MarkedSafeAt, &e.MarkedSafeNote,
			&e.CreatedAt, &e.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Either not found or already non-missing — no-op
				entry = nil
				return nil
			}
			return fmt.Errorf("MarkEntryAtMuster update: %w", err)
		}
		entry = &e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// UpdateEntryStatus applies a "verify" or "mark_safe" action to a muster
// entry:
//   - "verify": only valid from status='at_muster'; transitions to 'verified';
//     stamps verified_by and verified_at.
//   - "mark_safe": valid from 'missing' or 'at_muster'; transitions to
//     'safe_manual'; stamps marked_safe_by, marked_safe_at, marked_safe_note.
//
// Returns ErrInvalidTransition when the current status does not permit the
// action. Handlers map this to 409 Conflict.
func (s *Storage) UpdateEntryStatus(ctx context.Context, orgID int, eventID, entryID int, action string, userID int, note string) (*muster.Entry, error) {
	// First, read current status
	var current string
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT status FROM trakrf.muster_event_entries
			WHERE id = $1 AND muster_event_id = $2 AND org_id = $3`,
			entryID, eventID, orgID,
		).Scan(&current)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("UpdateEntryStatus read current: %w", err)
	}

	// Validate transition
	switch action {
	case "verify":
		if current != "at_muster" {
			return nil, muster.ErrInvalidTransition{Current: current, Action: action}
		}
	case "mark_safe":
		if current != "missing" && current != "at_muster" {
			return nil, muster.ErrInvalidTransition{Current: current, Action: action}
		}
	default:
		return nil, fmt.Errorf("unknown action: %q", action)
	}

	// Apply the transition
	var entry muster.Entry
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var q string
		var args []any
		now := time.Now()

		switch action {
		case "verify":
			q = `
				UPDATE trakrf.muster_event_entries
				   SET status      = 'verified',
				       verified_by = $4,
				       verified_at = $5,
				       updated_at  = now()
				WHERE id             = $1
				  AND muster_event_id = $2
				  AND org_id         = $3
				  AND status         = 'at_muster'
				RETURNING id, org_id, muster_event_id, asset_id, label,
				          expected_location_id, status, muster_location_id,
				          first_muster_seen_at, verified_by, verified_at,
				          marked_safe_by, marked_safe_at, COALESCE(marked_safe_note, ''),
				          created_at, updated_at`
			args = []any{entryID, eventID, orgID, userID, now}
		case "mark_safe":
			q = `
				UPDATE trakrf.muster_event_entries
				   SET status           = 'safe_manual',
				       marked_safe_by   = $4,
				       marked_safe_at   = $5,
				       marked_safe_note = $6,
				       updated_at       = now()
				WHERE id             = $1
				  AND muster_event_id = $2
				  AND org_id         = $3
				  AND status         IN ('missing', 'at_muster')
				RETURNING id, org_id, muster_event_id, asset_id, label,
				          expected_location_id, status, muster_location_id,
				          first_muster_seen_at, verified_by, verified_at,
				          marked_safe_by, marked_safe_at, COALESCE(marked_safe_note, ''),
				          created_at, updated_at`
			args = []any{entryID, eventID, orgID, userID, now, note}
		}

		err := tx.QueryRow(ctx, q, args...).Scan(
			&entry.ID, &entry.OrgID, &entry.MusterEventID, &entry.AssetID,
			&entry.Label, &entry.ExpectedLocationID, &entry.Status,
			&entry.MusterLocationID, &entry.FirstMusterSeenAt,
			&entry.VerifiedBy, &entry.VerifiedAt,
			&entry.MarkedSafeBy, &entry.MarkedSafeAt, &entry.MarkedSafeNote,
			&entry.CreatedAt, &entry.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("entry %d not found or transition race", entryID)
			}
			return fmt.Errorf("UpdateEntryStatus update: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// CompleteMusterEvent sets the event status to 'completed' or 'cancelled',
// records ended_at and the report JSON. Returns the updated event.
func (s *Storage) CompleteMusterEvent(ctx context.Context, orgID int, eventID int, status string, report json.RawMessage) (*muster.Event, error) {
	var event muster.Event
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE trakrf.muster_events
			   SET status     = $3,
			       ended_at   = now(),
			       report     = $4,
			       updated_at = now()
			WHERE id     = $2
			  AND org_id = $1
			RETURNING id, org_id, status, started_at, ended_at, window_minutes,
			          started_by, report, metadata, created_at, updated_at`,
			orgID, eventID, status, report,
		).Scan(
			&event.ID, &event.OrgID, &event.Status,
			&event.StartedAt, &event.EndedAt, &event.WindowMinutes,
			&event.StartedBy, &event.Report, &event.Metadata,
			&event.CreatedAt, &event.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("muster event %d not found for org %d", eventID, orgID)
			}
			return fmt.Errorf("CompleteMusterEvent update: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// AppendMusterUnlock appends an unlock record to metadata.unlocks (JSONB array).
// If metadata.unlocks does not exist it is created.
func (s *Storage) AppendMusterUnlock(ctx context.Context, orgID int, eventID int, unlock map[string]any) error {
	unlockJSON, err := json.Marshal(unlock)
	if err != nil {
		return fmt.Errorf("AppendMusterUnlock marshal: %w", err)
	}
	return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE trakrf.muster_events
			   SET metadata   = jsonb_set(
			                      COALESCE(metadata, '{}'::jsonb),
			                      '{unlocks}',
			                      COALESCE(metadata->'unlocks', '[]'::jsonb) || $3::jsonb,
			                      true),
			       updated_at = now()
			WHERE id     = $2
			  AND org_id = $1`,
			orgID, eventID, unlockJSON,
		)
		return err
	})
}

// ── internal helpers ──────────────────────────────────────────────────────────

// getEventByFilter fetches a single muster event + its entries + counts,
// filtered by the given additional WHERE clause (e.g. "AND me.id = 42" or
// "AND me.status = 'active'"). Returns (nil, nil) when no row matches.
func (s *Storage) getEventByFilter(ctx context.Context, orgID int, filter string) (*muster.Event, error) {
	eventQuery := fmt.Sprintf(`
		SELECT me.id, me.org_id, me.status, me.started_at, me.ended_at,
		       me.window_minutes, me.started_by, me.report, me.metadata,
		       me.created_at, me.updated_at,
		       COUNT(e.id)                                          AS expected,
		       COUNT(e.id) FILTER (WHERE e.status = 'missing')     AS missing,
		       COUNT(e.id) FILTER (WHERE e.status = 'at_muster')   AS at_muster,
		       COUNT(e.id) FILTER (WHERE e.status = 'verified')    AS verified,
		       COUNT(e.id) FILTER (WHERE e.status = 'safe_manual') AS safe_manual
		FROM trakrf.muster_events me
		LEFT JOIN trakrf.muster_event_entries e
		       ON e.muster_event_id = me.id AND e.org_id = me.org_id
		WHERE me.org_id = $1
		%s
		GROUP BY me.id
		LIMIT 1`, filter)

	entriesQuery := `
		SELECT id, org_id, muster_event_id, asset_id, label,
		       expected_location_id, status, muster_location_id,
		       first_muster_seen_at, verified_by, verified_at,
		       marked_safe_by, marked_safe_at, COALESCE(marked_safe_note, ''),
		       created_at, updated_at
		FROM trakrf.muster_event_entries
		WHERE muster_event_id = $1 AND org_id = $2
		ORDER BY id`

	var event muster.Event
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if err := scanEventRow(tx.QueryRow(ctx, eventQuery, orgID), &event); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgx.ErrNoRows // signal not found
			}
			return fmt.Errorf("getEventByFilter event: %w", err)
		}

		rows, err := tx.Query(ctx, entriesQuery, event.ID, orgID)
		if err != nil {
			return fmt.Errorf("getEventByFilter entries: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e muster.Entry
			if err := scanEntryRow(rows, &e); err != nil {
				return fmt.Errorf("getEventByFilter scan entry: %w", err)
			}
			event.Entries = append(event.Entries, e)
		}
		return rows.Err()
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

// scanEventRow scans the columns returned by event queries (SELECT + aggregate
// counts). Works with both pgx.Row and pgx.Rows (both implement pgx.Row).
func scanEventRow(row pgx.Row, ev *muster.Event) error {
	return row.Scan(
		&ev.ID, &ev.OrgID, &ev.Status,
		&ev.StartedAt, &ev.EndedAt, &ev.WindowMinutes,
		&ev.StartedBy, &ev.Report, &ev.Metadata,
		&ev.CreatedAt, &ev.UpdatedAt,
		&ev.Counts.Expected, &ev.Counts.Missing, &ev.Counts.AtMuster,
		&ev.Counts.Verified, &ev.Counts.SafeManual,
	)
}

// scanEntryRow scans the standard entry columns.
func scanEntryRow(row pgx.Row, e *muster.Entry) error {
	return row.Scan(
		&e.ID, &e.OrgID, &e.MusterEventID, &e.AssetID,
		&e.Label, &e.ExpectedLocationID, &e.Status,
		&e.MusterLocationID, &e.FirstMusterSeenAt,
		&e.VerifiedBy, &e.VerifiedAt,
		&e.MarkedSafeBy, &e.MarkedSafeAt, &e.MarkedSafeNote,
		&e.CreatedAt, &e.UpdatedAt,
	)
}

// computeCounts tallies entry statuses. Used immediately after CreateMusterEvent
// before the event is returned (counts are also computed by the aggregate query
// in getEventByFilter, but not yet populated at create time).
func computeCounts(entries []muster.Entry) muster.Counts {
	c := muster.Counts{Expected: len(entries)}
	for _, e := range entries {
		switch e.Status {
		case "missing":
			c.Missing++
		case "at_muster":
			c.AtMuster++
		case "verified":
			c.Verified++
		case "safe_manual":
			c.SafeManual++
		}
	}
	return c
}
