package twilio

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	twilioclient "github.com/twilio/twilio-go/client"
	"github.com/twilio/twilio-go/rest/api/v2010"
)

var _ sms.Sender = (*Sender)(nil)

const (
	testAccountSID          = "AC1234567890"
	testAPIKeySID           = "SK1234567890"
	testAPIKeySecret        = "api-key-secret-value"
	testAuthToken           = "auth-token-secret-value"
	testMessagingServiceSID = "MG1234567890"
	testPublicBaseURL       = "https://api.example.com"
)

// This fails if a disabled or malformed configuration can create an outbound sender.
func TestNewSender_RejectsDisabledAndInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "disabled"},
		{
			name: "invalid callback origin",
			config: Config{
				AccountSID:          testAccountSID,
				APIKeySID:           testAPIKeySID,
				APIKeySecret:        testAPIKeySecret,
				AuthToken:           testAuthToken,
				MessagingServiceSID: testMessagingServiceSID,
				PublicBaseURL:       "https://api.example.com/",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, err := NewSender(test.config)

			require.Error(t, err)
			require.Nil(t, sender)
			require.NotContains(t, err.Error(), testAPIKeySecret)
			require.NotContains(t, err.Error(), testAuthToken)
		})
	}
}

// This fails if outbound construction authenticates with the Auth Token or omits Account SID request context.
func TestNewSender_UsesAPIKeyCredentialsAndAccountContext(t *testing.T) {
	sender, err := NewSender(completeSenderConfig())

	require.NoError(t, err)
	sdkMessages, ok := sender.messages.(*sdkMessageCreator)
	require.True(t, ok)
	client, ok := sdkMessages.client.Api.RequestHandler().Client.(*twilioclient.Client)
	require.True(t, ok)
	require.Equal(t, testAPIKeySID, client.Username)
	require.Equal(t, testAPIKeySecret, client.Password)
	require.Equal(t, testAccountSID, client.AccountSid())
}

// This fails if a submission omits a required Twilio request field, selects a raw sender, or returns the wrong accepted-message identity.
func TestSendSMS_SubmitsThroughMessagingService(t *testing.T) {
	creator := &fakeMessageCreator{response: messageResponse("SM123", "queued")}
	sender := newSender(completeSenderConfig(), creator)
	command := sms.Command{
		DeliveryID: "delivery-123",
		ToE164:     "+15555550123",
		Body:       "Your tracker is ready.",
	}

	submission, err := sender.SendSMS(context.Background(), command)

	require.NoError(t, err)
	require.Equal(t, sms.Submission{ProviderMessageID: "SM123", Status: "queued"}, submission)
	request := creator.last()
	require.NotNil(t, request)
	require.Equal(t, command.ToE164, value(request.To))
	require.Equal(t, command.Body, value(request.Body))
	require.Equal(t, testMessagingServiceSID, value(request.MessagingServiceSid))
	require.Equal(t, "https://api.example.com/api/v1/notifications/twilio/status", value(request.StatusCallback))
	require.Nil(t, request.From)
}

// This fails if provider errors can disclose the submitted request or bypass the normalized SMS error contract.
func TestSendSMS_NormalizesAndRedactsProviderErrors(t *testing.T) {
	command := sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody}
	creator := &fakeMessageCreator{err: &twilioclient.TwilioRestError{
		Code:    21211,
		Status:  400,
		Message: fmt.Sprintf("destination=%s body=%s secret=%s", command.ToE164, command.Body, sensitiveCredential),
	}}
	sender := newSender(completeSenderConfig(), creator)

	submission, err := sender.SendSMS(context.Background(), command)

	require.Equal(t, sms.Submission{}, submission)
	var normalized *providerError
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21211", HTTPStatus: 400}, normalized.ProviderError)
	for _, sensitive := range []string{command.ToE164, command.Body, sensitiveCredential} {
		require.NotContains(t, err.Error(), sensitive)
	}
}

// This fails if concurrent submissions mutate shared sender state or produce malformed provider requests.
func TestSendSMS_IsSafeForConcurrentUse(t *testing.T) {
	creator := &fakeMessageCreator{respond: func(params *openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
		return messageResponse("SM"+value(params.To), "queued"), nil
	}}
	sender := newSender(completeSenderConfig(), creator)

	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for i := range workers {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			to := fmt.Sprintf("+15555550%03d", i)
			submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: to, Body: "ready"})
			if err != nil {
				errs <- err
				return
			}
			if submission != (sms.Submission{ProviderMessageID: "SM" + to, Status: "queued"}) {
				errs <- fmt.Errorf("submission = %#v", submission)
			}
		}(i)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	for _, request := range creator.snapshot() {
		require.Equal(t, testMessagingServiceSID, value(request.MessagingServiceSid))
		require.Equal(t, "ready", value(request.Body))
		require.Equal(t, "https://api.example.com/api/v1/notifications/twilio/status", value(request.StatusCallback))
		require.Nil(t, request.From)
	}
}

type fakeMessageCreator struct {
	mu       sync.Mutex
	response *openapi.ApiV2010Message
	err      error
	respond  func(*openapi.CreateMessageParams) (*openapi.ApiV2010Message, error)
	requests []*openapi.CreateMessageParams
}

func (fake *fakeMessageCreator) CreateMessage(params *openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.requests = append(fake.requests, params)
	if fake.respond != nil {
		return fake.respond(params)
	}
	return fake.response, fake.err
}

func (fake *fakeMessageCreator) snapshot() []*openapi.CreateMessageParams {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]*openapi.CreateMessageParams(nil), fake.requests...)
}

func (fake *fakeMessageCreator) last() *openapi.CreateMessageParams {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) == 0 {
		return nil
	}
	return fake.requests[len(fake.requests)-1]
}

func completeSenderConfig() Config {
	return Config{
		AccountSID:          testAccountSID,
		APIKeySID:           testAPIKeySID,
		APIKeySecret:        testAPIKeySecret,
		AuthToken:           testAuthToken,
		MessagingServiceSID: testMessagingServiceSID,
		PublicBaseURL:       testPublicBaseURL,
	}
}

func messageResponse(sid, status string) *openapi.ApiV2010Message {
	return &openapi.ApiV2010Message{Sid: &sid, Status: &status}
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
