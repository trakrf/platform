package dispatch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/dispatch"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

type fakeEmail struct {
	calls []email.Command
	sub   email.Submission
	err   error
}

func (f *fakeEmail) SendEmail(_ context.Context, c email.Command) (email.Submission, error) {
	f.calls = append(f.calls, c)
	return f.sub, f.err
}

type fakeSMS struct {
	calls []sms.Command
	sub   sms.Submission
	err   error
}

func (f *fakeSMS) SendSMS(_ context.Context, c sms.Command) (sms.Submission, error) {
	f.calls = append(f.calls, c)
	return f.sub, f.err
}

type fakeChecker struct {
	calls      int
	gotOrg     int
	gotChannel notificationdelivery.Channel
	gotAddr    string
	suppressed bool
	err        error
}

func (f *fakeChecker) IsSuppressed(_ context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error) {
	f.calls++
	f.gotOrg, f.gotChannel, f.gotAddr = orgID, ch, address
	return f.suppressed, f.err
}

const (
	addrEmail = "owner@acme.test"
	addrPhone = "+15551234567"
)

func payload(t *testing.T, m routing.DeliveryMessage) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func emailCmd(t *testing.T, subject string) outbox.Command {
	return outbox.Command{DeliveryID: "d-1", OrgID: 7, Channel: notificationdelivery.ChannelEmail,
		Payload: payload(t, routing.DeliveryMessage{Channel: notificationdelivery.ChannelEmail, To: addrEmail, Subject: subject, Body: "line1\nline2"})}
}

func smsCmd(t *testing.T) outbox.Command {
	return outbox.Command{DeliveryID: "d-1", OrgID: 7, Channel: notificationdelivery.ChannelSMS,
		Payload: payload(t, routing.DeliveryMessage{Channel: notificationdelivery.ChannelSMS, To: addrPhone, Body: "line1\nline2"})}
}

func newAdapter(t *testing.T, e email.Sender, s sms.Sender, c dispatch.SuppressionChecker) (*dispatch.Adapter, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	a, err := dispatch.New(dispatch.Deps{Email: e, SMS: s, Suppressions: c, Log: zerolog.New(&buf)})
	if err != nil {
		t.Fatal(err)
	}
	return a, &buf
}

func asDispatchErr(t *testing.T, err error) *dispatch.Error {
	t.Helper()
	var de *dispatch.Error
	if !errors.As(err, &de) {
		t.Fatalf("error %v is not *dispatch.Error", err)
	}
	return de
}

func TestAdapter_EmailMapsPlainTextAndStripsSubjectCRLF(t *testing.T) {
	fe := &fakeEmail{sub: email.Submission{ProviderMessageID: "re_1"}}
	a, _ := newAdapter(t, fe, nil, &fakeChecker{})
	res, err := a.Send(context.Background(), emailCmd(t, "Gate\r\nexit: Pallet\n7"))
	if err != nil {
		t.Fatal(err)
	}
	if res.ProviderMessageID != "re_1" {
		t.Fatalf("id = %q", res.ProviderMessageID)
	}
	want := email.Command{DeliveryID: "d-1", To: addrEmail, Subject: "Gate exit: Pallet 7", Text: "line1\nline2", HTML: ""}
	if len(fe.calls) != 1 || fe.calls[0] != want {
		t.Fatalf("calls = %+v, want %+v", fe.calls, want)
	}
}

