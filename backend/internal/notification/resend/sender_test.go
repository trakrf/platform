package resend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func command() email.Command {
	return email.Command{DeliveryID: "delivery-1", To: "recipient@example.com", Subject: "Asset moved", Text: "Moved to receiving", HTML: "<p>Moved to receiving</p>"}
}
func newTestSender(t *testing.T, transport http.RoundTripper) *Sender {
	t.Helper()
	sender, err := NewSenderWithHTTPClient(enabledConfig(), &http.Client{Transport: transport})
	require.NoError(t, err)
	return sender
}

func TestSenderMapsCommandThroughSDK(t *testing.T) {
	for _, content := range []string{"both", "text", "html"} {
		t.Run(content, func(t *testing.T) {
			cmd := command()
			if content == "text" {
				cmd.HTML = ""
			}
			if content == "html" {
				cmd.Text = ""
			}
			var calls int
			sender := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "https://api.resend.com/emails", r.URL.String())
				require.Equal(t, "Bearer re_test_secret", r.Header.Get("Authorization"))
				require.Contains(t, r.Header.Get("User-Agent"), "resend-go/")
				var payload struct {
					From                string
					To                  []string
					Subject, Text, HTML string
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, "TrakRF <alerts@example.com>", payload.From)
				require.Equal(t, []string{"recipient@example.com"}, payload.To)
				require.Equal(t, "Asset moved", payload.Subject)
				require.Equal(t, cmd.Text, payload.Text)
				require.Equal(t, cmd.HTML, payload.HTML)
				deadline, ok := r.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
				return response(200, `{"id":"provider-message-1"}`), nil
			}))
			result, err := sender.SendEmail(context.Background(), cmd)
			require.NoError(t, err)
			require.Equal(t, email.Submission{ProviderMessageID: "provider-message-1"}, result)
			require.Equal(t, 1, calls)
		})
	}
}

func TestSenderRejectsInvalidInputWithoutNetwork(t *testing.T) {
	sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid command made a request")
		return nil, nil
	}))
	for _, tc := range []struct {
		name   string
		mutate func(*email.Command)
	}{
		{"delivery", func(c *email.Command) { c.DeliveryID = " \t " }},
		{"recipient", func(c *email.Command) { c.To = "invalid" }},
		{"recipient list", func(c *email.Command) { c.To = "a@example.com, b@example.com" }},
		{"recipient display name", func(c *email.Command) { c.To = "Recipient <a@example.com>" }},
		{"recipient newline", func(c *email.Command) { c.To = "a@example.com\r\nBcc: b@example.com" }},
		{"subject", func(c *email.Command) { c.Subject = " " }},
		{"subject newline", func(c *email.Command) { c.Subject = "hello\r\nBcc: b@example.com" }},
		{"body", func(c *email.Command) { c.Text = " \n"; c.HTML = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := command()
			tc.mutate(&cmd)
			result, err := sender.SendEmail(context.Background(), cmd)
			require.Equal(t, email.Submission{}, result)
			require.Equal(t, &email.ProviderError{Kind: email.ErrorInvalid}, err)
		})
	}
}

func TestSenderRequiresEnabledValidConfiguration(t *testing.T) {
	sender, err := NewSender(Config{})
	require.Nil(t, sender)
	require.Equal(t, &email.ProviderError{Kind: email.ErrorDisabled}, err)
	c := enabledConfig()
	c.APIKey = ""
	sender, err = NewSender(c)
	require.Nil(t, sender)
	require.Error(t, err)
	sender, err = NewSender(enabledConfig()) // Constructor itself never sends.
	require.NoError(t, err)
	require.NotNil(t, sender)
}

func TestSenderClassifiesHTTPFailuresWithoutProviderDetails(t *testing.T) {
	for _, tc := range []struct {
		status    int
		kind      email.ErrorKind
		ambiguous bool
	}{
		{301, email.ErrorPermanent, false}, {400, email.ErrorPermanent, false},
		{401, email.ErrorPermanent, false}, {403, email.ErrorPermanent, false},
		{404, email.ErrorPermanent, false}, {408, email.ErrorTransient, true},
		{409, email.ErrorPermanent, true}, {422, email.ErrorPermanent, false},
		{429, email.ErrorTransient, false}, {500, email.ErrorTransient, true},
		{502, email.ErrorTransient, true}, {503, email.ErrorTransient, true},
	} {
		for _, body := range []string{`{"name":"validation_error","message":"recipient@example.com re_secret private body","statusCode":400}`, "upstream private body"} {
			t.Run(fmt.Sprint(tc.status, "/", strings.HasPrefix(body, "{")), func(t *testing.T) {
				calls := 0
				sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return response(tc.status, body), nil }))
				result, err := sender.SendEmail(context.Background(), command())
				require.Equal(t, email.Submission{}, result)
				require.Equal(t, &email.ProviderError{Kind: tc.kind, HTTPStatus: tc.status, OutcomeUnknown: tc.ambiguous}, err)
				require.Nil(t, errors.Unwrap(err))
				require.NotContains(t, fmt.Sprintf("%+v", err), "private")
				require.NotContains(t, fmt.Sprintf("%+v", err), "recipient")
				require.NotContains(t, fmt.Sprintf("%+v", err), "re_secret")
				require.Equal(t, 1, calls, "no implicit retry")
			})
		}
	}
}

