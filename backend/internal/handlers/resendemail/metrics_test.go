package resendemail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

func TestWebhookMetricsReflectHandoff(t *testing.T) {
	for _, tc := range []struct {
		name, body, method, outcome string
		err                         error
		status                      int
	}{
		{"persisted", fmt.Sprintf(fixture, "email.delivered"), "POST", "persisted", nil, 204},
		{"failure", fmt.Sprintf(fixture, "email.failed"), "POST", "consumer_failure", errors.New("private failure"), 503},
		{"ignored", `{"type":"future"}`, "POST", "ignored", nil, 204},
		{"malformed", `{`, "POST", "malformed", nil, 400},
		{"too large", strings.Repeat("x", maxBodyBytes+1), "POST", "too_large", nil, 413},
		{"method", "", "GET", "method_not_allowed", nil, 405},
		{"signature", "", "POST", "invalid_signature", nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics, err := resend.NewMetrics(registry)
			require.NoError(t, err)
			h, err := NewHandler(config(), consumeFunc(func(context.Context, email.CallbackEvent) error { return tc.err }))
			require.NoError(t, err)
			req := signed(tc.body, time.Now())
			req.Method = tc.method
			if tc.name == "signature" {
				req.Header.Del("svix-signature")
			}
			rec := httptest.NewRecorder()
			h.WithMetrics(metrics).ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code)
			families, err := registry.Gather()
			require.NoError(t, err)
			found := false
			for _, family := range families {
				if family.GetName() != "trakrf_resend_callbacks_total" {
					continue
				}
				found = true
				require.Len(t, family.Metric, 1)
				require.Equal(t, tc.outcome, family.Metric[0].Label[0].GetValue())
				require.Equal(t, float64(1), family.Metric[0].Counter.GetValue())
			}
			require.True(t, found)
		})
	}
	var _ http.Handler = (*Handler)(nil)
}
