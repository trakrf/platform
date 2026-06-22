// Package apiclient adapts the generated OpenAPI client into the small surface
// the command layer needs: an authenticated *api.ClientWithResponses whose
// bearer token is minted and refreshed on demand, a concrete token Minter
// implementing auth.TokenMinter over the real /oauth/token endpoint, and a
// helper that turns the API's error envelope into a readable Go error.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trakrf/platform/cli/api"
	"github.com/trakrf/platform/cli/internal/auth"
	"github.com/trakrf/platform/cli/internal/config"
)

// Minter exchanges client credentials for an access token via POST /oauth/token.
type Minter struct {
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

func (m *Minter) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Mint implements auth.TokenMinter using the client_credentials grant.
func (m *Minter) Mint(ctx context.Context, clientID, clientSecret string) (config.CachedToken, error) {
	opts := []api.ClientOption{}
	if m.HTTP != nil {
		opts = append(opts, api.WithHTTPClient(m.HTTP))
	}
	c, err := api.NewClientWithResponses(m.BaseURL, opts...)
	if err != nil {
		return config.CachedToken{}, fmt.Errorf("building token client: %w", err)
	}

	resp, err := c.CreateTokenWithResponse(ctx, api.TokenRequest{
		GrantType:    "client_credentials",
		ClientId:     &clientID,
		ClientSecret: &clientSecret,
	})
	if err != nil {
		return config.CachedToken{}, fmt.Errorf("requesting token: %w", err)
	}
	if resp.JSON200 == nil {
		return config.CachedToken{}, formatAPIError(resp.StatusCode(), resp.Body)
	}
	return config.CachedToken{
		AccessToken: resp.JSON200.AccessToken,
		ExpiresAt:   m.now().Add(time.Duration(resp.JSON200.ExpiresIn) * time.Second),
	}, nil
}

// New builds an authenticated client for baseURL. Every request is decorated
// with a freshly resolved bearer token from the provider.
func New(baseURL string, provider *auth.Provider) (*api.ClientWithResponses, error) {
	editor := func(ctx context.Context, req *http.Request) error {
		token, err := provider.Token(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
	return api.NewClientWithResponses(baseURL, api.WithRequestEditorFn(editor))
}

// formatAPIError renders the TrakRF error envelope into a readable error,
// falling back to the raw status when the body is not a recognizable envelope.
func formatAPIError(statusCode int, body []byte) error {
	var env api.ErrorResponse
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Title != "" {
		e := env.Error
		msg := fmt.Sprintf("API error %d: %s", statusCode, e.Title)
		if e.Detail != "" {
			msg += ": " + e.Detail
		}
		if e.RequestId != "" {
			msg += fmt.Sprintf(" (request_id=%s)", e.RequestId)
		}
		return fmt.Errorf("%s", msg)
	}
	if len(body) > 0 {
		return fmt.Errorf("API error %d: %s", statusCode, string(body))
	}
	return fmt.Errorf("API error %d: %s", statusCode, http.StatusText(statusCode))
}

// APIError exposes formatAPIError to the command layer for non-200 responses.
func APIError(statusCode int, body []byte) error { return formatAPIError(statusCode, body) }
