package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trakrf/platform/cli/internal/config"
)

type fakeMinter struct {
	calls  int
	token  config.CachedToken
	err    error
	gotID  string
	gotSec string
}

func (f *fakeMinter) Mint(_ context.Context, clientID, clientSecret string) (config.CachedToken, error) {
	f.calls++
	f.gotID = clientID
	f.gotSec = clientSecret
	return f.token, f.err
}

func fixedNow() time.Time { return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC) }

func TestTokenUsesValidCacheWithoutMinting(t *testing.T) {
	prof := &config.Profile{
		ClientID:     "cid",
		ClientSecret: "secret",
		Token:        &config.CachedToken{AccessToken: "cached", ExpiresAt: fixedNow().Add(time.Hour)},
	}
	m := &fakeMinter{}
	p := &Provider{Profile: prof, Minter: m, Now: fixedNow}

	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "cached" {
		t.Fatalf("token = %q, want cached", got)
	}
	if m.calls != 0 {
		t.Fatalf("minter called %d times, want 0 for a valid cache", m.calls)
	}
}

func TestTokenMintsWhenExpiredAndPersists(t *testing.T) {
	prof := &config.Profile{
		ClientID:     "cid",
		ClientSecret: "secret",
		Token:        &config.CachedToken{AccessToken: "old", ExpiresAt: fixedNow().Add(-time.Minute)},
	}
	fresh := config.CachedToken{AccessToken: "fresh", ExpiresAt: fixedNow().Add(15 * time.Minute)}
	m := &fakeMinter{token: fresh}

	var persisted *config.CachedToken
	p := &Provider{
		Profile: prof,
		Minter:  m,
		Now:     fixedNow,
		Persist: func(tok config.CachedToken) error { persisted = &tok; return nil },
	}

	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh" {
		t.Fatalf("token = %q, want fresh", got)
	}
	if m.calls != 1 || m.gotID != "cid" || m.gotSec != "secret" {
		t.Fatalf("mint not called correctly: calls=%d id=%q sec=%q", m.calls, m.gotID, m.gotSec)
	}
	if prof.Token == nil || prof.Token.AccessToken != "fresh" {
		t.Fatalf("profile token not updated: %+v", prof.Token)
	}
	if persisted == nil || persisted.AccessToken != "fresh" {
		t.Fatalf("token not persisted: %+v", persisted)
	}
}

func TestTokenMintsWhenNoCache(t *testing.T) {
	prof := &config.Profile{ClientID: "cid", ClientSecret: "secret"}
	m := &fakeMinter{token: config.CachedToken{AccessToken: "minted", ExpiresAt: fixedNow().Add(time.Hour)}}
	p := &Provider{Profile: prof, Minter: m, Now: fixedNow}

	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "minted" || m.calls != 1 {
		t.Fatalf("got %q calls %d, want minted/1", got, m.calls)
	}
}

func TestTokenRequiresCredentials(t *testing.T) {
	p := &Provider{Profile: &config.Profile{}, Minter: &fakeMinter{}, Now: fixedNow}
	if _, err := p.Token(context.Background()); err == nil {
		t.Fatal("want error when credentials are missing")
	}
}

func TestTokenPropagatesMintError(t *testing.T) {
	prof := &config.Profile{ClientID: "cid", ClientSecret: "secret"}
	m := &fakeMinter{err: errors.New("401 invalid client")}
	p := &Provider{Profile: prof, Minter: m, Now: fixedNow}
	if _, err := p.Token(context.Background()); err == nil {
		t.Fatal("want mint error to propagate")
	}
}

func TestPersistFailureIsNonFatal(t *testing.T) {
	// A token that mints fine but fails to cache should still be returned —
	// failing to write the cache must not break the command.
	prof := &config.Profile{ClientID: "cid", ClientSecret: "secret"}
	m := &fakeMinter{token: config.CachedToken{AccessToken: "minted", ExpiresAt: fixedNow().Add(time.Hour)}}
	p := &Provider{
		Profile: prof,
		Minter:  m,
		Now:     fixedNow,
		Persist: func(config.CachedToken) error { return errors.New("disk full") },
	}
	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("persist failure should be non-fatal, got %v", err)
	}
	if got != "minted" {
		t.Fatalf("token = %q, want minted", got)
	}
}