func TestAdapter_SMSMapsCommand(t *testing.T) {
	fs := &fakeSMS{sub: sms.Submission{ProviderMessageID: "SM1"}}
	a, _ := newAdapter(t, nil, fs, &fakeChecker{})
	res, err := a.Send(context.Background(), smsCmd(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.ProviderMessageID != "SM1" {
		t.Fatalf("id = %q", res.ProviderMessageID)
	}
	want := sms.Command{DeliveryID: "d-1", ToE164: addrPhone, Body: "line1\nline2"}
	if len(fs.calls) != 1 || fs.calls[0] != want {
		t.Fatalf("calls = %+v, want %+v", fs.calls, want)
	}
}

func assertPermanent(t *testing.T, err error, reason string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := outbox.ClassifyOutcome(err); got != outbox.DeliveryPermanentFailure {
		t.Fatalf("classify = %v", got)
	}
	if got := asDispatchErr(t, err).ErrorReason(); got != reason {
		t.Fatalf("reason = %q, want %q", got, reason)
	}
}

func TestAdapter_NilPayloadIsPermanent(t *testing.T) {
	fe := &fakeEmail{}
	a, _ := newAdapter(t, fe, nil, &fakeChecker{})
	_, err := a.Send(context.Background(), outbox.Command{DeliveryID: "d-1", Channel: notificationdelivery.ChannelEmail})
	assertPermanent(t, err, "invalid_payload")
	if len(fe.calls) != 0 {
		t.Fatal("sent")
	}
}

func TestAdapter_UndecodablePayloadIsPermanent(t *testing.T) {
	fe := &fakeEmail{}
	a, _ := newAdapter(t, fe, nil, &fakeChecker{})
	_, err := a.Send(context.Background(), outbox.Command{DeliveryID: "d-1", Channel: notificationdelivery.ChannelEmail, Payload: []byte("{")})
	assertPermanent(t, err, "invalid_payload")
	if len(fe.calls) != 0 {
		t.Fatal("sent")
	}
}

func TestAdapter_ChannelMismatchIsPermanent(t *testing.T) {
	fe, fs := &fakeEmail{}, &fakeSMS{}
	a, _ := newAdapter(t, fe, fs, &fakeChecker{})
	cmd := emailCmd(t, "s")
	cmd.Channel = notificationdelivery.ChannelSMS
	_, err := a.Send(context.Background(), cmd)
	assertPermanent(t, err, "channel_mismatch")
	if len(fe.calls)+len(fs.calls) != 0 {
		t.Fatal("sent")
	}
}

func TestAdapter_NilSenderLogsAndDoesNotSend(t *testing.T) {
	chk := &fakeChecker{}
	a, buf := newAdapter(t, nil, nil, chk)
	_, err := a.Send(context.Background(), emailCmd(t, "s"))
	assertPermanent(t, err, "disabled")
	out := buf.String()
	for _, want := range []string{`"level":"error"`, `"error_kind":"disabled"`, `"delivery_id":"d-1"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log %s lacks %s", out, want)
		}
	}
	if strings.Contains(out, addrEmail) || strings.Contains(out, "line1") {
		t.Fatalf("log leaks PII: %s", out)
	}
	if chk.calls != 0 {
		t.Fatal("checker called")
	}
}

func TestAdapter_SuppressedAddressIsNotSent(t *testing.T) {
	fe := &fakeEmail{}
	chk := &fakeChecker{suppressed: true}
	a, buf := newAdapter(t, fe, nil, chk)
	_, err := a.Send(context.Background(), emailCmd(t, "s"))
	assertPermanent(t, err, "opted_out")
	if len(fe.calls) != 0 {
		t.Fatal("sent")
	}
	if chk.gotAddr != addrEmail || chk.gotOrg != 7 || chk.gotChannel != notificationdelivery.ChannelEmail {
		t.Fatalf("checker got %d %q %q", chk.gotOrg, chk.gotChannel, chk.gotAddr)
	}
	out := buf.String()
	if !strings.Contains(out, `"error_kind":"opted_out"`) || strings.Contains(out, addrEmail) {
		t.Fatalf("bad log: %s", out)
	}
}

func TestAdapter_SuppressionLookupErrorIsRetryable(t *testing.T) {
	fe := &fakeEmail{}
	a, _ := newAdapter(t, fe, nil, &fakeChecker{err: errors.New("db down")})
	_, err := a.Send(context.Background(), emailCmd(t, "s"))
	if err == nil || outbox.ClassifyOutcome(err) != outbox.DeliveryRetryableFailure {
		t.Fatalf("err = %v", err)
	}
	if len(fe.calls) != 0 {
		t.Fatal("sent")
	}
}

func TestAdapter_RawSenderErrorIsBoundedAndRetryable(t *testing.T) {
	fe := &fakeEmail{err: errors.New("smtp said no to " + addrEmail)}
	a, _ := newAdapter(t, fe, nil, &fakeChecker{})
	_, err := a.Send(context.Background(), emailCmd(t, "s"))
	if err == nil || outbox.ClassifyOutcome(err) != outbox.DeliveryRetryableFailure {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), addrEmail) {
		t.Fatalf("error leaks address: %v", err)
	}
}

func TestAdapter_ProviderPermanentErrorIsLoggedWithReason(t *testing.T) {
	fs := &fakeSMS{err: &sms.ProviderError{Kind: sms.ErrorRejected}}
	a, buf := newAdapter(t, nil, fs, &fakeChecker{})
	_, err := a.Send(context.Background(), smsCmd(t))
	assertPermanent(t, err, "rejected")
	if !strings.Contains(err.Error(), "rejected") || !strings.Contains(buf.String(), `"error_kind":"rejected"`) {
		t.Fatalf("err %v log %s", err, buf.String())
	}
}

func TestNew_RequiresSuppressions(t *testing.T) {
	if _, err := dispatch.New(dispatch.Deps{}); err == nil {
		t.Fatal("expected error")
	}
}
