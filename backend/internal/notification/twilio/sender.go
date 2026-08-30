package twilio

import (
	"context"
	"errors"
	"net/http"

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
	if !config.Enabled() {
		return nil, errors.New("Twilio sender configuration is incomplete")
	}
	if !validPublicBaseURL(config.PublicBaseURL) {
		return nil, errors.New("Twilio sender public base URL must be a canonical HTTPS origin")
	}

	return newSender(config, nil), nil
}

func newSender(config Config, httpClient *http.Client) *Sender {
	client := &twilioclient.Client{
		Credentials: twilioclient.NewCredentials(config.APIKeySID, config.APIKeySecret),
		HTTPClient:  httpClient,
	}
	client.SetAccountSid(config.AccountSID)
	restClient := twiliogo.NewRestClientWithParams(twiliogo.ClientParams{Client: client})

	return newSenderWithMessages(config, &sdkMessageCreator{client: restClient})
}

func newSenderWithMessages(config Config, messages messageCreator) *Sender {
	return &Sender{
		messages:            messages,
		messagingServiceSID: config.MessagingServiceSID,
		statusCallbackURL:   config.PublicBaseURL + statusCallbackPath,
	}
}

// SendSMS submits a message using the configured Messaging Service and returns
// the provider's accepted-message identity and initial status.
func (sender *Sender) SendSMS(ctx context.Context, command sms.Command) (sms.Submission, error) {
	if cause := context.Cause(ctx); cause != nil {
		return sms.Submission{}, cause
	}

	params := &openapi.CreateMessageParams{}
	params.SetTo(command.ToE164)
	params.SetBody(command.Body)
	params.SetMessagingServiceSid(sender.messagingServiceSID)
	params.SetStatusCallback(sender.statusCallbackURL)

	message, err := sender.messages.CreateMessage(params)
	if err != nil {
		return sms.Submission{}, classifyError(err)
	}
	if message == nil {
		return sms.Submission{}, classifyError(errors.New("Twilio returned no message"))
	}

	return sms.Submission{
		ProviderMessageID: stringValue(message.Sid),
		Status:            stringValue(message.Status),
	}, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
