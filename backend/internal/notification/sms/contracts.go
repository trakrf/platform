// Package sms defines provider-neutral contracts for sending SMS messages and
// receiving provider callback events.
package sms

import (
	"context"
	"fmt"
	"time"
)

// Command is a request to submit an SMS message to a recipient.
type Command struct {
	DeliveryID string
	ToE164     string
	Body       string
}

// Submission identifies an SMS message accepted by a provider.
type Submission struct {
	ProviderMessageID string
	Status            string
}

// ErrorKind classifies provider failures into stable handling categories.
type ErrorKind string

const (
	ErrorTransient ErrorKind = "transient"
	ErrorPermanent ErrorKind = "permanent"
	ErrorRejected  ErrorKind = "rejected"
)

// ProviderError contains normalized provider failure details.
type ProviderError struct {
	Kind       ErrorKind
	Code       string
	HTTPStatus int
}

// Error returns a bounded, provider-neutral description. It intentionally
// excludes provider response text and request data.
func (err *ProviderError) Error() string {
	if err == nil || err.Kind == "" {
		return "SMS provider failure"
	}
	if err.Code == "" {
		if err.HTTPStatus == 0 {
			return fmt.Sprintf("SMS provider %s failure", err.Kind)
		}
		return fmt.Sprintf("SMS provider %s failure (HTTP %d)", err.Kind, err.HTTPStatus)
	}
	if err.HTTPStatus == 0 {
		return fmt.Sprintf("SMS provider %s failure (code %s)", err.Kind, err.Code)
	}
	return fmt.Sprintf("SMS provider %s failure (code %s, HTTP %d)", err.Kind, err.Code, err.HTTPStatus)
}

// Sender submits SMS commands through a provider.
type Sender interface {
	SendSMS(context.Context, Command) (Submission, error)
}

// ProviderStatus is a normalized delivery-status callback from a provider.
type ProviderStatus struct {
	ProviderMessageID string
	Status            string
	ErrorCode         string
	OccurredAt        time.Time
}

// InboundKeyword is a normalized inbound consent keyword from a provider.
type InboundKeyword struct {
	ProviderMessageID string
	FromE164          string
	ToE164            string
	Keyword           string
	ReceivedAt        time.Time
}

// CallbackConsumer receives normalized provider callback events.
type CallbackConsumer interface {
	HandleStatus(context.Context, ProviderStatus) error
	HandleKeyword(context.Context, InboundKeyword) error
}
