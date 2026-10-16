package storage

import (
	"context"
	"fmt"
	"time"
)

// SubscriptionNoticeKind is one stage of the TRA-1047 expiry nag sequence.
// Values match the trakrf.subscription_notices.kind CHECK constraint.
type SubscriptionNoticeKind string

const (
	NoticeTMinus14 SubscriptionNoticeKind = "t_minus_14"
	NoticeTMinus3  SubscriptionNoticeKind = "t_minus_3"
	NoticeExpired  SubscriptionNoticeKind = "expired"
	NoticeCutoff   SubscriptionNoticeKind = "cutoff"
)

// SubscriptionNotice is a nag an org is due for and has not been sent yet.
type SubscriptionNotice struct {
	OrgID         int
	OrgName       string
	OrgIdentifier string
	Kind          SubscriptionNoticeKind
	ExpiresAt     time.Time
	// CutoffAt is ExpiresAt plus the deployment grace window — when capture,
	// paid writes and webhook delivery actually stop.
	CutoffAt time.Time
}

// ListDueSubscriptionNotices returns, for every org with a manual expiry, the one
// nag stage it is currently in, unless that (org, stage, expiry) was already
// sent. Stages are disjoint windows, so an org that crosses two between runs
// gets only the later one. Orgs whose cutoff passed more than 7 days ago are
// ignored (no mail blast for long-dead trials on first deploy), as are orgs kept
// entitled by an active subscription row.
func (s *Storage) ListDueSubscriptionNotices(ctx context.Context) ([]SubscriptionNotice, error) {
	query := `
		WITH g AS (SELECT trakrf.subscription_grace_period() AS grace),
		staged AS (
			SELECT o.id, o.name, o.identifier, o.subscription_expires_at AS expires_at,
			       o.subscription_expires_at + g.grace AS cutoff_at,
			       CASE
			           WHEN now() >= o.subscription_expires_at + g.grace THEN 'cutoff'
			           WHEN now() >= o.subscription_expires_at THEN 'expired'
			           WHEN now() >= o.subscription_expires_at - INTERVAL '3 days' THEN 't_minus_3'
			           ELSE 't_minus_14'
			       END AS kind
			FROM trakrf.organizations o
			CROSS JOIN g
			WHERE o.deleted_at IS NULL
			  AND o.subscription_enabled
			  AND o.subscription_expires_at IS NOT NULL
			  AND now() >= o.subscription_expires_at - INTERVAL '14 days'
			  AND now() <  o.subscription_expires_at + g.grace + INTERVAL '7 days'
			  AND NOT trakrf.org_has_active_subscription(o.id)
		)
		SELECT st.id, st.name, st.identifier, st.kind, st.expires_at, st.cutoff_at
		FROM staged st
		WHERE NOT EXISTS (
			SELECT 1 FROM trakrf.subscription_notices n
			WHERE n.org_id = st.id AND n.kind = st.kind AND n.expires_at = st.expires_at
		)
		ORDER BY st.id
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list due subscription notices: %w", err)
	}
	defer rows.Close()

	var out []SubscriptionNotice
	for rows.Next() {
		var n SubscriptionNotice
		var kind string
		if err := rows.Scan(&n.OrgID, &n.OrgName, &n.OrgIdentifier, &kind, &n.ExpiresAt, &n.CutoffAt); err != nil {
			return nil, fmt.Errorf("failed to scan subscription notice: %w", err)
		}
		n.Kind = SubscriptionNoticeKind(kind)
		out = append(out, n)
	}
	return out, rows.Err()
}

// ClaimSubscriptionNotice records that a notice is being sent. It returns false
// when another run (or replica) already claimed it. Claim-then-send makes
// delivery at-most-once: a failed send is logged, not retried.
func (s *Storage) ClaimSubscriptionNotice(ctx context.Context, orgID int, kind SubscriptionNoticeKind, expiresAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO trakrf.subscription_notices (org_id, kind, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, orgID, string(kind), expiresAt)
	if err != nil {
		return false, fmt.Errorf("failed to claim subscription notice: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ListOrgAdminEmails returns the emails of an org's active admins, sorted.
func (s *Storage) ListOrgAdminEmails(ctx context.Context, orgID int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.email
		FROM trakrf.org_users ou
		JOIN trakrf.users u ON u.id = ou.user_id
		WHERE ou.org_id = $1
		  AND ou.role = 'admin'
		  AND ou.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		ORDER BY u.email`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list org admin emails: %w", err)
	}
	defer rows.Close()

	emails := []string{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("failed to scan org admin email: %w", err)
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}
