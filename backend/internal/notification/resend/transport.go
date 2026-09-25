package resend

import "net/http"

// Resend SDK v2.28.0 flattens most API errors to strings, discarding status.
// Preserve only that status before SDK decoding; never read or retain provider
// error bodies. Authentication, payload encoding and success decoding remain
// SDK responsibilities. No mutable per-request status is stored on the sender.
type providerTransport struct{ base http.RoundTripper }
type httpFailure struct{ status int }

func (*httpFailure) Error() string { return "email provider HTTP failure" }

func (t providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	response.Body.Close()
	return nil, &httpFailure{status: response.StatusCode}
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
