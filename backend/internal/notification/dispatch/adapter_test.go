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

func rawCmd(t *testing.T, ch notificationdelivery.Channel, payloadCh notificationdelivery.Channel, to string) outbox.Command {
	return outbox.Command{DeliveryID: "d-1", OrgID: 7, Channel: ch,
		Payload: payload(t, routing.DeliveryMessage{Channel: payloadCh, To: to, Subject: "s", Body: "line1\nline2"})}
}

func TestAdapter_UnknownChannelIsPermanentMismatch(t *testing.T) {
	fe, fs, chk := &fakeEmail{}, &fakeSMS{}, &fakeChecker{}
	a, _ := newAdapter(t, fe, fs, chk)
	_, err := a.Send(context.Background(), rawCmd(t, "fax", "fax", "123"))
	assertPermanent(t, err, "channel_mismatch")
	if len(fe.calls)+len(fs.calls) != 0 || chk.calls != 0 {
		t.Fatal("sent or looked up")
	}
}

func TestAdapter_NullAndEmptyObjectPayloads(t *testing.T) {
	for _, tc := range []struct{ name, payload, reason string }{
		{"null", "null", "invalid_payload"},
		{"empty object", "{}", "invalid_payload"},
		{"empty To", `{"channel":"email","body":"b"}`, "invalid_payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe, chk := &fakeEmail{}, &fakeChecker{}
			a, _ := newAdapter(t, fe, nil, chk)
			_, err := a.Send(context.Background(), outbox.Command{DeliveryID: "d-1", Channel: notificationdelivery.ChannelEmail, Payload: []byte(tc.payload)})
			assertPermanent(t, err, tc.reason)
			if len(fe.calls) != 0 || chk.calls != 0 {
				t.Fatal("sent or looked up")
			}
		})
	}
}

func TestAdapter_NilSMSSenderLogsAndEmailStillSends(t *testing.T) {
	chk := &fakeChecker{}
	a, buf := newAdapter(t, &fakeEmail{sub: email.Submission{ProviderMessageID: "re_1"}}, nil, chk)
	_, err := a.Send(context.Background(), smsCmd(t))
	assertPermanent(t, err, "disabled")
	out := buf.String()
	if !strings.Contains(out, `"error_kind":"disabled"`) || !strings.Contains(out, `"channel":"sms"`) ||
		strings.Contains(out, addrPhone) || strings.Contains(out, "line1") {
		t.Fatalf("bad log: %s", out)
	}
	if chk.calls != 0 {
		t.Fatal("checker called")
	}
	res, err := a.Send(context.Background(), emailCmd(t, "s"))
	if err != nil || res.ProviderMessageID != "re_1" {
		t.Fatalf("email: %v %+v", err, res)
	}
}

func TestAdapter_ProviderPermanentErrors(t *testing.T) {
	cases := []struct {
		name   string
		send   func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command)
		reason string
	}{}
	for _, k := range []email.ErrorKind{email.ErrorInvalid, email.ErrorDisabled, email.ErrorPermanent} {
		k := k
		cases = append(cases, struct {
			name   string
			send   func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command)
			reason string
		}{"email " + string(k), func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command) {
			a, buf := newAdapter(t, &fakeEmail{err: &email.ProviderError{Kind: k}}, nil, &fakeChecker{})
			return a, buf, emailCmd(t, "s")
		}, string(k)})
	}
	cases = append(cases, struct {
		name   string
		send   func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command)
		reason string
	}{"sms permanent", func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command) {
		a, buf := newAdapter(t, nil, &fakeSMS{err: &sms.ProviderError{Kind: sms.ErrorPermanent}}, &fakeChecker{})
		return a, buf, smsCmd(t)
	}, "permanent"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, buf, cmd := tc.send(t)
			_, err := a.Send(context.Background(), cmd)
			assertPermanent(t, err, tc.reason)
			if !strings.Contains(buf.String(), `"error_kind":"`+tc.reason+`"`) || !strings.Contains(buf.String(), `"level":"error"`) {
				t.Fatalf("log: %s", buf.String())
			}
		})
	}
}

