// Package subscriptionnotice sends the subscription expiry nag emails to org
// admins: 14 days and 3 days before expiry, at expiry (grace begins), and at
// cutoff (grace ends). Which stage an org is in, and whether it was already
// sent, is decided in SQL; this package only fans each due notice out to the
// org's admins.
package subscriptionnotice

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// Store is the storage dependency (satisfied by *storage.Storage).
type Store interface {
	ListDueSubscriptionNotices(ctx context.Context) ([]storage.SubscriptionNotice, error)
	ClaimSubscriptionNotice(ctx context.Context, orgID int, kind storage.SubscriptionNoticeKind, expiresAt time.Time) (bool, error)
	ListOrgAdminEmails(ctx context.Context, orgID int) ([]string, error)
}

// Sender delivers one notice email (satisfied by *email.Client).
type Sender interface {
	SendSubscriptionNotice(toEmail, kind, orgName string, expiresAt, cutoffAt time.Time) error
}

// Notifier runs the nag sequence.
type Notifier struct {
	store  Store
	sender Sender
	log    zerolog.Logger
}

// New builds a Notifier.
func New(store Store, sender Sender, log zerolog.Logger) *Notifier {
	return &Notifier{
		store:  store,
		sender: sender,
		log:    log.With().Str("component", "subscriptionnotice").Logger(),
	}
}

// RunOnce sends every due notice and returns how many emails went out. Each
// notice is claimed before sending, so concurrent runs (or replicas) never
// double-send; a failed send is logged and not retried.
func (n *Notifier) RunOnce(ctx context.Context) (int, error) {
	due, err := n.store.ListDueSubscriptionNotices(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, d := range due {
		claimed, err := n.store.ClaimSubscriptionNotice(ctx, d.OrgID, d.Kind, d.ExpiresAt)
		if err != nil {
			n.log.Warn().Err(err).Int("org_id", d.OrgID).Str("kind", string(d.Kind)).Msg("failed to claim subscription notice")
			continue
		}
		if !claimed {
			continue
		}
		admins, err := n.store.ListOrgAdminEmails(ctx, d.OrgID)
		if err != nil {
			n.log.Warn().Err(err).Int("org_id", d.OrgID).Msg("failed to list org admins for subscription notice")
			continue
		}
		if len(admins) == 0 {
			n.log.Warn().Int("org_id", d.OrgID).Str("kind", string(d.Kind)).Msg("subscription notice has no admin recipients")
			continue
		}
		orgSent := 0
		for _, to := range admins {
			if err := n.sender.SendSubscriptionNotice(to, string(d.Kind), d.OrgName, d.ExpiresAt, d.CutoffAt); err != nil {
				n.log.Warn().Err(err).Int("org_id", d.OrgID).Str("kind", string(d.Kind)).Msg("failed to send subscription notice")
				continue
			}
			orgSent++
		}
		sent += orgSent
		n.log.Info().Int("org_id", d.OrgID).Str("kind", string(d.Kind)).
			Int("sent", orgSent).Int("recipients", len(admins)).Msg("subscription notice sent")
	}
	return sent, nil
}

// Run calls RunOnce immediately and then every interval until ctx is done.
func (n *Notifier) Run(ctx context.Context, interval time.Duration) {
	tick := func() {
		if _, err := n.RunOnce(ctx); err != nil {
			n.log.Warn().Err(err).Msg("subscription notice run failed")
		}
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}
