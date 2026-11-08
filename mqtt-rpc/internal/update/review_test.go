package update

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// SwapLink can fail after its rename landed (the directory fsync). The link
// then points at the new build: disarming the guard there would leave an unvetted
// build that starts with no rollback at the next reboot.
func TestUpdate_SwapErrorAfterTheLinkMovedKeepsTheGuardArmed(t *testing.T) {
	f := newFixture(t)
	f.u.swap = func(l Layout, v string) error {
		require.NoError(t, SwapLink(l, v))
		return errors.New("fsync versions dir: input/output error")
	}
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	require.NoError(t, err)

	select {
	case <-f.restarts:
	case <-time.After(5 * time.Second):
		t.Fatal("the link moved, so the daemon must restart onto it under probation")
	}
	v, _ := ActiveVersion(f.l)
	assert.Equal(t, "v2", v)
	_, ok, _ := ReadPending(f.l)
	assert.True(t, ok, "the guard stays armed")
}

func TestUpdate_SwapErrorBeforeTheLinkMovedDisarmsAndReports(t *testing.T) {
	f := newFixture(t)
	f.u.swap = func(Layout, string) error { return errors.New("symlink: no space left on device") }
	_, err := f.u.Start(f.params("v2", "/mqtt-rpcd"))
	require.NoError(t, err)

	got := f.waitState(t, readerrpc.UpdateStateFailed)
	assert.Contains(t, got.Error, "no space")
	assertStillOnV1(t, f.l)
	assert.Empty(t, f.restarts)
}

// A build whose compiled-in version differs from the label it was installed
// under (a hand-signed release, or an unset -X main.version) must not clear
// pending as "swap incomplete": the link already points at it, so it is the
// build on probation and only the guard may decide.
func TestCheckPending_LinkAtTargetKeepsPendingForTheGuard(t *testing.T) {
	l := installed(t, "v2", "v1")
	require.NoError(t, WritePending(l, Pending{Target: "v2", From: "v1"}))

	on, err := CheckPending(l, "dev")
	require.NoError(t, err)
	assert.False(t, on, "it cannot commit: it is not the version it was installed as")
	_, ok, _ := ReadPending(l)
	assert.True(t, ok, "the guard keeps its claim on the mislabelled build")
	_, err = os.Stat(l.VersionPath("v2"))
	assert.NoError(t, err)
}
