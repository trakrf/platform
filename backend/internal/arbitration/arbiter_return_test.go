package arbitration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	locC = 3
	spC  = 33
)

// moveAtoB assigns the asset to A, then hands it to B on a clear 6 dB lead
// held for the dwell. It returns the ms of the last read.
func moveAtoB(t *testing.T, a *Arbiter) int {
	t.Helper()
	step(t, a, rd(spA, locA, -60, 0))
	ms := 1000
	for ; ms <= 8000; ms += 1000 {
		step(t, a, rd(spB, locB, -50, ms))
	}
	require.Equal(t, locB, step(t, a, rd(spB, locB, -50, ms)).LocationID, "walked to B")
	return ms
}

func TestReturnToRoomJustLeftNeedsReturnDwell(t *testing.T) {
	a := New(testConfig())
	ms := moveAtoB(t, a)
	// Back at A with a clear lead: an ordinary challenger would take over after
	// Dwell (5 s). A return holds out for ReturnDwell (30 s).
	start := ms + 1000
	took := switchAfter(t, a, start)
	// A's lead starts once enough of B's strong reads age out (a few seconds);
	// an ordinary move would follow Dwell later (see the after-window test).
	assert.GreaterOrEqual(t, took, 30_000, "return held out for ReturnDwell")
	assert.Less(t, took, 45_000, "return still taken once its lead holds")
}

func TestReturnNeverTakesOverOnAGap(t *testing.T) {
	a := New(testConfig())
	ms := moveAtoB(t, a)
	// B's gateway skips the tag; A hears it about as strongly as B last was.
	// Going to any other room on that evidence is a move; going straight back
	// is the boundary ping-pong.
	for ms += 11_000; ms < 50_000; ms += 1000 {
		require.Equal(t, locB, step(t, a, rd(spA, locA, -52, ms)).LocationID, "returned on a gap at %dms", ms)
	}
}

func TestGapStillHandsOverToANewRoom(t *testing.T) {
	a := New(testConfig())
	ms := moveAtoB(t, a)
	got := step(t, a, rd(spC, locC, -52, ms+11_000))
	assert.True(t, got.Switched, "walked on to C, a room not just left")
	assert.Equal(t, locC, got.LocationID)
}

func TestReturnAfterWindowIsOrdinary(t *testing.T) {
	a := New(testConfig())
	ms := moveAtoB(t, a)
	// Five minutes at B, then back to A: an ordinary move, not ping-pong.
	for ms += 1000; ms < 5*60_000+9000; ms += 1000 {
		step(t, a, rd(spB, locB, -50, ms))
	}
	// The same signals as the return test: the ordinary Dwell, well under
	// ReturnDwell.
	took := switchAfter(t, a, ms)
	assert.Less(t, took, 15_000, "back to A after the window takes the ordinary dwell")
}

// switchAfter feeds B at -60 and A at -50, twice a second from start, until the
// asset moves to A, and returns how long that took.
func switchAfter(t *testing.T, a *Arbiter, start int) int {
	t.Helper()
	for ms := start; ms < start+120_000; ms += 500 {
		got := step(t, a, rd(spB, locB, -60, ms))
		if !got.Switched {
			got = step(t, a, rd(spA, locA, -50, ms+250))
		}
		if got.Switched {
			require.Equal(t, locA, got.LocationID)
			return ms - start
		}
	}
	t.Fatal("never moved back to A")
	return 0
}
