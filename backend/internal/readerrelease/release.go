// Package readerrelease resolves an mqtt-rpcd release version to its signed
// manifest — the exact Daemon.Update params — published by
// .github/workflows/mqtt-rpcd-release.yml at
//
//	<base>/mqtt-rpcd-<version>/manifest.json
//
// The cloud only relays the manifest. It verifies nothing: the reader checks the
// signature against keys compiled into its own daemon and refuses anything else.
package readerrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// DefaultBaseURL is where the release workflow publishes. Override with
// MQTT_RPCD_RELEASE_BASE_URL.
const DefaultBaseURL = "https://github.com/trakrf/platform/releases/download"

var (
	// ErrNotFound: no release is published under that version.
	ErrNotFound = errors.New("readerrelease: release not found")
	// ErrInvalidVersion: the version is not a well-formed daemon version.
	ErrInvalidVersion = errors.New("readerrelease: invalid version")

	// Same alphabet the daemon accepts (mqtt-rpc/internal/update.ValidVersion).
	versionRE = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)
)

// ValidVersion reports whether v is a well-formed daemon version.
func ValidVersion(v string) bool { return versionRE.MatchString(v) && v != "." && v != ".." }

// Source fetches release manifests.
type Source struct {
	BaseURL string
	HTTP    *http.Client
}

// NewSource returns a Source for baseURL, or DefaultBaseURL when empty.
func NewSource(baseURL string) *Source {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Source{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Fetch returns the signed manifest for version.
func (s *Source) Fetch(ctx context.Context, version string) (readerrpc.DaemonUpdateParams, error) {
	if !ValidVersion(version) {
		return readerrpc.DaemonUpdateParams{}, ErrInvalidVersion
	}
	url := s.BaseURL + "/mqtt-rpcd-" + version + "/manifest.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return readerrpc.DaemonUpdateParams{}, err
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return readerrpc.DaemonUpdateParams{}, fmt.Errorf("readerrelease: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return readerrpc.DaemonUpdateParams{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return readerrpc.DaemonUpdateParams{}, fmt.Errorf("readerrelease: fetch %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return readerrpc.DaemonUpdateParams{}, err
	}
	var m readerrpc.DaemonUpdateParams
	if err := json.Unmarshal(raw, &m); err != nil {
		return readerrpc.DaemonUpdateParams{}, fmt.Errorf("readerrelease: parse manifest: %w", err)
	}
	if m.Version != version {
		return readerrpc.DaemonUpdateParams{}, fmt.Errorf("readerrelease: manifest is for %q, not %q", m.Version, version)
	}
	if m.URL == "" || m.SHA256 == "" || m.Signature == "" {
		return readerrpc.DaemonUpdateParams{}, errors.New("readerrelease: manifest is missing url, sha256 or signature")
	}
	return m, nil
}
