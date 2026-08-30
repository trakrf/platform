package sms

import (
	"context"
	"testing"
	"time"
)

type contractSender struct {
	command Command
}

func (s *contractSender) SendSMS(_ context.Context, command Command) (Submission, error) {
	s.command = command
	return Submission{
		ProviderMessageID: "SM123",
		Status:            "queued",
	}, nil
}

type contractCallbackConsumer struct {
	status  ProviderStatus
	keyword InboundKeyword
}

func (c *contractCallbackConsumer) HandleStatus(_ context.Context, status ProviderStatus) error {
	c.status = status
	return nil
}

func (c *contractCallbackConsumer) HandleKeyword(_ context.Context, keyword InboundKeyword) error {
	c.keyword = keyword
	return nil
}

var _ Sender = (*contractSender)(nil)
var _ CallbackConsumer = (*contractCallbackConsumer)(nil)

func TestProviderNeutralSMSContracts(t *testing.T) {
	command := Command{
		DeliveryID: "delivery-123",
		ToE164:     "+15551234567",
		Body:       "Your tracker is ready.",
	}
	sender := &contractSender{}

	submission, err := sender.SendSMS(context.Background(), command)
	if err != nil {
		t.Fatalf("SendSMS() error = %v", err)
	}
	if sender.command != command {
		t.Errorf("SendSMS() command = %#v, want %#v", sender.command, command)
	}
	if submission != (Submission{ProviderMessageID: "SM123", Status: "queued"}) {
		t.Errorf("SendSMS() submission = %#v", submission)
	}

	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	consumer := &contractCallbackConsumer{}
	status := ProviderStatus{
		ProviderMessageID: "SM123",
		Status:            "delivered",
		ErrorCode:         "",
		OccurredAt:        now,
	}
	keyword := InboundKeyword{
		ProviderMessageID: "SM124",
		FromE164:          "+15551234567",
		ToE164:            "+15557654321",
		Keyword:           "STOP",
		ReceivedAt:        now,
	}

	if err := consumer.HandleStatus(context.Background(), status); err != nil {
		t.Fatalf("HandleStatus() error = %v", err)
	}
	if err := consumer.HandleKeyword(context.Background(), keyword); err != nil {
		t.Fatalf("HandleKeyword() error = %v", err)
	}
	if consumer.status != status {
		t.Errorf("HandleStatus() status = %#v, want %#v", consumer.status, status)
	}
	if consumer.keyword != keyword {
		t.Errorf("HandleKeyword() keyword = %#v, want %#v", consumer.keyword, keyword)
	}

	providerError := ProviderError{Kind: ErrorTransient, Code: "429", HTTPStatus: 429}
	if providerError.Kind != ErrorTransient {
		t.Errorf("ProviderError.Kind = %q, want %q", providerError.Kind, ErrorTransient)
	}
	if ErrorPermanent != "permanent" || ErrorRejected != "rejected" {
		t.Errorf("provider error kinds = %q, %q", ErrorPermanent, ErrorRejected)
	}
}
