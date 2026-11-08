package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/models/notificationalert"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// ListNotificationAlerts returns the org's sent notifications, newest first,
// and the total matching f before paging. Only delivered rows that carry their
// routing context count as sent: pending, in-flight and failed deliveries, and
// rows written before routing was recorded, are left out.
//
// The count and the page both run on the deliveries alone (plus the event for
// a type filter); names, places and the message are read only for the page's
// rows. Paging is by offset, so rows sent between two page reads shift the
// later page.
func (s *Storage) ListNotificationAlerts(ctx context.Context, orgID int, f notificationalert.ListFilter) ([]notificationalert.Alert, int, error) {
	from, where, args := notificationAlertsFilter(orgID, f)

	countQuery := `SELECT count(*)` + from + where
	pageQuery := `SELECT d.id` + from + where +
		fmt.Sprintf(` ORDER BY d.finalized_at DESC, d.id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

	// Asset, recipient and location names are read as they are now, while
	// subject and body are the message as sent: after a rename the two can
	// disagree. Soft-deleted rows still resolve.
	listQuery := `WITH page AS (` + pageQuery + `)
		SELECT d.public_id, d.asset_id, a.name, a.external_key, e.source, e.occurred_at,
			d.finalized_at, d.recipient_id, r.name, d.channel,
			d.payload->>'subject', COALESCE(d.payload->>'body', ''),
			al.name, lt.name, lf.name
		FROM page
		JOIN trakrf.notification_deliveries d ON d.id = page.id
		JOIN trakrf.notification_events e ON e.org_id = d.org_id AND e.id = d.event_id
		JOIN trakrf.assets a ON a.id = d.asset_id
		JOIN trakrf.notification_recipients r ON r.id = d.recipient_id
		LEFT JOIN trakrf.alarm_events ae ON ae.id = e.alarm_event_id
		LEFT JOIN trakrf.locations al ON al.id = ae.location_id
		LEFT JOIN trakrf.location_changes lc ON lc.id = e.location_change_id
		LEFT JOIN trakrf.locations lt ON lt.id = lc.to_location_id
		LEFT JOIN trakrf.locations lf ON lf.id = lc.from_location_id
		ORDER BY d.finalized_at DESC, d.id DESC`

	out := []notificationalert.Alert{}
	var total int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, listQuery, append(args, f.Limit, f.Offset)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				a                                    notificationalert.Alert
				source, channel                      string
				alarmLocation, toLocation, fromPlace *string
			)
			if err := rows.Scan(&a.ID, &a.AssetID, &a.AssetName, &a.AssetExternalKey, &source, &a.OccurredAt,
				&a.SentAt, &a.RecipientID, &a.RecipientName, &channel,
				&a.Subject, &a.Body, &alarmLocation, &toLocation, &fromPlace); err != nil {
				return err
			}
			a.Type = notificationevent.Source(source)
			a.Channel = notificationdelivery.Channel(channel)
			a.Details = alertDetails(a.Type, alarmLocation, toLocation, fromPlace)
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list notification alerts: %w", err)
	}
	return out, total, nil
}

// notificationAlertsFilter returns the FROM and WHERE clauses that select the
// matching delivered rows, and their arguments. The routing columns are
// foreign keys, so a non-NULL one always resolves to its event, asset and
// recipient; only a type filter needs the event joined.
func notificationAlertsFilter(orgID int, f notificationalert.ListFilter) (string, string, []any) {
	from := ` FROM trakrf.notification_deliveries d`
	conds := []string{
		"d.org_id = $1",
		"d.state = 'delivered'",
		"d.finalized_at IS NOT NULL",
		"d.event_id IS NOT NULL",
		"d.asset_id IS NOT NULL",
		"d.recipient_id IS NOT NULL",
		"d.payload IS NOT NULL",
	}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.AssetID != nil {
		add("d.asset_id = $%d", *f.AssetID)
	}
	if f.RecipientID != nil {
		add("d.recipient_id = $%d", *f.RecipientID)
	}
	if f.Type != nil {
		from += ` JOIN trakrf.notification_events e ON e.org_id = d.org_id AND e.id = d.event_id`
		add("e.source = $%d", string(*f.Type))
	}
	if f.Channel != nil {
		add("d.channel = $%d", string(*f.Channel))
	}
	if f.From != nil {
		add("d.finalized_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("d.finalized_at < $%d", *f.To)
	}
	return from, " WHERE " + strings.Join(conds, " AND "), args
}

// alertDetails names the places an event refers to. A name that no longer
// resolves is left out rather than sent empty.
func alertDetails(source notificationevent.Source, alarmLocation, toLocation, fromLocation *string) map[string]string {
	d := map[string]string{}
	set := func(key string, v *string) {
		if v != nil && *v != "" {
			d[key] = *v
		}
	}
	switch source {
	case notificationevent.SourceAlarm:
		set("location", alarmLocation)
	case notificationevent.SourceLocationChange:
		set("from", fromLocation)
		set("to", toLocation)
	}
	return d
}
