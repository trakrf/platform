package arbitration

import (
	"compress/gzip"
	"encoding/csv"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRow is one read from testdata/homelab_stationary_30m.csv.gz: 30 minutes
// of real traffic from three GL-S10 gateways in three rooms, hearing four
// stationary BLE tags (captured from preview, 2026-11-27). Columns are ms since
// the first read (server receive time), gateway, tag and RSSI. Gateway and tag
// identities are masked to letters; every read in one gateway message shares
// its receive time, so consecutive rows with the same ms and gateway are one
// message.
type fixtureRow struct {
	ms      int
	gateway string
	tag     string
	rssi    int
}

func loadFixture(t *testing.T, name string) []fixtureRow {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	recs, err := csv.NewReader(gz).ReadAll()
	require.NoError(t, err)
	rows := make([]fixtureRow, 0, len(recs))
	for _, r := range recs {
		ms, err := strconv.Atoi(r[0])
		require.NoError(t, err)
		rssi, err := strconv.Atoi(r[3])
		require.NoError(t, err)
		rows = append(rows, fixtureRow{ms: ms, gateway: r[1], tag: r[2], rssi: rssi})
	}
	return rows
}

func median(v []int) int {
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[len(s)/2]
}

// TestHomelabStationaryFixtureNoFlaps replays the capture message by message.
// Unarbitrated, preview recorded 147-236 location changes per asset in the six
// hours before the capture. Arbitrated, each tag must settle once and then
// never move, at the gateway that hears it strongest.
func TestHomelabStationaryFixtureNoFlaps(t *testing.T) {
	rows := loadFixture(t, "homelab_stationary_30m.csv.gz")
	require.NotEmpty(t, rows)

	gateways := map[string]int{"A": 1, "B": 2, "C": 3} // scan point id == location id
	tags := map[string]int{"T1": 1, "T2": 2, "T3": 3, "T4": 4}
	heard := map[int]map[int][]int{} // asset -> location -> rssi

	a := New(DefaultConfig())
	switches := map[int]int{}
	final := map[int]int{}
	settledBy := map[int]time.Duration{}
	for i := 0; i < len(rows); {
		j := i
		var msg []Read
		for ; j < len(rows) && rows[j].ms == rows[i].ms && rows[j].gateway == rows[i].gateway; j++ {
			r := rows[j]
			loc, id := gateways[r.gateway], tags[r.tag]
			msg = append(msg, Read{AssetID: id, ScanPointID: loc, LocationID: ptr(loc), RSSI: r.rssi, At: at(r.ms)})
			if heard[id] == nil {
				heard[id] = map[int][]int{}
			}
			if usable(r.rssi) {
				heard[id][loc] = append(heard[id][loc], r.rssi)
			}
		}
		i = j

		res := a.Evaluate(org, msg, nil)
		a.Apply(res)
		for id, as := range res.Assignments {
			if as.Switched {
				switches[id]++
				settledBy[id] = time.Duration(rows[j-1].ms) * time.Millisecond
			}
			final[id] = as.LocationID
		}
	}

	for name, id := range tags {
		strongest, best := 0, -1000
		for loc, v := range heard[id] {
			if m := median(v); m > best {
				strongest, best = loc, m
			}
		}
		t.Logf("%s: %d switch(es), last at %s, held %d, strongest %d (median %d dBm)",
			name, switches[id], settledBy[id], final[id], strongest, best)
		// The first assignment is the first gateway to publish; at most one
		// correction after it, within the first minute, and never again.
		assert.LessOrEqual(t, switches[id], 2, "%s switched %d times", name, switches[id])
		assert.Less(t, settledBy[id], time.Minute, "%s still switching at %s", name, settledBy[id])
		assert.Equal(t, strongest, final[id], "%s held the strongest gateway", name)
	}
}
