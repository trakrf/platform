package update

// These tests run the real deploy/edge/mqtt-rpc/mqtt-rpcd-guard shell script
// against a temp install, so the Go writer and the sh reader of the state files
// are proven compatible — the guard is the half of the update that never gets
// updated over the air, so a format drift between them would be permanent.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const guardScript = "../../../deploy/edge/mqtt-rpc/mqtt-rpcd-guard"

// runGuard runs the guard with a stub systemctl that records its arguments.
func runGuard(t *testing.T, l Layout, args ...string) (out string, exitCode int, systemctlCalls string) {
	t.Helper()
	script, err := filepath.Abs(guardScript)
	require.NoError(t, err)
	calls := filepath.Join(l.Dir, "systemctl.calls")
	stub := filepath.Join(l.Dir, "systemctl")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\necho \"$*\" >> "+calls+"\n"), 0o755))

	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "GUARD_DIR="+l.Dir, "GUARD_SYSTEMCTL="+stub, "GUARD_SYNC=true")
	b, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else {
		require.NoError(t, err)
	}
	c, _ := os.ReadFile(calls)
	return string(b), code, string(c)
}

func activeOf(t *testing.T, l Layout) string {
	t.Helper()
	v, err := ActiveVersion(l)
	require.NoError(t, err)
	return v
}

func TestGuard_NoPendingIsANoOp(t *testing.T) {
	l := installed(t, "v1")
	for _, mode := range []string{"start", "tick"} {
		_, code, calls := runGuard(t, l, mode)
		assert.Equal(t, 0, code)
		assert.Empty(t, calls)
	}
	assert.Equal(t, "v1", activeOf(t, l))
	assert.NoFileExists(t, filepath.Join(l.StateDir(), "starts"))
}

func TestGuard_StartCountsAndRollsBackOnCrashLoop(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))

	for i := 0; i < 5; i++ {
		_, code, _ := runGuard(t, l, "start")
		require.Equal(t, 0, code)
		assert.Equal(t, "v2", activeOf(t, l), "start %d is under the threshold", i+1)
	}
	_, code, calls := runGuard(t, l, "start")
	require.Equal(t, 0, code, "ExecStartPre must never fail the unit")
	assert.Equal(t, "v1", activeOf(t, l))
	assert.Empty(t, calls, "in start mode systemd is already starting the restored binary")

	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	last, ok := ReadLast(l)
	require.True(t, ok)
	assert.Equal(t, readerrpc.UpdateStateRolledBack, last.State)
	assert.Equal(t, "v2", last.Target)
	assert.Equal(t, "crash_loop", last.Error)
	assert.NoFileExists(t, filepath.Join(l.StateDir(), "starts"))
}

func TestGuard_TickRollsBackWhenNoCommitAndRestarts(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))

	for i := 0; i < 3; i++ {
		_, _, calls := runGuard(t, l, "tick")
		assert.Empty(t, calls)
		assert.Equal(t, "v2", activeOf(t, l))
	}
	_, code, calls := runGuard(t, l, "tick")
	require.Equal(t, 0, code)
	assert.Equal(t, "v1", activeOf(t, l))
	assert.Equal(t, "restart mqtt-rpcd\n", calls)
	last, _ := ReadLast(l)
	assert.Equal(t, "no_commit", last.Error)
}

func TestGuard_MissingFromClearsWithoutSwap(t *testing.T) {
	l := installed(t, "v2")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), "ticks"), []byte("3\n"), 0o644))

	_, code, calls := runGuard(t, l, "tick")
	require.Equal(t, 0, code)
	assert.Equal(t, "v2", activeOf(t, l), "nothing to roll back to: the link must not point at nothing")
	assert.Empty(t, calls)
	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	last, _ := ReadLast(l)
	assert.Equal(t, readerrpc.UpdateStateFailed, last.State)
	assert.Equal(t, "no_commit_no_from", last.Error)
}

func TestGuard_GarbageCounterIsTreatedAsZero(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), "starts"), []byte("x9\n"), 0o644))
	_, code, _ := runGuard(t, l, "start")
	require.Equal(t, 0, code)
	b, _ := os.ReadFile(filepath.Join(l.StateDir(), "starts"))
	assert.Equal(t, "1", strings.TrimSpace(string(b)))
}

func TestGuard_ManualRollbackToANamedVersion(t *testing.T) {
	l := installed(t, "v3", "v1", "v2")
	out, code, calls := runGuard(t, l, "rollback", "v1")
	require.Equal(t, 0, code, out)
	assert.Equal(t, "v1", activeOf(t, l))
	assert.Equal(t, "restart mqtt-rpcd\n", calls)
	last, _ := ReadLast(l)
	assert.Equal(t, readerrpc.UpdateStateRolledBack, last.State)
	assert.Equal(t, "v3", last.Target, "target is the version rolled away from")
	assert.Equal(t, "manual", last.Error)
}

func TestGuard_ManualRollbackClearsAPendingUpdate(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	_, code, _ := runGuard(t, l, "rollback", "v1")
	require.Equal(t, 0, code)
	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	assert.ErrorIs(t, Commit(l, "v2"), ErrNotPending, "the guard's claim beats a late commit")
}

func TestGuard_ManualRollbackToAnUnknownVersionChangesNothing(t *testing.T) {
	l := installed(t, "v2", "v1")
	for _, v := range []string{"v9", "../../etc/passwd", ""} {
		_, code, calls := runGuard(t, l, "rollback", v)
		assert.NotEqual(t, 0, code, "version %q", v)
		assert.Empty(t, calls)
		assert.Equal(t, "v2", activeOf(t, l))
	}
}

func TestGuard_ListShowsVersionsAndMarksActive(t *testing.T) {
	l := installed(t, "v2", "v1")
	out, code, _ := runGuard(t, l, "list")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "* v2")
	assert.Contains(t, out, "  v1")
}

func TestGuard_UnknownModeIsUsageError(t *testing.T) {
	l := installed(t, "v1")
	_, code, _ := runGuard(t, l, "bogus")
	assert.Equal(t, 2, code)
}
