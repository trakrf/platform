package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

type updaterFixture struct {
	u        *Updater
	l        Layout
	priv     ed25519.PrivateKey
	srv      *httptest.Server
	body     []byte
	restarts chan struct{}
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// newFixture: v1 is installed and running, the guard and its timer are in place,
// and the TLS server serves body at /mqtt-rpcd.
func newFixture(t *testing.T) *updaterFixture {
	t.Helper()
	f := &updaterFixture{body: []byte("the v2 binary"), restarts: make(chan struct{}, 4)}
	f.l = installed(t, "v1")
	require.NoError(t, os.WriteFile(f.l.Guard(), []byte("#!/bin/sh\n"), 0o755))
	timer := filepath.Join(f.l.Dir, "mqtt-rpcd-guard.timer")
	require.NoError(t, os.WriteFile(timer, []byte("[Timer]\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(guardTimerWants(timer)), 0o755))
	require.NoError(t, os.Symlink(timer, guardTimerWants(timer)))

	mux := http.NewServeMux()
	mux.HandleFunc("/mqtt-rpcd", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(f.body) })
	mux.HandleFunc("/redirect-to-http", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/mqtt-rpcd", http.StatusFound)
	})
	mux.HandleFunc("/missing", http.NotFound)
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)

	key, priv := testKey(t, "ci")
	f.priv = priv
	f.u = New(f.l, "v1", func() { f.restarts <- struct{}{} }, zerolog.Nop())
	f.u.Keys = []TrustedKey{key}
	f.u.HTTP = f.srv.Client()
	f.u.GuardTimerUnit = timer
	return f
}

func (f *updaterFixture) params(version, path string) readerrpc.DaemonUpdateParams {
	return readerrpc.DaemonUpdateParams{
		Version:   version,
		URL:       f.srv.URL + path,
		SHA256:    sha(f.body),
		Signature: Sign(f.priv, version, sha(f.body)),
	}
}

func (f *updaterFixture) waitState(t *testing.T, states ...string) readerrpc.UpdateInfo {
	t.Helper()
	var got readerrpc.UpdateInfo
	require.Eventually(t, func() bool {
		info := f.u.Info()
		if info.Update == nil {
			return false
		}
		got = *info.Update
		for _, s := range states {
			if got.State == s {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	return got
}

func assertStillOnV1(t *testing.T, l Layout) {
	t.Helper()
	v, err := ActiveVersion(l)
	require.NoError(t, err)
	assert.Equal(t, "v1", v)
	_, ok, _ := ReadPending(l)
	assert.False(t, ok, "a refused or failed update must not arm the guard")
}

func refused(t *testing.T, err error) {
	t.Helper()
	var re *readerrpc.UpdateRefusedError
	require.True(t, errors.As(err, &re), "want UpdateRefusedError, got %v", err)
}

func TestUpdate_HappyPathInstallsArmsGuardAndRestarts(t *testing.T) {
	f := newFixture(t)
	res, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	require.NoError(t, err)
	assert.Equal(t, readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: "v2"}, res)

	select {
	case <-f.restarts:
	case <-time.After(5 * time.Second):
		t.Fatal("restart was never requested")
	}
	v, err := ActiveVersion(f.l)
	require.NoError(t, err)
	assert.Equal(t, "v2", v)
	got, _ := os.ReadFile(f.l.Link())
	assert.Equal(t, f.body, got)
	assert.FileExists(t, f.l.VersionPath("v1"), "the rollback target is kept")
	p, ok, _ := ReadPending(f.l)
	require.True(t, ok)
	assert.Equal(t, "v2", p.Target)
	assert.Equal(t, "v1", p.From)
	info, err := os.Stat(f.l.VersionPath("v2"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "installed binary is executable")
	assert.Equal(t, readerrpc.UpdateStateInstalling, f.u.Info().Update.State)
}

func TestUpdate_HashMismatchReplacesNothing(t *testing.T) {
	f := newFixture(t)
	p := f.params("v2", "/mqtt-rpcd")
	f.body = []byte("tampered in transit")
	_, err := f.u.Start(p)
	require.NoError(t, err, "the hash can only be checked after download")

	got := f.waitState(t, readerrpc.UpdateStateFailed)
	assert.Contains(t, got.Error, "sha256")
	assertStillOnV1(t, f.l)
	assert.NoFileExists(t, f.l.VersionPath("v2"))
	entries, _ := os.ReadDir(f.l.VersionsDir())
	assert.Len(t, entries, 1, "no partial download left behind")
	assert.Empty(t, f.restarts)

	last, ok := ReadLast(f.l)
	require.True(t, ok, "a failed attempt survives a daemon restart")
	assert.Equal(t, readerrpc.UpdateStateFailed, last.State)
}

func TestUpdate_RefusedSynchronously(t *testing.T) {
	cases := map[string]func(f *updaterFixture) readerrpc.DaemonUpdateParams{
		"bad signature": func(f *updaterFixture) readerrpc.DaemonUpdateParams {
			p := f.params("v2", "/mqtt-rpcd")
			p.Signature = Sign(f.priv, "v3", p.SHA256)
			return p
		},
		"unsigned": func(f *updaterFixture) readerrpc.DaemonUpdateParams {
			p := f.params("v2", "/mqtt-rpcd")
			p.Signature = ""
			return p
		},
		"plain http": func(f *updaterFixture) readerrpc.DaemonUpdateParams {
			p := f.params("v2", "/mqtt-rpcd")
			p.URL = "http://example.test/mqtt-rpcd"
			return p
		},
		"already running": func(f *updaterFixture) readerrpc.DaemonUpdateParams { return f.params("v1", "/mqtt-rpcd") },
		"bad version":     func(f *updaterFixture) readerrpc.DaemonUpdateParams { return f.params("v2/../x", "/mqtt-rpcd") },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.u.Start(mk(f))
			refused(t, err)
			assertStillOnV1(t, f.l)
			assert.Nil(t, f.u.Info().Update)
		})
	}
}

func TestUpdate_RefusedWithoutTheRollbackGuard(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.Remove(f.l.Guard()))
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	refused(t, err)

	f = newFixture(t)
	require.NoError(t, os.Remove(f.u.GuardTimerUnit))
	_, err = f.u.Start(f.params("v2", "/mqtt-rpcd"))
	refused(t, err)
}

func TestUpdate_RefusedWhenTheLinkIsNotASymlink(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.Remove(f.l.Link()))
	require.NoError(t, os.WriteFile(f.l.Link(), []byte("binary"), 0o755))
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	refused(t, err)
}

func TestStart_RefusesWhileBusyOrPending(t *testing.T) {
	f := newFixture(t)
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	require.NoError(t, err)
	<-f.restarts
	_, err = f.u.Start(f.params("v3", "/mqtt-rpcd"))
	refused(t, err)

	g := newFixture(t)
	require.NoError(t, WritePending(g.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, g.u.CheckPending())
	_, err = g.u.Start(g.params("v2", "/mqtt-rpcd"))
	refused(t, err)
}

func TestDownload_RefusesHTTPRedirect(t *testing.T) {
	f := newFixture(t)
	_, err := f.u.Start(f.params("v2", "/redirect-to-http"))
	require.NoError(t, err)
	got := f.waitState(t, readerrpc.UpdateStateFailed)
	assert.Contains(t, got.Error, "https")
	assertStillOnV1(t, f.l)
}

func TestDownload_RefusesOversizeAndHTTPErrors(t *testing.T) {
	f := newFixture(t)
	f.u.MaxBytes = 4
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	require.NoError(t, err)
	f.waitState(t, readerrpc.UpdateStateFailed)
	assertStillOnV1(t, f.l)

	g := newFixture(t)
	_, err = g.u.Start(g.params("v2", "/missing"))
	require.NoError(t, err)
	got := g.waitState(t, readerrpc.UpdateStateFailed)
	assert.Contains(t, got.Error, "404")
	assertStillOnV1(t, g.l)
}

func TestProbationAndCommit(t *testing.T) {
	f := newFixture(t)
	assert.False(t, f.u.OnProbation())
	assert.ErrorIs(t, f.u.Commit(), ErrNotPending)

	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	assert.True(t, f.u.OnProbation())
	assert.Equal(t, readerrpc.UpdateStatePending, f.u.Info().Update.State)

	require.NoError(t, f.u.Commit())
	assert.False(t, f.u.OnProbation())
	info := f.u.Info()
	assert.Equal(t, "v1", info.Version)
	assert.Equal(t, readerrpc.UpdateStateCommitted, info.Update.State)
}

func TestCommitRefusedWhenUpdaterPreflightFails(t *testing.T) {
	// A build whose own updater cannot work must not commit: the guard then rolls
	// it back, instead of leaving a reader that can never be updated again.
	f := newFixture(t)
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	f.u.Keys = nil
	assert.Error(t, f.u.Commit())
	assert.True(t, f.u.OnProbation())
	_, ok, _ := ReadPending(f.l)
	assert.True(t, ok)
}

func TestCommit_LosesToGuardClaim(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	require.NoError(t, os.Rename(f.l.Pending(), filepath.Join(f.l.StateDir(), "rollback.claim")))
	assert.ErrorIs(t, f.u.Commit(), ErrNotPending)
}
