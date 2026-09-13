package twilio

import (
	"errors"
	"strconv"

	"github.com/trakrf/platform/backend/internal/notification/sms"
	twilioclient "github.com/twilio/twilio-go/client"
)

const (
	timeoutCode          = "timeout"
	temporaryNetworkCode = "temporary_network"
	unknownCode          = "unknown"
)

// classifyError converts provider and transport failures into bounded handling
// categories without retaining raw error text or request data.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	if code, status, ok := twilioErrorDetails(err); ok {
		return newProviderError(classifyTwilioFailure(code, status), code, status)
	}

	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return newProviderError(sms.ErrorTransient, timeoutCode, 0)
	}

	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		return newProviderError(sms.ErrorTransient, temporaryNetworkCode, 0)
	}

	return newProviderError(sms.ErrorPermanent, unknownCode, 0)
}

func twilioErrorDetails(err error) (code string, status int, ok bool) {
	var legacy *twilioclient.TwilioRestError
	if errors.As(err, &legacy) {
		return twilioCode(legacy.Code), legacy.Status, true
	}

	var v1 *twilioclient.RestErrorV1
	if errors.As(err, &v1) {
		return twilioCode(v1.Code), v1.HttpStatusCode, true
	}

	return "", 0, false
}

func twilioCode(code int) string {
	if code == 0 {
		return ""
	}
	return strconv.Itoa(code)
}

func classifyTwilioFailure(code string, status int) sms.ErrorKind {
	switch code {
	case "30007", "30450":
		return sms.ErrorRejected
	case "21211", "21408", "21610", "21612", "30034":
		return sms.ErrorPermanent
	}

	if status == 429 || (status >= 500 && status < 600) {
		return sms.ErrorTransient
	}
	if status >= 400 && status < 500 {
		return sms.ErrorPermanent
	}
	return sms.ErrorPermanent
}

func newProviderError(kind sms.ErrorKind, code string, status int) error {
	return &sms.ProviderError{
		Kind:       kind,
		Code:       code,
		HTTPStatus: status,
	}
}
