package arbitration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The homelab captures share one masking: gateways A, B and C are three rooms
// (A = Front Office, B = Main Entrance, C = Main Warehouse) and tags T1-T4 are
// four BLE tags. Scan point id == location id.
var (
	homelabGateways = map[string]int{"A": 1, "B": 2, "C": 3}
	homelabTags     = map[string]int{"T1": 1, "T2": 2, "T3": 3, "T4": 4}
)

type switchAt struct {
	ms  int
	loc int
}

// replay runs a capture through a fresh arbiter message by message and
// returns every switch, per tag.
func replay(t *testing.T, rows []fixtureRow, cfg Config) map[int][]switchAt {
	t.Helper()
	require.NotEmpty(t, rows)
	a := New(cfg)
	out := map[int][]switchAt{}
	for i := 0; i < len(rows); {
		j := i
		var msg []Read
		for ; j < len(rows) && rows[j].ms == rows[i].ms && rows[j].gateway == rows[i].gateway; j++ {
			r := rows[j]
			loc, id := homelabGateways[r.gateway], homelabTags[r.tag]
			msg = append(msg, Read{AssetID: id, ScanPointID: loc, LocationID: ptr(loc), RSSI: r.rssi, At: at(r.ms)})
		}
		res := a.Evaluate(org, msg, nil)
		a.Apply(res)
		for id, as := range res.Assignments {
			if as.Switched {
				out[id] = append(out[id], switchAt{ms: rows[i].ms, loc: as.LocationID})
			}
		}
		i = j
	}
	return out
}

func after(sw []switchAt, ms int) []switchAt {
	var out []switchAt
	for _, s := range sw {
		if s.ms >= ms {
			out = append(out, s)
		}
	}
	return out
}

// TestHomelabBoundaryFixtureRarelyFlaps replays 57 minutes in which T3 sat
// still on a bookshelf that rooms A and B hear about equally (medians -65 and
// -64 dBm), with C behind a wall and patchy. The gateways routinely leave T3
// out of their reports for 10-30 s, and each gap handed T3 to the other room
// and back. Without the return rule the replay moves T3 16 times after it
// settles; preview stored 15 in 41 minutes. T2 and T4 are controls with a
// clear winner; T1 is spinning on a turntable and is not asserted.
//
// Room A first heard T3 at about 170 s, when its gateway was shifted clear of
// a structural post, so corrections up to 360 s are real. After that the
// return rule leaves five moves, one every 8-12 minutes, each a real 6 dB
// lead or a 10 s gap. RSSI alone cannot tell those from a short move; stored
// history that records stays rather than passes would.
func TestHomelabBoundaryFixtureRarelyFlaps(t *testing.T) {
	sw := replay(t, loadFixture(t, "homelab_boundary_57m.csv.gz"), DefaultConfig())
	for name, id := range homelabTags {
		t.Logf("%s: %d switch(es): %v", name, len(sw[id]), sw[id])
	}
	assert.LessOrEqual(t, len(after(sw[3], 360_000)), 5, "T3 flapped at the boundary: %v", after(sw[3], 360_000))
	assert.Empty(t, after(sw[2], 60_000), "control T2 moved")
	assert.Empty(t, after(sw[4], 60_000), "control T4 moved")
}

// TestHomelabWalkFollowsEveryRoom replays a walk: T1 and T2 carried together
// from room A through B, C, A, B, C and back to A, with arrival times noted by
// hand on a phone (so good to a second or two). Each arrival must be recorded,
// in order, with nothing else, and within 15 s. On the first arbitration rules
// preview recorded all 12, a median of about 4 s and a worst of 8.8 s late.
func TestHomelabWalkFollowsEveryRoom(t *testing.T) {
	const walkStart = 90_000
	arrivals := []switchAt{
		{100_000, 2}, {113_000, 3}, {127_000, 1},
		{141_000, 2}, {152_000, 3}, {167_000, 1},
	}
	sw := replay(t, loadFixture(t, "homelab_walk_4m.csv.gz"), DefaultConfig())
	for _, id := range []int{1, 2} {
		got := after(sw[id], walkStart)
		var lags []time.Duration
		for i := range got {
			if i < len(arrivals) {
				lags = append(lags, time.Duration(got[i].ms-arrivals[i].ms)*time.Millisecond)
			}
		}
		t.Logf("T%d: switches %v, lags %v", id, got, lags)
		require.Len(t, got, len(arrivals), "T%d: one switch per arrival", id)
		for i, want := range arrivals {
			assert.Equal(t, want.loc, got[i].loc, "T%d arrival %d room", id, i)
			lag := got[i].ms - want.ms
			assert.GreaterOrEqual(t, lag, -3000, "T%d arrival %d switched %d ms early", id, i, -lag)
			assert.LessOrEqual(t, lag, 15_000, "T%d arrival %d switched %d ms late", id, i, lag)
		}
	}
}
