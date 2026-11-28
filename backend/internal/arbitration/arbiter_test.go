package arbitration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	org   = 7
	asset = 100
	locA  = 1
	locB  = 2
	spA   = 11
	spB   = 22
)

var t0 = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

func ptr(v int) *int { return &v }

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func rd(sp, loc, rssi, ms int) Read {
	return Read{AssetID: asset, ScanPointID: sp, LocationID: ptr(loc), RSSI: rssi, At: at(ms)}
}

func testConfig() Config {
	c := DefaultConfig()
	c.Window, c.MarginDB, c.Dwell, c.Silence = 10*time.Second, 6, 5*time.Second, 60*time.Second
	return c
}

// step runs one message through the arbiter and applies it, returning the
// asset's assignment.
func step(t *testing.T, a *Arbiter, reads ...Read) Assignment {
	t.Helper()
	res := a.Evaluate(org, reads, nil)
	a.Apply(res)
	got, ok := res.Assignments[asset]
	require.True(t, ok, "asset has an assignment")
	return got
}

// alternate interleaves one read from each scan point per second for n seconds,
// starting at startMs.
func alternate(t *testing.T, a *Arbiter, startMs, n, rssiA, rssiB int) (switches int, last Assignment) {
	t.Helper()
	for i := 0; i < n; i++ {
		ms := startMs + i*1000
		for _, r := range []Read{rd(spA, locA, rssiA, ms), rd(spB, locB, rssiB, ms+500)} {
			last = step(t, a, r)
			if last.Switched {
				switches++
			}
		}
	}
	return switches, last
}

func TestStrongerHoldsNoFlapUnderMargin(t *testing.T) {
	a := New(testConfig())
	// A is the incumbent; B is 5 dB stronger, inside the 6 dB margin, for a minute.
	first := step(t, a, rd(spA, locA, -50, 0))
	assert.Equal(t, locA, first.LocationID)
	assert.True(t, first.Switched, "first assignment counts as a switch")

	switches, last := alternate(t, a, 1000, 60, -50, -45)
	assert.Zero(t, switches)
	assert.Equal(t, locA, last.LocationID)
	assert.Equal(t, spA, last.ScanPointID)
}

func TestWeakerFirstIsCorrectedOnceAfterDwell(t *testing.T) {
	a := New(testConfig())
	assert.Equal(t, locB, step(t, a, rd(spB, locB, -80, 0)).LocationID)

	// A is 30 dB stronger from here on: exactly one switch, not before D.
	var switchedAt time.Time
	switches := 0
	for i := 0; i < 30; i++ {
		ms := 1000 + i*1000
		for _, r := range []Read{rd(spA, locA, -50, ms), rd(spB, locB, -80, ms+500)} {
			got := step(t, a, r)
			if got.Switched {
				switches++
				switchedAt = r.At
				assert.Equal(t, locA, got.LocationID)
			}
		}
	}
	assert.Equal(t, 1, switches)
	assert.GreaterOrEqual(t, switchedAt.Sub(at(1000)), 5*time.Second, "dwell respected")
}

func TestChallengerLeadInterruptedResetsDwell(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -60, 0))
	// B leads by >= M for 4s, then falls back inside the margin, then leads again.
	for ms := 100; ms <= 4100; ms += 1000 {
		assert.False(t, step(t, a, rd(spB, locB, -40, ms)).Switched)
	}
	// A strong burst pulls A's window mean up so B no longer beats it by M.
	for ms := 4200; ms <= 4600; ms += 100 {
		step(t, a, rd(spA, locA, -30, ms))
	}
	got := step(t, a, rd(spB, locB, -40, 5200))
	assert.False(t, got.Switched, "lead was interrupted, dwell restarts")
	assert.Equal(t, locA, got.LocationID)
}

func TestUnheardIncumbentHandsOverToComparableSignal(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -60, 0))
	// A goes unheard; B turns up about as strong as A last was: the asset
	// walked. B takes over once A is out of its window, with no dwell.
	got := step(t, a, rd(spB, locB, -62, 9000))
	assert.Equal(t, locA, got.LocationID, "A still inside its window")
	got = step(t, a, rd(spB, locB, -62, 10500))
	assert.True(t, got.Switched)
	assert.Equal(t, locB, got.LocationID)
	assert.Equal(t, spB, got.ScanPointID)
}

func TestUnheardIncumbentHoldsAgainstWeakSignalUntilSilence(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -37, 0))
	// A's gateway skips the tag; the next room hears it far weaker, as it did
	// all along. That is a missed report, not a move.
	for ms := 1000; ms < 60000; ms += 1000 {
		got := step(t, a, rd(spB, locB, -71, ms))
		require.Equal(t, locA, got.LocationID, "held at %dms", ms)
	}
	got := step(t, a, rd(spB, locB, -71, 60000))
	assert.True(t, got.Switched, "silence backstop hands over")
	assert.Equal(t, locB, got.LocationID)
}

