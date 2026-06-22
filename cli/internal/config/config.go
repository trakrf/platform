// Package config manages the on-disk TrakRF CLI configuration
// (~/.trakrf/config.yaml): named profiles, each binding an environment
// (prod/preview) to a {client_id, client_secret} credential pair and a cached
// access token, plus the precedence rules that fold flags and environment
// variables over the stored profile.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// refreshSkew is how long before real expiry a cached token is treated as
// already stale, so a request never goes out with a token about to expire.
const refreshSkew = 60 * time.Second

// Built-in environment base URLs, mirroring the spec's `servers` block.
const (
	prodBaseURL    = "https://app.trakrf.id"
	previewBaseURL = "https://app.preview.trakrf.id"
)

// Config is the root of the on-disk configuration file.
type Config struct {
	CurrentProfile string              `yaml:"current_profile,omitempty"`
	Profiles       map[string]*Profile `yaml:"profiles,omitempty"`
}

// Profile binds an environment to an API credential pair and its cached token.
type Profile struct {
	Env          string       `yaml:"env"`
	ClientID     string       `yaml:"client_id"`
	ClientSecret string       `yaml:"client_secret"`
	Token        *CachedToken `yaml:"token,omitempty"`
}

// CachedToken is a short-lived access token persisted between invocations so a
// single login serves many commands until the token expires.
type CachedToken struct {
	AccessToken string    `yaml:"access_token"`
	ExpiresAt   time.Time `yaml:"expires_at"`
}

// Valid reports whether the token can still be used at time now, accounting for
// the refresh skew so callers never present a token on the edge of expiry.
func (t *CachedToken) Valid(now time.Time) bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return now.Add(refreshSkew).Before(t.ExpiresAt)
}

// BaseURL maps an environment name to its API base URL. Empty means prod.
func BaseURL(env string) (string, error) {
	switch env {
	case "", "prod", "production":
		return prodBaseURL, nil
	case "preview":
		return previewBaseURL, nil
	default:
		return "", fmt.Errorf("unknown environment %q (want prod or preview)", env)
	}
}

// DefaultPath returns ~/.trakrf/config.yaml, honoring TRAKRF_CONFIG_HOME as an
// override for the parent directory (used in tests and for non-standard homes).
func DefaultPath() (string, error) {
	if home := os.Getenv("TRAKRF_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".trakrf", "config.yaml"), nil
}

// Load reads the config at path. A missing file yields an empty Config (not an
// error): a fresh install simply has nothing configured yet.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Profiles: map[string]*Profile{}}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*Profile{}
	}
	return &cfg, nil
}

// Save writes the config to path, creating the parent directory if needed. The
// file holds client secrets and tokens, so it is written 0600 in a 0700 dir.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}
	return nil
}

// ResolveInput carries the flag and environment-variable inputs that override
// the stored configuration when selecting the active profile.
type ResolveInput struct {
	Profile   string // --profile flag
	Env       string // --env flag (overrides the profile's env)
	OrgEnv    string // TRAKRF_ORG (selects a profile when --profile is absent)
	APIKeyEnv string // TRAKRF_API_KEY as "client_id:client_secret"
}

// Resolve selects the effective profile by precedence and returns a copy with
// any ephemeral overrides applied (it never mutates the stored config).
//
// Profile name:  --profile  >  TRAKRF_ORG  >  current_profile  >  "default".
// Credentials:   TRAKRF_API_KEY overrides the stored pair for this run.
// Environment:   --env overrides the profile's stored env.
func (c *Config) Resolve(in ResolveInput) (string, *Profile, error) {
	name := firstNonEmpty(in.Profile, in.OrgEnv, c.CurrentProfile, "default")

	var resolved Profile
	if stored, ok := c.Profiles[name]; ok && stored != nil {
		resolved = *stored
	} else if in.APIKeyEnv == "" {
		// No stored profile and no ephemeral credentials to stand in for one.
		return "", nil, fmt.Errorf("profile %q not found (run `trakrf auth login` or set TRAKRF_API_KEY)", name)
	}

	if in.APIKeyEnv != "" {
		id, secret, err := splitAPIKey(in.APIKeyEnv)
		if err != nil {
			return "", nil, err
		}
		resolved.ClientID = id
		resolved.ClientSecret = secret
		resolved.Token = nil // env credentials don't reuse the stored token
	}

	if in.Env != "" {
		resolved.Env = in.Env
	}

	return name, &resolved, nil
}

func splitAPIKey(v string) (id, secret string, err error) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("TRAKRF_API_KEY must be \"client_id:client_secret\"")
	}
	return parts[0], parts[1], nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
