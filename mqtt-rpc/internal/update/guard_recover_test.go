package update

// `mqtt-rpcd-guard recover` is the on-site operator's one command: fetch the
// newest daemon release from GitHub, check it against its manifest, switch to
// it, then reboot cleanly. These tests serve a fake release host over file://
// URLs, so they run the real script with the real curl and sha256sum.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// releaseHost is a fake GitHub: a releases API document plus, per release, the
// assets the workflow publishes.
type releaseHost struct {
	t    *testing.T
	root string
	tags []string // newest first, as the API lists them
}

func newReleaseHost(t *testing.T) *releaseHost {
	return &releaseHost{t: t, root: t.TempDir()}
}

// publish adds a daemon release whose manifest names manifestVersion and
// manifestSHA (pass "" to use the binary's real hash).
func (h *releaseHost) publish(tag, binary, manifestVersion, manifestSHA string) {
	h.t.Helper()
	dir := filepath.Join(h.root, "download", tag)
	require.NoError(h.t, os.MkdirAll(dir, 0o755))
	require.NoError(h.t, os.WriteFile(filepath.Join(dir, "mqtt-rpcd-linux-armv7"), []byte(binary), 0o644))
	if manifestSHA == "" {
		sum := sha256.Sum256([]byte(binary))
		manifestSHA = hex.EncodeToString(sum[:])
	}
	manifest := fmt.Sprintf("{\n  \"version\": %q,\n  \"url\": \"https://example.invalid/x\",\n  \"sha256\": %q,\n  \"signature\": \"c2ln\"\n}\n", manifestVersion, manifestSHA)
	require.NoError(h.t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))
	h.tags = append(h.tags, tag)
}

// env points the guard at this host. The API document lists the tags in the
// order given, with the shape (and unrelated fields) GitHub returns.
func (h *releaseHost) env() []string {
	h.t.Helper()
	doc := "[\n"
	for i, tag := range h.tags {
		if i > 0 {
			doc += ",\n"
		}
		doc += fmt.Sprintf("  {\n    \"url\": \"https://api.github.com/x/%d\",\n    \"tag_name\": %q,\n    \"prerelease\": true\n  }", i, tag)
	}
	doc += "\n]\n"
	api := filepath.Join(h.root, "releases.json")
	require.NoError(h.t, os.WriteFile(api, []byte(doc), 0o644))
	return []string{
		"GUARD_RELEASES_API=file://" + api,
		"GUARD_RELEASE_BASE=file://" + filepath.Join(h.root, "download"),
		"GUARD_PROTO==file",
	}
}

func binaryOf(t *testing.T, l Layout, v string) string {
	t.Helper()
	b, err := os.ReadFile(l.VersionPath(v))
	require.NoError(t, err)
	return string(b)
}

func assertNoDownloadLeftovers(t *testing.T, l Layout) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(l.VersionsDir(), ".*"))
	assert.Empty(t, left, "a failed or finished download leaves nothing hidden behind")
}

func TestRecover_InstallsTheNewestDaemonReleaseAndReboots(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	h := newReleaseHost(t)
	h.publish("v1.6.0", "not ours", "v1.6.0", "") // a platform release, newer, not a daemon
	h.publish("mqtt-rpcd-v3", "binary v3", "v3", "")
	h.publish("mqtt-rpcd-v2", "binary v2", "v2", "")

	out, code, calls := runGuardEnv(t, l, h.env(), "recover")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "v3", activeOf(t, l))
	assert.Equal(t, "binary v3", binaryOf(t, l, "v3"))
	assert.Equal(t, "reboot\n", calls, "ends with a clean reboot, which also clears a wedged vendor app")
	_, ok, _ := ReadPending(l)
	assert.False(t, ok, "recovery supersedes any update on probation")
	last, ok := ReadLast(l)
	require.True(t, ok)
	assert.Equal(t, readerrpc.UpdateStateRolledBack, last.State)
	assert.Equal(t, "v2", last.Target, "target is the version moved away from, as for a manual rollback")
	assert.Equal(t, "recover", last.Error)
	assert.FileExists(t, l.VersionPath("v2"), "the old version stays, so it can be rolled back to")
	assert.Contains(t, out, "v3")
	assertNoDownloadLeftovers(t, l)
	assertRecoverLogSays(t, l, "Switched the daemon from v2 to v3")
}

// The CS463 journal does not survive a reboot, and recover always reboots, so
// what the operator saw is also kept in update/recover.log (the last run only).
func assertRecoverLogSays(t *testing.T, l Layout, want string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(l.StateDir(), "recover.log"))
	require.NoError(t, err)
	assert.Contains(t, string(b), want)
	assert.Contains(t, string(b), "Restarting the reader cleanly")
}

