package update

// When the version an update would roll back to is gone, the guard falls back to
// any other installed version rather than leave the failing build live with no
// guard. Only when there is nothing at all does it record "unguarded".

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// installedAged installs versions with mtimes one hour apart, newest first.
func installedAged(t *testing.T, active string, others ...string) Layout {
	t.Helper()
	l := installed(t, active, others...)
	for i, v := range append([]string{active}, others...) {
		at := time.Now().Add(-time.Duration(i) * time.Hour)
		require.NoError(t, os.Chtimes(l.VersionPath(v), at, at))
	}
	return l
}

func armedForRollback(t *testing.T, l Layout, target, from string) {
	t.Helper()
	require.NoError(t, WritePending(l, Pending{Target: target, From: from}))
	require.NoError(t, os.WriteFile(l.StateDir()+"/ticks", []byte("3\n"), 0o644))
}

func TestGuard_MissingFromFallsBackToTheNewestOtherVersion(t *testing.T) {
	l := installedAged(t, "v3", "v2", "v1")
	armedForRollback(t, l, "v3", "v0")

	_, code, calls := runGuard(t, l, "tick")
	require.Equal(t, 0, code)
	assert.Equal(t, "v2", activeOf(t, l))
	assert.Equal(t, "restart mqtt-rpcd\n", calls)
	assert.NoFileExists(t, l.VersionPath("v3"), "the build that failed probation is removed")
	assert.FileExists(t, l.VersionPath("v1"))
	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	last, _ := ReadLast(l)
	assert.Equal(t, readerrpc.UpdateStateRolledBack, last.State)
	assert.Equal(t, "v3", last.Target)
	assert.Equal(t, "no_commit_fallback", last.Error)
}

func TestGuard_CrashLoopWithMissingFromFallsBackToo(t *testing.T) {
	l := installedAged(t, "v3", "v2")
	require.NoError(t, WritePending(l, Pending{Target: "v3", From: "v0"}))
	require.NoError(t, os.WriteFile(l.StateDir()+"/starts", []byte("5\n"), 0o644))

	_, code, calls := runGuard(t, l, "start")
	require.Equal(t, 0, code)
	assert.Equal(t, "v2", activeOf(t, l))
	assert.Empty(t, calls, "start runs before systemd execs the link: no restart")
	last, _ := ReadLast(l)
	assert.Equal(t, "crash_loop_fallback", last.Error)
}