func TestAdapter_OutcomeUnknownIsRetryableAndNotLogged(t *testing.T) {
	fe := &fakeEmail{err: &email.ProviderError{Kind: email.ErrorTimeout, OutcomeUnknown: true}}
	a, buf := newAdapter(t, fe, nil, &fakeChecker{})
	_, err := a.Send(context.Background(), emailCmd(t, "s"))
	if got := outbox.ClassifyOutcome(err); got != outbox.DeliveryRetryableFailure {
		t.Fatalf("classify = %v", got)
	}
	var pe *outbox.ProviderError
	if !errors.As(err, &pe) || pe.Kind != outbox.ErrorOutcomeUnknown {
		t.Fatalf("provider error = %+v", pe)
	}
	if r := asDispatchErr(t, err).ErrorReason(); r != "" {
		t.Fatalf("reason = %q", r)
	}
	if buf.Len() != 0 {
		t.Fatalf("logged: %s", buf.String())
	}
}

func TestAdapter_TransientErrorsAreRetryableAndSilent(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command)
	}{
		{"email", func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command) {
			a, b := newAdapter(t, &fakeEmail{err: &email.ProviderError{Kind: email.ErrorTransient}}, nil, &fakeChecker{})
			return a, b, emailCmd(t, "s")
		}},
		{"sms", func(t *testing.T) (*dispatch.Adapter, *bytes.Buffer, outbox.Command) {
			a, b := newAdapter(t, nil, &fakeSMS{err: &sms.ProviderError{Kind: sms.ErrorTransient}}, &fakeChecker{})
			return a, b, smsCmd(t)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, buf, cmd := tc.mk(t)
			_, err := a.Send(context.Background(), cmd)
			if outbox.ClassifyOutcome(err) != outbox.DeliveryRetryableFailure {
				t.Fatalf("err = %v", err)
			}
			if r := asDispatchErr(t, err).ErrorReason(); r != "" {
				t.Fatalf("reason = %q", r)
			}
			if buf.Len() != 0 {
				t.Fatalf("logged: %s", buf.String())
			}
		})
	}
}

func TestAdapter_SuppressionCheckReceivesOrgChannelAndAddress(t *testing.T) {
	for _, tc := range []struct {
		cmd  func(t *testing.T) outbox.Command
		ch   notificationdelivery.Channel
		addr string
	}{
		{func(t *testing.T) outbox.Command { return emailCmd(t, "s") }, notificationdelivery.ChannelEmail, addrEmail},
		{smsCmd, notificationdelivery.ChannelSMS, addrPhone},
	} {
		t.Run(string(tc.ch), func(t *testing.T) {
			chk := &fakeChecker{}
			a, _ := newAdapter(t, &fakeEmail{}, &fakeSMS{}, chk)
			cmd := tc.cmd(t)
			if _, err := a.Send(context.Background(), cmd); err != nil {
				t.Fatal(err)
			}
			if chk.calls != 1 || chk.gotOrg != cmd.OrgID || chk.gotChannel != tc.ch || chk.gotAddr != tc.addr {
				t.Fatalf("checker: %+v", chk)
			}
		})
	}
}

func TestAdapter_SubjectAndBodyHandling(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"empty", "", ""},
		{"whitespace only", " \t\r\n ", ""},
		{"unicode", "Alerte élévation — 日本語 \U0001F6A8", "Alerte élévation — 日本語 \U0001F6A8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe := &fakeEmail{}
			a, _ := newAdapter(t, fe, nil, &fakeChecker{})
			if _, err := a.Send(context.Background(), emailCmd(t, tc.in)); err != nil {
				t.Fatal(err)
			}
			if fe.calls[0].Subject != tc.want || fe.calls[0].Text != "line1\nline2" {
				t.Fatalf("got %+v", fe.calls[0])
			}
		})
	}
}

type ctxKey struct{}

type ctxEmail struct{ got string }

func (c *ctxEmail) SendEmail(ctx context.Context, _ email.Command) (email.Submission, error) {
	c.got, _ = ctx.Value(ctxKey{}).(string)
	return email.Submission{}, nil
}

type ctxSMS struct{ got string }

func (c *ctxSMS) SendSMS(ctx context.Context, _ sms.Command) (sms.Submission, error) {
	c.got, _ = ctx.Value(ctxKey{}).(string)
	return sms.Submission{}, nil
}

func TestAdapter_ContextReachesSenders(t *testing.T) {
	ce, cs := &ctxEmail{}, &ctxSMS{}
	a, _ := newAdapter(t, ce, cs, &fakeChecker{})
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	if _, err := a.Send(ctx, emailCmd(t, "s")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Send(ctx, smsCmd(t)); err != nil {
		t.Fatal(err)
	}
	if ce.got != "marker" || cs.got != "marker" {
		t.Fatalf("ctx not passed: %v %v", ce.got, cs.got)
	}
}
