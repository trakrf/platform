package update

// `mqtt-rpcd-guard place <version>` is install.sh's binary-and-link step, run
// on the reader over SSH. It lives in the guard so these tests can run it: an
// SSH drop at any instant must leave a runnable /opt/trakrf/mqtt-rpcd, and no
// guard may act on the install while it happens.

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upload stages versions/.mqtt-rpcd-<v>.upload as install.sh's scp does.
func upload(t *testing.T, l Layout, v string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(l.VersionsDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(l.VersionsDir(), ".mqtt-rpcd-"+v+".upload"), []byte("binary "+v), 0o644))
}

// snapshotStub is a systemctl stub that records, with each call, whether
// pending and the new binary existed at that moment.
func snapshotStub(t *testing.T, l Layout, v string) []string {
	t.Helper()
	bin := l.Dir + "-snap"
	require.NoError(t, os.MkdirAll(bin, 0o755))
	stub := filepath.Join(bin, "systemctl")
	script := "#!/bin/sh\n" +
		"p=no; [ -f '" + l.Pending() + "' ] && p=yes\n" +
		"b=no; [ -f '" + l.VersionPath(v) + "' ] && b=yes\n" +
		"echo \"$* pending=$p binary=$b\" >> '" + filepath.Join(bin, "calls") + "'\n"
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755))
	return []string{"GUARD_SYSTEMCTL=" + stub}
}

func snapshotCalls(t *testing.T, l Layout) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(l.Dir+"-snap", "calls"))
	return string(b)
}

func inode(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Sys().(*syscall.Stat_t).Ino
}

func TestGuardPlace_MigratesAPlainBinaryWithNoWindow(t *testing.T) {
	l := Layout{Dir: t.TempDir()}
	require.NoError(t, os.WriteFile(l.Link(), []byte("plain"), 0o755))
	plain := inode(t, l.Link())
	upload(t, l, "v2")

	out, code, _ := runGuard(t, l, "place", "v2")
	require.Equal(t, 0, code, out)

	assert.Equal(t, "v2", activeOf(t, l))
	// A hardlink, not a move: the plain file stayed at the link path until the
	// symlink was renamed over it.
	assert.Equal(t, plain, inode(t, l.VersionPath("legacy")))
	got, _ := os.ReadFile(l.Link())
	assert.Equal(t, "binary v2", string(got))
	fi, _ := os.Stat(l.VersionPath("v2"))
	assert.NotZero(t, fi.Mode()&0o111)
	assert.NoFileExists(t, filepath.Join(l.VersionsDir(), ".mqtt-rpcd-v2.upload"))
}

// An SSH drop after the hardlink, before the swap: the plain binary is still in
// place, and running install again converges.
func TestGuardPlace_ReRunAfterAnInterruptedMigrationConverges(t *testing.T) {
	l := Layout{Dir: t.TempDir()}
	require.NoError(t, os.WriteFile(l.Link(), []byte("plain"), 0o755))
	require.NoError(t, os.MkdirAll(l.VersionsDir(), 0o755))
	require.NoError(t, os.Link(l.Link(), l.VersionPath("legacy")))
	upload(t, l, "v2")

	out, code, _ := runGuard(t, l, "place", "v2")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "v2", activeOf(t, l))
	legacy, _ := os.ReadFile(l.VersionPath("legacy"))
	assert.Equal(t, "plain", string(legacy))
}

func TestGuardPlace_StopsTheTimerAndDisarmsBeforeTheBinaryMovesIn(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	for _, f := range []string{"starts", "ticks", "rollback.claim", "pending.commit"} {
		require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), f), []byte("1\n"), 0o644))
	}
	// Reinstalling the very version on probation: a guard tick after the binary
	// moved in would roll back and delete the just-installed v2.
	require.NoError(t, os.Remove(l.VersionPath("v2")))
	upload(t, l, "v2")

	out, code, _ := runGuardEnv(t, l, snapshotStub(t, l, "v2"), "place", "v2")
	require.Equal(t, 0, code, out)

	assert.Equal(t, "stop mqtt-rpcd-guard.timer pending=yes binary=no\n", snapshotCalls(t, l),
		"the timer stops first, while nothing has changed yet")
	for _, f := range []string{"pending", "starts", "ticks", "rollback.claim", "pending.commit"} {
		assert.NoFileExists(t, filepath.Join(l.StateDir(), f))
	}
	assert.Equal(t, "v2", activeOf(t, l))
	got, _ := os.ReadFile(l.Link())
	assert.Equal(t, "binary v2", string(got))
	assert.FileExists(t, l.VersionPath("v1"))
}

func TestGuardPlace_RefusesWithoutAnUploadOrAValidVersion(t *testing.T) {
	l := installed(t, "v1")
	_, code, calls := runGuard(t, l, "place", "v2")
	assert.NotEqual(t, 0, code)
	assert.Empty(t, calls, "nothing stopped for an install that cannot happen")
	assert.Equal(t, "v1", activeOf(t, l))

	_, code, _ = runGuard(t, l, "place", "../x")
	assert.NotEqual(t, 0, code)
	assert.Equal(t, "v1", activeOf(t, l))
}
