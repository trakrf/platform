package notification

import (
	"context"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

type observedEmailSender struct {
	sender  email.Sender
	metrics *resend.Metrics
}

func (s *observedEmailSender) SendEmail(ctx context.Context, cmd email.Command) (email.Submission, error) {
	start := time.Now()
	result, err := s.sender.SendEmail(ctx, cmd)
	s.metrics.RecordSubmission(err, time.Since(start))
	return result, err
}
