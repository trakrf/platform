package twilio

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
	twiliogo "github.com/twilio/twilio-go"
	twilioclient "github.com/twilio/twilio-go/client"
	"github.com/twilio/twilio-go/rest/api/v2010"
)

const statusCallbackPath = "/api/v1/notifications/twilio/status"

// Sender submits SMS messages through the configured Twilio Messaging Service.
type Sender struct {
	messages            messageCreator
	messagingServiceSID string
	statusCallbackURL   string
	metrics             *Metrics
}

type messageCreator interface {
	CreateMessage(*openapi.CreateMessageParams) (*openapi.ApiV2010Message, error)
}

type sdkMessageCreator struct {
	client *twiliogo.RestClient
}

func (creator *sdkMessageCreator) CreateMessage(params *openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
	return creator.client.Api.CreateMessage(params)
}

// NewSender constructs a sender using API-key credentials and Account SID context.
func NewSender(config Config) (*Sender, error) {
	return NewSenderWithMetrics(config, nil)
}

// NewSenderWithMetrics constructs a sender with an optional metrics recorder.
// A nil recorder leaves sender behavior uninstrumented.
func NewSenderWithMetrics(config Config, metrics *Metrics) (*Sender, error) {
	if !config.Enabled() {
		return nil, errors.New("Twilio sender configuration is incomplete")
	}
	if !validPublicBaseURL(config.PublicBaseURL) {
		return nil, errors.New("Twilio sender public base URL must be a canonical HTTPS origin")
	}

	return newSender(config, nil, metrics), nil
}

func newSender(config Config, httpClient *http.Client, metrics *Metrics) *Sender {
	client := &twilioclient.Client{
		Credentials: twilioclient.NewCredentials(config.APIKeySID, config.APIKeySecret),
		HTTPClient:  httpClient,
	}
	client.SetAccountSid(config.AccountSID)
	restClient := twiliogo.NewRestClientWithParams(twiliogo.ClientParams{Client: client})

	return newSenderWithMessages(config, &sdkMessageCreator{client: restClient}, metrics)
}

func newSenderWithMessages(config Config, messages messageCreator, metrics *Metrics) *Sender {
	return &Sender{
		messages:            messages,
		messagingServiceSID: config.MessagingServiceSID,
		statusCallbackURL:   config.PublicBaseURL + statusCallbackPath,
		metrics:             metrics,
	}
}

// SendSMS submits a message using the configured Messaging Service and returns
// the provider's accepted-message identity and initial status.
func (sender *Sender) SendSMS(ctx context.Context, command sms.Command) (sms.Submission, error) {
	if cause := context.Cause(ctx); cause != nil {
		sender.recordSubmission(metricSubmissionResultUnknown)
		return sms.Submission{}, cause
	}

	params := &openapi.CreateMessageParams{}
	params.SetTo(command.ToE164)
	params.SetBody(command.Body)
	params.SetMessagingServiceSid(sender.messagingServiceSID)
	params.SetStatusCallback(sender.statusCallbackURL)

	requestStarted := time.Now()
	message, err := sender.messages.CreateMessage(params)
	sender.observeRequestDuration(time.Since(requestStarted))
	if err != nil {
		providerErr := classifyError(err)
		sender.recordSubmission(submissionResult(providerErr))
		return sms.Submission{}, providerErr
	}
	if message == nil {
		providerErr := classifyError(errors.New("Twilio returned no message"))
		sender.recordSubmission(submissionResult(providerErr))
		return sms.Submission{}, providerErr
	}

	sender.recordSubmission(SubmissionAccepted)
	return sms.Submission{
		ProviderMessageID: stringValue(message.Sid),
		Status:            stringValue(message.Status),
	}, nil
}

func (sender *Sender) recordSubmission(result SubmissionResult) {
	if sender.metrics != nil {
		sender.metrics.RecordSubmission(result)
	}
}

func (sender *Sender) observeRequestDuration(duration time.Duration) {
	if sender.metrics != nil {
		sender.metrics.ObserveRequestDuration(duration)
	}
}

func submissionResult(err error) SubmissionResult {
	var providerErr *providerError
	if !errors.As(err, &providerErr) {
		return metricSubmissionResultUnknown
	}
	if providerErr.Code == unknownCode {
		return metricSubmissionResultUnknown
	}

	switch providerErr.Kind {
	case sms.ErrorTransient:
		return SubmissionTransientError
	case sms.ErrorPermanent:
		return SubmissionPermanentError
	case sms.ErrorRejected:
		return SubmissionRejected
	default:
		return metricSubmissionResultUnknown
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
