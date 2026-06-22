// Package auth turns a profile's stored {client_id, client_secret} into a usable
// bearer token, minting a short-lived access token via the OAuth2
// client_credentials grant and caching it until it expires.
//
// The refresh_token rotation grant is intentionally not used: re-minting from
// the stored credentials is simpler and avoids the spec's reuse-revokes-the-
// chain failure mode for a CLI that may run many independent processes.
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/trakrf/platform/cli/internal/config"
)

// TokenMinter exchanges client credentials for a fresh access token.
type TokenMinter interface {
	Mint(ctx context.Context, clientID, clientSecret string) (config.CachedToken, error)
}

// Provider yields a valid bearer token for a profile, minting and caching as
// needed. Persist (optional) writes an updated token back to the config file.
type Provider struct {
	Profile *config.Profile
	Minter  TokenMinter
	Now     func() time.Time
	Persist func(config.CachedToken) error
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Token returns a valid access token, reusing the cached one when it is still
// good and otherwise minting (and caching) a new one.
func (p *Provider) Token(ctx context.Context) (string, error) {
	if p.Profile == nil {
		return "", fmt.Errorf("no active profile")
	}
	if p.Profile.Token.Valid(p.now()) {
		return p.Profile.Token.AccessToken, nil
	}
	if p.Profile.ClientID == "" || p.Profile.ClientSecret == "" {
		return "", fmt.Errorf("no API credentials configured (run `trakrf auth login` or set TRAKRF_API_KEY)")
	}

	tok, err := p.Minter.Mint(ctx, p.Profile.ClientID, p.Profile.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("minting access token: %w", err)
	}
	p.Profile.Token = &tok
	if p.Persist != nil {
		// Caching is best-effort: a write failure must not break the command.
		_ = p.Persist(tok)
	}
	return tok.AccessToken, nil
}
