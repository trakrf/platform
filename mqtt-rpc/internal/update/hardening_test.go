package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// A timer unit that exists but is not enabled never ticks, so an update would
// run with no no-commit rollback. systemd records "enabled" as the wants link.
func TestPreflight_RefusesATimerThatIsNotEnabled(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.Remove(guardTimerWants(f.u.GuardTimerUnit)))
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	refused(t, err)
	assert.Contains(t, err.Error(), "not enabled")
	assertStillOnV1(t, f.l)
}

// The guard leaves a never-committed build running when it has nothing to roll
// back to, and does not restart it. The running daemon must then report that
// outcome, not "pending" for as long as it lives.
func TestInfo_ReportsUnguardedWhileTheTargetKeepsRunning(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"})) // v0 is not installed
	require.NoError(t, f.u.CheckPending())
	require.Equal(t, readerrpc.UpdateStatePending, f.u.Info().Update.State)

	require.NoError(t, os.WriteFile(filepath.Join(f.l.StateDir(), "ticks"), []byte("3\n"), 0o644))
	_, code, calls := runGuard(t, f.l, "tick")
	require.Equal(t, 0, code)
	require.Empty(t, calls, "the guard must not restart a target it cannot replace")

	info := f.u.Info()
	require.NotNil(t, info.Update)
	assert.Equal(t, readerrpc.UpdateStateUnguarded, info.Update.State)
	assert.Equal(t, "v1", info.Update.Target)
	assert.Equal(t, "no_commit_no_from", info.Update.Error)
}

// While pending is armed, or the guard holds it mid-rollback, an earlier
// recorded outcome for the same version is history: the update is still pending.
func TestInfo_StaysPendingUntilTheGuardSettlesIt(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, writeLast(f.l, readerrpc.UpdateStateUnguarded, "v1", "no_commit_no_from"))
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	assert.Equal(t, readerrpc.UpdateStatePending, f.u.Info().Update.State)

	require.NoError(t, os.Rename(f.l.Pending(), filepath.Join(f.l.StateDir(), "rollback.claim")))
	assert.Equal(t, readerrpc.UpdateStatePending, f.u.Info().Update.State)
}

// A rollback records its outcome and drops its claim a moment before it
// restarts this process. In that moment this build is still the one running,
// so it must not report itself rolled back.
func TestInfo_StaysPendingThroughARollbackUntilTheRestart(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	require.NoError(t, os.Remove(f.l.Pending()))
	require.NoError(t, writeLast(f.l, readerrpc.UpdateStateRolledBack, "v1", "no_commit"))
	assert.Equal(t, readerrpc.UpdateStatePending, f.u.Info().Update.State)
}

func TestGuardTimerWants_IsTheTimersTargetWantsLink(t *testing.T) {
	assert.Equal(t, "/etc/systemd/system/timers.target.wants/mqtt-rpcd-guard.timer",
		guardTimerWants(DefaultGuardTimerUnit))
}

func ageFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("partial"), 0o644))
	old := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, old, old))
}

// A kill or power cut mid-download leaves the hidden temp file behind, and Prune
// skips hidden files, so nothing else would ever remove it.
func TestCleanStale_RemovesOnlyAbandonedTempFiles(t *testing.T) {
	l := installed(t, "v1")
	dir := l.VersionsDir()
	oldDownload := filepath.Join(dir, ".mqtt-rpcd-v2.download")
	oldUpload := filepath.Join(dir, ".mqtt-rpcd-v4.upload")
	live := filepath.Join(dir, ".mqtt-rpcd-v3.download")
	other := filepath.Join(dir, ".something-else")
	ageFile(t, oldDownload, time.Hour)
	ageFile(t, oldUpload, time.Hour)
	ageFile(t, live, time.Minute)
	ageFile(t, other, time.Hour)

	require.NoError(t, CleanStale(l, staleDownloadAge))

	assert.NoFileExists(t, oldDownload)
	assert.NoFileExists(t, oldUpload)
	assert.FileExists(t, live, "a download still in progress is never removed")
	assert.FileExists(t, other, "only the updater's own temp files are touched")
	assert.FileExists(t, l.VersionPath("v1"))
}

func TestCheckPending_CleansAbandonedDownloads(t *testing.T) {
	f := newFixture(t)
	stale := filepath.Join(f.l.VersionsDir(), ".mqtt-rpcd-v9.download")
	ageFile(t, stale, time.Hour)
	require.NoError(t, f.u.CheckPending())
	assert.NoFileExists(t, stale)
}

// Two replies racing through answered() both see probation, then both call
// Commit: the loser must learn "already committed", not a failure.
func TestCommit_SecondCommitIsAlreadyCommitted(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, WritePending(f.l, Pending{Target: "v1", From: "v0"}))
	require.NoError(t, f.u.CheckPending())
	require.NoError(t, f.u.Commit())
	assert.ErrorIs(t, f.u.Commit(), ErrAlreadyCommitted)
	assert.Equal(t, readerrpc.UpdateStateCommitted, f.u.Info().Update.State)
}
