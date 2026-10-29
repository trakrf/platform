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

// notificationAlertsFrom joins a delivery to its event, asset, recipient and
// the location names the event refers to. Soft-deleted assets, recipients and
// locations still resolve, so history keeps its names.
const notificationAlertsFrom = `
	FROM trakrf.notification_deliveries d
	JOIN trakrf.notification_events e ON e.org_id = d.org_id AND e.id = d.event_id
	JOIN trakrf.assets a ON a.id = d.asset_id
	JOIN trakrf.notification_recipients r ON r.id = d.recipient_id
	LEFT JOIN trakrf.alarm_events ae ON ae.id = e.alarm_event_id
	LEFT JOIN trakrf.locations al ON al.id = ae.location_id
	LEFT JOIN trakrf.location_changes lc ON lc.id = e.location_change_id
	LEFT JOIN trakrf.locations lt ON lt.id = lc.to_location_id
	LEFT JOIN trakrf.locations lf ON lf.id = lc.from_location_id`

// ListNotificationAlerts returns the org's sent notifications, newest first,
// and the total matching f before paging. Only delivered rows that carry their
// routing context count as sent: pending, in-flight and failed deliveries, and
// rows written before routing was recorded, are left out.
func (s *Storage) ListNotificationAlerts(ctx context.Context, orgID int, f notificationalert.ListFilter) ([]notificationalert.Alert, int, error) {
	where, args := notificationAlertsWhere(orgID, f)

	countQuery := `SELECT count(*)` + notificationAlertsFrom + where
	listQuery := `SELECT d.id, d.asset_id, a.name, a.external_key, e.source, e.occurred_at,
			d.finalized_at, d.recipient_id, r.name, d.channel,
			d.payload->>'subject', COALESCE(d.payload->>'body', ''),
			al.name, lt.name, lf.name` +
		notificationAlertsFrom + where +
		fmt.Sprintf(` ORDER BY d.finalized_at DESC, d.id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

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

func notificationAlertsWhere(orgID int, f notificationalert.ListFilter) (string, []any) {
	conds := []string{
		"d.org_id = $1",
		"d.state = 'delivered'",
		"d.finalized_at IS NOT NULL",
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
	return " WHERE " + strings.Join(conds, " AND "), args
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
