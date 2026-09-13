package twilio

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// This fails if concurrent sends share mutable SDK request state on the production HTTP path.
func TestSendSMS_IsSafeForConcurrentUse(t *testing.T) {
	metrics, err := NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return httpJSONResponse(request, http.StatusCreated, `{"sid":"SM123","status":"queued"}`), nil
	})}, metrics)

	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for i := range workers {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			submission, err := sender.SendSMS(context.Background(), sms.Command{
				ToE164: fmt.Sprintf("+15555550%03d", i),
				Body:   "ready",
			})
			if err != nil {
				errs <- err
				return
			}
			if submission != (sms.Submission{ProviderMessageID: "SM123", Status: "queued"}) {
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
}
