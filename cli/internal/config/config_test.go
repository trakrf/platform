package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBaseURL(t *testing.T) {
	cases := map[string]struct {
		env     string
		want    string
		wantErr bool
	}{
		"prod":                   {"prod", "https://app.trakrf.id", false},
		"preview":                {"preview", "https://app.preview.trakrf.id", false},
		"empty defaults to prod": {"", "https://app.trakrf.id", false},
		"unknown":                {"staging", "", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := BaseURL(tc.env)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("BaseURL(%q): want error, got nil", tc.env)
				}
				return
			}
			if err != nil {
				t.Fatalf("BaseURL(%q): unexpected error %v", tc.env, err)
			}
			if got != tc.want {
				t.Fatalf("BaseURL(%q) = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}

func TestCachedTokenValid(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		tok  *CachedToken
		want bool
	}{
		"nil token":           {nil, false},
		"empty access token":  {&CachedToken{AccessToken: "", ExpiresAt: now.Add(time.Hour)}, false},
		"expires in an hour":  {&CachedToken{AccessToken: "x", ExpiresAt: now.Add(time.Hour)}, true},
		"already expired":     {&CachedToken{AccessToken: "x", ExpiresAt: now.Add(-time.Minute)}, false},
		"inside refresh skew": {&CachedToken{AccessToken: "x", ExpiresAt: now.Add(15 * time.Second)}, false},
		"just outside skew":   {&CachedToken{AccessToken: "x", ExpiresAt: now.Add(90 * time.Second)}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.tok.Valid(now); got != tc.want {
				t.Fatalf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "config.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing: unexpected error %v", err)
	}
	if cfg == nil {
		t.Fatal("Load missing: want non-nil empty config")
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("Load missing: want 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	want := &Config{
		CurrentProfile: "work",
		Profiles: map[string]*Profile{
			"work": {
				Env:          "preview",
				ClientID:     "cid",
				ClientSecret: "secret",
				Token: &CachedToken{
					AccessToken: "jwt",
					ExpiresAt:   time.Date(2026, 6, 22, 13, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must be private (contains a client secret + token).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config perms = %o, want 600", perm)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CurrentProfile != "work" {
		t.Fatalf("CurrentProfile = %q, want work", got.CurrentProfile)
	}
	p := got.Profiles["work"]
	if p == nil {
		t.Fatal("work profile missing after round-trip")
	}
	if p.Env != "preview" || p.ClientID != "cid" || p.ClientSecret != "secret" {
		t.Fatalf("profile fields not preserved: %+v", p)
	}
	if p.Token == nil || p.Token.AccessToken != "jwt" || !p.Token.ExpiresAt.Equal(want.Profiles["work"].Token.ExpiresAt) {
		t.Fatalf("token not preserved: %+v", p.Token)
	}
}

func TestResolvePrecedence(t *testing.T) {
	cfg := &Config{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {Env: "prod", ClientID: "did", ClientSecret: "dsecret"},
			"work":    {Env: "preview", ClientID: "wid", ClientSecret: "wsecret"},
		},
	}

	t.Run("falls back to current profile", func(t *testing.T) {
		name, p, err := cfg.Resolve(ResolveInput{})
		if err != nil {
			t.Fatal(err)
		}
		if name != "default" || p.ClientID != "did" {
			t.Fatalf("got %q/%s, want default/did", name, p.ClientID)
		}
	})

	t.Run("flag profile beats current", func(t *testing.T) {
		name, p, err := cfg.Resolve(ResolveInput{Profile: "work"})
		if err != nil {
			t.Fatal(err)
		}
		if name != "work" || p.ClientID != "wid" || p.Env != "preview" {
			t.Fatalf("got %q/%s/%s, want work/wid/preview", name, p.ClientID, p.Env)
		}
	})

	t.Run("TRAKRF_ORG selects profile when no flag", func(t *testing.T) {
		_, p, err := cfg.Resolve(ResolveInput{OrgEnv: "work"})
		if err != nil {
			t.Fatal(err)
		}
		if p.ClientID != "wid" {
			t.Fatalf("got %s, want wid", p.ClientID)
		}
	})

	t.Run("env flag overrides profile env", func(t *testing.T) {
		_, p, err := cfg.Resolve(ResolveInput{Profile: "work", Env: "prod"})
		if err != nil {
			t.Fatal(err)
		}
		if p.Env != "prod" {
			t.Fatalf("env = %s, want prod (overridden)", p.Env)
		}
	})

	t.Run("TRAKRF_API_KEY id:secret overrides stored creds ephemerally", func(t *testing.T) {
		_, p, err := cfg.Resolve(ResolveInput{APIKeyEnv: "envid:envsecret"})
		if err != nil {
			t.Fatal(err)
		}
		if p.ClientID != "envid" || p.ClientSecret != "envsecret" {
			t.Fatalf("got %s/%s, want envid/envsecret", p.ClientID, p.ClientSecret)
		}
	})

	t.Run("unknown profile is an error", func(t *testing.T) {
		if _, _, err := cfg.Resolve(ResolveInput{Profile: "ghost"}); err == nil {
			t.Fatal("want error for unknown profile")
		}
	})

	t.Run("API key env with no stored profile synthesizes one", func(t *testing.T) {
		empty := &Config{}
		_, p, err := empty.Resolve(ResolveInput{APIKeyEnv: "x:y", Env: "preview"})
		if err != nil {
			t.Fatal(err)
		}
		if p.ClientID != "x" || p.ClientSecret != "y" || p.Env != "preview" {
			t.Fatalf("synthesized profile wrong: %+v", p)
		}
	})
}
