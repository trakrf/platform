package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/locationmap"
)

// UpsertLocationMapImage stores (or replaces) the map image on a location.
// Replacing the image keeps its pins: positions are percentages, so a
// same-framing re-export keeps them valid. The caller has already checked the
// location exists and sniffed the content type.
func (s *Storage) UpsertLocationMapImage(ctx context.Context, orgID, locationID int, contentType string, img []byte) error {
	sum := sha256.Sum256(img)
	return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO trakrf.location_maps (location_id, org_id, content_type, image, sha256)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (location_id) DO UPDATE
			SET content_type = EXCLUDED.content_type,
			    image        = EXCLUDED.image,
			    sha256       = EXCLUDED.sha256`,
			locationID, orgID, contentType, img, hex.EncodeToString(sum[:]))
		if err != nil {
			return fmt.Errorf("upsert location map image: %w", err)
		}
		return nil
	})
}

// GetLocationMap returns a location's map metadata and its pins, or nil when
// the location has no map (or is not a live location of this org). Pins on a
// soft-deleted location are left out.
func (s *Storage) GetLocationMap(ctx context.Context, orgID, locationID int) (*locationmap.Map, error) {
	var m locationmap.Map
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT m.location_id, l.name, m.content_type, octet_length(m.image), m.sha256, m.updated_at
			FROM trakrf.location_maps m
			JOIN trakrf.locations l ON l.id = m.location_id AND l.org_id = m.org_id AND l.deleted_at IS NULL
			WHERE m.org_id = $1 AND m.location_id = $2`, orgID, locationID).
			Scan(&m.LocationID, &m.LocationName, &m.ContentType, &m.ByteSize, &m.SHA256, &m.UpdatedAt)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT p.location_id, p.x_pct, p.y_pct
			FROM trakrf.location_map_pins p
			JOIN trakrf.locations l ON l.id = p.location_id AND l.org_id = p.org_id AND l.deleted_at IS NULL
			WHERE p.org_id = $1 AND p.map_location_id = $2
			ORDER BY p.location_id`, orgID, locationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		m.Pins = []locationmap.Pin{}
		for rows.Next() {
			var p locationmap.Pin
			if err := rows.Scan(&p.LocationID, &p.XPct, &p.YPct); err != nil {
				return err
			}
			m.Pins = append(m.Pins, p)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get location map: %w", err)
	}
	return &m, nil
}

// GetLocationMapImage returns the image bytes, content type and hex SHA-256.
// img is nil when the location has no map.
func (s *Storage) GetLocationMapImage(ctx context.Context, orgID, locationID int) (contentType string, img []byte, sha string, err error) {
	err = s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT m.content_type, m.image, m.sha256
			FROM trakrf.location_maps m
			JOIN trakrf.locations l ON l.id = m.location_id AND l.org_id = m.org_id AND l.deleted_at IS NULL
			WHERE m.org_id = $1 AND m.location_id = $2`, orgID, locationID).
			Scan(&contentType, &img, &sha)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, "", nil
	}
	if err != nil {
		return "", nil, "", fmt.Errorf("get location map image: %w", err)
	}
	return contentType, img, sha, nil
}

// ReplaceLocationMapPins full-replaces a map's pins in one transaction. The
// caller has validated ranges and that every pinned location is a live
// descendant of the map location.
func (s *Storage) ReplaceLocationMapPins(ctx context.Context, orgID, mapLocationID int, pins []locationmap.Pin) error {
	return s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM trakrf.location_map_pins WHERE org_id = $1 AND map_location_id = $2`,
			orgID, mapLocationID); err != nil {
			return fmt.Errorf("clear location map pins: %w", err)
		}
		for _, p := range pins {
			if _, err := tx.Exec(ctx, `
				INSERT INTO trakrf.location_map_pins (map_location_id, location_id, org_id, x_pct, y_pct)
				VALUES ($1, $2, $3, $4, $5)`,
				mapLocationID, p.LocationID, orgID, p.XPct, p.YPct); err != nil {
				return fmt.Errorf("insert location map pin: %w", err)
			}
		}
		return nil
	})
}

// DeleteLocationMap removes a location's map and (by cascade) its pins.
// Reports whether a map existed.
func (s *Storage) DeleteLocationMap(ctx context.Context, orgID, locationID int) (bool, error) {
	var n int64
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM trakrf.location_maps WHERE org_id = $1 AND location_id = $2`, orgID, locationID)
		n = tag.RowsAffected()
		return err
	})
	if err != nil {
		return false, fmt.Errorf("delete location map: %w", err)
	}
	return n > 0, nil
}

// ListLocationMaps lists the org's maps on live locations, by location name.
func (s *Storage) ListLocationMaps(ctx context.Context, orgID int) ([]locationmap.Summary, error) {
	out := []locationmap.Summary{}
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.location_id, l.name, m.updated_at,
			       (SELECT count(*) FROM trakrf.location_map_pins p
			        JOIN trakrf.locations pl ON pl.id = p.location_id AND pl.deleted_at IS NULL
			        WHERE p.org_id = m.org_id AND p.map_location_id = m.location_id)
			FROM trakrf.location_maps m
			JOIN trakrf.locations l ON l.id = m.location_id AND l.org_id = m.org_id AND l.deleted_at IS NULL
			WHERE m.org_id = $1
			ORDER BY l.name, m.location_id`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sm locationmap.Summary
			if err := rows.Scan(&sm.LocationID, &sm.LocationName, &sm.UpdatedAt, &sm.PinCount); err != nil {
				return err
			}
			out = append(out, sm)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list location maps: %w", err)
	}
	return out, nil
}
