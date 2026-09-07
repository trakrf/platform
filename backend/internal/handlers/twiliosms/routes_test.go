package twiliosms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

type routeConsumer struct {
	statuses []sms.ProviderStatus
	keywords []sms.InboundKeyword
}

func (c *routeConsumer) HandleStatus(_ context.Context, status sms.ProviderStatus) error {
	c.statuses = append(c.statuses, status)
	return nil
}

func (c *routeConsumer) HandleKeyword(_ context.Context, keyword sms.InboundKeyword) error {
	c.keywords = append(c.keywords, keyword)
	return nil
}

// This fails if either public callback path cannot reach its existing signed
// handler without a TrakRF session or API-key credential.
func TestRoutes_PostSignedCallbacksReachNormalizedConsumer(t *testing.T) {
	for _, test := range []struct {
		name         string
		request      func(*testing.T) *http.Request
		wantStatuses []sms.ProviderStatus
		wantKeywords []sms.InboundKeyword
	}{
		{
			name: "status",
			request: func(t *testing.T) *http.Request {
				return signedStatusRequest(t, url.Values{
					"MessageSid":    {"SM123"},
					"MessageStatus": {"delivered"},
				})
			},
			wantStatuses: []sms.ProviderStatus{{
				ProviderMessageID: "SM123",
				Status:            "delivered",
				OccurredAt:        time.Time{},
			}},
		},
		{
			name: "inbound",
			request: func(t *testing.T) *http.Request {
				return signedInboundRequest(t, inboundKeywordForm("STOP"))
			},
			wantKeywords: []sms.InboundKeyword{{
				ProviderMessageID: "SM123",
				FromE164:          "+15550001111",
				ToE164:            "+15550002222",
				Keyword:           "STOP",
				ReceivedAt:        time.Time{},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &routeConsumer{}
			router := newRoutesTestRouter(t, consumer)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, test.request(t))

			require.Equal(t, http.StatusNoContent, recorder.Code)
			require.Equal(t, test.wantStatuses, consumer.statuses)
			require.Equal(t, test.wantKeywords, consumer.keywords)
		})
	}
}

// This fails if a forged form reaches either callback consumer through a
// registered public route.
func TestRoutes_RejectForgedPostsBeforeConsumerHandoff(t *testing.T) {
	for _, path := range []string{
		"/api/v1/notifications/twilio/status",
		"/api/v1/notifications/twilio/inbound",
	} {
		t.Run(path, func(t *testing.T) {
			consumer := &routeConsumer{}
			router := newRoutesTestRouter(t, consumer)
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("Body=private+message"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Twilio-Signature", "forged")

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Empty(t, consumer.statuses)
			require.Empty(t, consumer.keywords)
		})
	}
}

// This fails if either public callback route accepts a method other than POST
// or does not advertise POST as its only supported method.
func TestRoutes_RejectsGetWithPostAllowHeader(t *testing.T) {
	for _, path := range []string{
		"/api/v1/notifications/twilio/status",
		"/api/v1/notifications/twilio/inbound",
	} {
		t.Run(path, func(t *testing.T) {
			router := newRoutesTestRouter(t, &routeConsumer{})
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

			require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
			require.Equal(t, http.MethodPost, recorder.Header().Get("Allow"))
		})
	}
}

// This fails if callback registration captures neighboring paths or registers
// a Twilio callback path beyond the two declared endpoints.
func TestRoutes_LeavesUndeclaredPathsNotFound(t *testing.T) {
	router := newRoutesTestRouter(t, &routeConsumer{})

	for _, path := range []string{
		"/api/v1/notifications/twilio",
		"/api/v1/notifications/twilio/status/extra",
		"/api/v1/notifications/twilio/inbound/extra",
		"/api/v1/notifications/twilio/other",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))

			require.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}

func newRoutesTestRouter(t *testing.T, consumer sms.CallbackConsumer) *chi.Mux {
	t.Helper()
	handler, err := NewHandler(newSignatureTestConfig(), consumer)
	require.NoError(t, err)
	handler.now = func() time.Time { return time.Time{} }

	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	return router
}
