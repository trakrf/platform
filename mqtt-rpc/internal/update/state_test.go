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

// installed lays out a reader install in a temp dir: the given versions exist
// under versions/ and the active symlink points at active.
func installed(t *testing.T, active string, others ...string) Layout {
	t.Helper()
	l := Layout{Dir: t.TempDir()}
	require.NoError(t, os.MkdirAll(l.VersionsDir(), 0o755))
	for _, v := range append([]string{active}, others...) {
		require.NoError(t, os.WriteFile(l.VersionPath(v), []byte("binary "+v), 0o755))
	}
	require.NoError(t, SwapLink(l, active))
	return l
}

func TestSwapLinkRepointsTheActiveVersionWithARelativeLink(t *testing.T) {
	l := installed(t, "v1", "v2")
	require.NoError(t, SwapLink(l, "v2"))

	target, err := os.Readlink(l.Link())
	require.NoError(t, err)
	assert.Equal(t, "versions/mqtt-rpcd-v2", target, "relative, so the install dir can move")
	got, err := os.ReadFile(l.Link())
	require.NoError(t, err)
	assert.Equal(t, "binary v2", string(got))

	v, err := ActiveVersion(l)
	require.NoError(t, err)
	assert.Equal(t, "v2", v)
}

func TestActiveVersionRejectsAPlainFile(t *testing.T) {
	l := Layout{Dir: t.TempDir()}
	require.NoError(t, os.WriteFile(l.Link(), []byte("legacy"), 0o755))
	_, err := ActiveVersion(l)
	assert.Error(t, err, "a pre-symlink install has no versions to roll back to")
}

func TestPendingRoundTrip(t *testing.T) {
	l := installed(t, "v1")
	_, ok, err := ReadPending(l)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1", At: "2026-11-08T00:00:00Z"}))
	p, ok, err := ReadPending(l)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Pending{Target: "v2", From: "v1", At: "2026-11-08T00:00:00Z"}, p)
}

func TestCheckPending_TargetRunningIsProbation(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	on, err := CheckPending(l, "v2")
	require.NoError(t, err)
	assert.True(t, on)
}

func TestCheckPending_StaleTargetIsCleared(t *testing.T) {
	// The updater wrote pending and died before swapping the link: the old
	// version restarted. That is not a probation, and the guard must not be left
	// armed to "roll back" a binary that was never replaced.
	l := installed(t, "v1", "v2")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), "starts"), []byte("2\n"), 0o644))

	on, err := CheckPending(l, "v1")
	require.NoError(t, err)
	assert.False(t, on)
	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	assert.NoFileExists(t, filepath.Join(l.StateDir(), "starts"))
	last, ok := ReadLast(l)
	require.True(t, ok)
	assert.Equal(t, readerrpc.UpdateStateFailed, last.State)
	assert.Equal(t, "v2", last.Target)
	assert.Equal(t, "swap_incomplete", last.Error)
}

func TestCommitClearsPendingAndRecordsCommitted(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir(), "ticks"), []byte("1\n"), 0o644))

	require.NoError(t, Commit(l, "v2"))
	_, ok, _ := ReadPending(l)
	assert.False(t, ok)
	assert.NoFileExists(t, filepath.Join(l.StateDir(), "ticks"))
	last, ok := ReadLast(l)
	require.True(t, ok)
	assert.Equal(t, readerrpc.UpdateStateCommitted, last.State)
	assert.Equal(t, "v2", last.Target)
	assert.NotEmpty(t, last.At)
}

func TestCommitWithoutPendingFails(t *testing.T) {
	l := installed(t, "v2")
	assert.ErrorIs(t, Commit(l, "v2"), ErrNotPending)
}

func TestPruneKeepsNewestAndProtected(t *testing.T) {
	l := installed(t, "v1", "v2", "v3", "v4", "v5")
	base := time.Now().Add(-time.Hour)
	for i, v := range []string{"v1", "v2", "v3", "v4", "v5"} {
		ts := base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(l.VersionPath(v), ts, ts))
	}
	require.NoError(t, Prune(l, 3, "v1"))

	for _, v := range []string{"v1", "v3", "v4", "v5"} {
		assert.FileExists(t, l.VersionPath(v))
	}
	assert.NoFileExists(t, l.VersionPath("v2"))
}
