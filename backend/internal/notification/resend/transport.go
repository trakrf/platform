package resend

import (
	"encoding/json"
	"io"
	"net/http"
)

// Resend SDK v2.28.0 flattens most API errors to strings, discarding status.
// Preserve status and only allowlisted conflict names before SDK decoding.
// No arbitrary provider response text is retained. Authentication, payload encoding and success decoding remain
// SDK responsibilities. No mutable per-request status is stored on the sender.
type providerTransport struct{ base http.RoundTripper }
type httpFailure struct {
	status   int
	conflict string
}

func (*httpFailure) Error() string { return "email provider HTTP failure" }

func (t providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	failure := &httpFailure{status: response.StatusCode}
	if response.StatusCode == http.StatusConflict {
		// The pinned SDK loses the distinction between the two idempotency
		// conflicts. Inspect a bounded body only for those two names.
		body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
		var detail struct {
			Name string `json:"name"`
		}
		if err == nil && len(body) <= 4096 && json.Unmarshal(body, &detail) == nil {
			switch detail.Name {
			case "invalid_idempotent_request", "concurrent_idempotent_requests":
				failure.conflict = detail.Name
			}
		}
	}
	return nil, failure
}

func providerHTTPClient(original *http.Client) *http.Client {
	client := http.Client{}
	if original != nil {
		client = *original
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = providerTransport{base: base}
	// POST redirects must not replay an email or disclose request contents.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}