func TestSenderMalformedAcceptanceIsAmbiguous(t *testing.T) {
	for _, body := range []string{"not-json", `{}`, `{"id":""}`, `{"id":" "}`, `null`} {
		t.Run(body, func(t *testing.T) {
			sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return response(200, body), nil }))
			result, err := sender.SendEmail(context.Background(), command())
			require.Equal(t, email.Submission{}, result)
			require.Equal(t, &email.ProviderError{Kind: email.ErrorUnknown, OutcomeUnknown: true}, err)
		})
	}
}

func TestSenderNetworkFailureIsAmbiguousAndRedacted(t *testing.T) {
	calls := 0
	sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("re_secret recipient@example.com private body")
	}))
	result, err := sender.SendEmail(context.Background(), command())
	require.Equal(t, email.Submission{}, result)
	require.Equal(t, &email.ProviderError{Kind: email.ErrorTransient, OutcomeUnknown: true}, err)
	require.Nil(t, errors.Unwrap(err))
	require.Equal(t, 1, calls)
}

func TestSenderCanceledBeforeSubmissionMakesNoRequest(t *testing.T) {
	sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("canceled command made request"); return nil, nil }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := sender.SendEmail(ctx, command())
	require.Equal(t, email.Submission{}, result)
	require.Equal(t, &email.ProviderError{Kind: email.ErrorCanceled}, err)
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = sender.SendEmail(ctx, command())
	require.Equal(t, &email.ProviderError{Kind: email.ErrorTimeout}, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSenderHonorsCancellationAndTimeoutsDuringRequest(t *testing.T) {
	for _, mode := range []string{"caller cancel", "caller deadline", "configured timeout"} {
		t.Run(mode, func(t *testing.T) {
			config := enabledConfig()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := email.ErrorCanceled
			if mode == "caller deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 20*time.Millisecond)
				defer stop()
				want = email.ErrorTimeout
			}
			if mode == "configured timeout" {
				config.Timeout = 20 * time.Millisecond
				want = email.ErrorTimeout
			}
			sender, err := NewSenderWithHTTPClient(config, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if mode == "caller cancel" {
					cancel()
				}
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-time.After(time.Second):
					return nil, errors.New("context not propagated")
				}
			})})
			require.NoError(t, err)
			result, err := sender.SendEmail(ctx, command())
			require.Equal(t, email.Submission{}, result)
			require.Equal(t, &email.ProviderError{Kind: want, OutcomeUnknown: true}, err)
		})
	}
}

func TestSenderDisablesRedirectsAndPreservesInjectedClient(t *testing.T) {
	var calls int
	original := &http.Client{Timeout: time.Minute, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		r := response(307, "")
		r.Header.Set("Location", "https://untrusted.example/emails")
		return r, nil
	})}
	sender, err := NewSenderWithHTTPClient(enabledConfig(), original)
	require.NoError(t, err)
	_, err = sender.SendEmail(context.Background(), command())
	require.Error(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, time.Minute, original.Timeout)
	require.Nil(t, original.CheckRedirect)
}

func TestSenderConcurrentRequestsKeepIndependentResults(t *testing.T) {
	var calls atomic.Int32
	sender := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var payload struct{ Subject string }
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload.Subject == "failure" {
			return response(429, `{"message":"private"}`), nil
		}
		return response(200, `{"id":"accepted"}`), nil
	}))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := command()
			if i%2 == 0 {
				cmd.Subject = "failure"
			}
			result, err := sender.SendEmail(context.Background(), cmd)
			if i%2 == 0 {
				if result != (email.Submission{}) || !errors.As(err, new(*email.ProviderError)) {
					t.Errorf("failure request lost classification: %v %v", result, err)
				}
			} else if err != nil || result.ProviderMessageID != "accepted" {
				t.Errorf("success request mixed up: %v %v", result, err)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(20), calls.Load())
}
