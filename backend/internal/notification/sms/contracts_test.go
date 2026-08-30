package sms_test

import (
	"context"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
)

type contractSender struct{}

func (contractSender) SendSMS(context.Context, sms.Command) (sms.Submission, error) {
	return sms.Submission{}, nil
}

type contractCallbackConsumer struct{}

func (contractCallbackConsumer) HandleStatus(context.Context, sms.ProviderStatus) error {
	return nil
}

func (contractCallbackConsumer) HandleKeyword(context.Context, sms.InboundKeyword) error {
	return nil
}

var _ sms.Sender = contractSender{}
var _ sms.CallbackConsumer = contractCallbackConsumer{}

func TestPublicSMSContracts(t *testing.T) {
	_ = sms.Command{
		DeliveryID: "delivery-123",
		ToE164:     "+15551234567",
		Body:       "Your tracker is ready.",
	}
	_ = sms.Submission{
		ProviderMessageID: "SM123",
		Status:            "queued",
	}
	_ = sms.ProviderError{
		Kind:       sms.ErrorTransient,
		Code:       "429",
		HTTPStatus: 429,
	}
	_ = sms.ProviderStatus{
		ProviderMessageID: "SM123",
		Status:            "delivered",
		ErrorCode:         "",
		OccurredAt:        time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
	}
	_ = sms.InboundKeyword{
		ProviderMessageID: "SM124",
		FromE164:          "+15551234567",
		ToE164:            "+15557654321",
		Keyword:           "STOP",
		ReceivedAt:        time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
	}

	const (
		wantTransient sms.ErrorKind = "transient"
		wantPermanent sms.ErrorKind = "permanent"
		wantRejected  sms.ErrorKind = "rejected"
	)
	if sms.ErrorTransient != wantTransient {
		t.Errorf("ErrorTransient = %q, want %q", sms.ErrorTransient, wantTransient)
	}
	if sms.ErrorPermanent != wantPermanent {
		t.Errorf("ErrorPermanent = %q, want %q", sms.ErrorPermanent, wantPermanent)
	}
	if sms.ErrorRejected != wantRejected {
		t.Errorf("ErrorRejected = %q, want %q", sms.ErrorRejected, wantRejected)
	}
}
