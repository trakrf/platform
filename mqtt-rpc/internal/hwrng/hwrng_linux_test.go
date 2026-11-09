//go:build linux

package hwrng

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A FIFO with no writer blocks in open, and then in read until one writes: a
// hardware RNG that has stopped answering, at either step.
func stalledDevice(t *testing.T, sys *fakeSystem) {
	t.Helper()
	require.NoError(t, os.Remove(sys.cfg.HWRNG))
	require.NoError(t, syscall.Mkfifo(sys.cfg.HWRNG, 0o600))
}

func TestSeedGivesUpOnASourceThatNeverAnswers(t *testing.T) {
	sys := newFakeSystem(t, "rng-caam", []byte{})
	stalledDevice(t, &sys)
	pool := &recordingPool{}
	sys.cfg.Pool = pool
	sys.cfg.Timeout = 50 * time.Millisecond

	start := time.Now()
	res := Seed(sys.cfg)

	require.ErrorIs(t, res.Err, ErrTimeout)
	assert.Less(t, time.Since(start), 2*time.Second, "the daemon's start must not wait on the RNG")
	assert.Zero(t, pool.Calls())

	// Let the stranded open return, so the test leaves nothing behind.
	w, err := os.OpenFile(sys.cfg.HWRNG, os.O_WRONLY, 0)
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

// Seed reported a timeout, so the late answer must not be credited after all:
// the log line would be wrong, and nobody asked for it any more.
func TestSeedNeverCreditsAnAnswerThatArrivesAfterItGaveUp(t *testing.T) {
	sys := newFakeSystem(t, "rng-caam", []byte{})
	stalledDevice(t, &sys)
	pool := &recordingPool{}
	sys.cfg.Pool = pool
	sys.cfg.Timeout = 50 * time.Millisecond

	res := Seed(sys.cfg)
	require.ErrorIs(t, res.Err, ErrTimeout)

	w, err := os.OpenFile(sys.cfg.HWRNG, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = w.Write(randomish(DefaultBytes))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, pool.Calls(), "a timed-out Seed credited the late answer")
}