func TestRecover_KeepsOnlyTheLastRunInItsLog(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, os.MkdirAll(l.StateDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), "recover.log"), []byte("an older run\n"), 0o644))
	env := []string{"GUARD_RELEASES_API=file:///nonexistent/releases.json", "GUARD_PROTO==file"}
	_, _, _ = runGuardEnv(t, l, env, "recover")
	b, err := os.ReadFile(filepath.Join(l.StateDir(), "recover.log"))
	require.NoError(t, err)
	assert.NotContains(t, string(b), "an older run")
	assert.Contains(t, string(b), "Nothing on this reader was changed")
}

func TestRecover_AlreadyOnTheNewestReleaseOnlyReboots(t *testing.T) {
	l := installed(t, "v3", "v2")
	h := newReleaseHost(t)
	h.publish("mqtt-rpcd-v3", "binary v3", "v3", "")

	out, code, calls := runGuardEnv(t, l, h.env(), "recover")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "v3", activeOf(t, l))
	assert.Equal(t, "reboot\n", calls)
	_, ok := ReadLast(l)
	assert.False(t, ok, "nothing changed, so there is no outcome to report")
}

// A damaged copy of the active version (bad flash block, half-written file) is
// what a reinstall exists to fix: it is replaced, not trusted by name.
func TestRecover_ReplacesAnInstalledCopyThatDoesNotMatchItsManifest(t *testing.T) {
	l := installed(t, "v3", "v2")
	require.NoError(t, os.WriteFile(l.VersionPath("v3"), []byte("corrupt"), 0o755))
	h := newReleaseHost(t)
	h.publish("mqtt-rpcd-v3", "binary v3", "v3", "")

	out, code, calls := runGuardEnv(t, l, h.env(), "recover")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "v3", activeOf(t, l))
	assert.Equal(t, "binary v3", binaryOf(t, l, "v3"))
	assert.Equal(t, "reboot\n", calls)
	assertNoDownloadLeftovers(t, l)
}

// Anything that does not check out changes nothing on the reader, and the
// reboot still happens: it is the fix for a wedged vendor app either way.
func TestRecover_ABadDownloadChangesNothingButStillReboots(t *testing.T) {
	cases := map[string]func(h *releaseHost){
		"hash mismatch": func(h *releaseHost) {
			h.publish("mqtt-rpcd-v3", "binary v3", "v3", "0000000000000000000000000000000000000000000000000000000000000000")
		},
		"manifest names another version": func(h *releaseHost) {
			h.publish("mqtt-rpcd-v3", "binary v3", "v9", "")
		},
		"unusable tag": func(h *releaseHost) {
			h.tags = append(h.tags, "mqtt-rpcd-../../etc")
		},
		"no daemon release listed": func(h *releaseHost) {
			h.publish("v1.6.0", "not ours", "v1.6.0", "")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			l := installed(t, "v2", "v1")
			h := newReleaseHost(t)
			setup(h)

			out, code, calls := runGuardEnv(t, l, h.env(), "recover")
			assert.NotEqual(t, 0, code, out)
			assert.Equal(t, "v2", activeOf(t, l))
			assert.NoFileExists(t, l.VersionPath("v3"))
			assert.Equal(t, "reboot\n", calls)
			_, ok := ReadLast(l)
			assert.False(t, ok)
			assertNoDownloadLeftovers(t, l)
		})
	}
}

func TestRecover_NoInternetChangesNothingButStillReboots(t *testing.T) {
	l := installed(t, "v2", "v1")
	env := []string{
		"GUARD_RELEASES_API=file:///nonexistent/releases.json",
		"GUARD_RELEASE_BASE=file:///nonexistent/download",
		"GUARD_PROTO==file",
	}
	out, code, calls := runGuardEnv(t, l, env, "recover")
	assert.NotEqual(t, 0, code)
	assert.Equal(t, "v2", activeOf(t, l))
	assert.Equal(t, "reboot\n", calls)
	assert.Contains(t, out, "internet", "the operator is told what to check")
}

// `legacy` is the pre-update daemon kept by install.sh. It has no updater, so
// switching to it ends remote updates until someone reinstalls over SSH.
func TestGuard_RollbackToLegacyWarnsThatItEndsRemoteUpdates(t *testing.T) {
	l := installed(t, "v2", "legacy")
	out, code, _ := runGuard(t, l, "rollback", "legacy")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "legacy", activeOf(t, l))
	assert.Contains(t, out, "no remote update")
}
