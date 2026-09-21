package twilio

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	twilioclient "github.com/twilio/twilio-go/client"
)

const maxProviderErrorBytes = 64 << 10

// providerTransport captures HTTP failures before the SDK can lose the status
// while decoding a non-JSON error response. It never retries a submission.
type providerTransport struct{ base http.RoundTripper }

func (t providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil || response.StatusCode < 400 {
		return response, err
	}
	defer response.Body.Close()
	var details struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxProviderErrorBytes)).Decode(&details); err != nil {
		details.Code = 0
	}
	return nil, &twilioclient.TwilioRestError{
		Status: response.StatusCode,
		Code:   details.Code,
	}
}

func newProviderHTTPClient(original *http.Client) *http.Client {
	// Preserve the pinned SDK's timeout and redirect defaults. Clone an
	// injected client so construction never mutates shared caller state.
	configured := http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if original != nil {
		configured = *original
	}
	base := configured.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	configured.Transport = providerTransport{base: base}
	return &configured
}
