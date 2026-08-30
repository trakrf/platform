package twiliosms

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
)

const maxCallbackFormBytes = 1 << 20

var (
	errInvalidSignature = errors.New("invalid Twilio signature")
	errMalformedForm    = errors.New("invalid Twilio form")
)

// verifiedForm parses a bounded form callback and returns it only when its
// signature matches the configured externally visible URL.
func (h *Handler) verifiedForm(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		return nil, errMalformedForm
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCallbackFormBytes)
	if err := r.ParseForm(); err != nil {
		return nil, errMalformedForm
	}

	params := make(map[string]string, len(r.PostForm))
	for key, values := range r.PostForm {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	signature := r.Header.Get("X-Twilio-Signature")
	if signature == "" || !h.validator.Validate(h.publicRequestURL(r), params, signature) {
		return nil, errInvalidSignature
	}

	return r.PostForm, nil
}

func (h *Handler) publicRequestURL(r *http.Request) string {
	callbackURL := h.publicBaseURL + r.URL.EscapedPath()
	if r.URL.ForceQuery || r.URL.RawQuery != "" {
		callbackURL += "?" + r.URL.RawQuery
	}
	return callbackURL
}
