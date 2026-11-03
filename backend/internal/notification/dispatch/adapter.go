package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// SuppressionChecker reports whether an address has opted out of a channel.
type SuppressionChecker interface {
	IsSuppressed(ctx context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error)
}

// Deps are the adapter's collaborators. A nil sender disables that channel.
type Deps struct {
	Email        email.Sender       // nil = email disabled
	SMS          sms.Sender         // nil = SMS disabled
	Suppressions SuppressionChecker // required
	Log          zerolog.Logger
}

// Adapter sends outbox deliveries through the email and SMS senders.
type Adapter struct {
	email email.Sender
	sms   sms.Sender
	supp  SuppressionChecker
	log   zerolog.Logger
}

var _ outbox.ChannelAdapter = (*Adapter)(nil)

// New returns an Adapter; it errors only when Suppressions is nil.
func New(d Deps) (*Adapter, error) {
	if d.Suppressions == nil {
		return nil, errors.New("dispatch: suppression checker is required")
	}
	return &Adapter{email: d.Email, sms: d.SMS, supp: d.Suppressions, log: d.Log}, nil
}

// Send decodes the stored message, re-checks opt-outs and calls the sender.
// The payload is PII: it never reaches a log line or an error string.
func (a *Adapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	var msg routing.DeliveryMessage
	if len(cmd.Payload) == 0 || json.Unmarshal(cmd.Payload, &msg) != nil {
		return outbox.Result{}, a.fail(cmd, permanent(reasonInvalidPayload))
	}
	if msg.Channel != cmd.Channel ||
		(cmd.Channel != notificationdelivery.ChannelEmail && cmd.Channel != notificationdelivery.ChannelSMS) {
		return outbox.Result{}, a.fail(cmd, permanent(reasonChannelMismatch))
	}
	if (cmd.Channel == notificationdelivery.ChannelEmail && a.email == nil) ||
		(cmd.Channel == notificationdelivery.ChannelSMS && a.sms == nil) {
		return outbox.Result{}, a.fail(cmd, permanent(reasonDisabled))
	}

	suppressed, err := a.supp.IsSuppressed(ctx, cmd.OrgID, cmd.Channel, msg.To)
	if err != nil {
		return outbox.Result{}, fmt.Errorf("notification suppression check failed: %w", err)
	}
	if suppressed {
		return outbox.Result{}, a.fail(cmd, permanent(reasonOptedOut))
	}

	if cmd.Channel == notificationdelivery.ChannelEmail {
		sub, err := a.email.SendEmail(ctx, email.Command{
			DeliveryID: cmd.DeliveryID,
			To:         msg.To,
			Subject:    cleanSubject(msg.Subject),
			Text:       msg.Body,
		})
		if err != nil {
			return outbox.Result{}, a.fail(cmd, mapEmailError(err))
		}
		return outbox.Result{ProviderMessageID: sub.ProviderMessageID}, nil
	}
	sub, err := a.sms.SendSMS(ctx, sms.Command{DeliveryID: cmd.DeliveryID, ToE164: msg.To, Body: msg.Body})
	if err != nil {
		return outbox.Result{}, a.fail(cmd, mapSMSError(err))
	}
	return outbox.Result{ProviderMessageID: sub.ProviderMessageID}, nil
}

// fail logs permanent failures and returns e as an error. e is never nil here,
// so no typed-nil error escapes.
func (a *Adapter) fail(cmd outbox.Command, e *Error) error {
	if e.Kind == outbox.ErrorPermanent {
		a.log.Error().
			Str("delivery_id", cmd.DeliveryID).
			Int("org_id", cmd.OrgID).
			Str("channel", string(cmd.Channel)).
			Str("error_kind", e.Reason).
			Msg("notification delivery not sent")
	}
	return e
}

func cleanSubject(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