func TestIncumbentHeardAgainResetsSilence(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -37, 0))
	// Heard again at 50s, so the backstop is 60s from then, not from 0.
	for ms := 1000; ms < 110000; ms += 1000 {
		r := rd(spB, locB, -71, ms)
		if ms == 50000 {
			r = rd(spA, locA, -37, ms)
		}
		require.Equal(t, locA, step(t, a, r).LocationID, "held at %dms", ms)
	}
}

func TestRSSIZeroIgnored(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -70, 0))
	// RSSI 0 would beat every real read in a signed comparison.
	for ms := 1000; ms <= 20000; ms += 1000 {
		got := step(t, a, rd(spA, locA, -70, ms), rd(spB, locB, 0, ms+1))
		assert.Equal(t, locA, got.LocationID)
		assert.False(t, got.Switched)
	}

	// An asset only ever heard with RSSI 0 gets no assignment at all.
	b := New(testConfig())
	res := b.Evaluate(org, []Read{rd(spB, locB, 0, 0)}, nil)
	_, ok := res.Assignments[asset]
	assert.False(t, ok)
}

func TestNilLocationNeverAssigned(t *testing.T) {
	a := New(testConfig())
	step(t, a, rd(spA, locA, -80, 0))
	for ms := 1000; ms <= 30000; ms += 1000 {
		got := step(t, a, Read{AssetID: asset, ScanPointID: spB, RSSI: -30, At: at(ms)}, rd(spA, locA, -80, ms+1))
		assert.Equal(t, locA, got.LocationID)
	}
}

func TestSeededIncumbentHoldsThroughGrace(t *testing.T) {
	a := New(testConfig())
	assert.Equal(t, []int{asset}, a.NeedsSeed(org, []int{asset}))

	// Restart: the first message is from B (weaker room), the asset was last
	// stored at A. A is unheard but only just seeded, so B must not win yet,
	// and the row is held since no A scan point is known.
	res := a.Evaluate(org, []Read{rd(spB, locB, -80, 0)}, map[int]Seed{asset: {LocationID: locA}})
	a.Apply(res)
	got := res.Assignments[asset]
	assert.Equal(t, locA, got.LocationID)
	assert.Zero(t, got.ScanPointID, "seeded incumbent not heard yet")
	assert.False(t, got.Switched)
	assert.Empty(t, a.NeedsSeed(org, []int{asset}))

	// A's own gateway reports a second later: A is confirmed with its scan point.
	got = step(t, a, rd(spA, locA, -50, 1000))
	assert.Equal(t, locA, got.LocationID)
	assert.Equal(t, spA, got.ScanPointID)
	assert.False(t, got.Switched)
}

func TestSeedNeverHeardHandsOverAfterSilence(t *testing.T) {
	a := New(testConfig())
	// A seed has no level to compare against, so only the backstop moves it.
	a.Apply(a.Evaluate(org, []Read{rd(spB, locB, -40, 0)}, map[int]Seed{asset: {LocationID: locA}}))
	assert.Equal(t, locA, step(t, a, rd(spB, locB, -40, 59000)).LocationID)
	got := step(t, a, rd(spB, locB, -40, 60000))
	assert.True(t, got.Switched)
	assert.Equal(t, locB, got.LocationID)
}

func TestEvaluateIsPureUntilApply(t *testing.T) {
	a := New(testConfig())
	res := a.Evaluate(org, []Read{rd(spA, locA, -50, 0)}, nil)
	assert.Equal(t, locA, res.Assignments[asset].LocationID)
	// Dropped (a rolled-back scan): the arbiter still has no state.
	assert.Equal(t, []int{asset}, a.NeedsSeed(org, []int{asset}))

	a.Apply(a.Evaluate(org, []Read{rd(spA, locA, -50, 0)}, nil))
	before := a.Evaluate(org, []Read{rd(spB, locB, -50, 10500)}, nil)
	assert.True(t, before.Assignments[asset].Switched)
	// Not applying it leaves A in place for the next evaluation.
	again := a.Evaluate(org, []Read{rd(spA, locA, -50, 10600)}, nil)
	assert.Equal(t, locA, again.Assignments[asset].LocationID)
	assert.False(t, again.Assignments[asset].Switched)
}

func TestOrgsAreIsolated(t *testing.T) {
	a := New(testConfig())
	a.Apply(a.Evaluate(org, []Read{rd(spA, locA, -50, 0)}, nil))
	assert.Equal(t, []int{asset}, a.NeedsSeed(org+1, []int{asset}))
}

func TestIdleStateIsEvicted(t *testing.T) {
	a := New(testConfig())
	other := Read{AssetID: asset + 1, ScanPointID: spA, LocationID: ptr(locA), RSSI: -50, At: at(0)}
	a.Apply(a.Evaluate(org, []Read{rd(spA, locA, -50, 0), other}, nil))
	other.At = at(60000)
	a.Apply(a.Evaluate(org, []Read{other}, nil))
	assert.Empty(t, a.NeedsSeed(org, []int{asset}), "within the silence horizon")
	other.At = at(121000)
	a.Apply(a.Evaluate(org, []Read{other}, nil))
	assert.Equal(t, []int{asset}, a.NeedsSeed(org, []int{asset}), "idle asset re-seeds from storage")
	assert.Empty(t, a.NeedsSeed(org, []int{asset + 1}))
}
