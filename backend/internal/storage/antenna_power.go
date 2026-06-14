package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/readerpower"
)

// Per-antenna power is stored on each scan_point's JSONB metadata under these
// keys (no migration — reuses the existing column). tx_power_dbm is the
// desired/confirmed power; power_status tracks the last command outcome.
const (
	mdPowerDBm           = "tx_power_dbm"
	mdPowerStatus        = "power_status" // pending | ok | busy | error
	mdPowerActiveProfile = "power_active_profile"
	mdPowerUpdatedAt     = "power_updated_at" // RFC3339
)

const powerStatusPending = "pending"

// AntennaPower is the per-antenna power view returned to the handler.
type AntennaPower struct {
	AntennaPort   int      `json:"antenna_port"`
	PowerDBm      *float64 `json:"power_dbm"`
	Status        string   `json:"status,omitempty"`
	ActiveProfile string   `json:"active_profile,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

// GetAntennaPower returns the per-antenna power view for a device's scan points,
// ordered by antenna port. Values come from each point's metadata (last-known /
// confirmed by the agent's state messages).
func (s *Storage) GetAntennaPower(ctx context.Context, orgID, deviceID int) ([]AntennaPower, error) {
	points, err := s.ListScanPointsByDevice(ctx, orgID, deviceID)
	if err != nil {
		return nil, err
	}
	out := make([]AntennaPower, 0, len(points))
	for _, p := range points {
		port := 1
		if p.AntennaPort != nil {
			port = *p.AntennaPort
		}
		md := asMap(p.Metadata)
		ap := AntennaPower{AntennaPort: port}
		if v, ok := md[mdPowerDBm].(float64); ok {
			ap.PowerDBm = &v
		}
		if v, ok := md[mdPowerStatus].(string); ok {
			ap.Status = v
		}
		if v, ok := md[mdPowerActiveProfile].(string); ok {
			ap.ActiveProfile = v
		}
		if v, ok := md[mdPowerUpdatedAt].(string); ok {
			ap.UpdatedAt = v
		}
		out = append(out, ap)
	}
	return out, nil
}

// SetAntennaPowerDesired optimistically records requested powers (status
// "pending") on the matching scan points before the agent confirms. powers maps
// antenna port -> dBm.
func (s *Storage) SetAntennaPowerDesired(ctx context.Context, orgID, deviceID int, powers map[int]float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		for port, dbm := range powers {
			updates := map[string]any{
				mdPowerDBm:       dbm,
				mdPowerStatus:    powerStatusPending,
				mdPowerUpdatedAt: now,
			}
			if err := mergePointMetadata(ctx, tx, orgID, deviceID, port, updates); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetAntennaPowerState records the confirmed result the agent published, keyed by
// the reader's publish_topic. status is one of readerpower.Status*. On ok it
// writes the confirmed powers; on busy/error it records the status only.
func (s *Storage) SetAntennaPowerState(ctx context.Context, route ScanRoute, st readerpower.State) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.WithOrgTx(ctx, route.OrgID, func(tx pgx.Tx) error {
		if st.Status != readerpower.StatusOK || len(st.Powers) == 0 {
			// No confirmed powers to write; stamp status on antenna 1 so the UI
			// can surface busy/error without guessing a port.
			return mergePointMetadata(ctx, tx, route.OrgID, route.ScanDeviceID, 1, map[string]any{
				mdPowerStatus:        st.Status,
				mdPowerActiveProfile: st.ActiveProfile,
				mdPowerUpdatedAt:     now,
			})
		}
		for portStr, dbm := range st.Powers {
			port := atoiSafe(portStr)
			if port == 0 {
				continue
			}
			updates := map[string]any{
				mdPowerDBm:           dbm,
				mdPowerStatus:        st.Status,
				mdPowerActiveProfile: st.ActiveProfile,
				mdPowerUpdatedAt:     now,
			}
			if err := mergePointMetadata(ctx, tx, route.OrgID, route.ScanDeviceID, port, updates); err != nil {
				return err
			}
		}
		return nil
	})
}

// mergePointMetadata read-modify-writes the JSONB metadata of the live scan
// point for (device, antennaPort), merging in updates. A missing point for the
// port is silently skipped (a CS463 may report 16 ports; only provisioned ones
// have rows).
func mergePointMetadata(ctx context.Context, tx pgx.Tx, orgID, deviceID, antennaPort int, updates map[string]any) error {
	var id int
	var rawMeta any
	err := tx.QueryRow(ctx, `
		SELECT id, metadata FROM trakrf.scan_points
		WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = $3 AND deleted_at IS NULL
		LIMIT 1`, orgID, deviceID, antennaPort).Scan(&id, &rawMeta)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	md := asMap(rawMeta)
	for k, v := range updates {
		md[k] = v
	}
	_, err = tx.Exec(ctx, `
		UPDATE trakrf.scan_points SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND org_id = $3`, md, id, orgID)
	return err
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
