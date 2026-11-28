package locationmaps

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/locationmap"
	"github.com/trakrf/platform/backend/internal/models/report"
)

func TestBuildPresence(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	m := &locationmap.Map{
		LocationID:   1,
		LocationName: "Site",
		Pins:         []locationmap.Pin{{LocationID: 3, XPct: 20, YPct: 30}, {LocationID: 4, XPct: 70, YPct: 30}},
	}
	names := map[int]string{1: "Site", 2: "Floor", 3: "Room A", 4: "Room B", 5: "Cupboard"}
	at := func(asset, loc int, ago time.Duration) report.CurrentLocationItem {
		l := loc
		return report.CurrentLocationItem{
			AssetID: asset, AssetName: "asset", AssetExternalKey: "K", LocationID: &l, LastSeen: now.Add(-ago),
		}
	}
	current := []report.CurrentLocationItem{
		at(100, 3, time.Minute),       // in Room A
		at(101, 5, time.Minute),       // in the cupboard, under Room A
		at(102, 2, time.Minute),       // on the floor, no pin
		at(103, 4, 2*time.Hour),       // Room B but stale
		at(104, 9, time.Minute),       // not in this subtree (defensive)
		{AssetID: 105, LastSeen: now}, // no location
	}

	p := buildPresence(m, tree, names, current, []int{101}, time.Hour, now)

	require.Equal(t, 1, p.MapLocationID)
	require.Equal(t, now, p.GeneratedAt)
	require.Len(t, p.Zones, 2)

	roomA := p.Zones[0]
	require.Equal(t, 3, roomA.LocationID)
	require.Equal(t, "Room A", roomA.Name)
	require.Equal(t, 20.0, roomA.XPct)
	require.Len(t, roomA.Occupants, 2)
	require.Equal(t, 100, roomA.Occupants[0].AssetID)
	require.False(t, roomA.Occupants[0].IsPerson)
	require.Equal(t, 101, roomA.Occupants[1].AssetID)
	require.True(t, roomA.Occupants[1].IsPerson)
	require.Equal(t, 5, roomA.Occupants[1].LocationID, "keeps where it actually is")
	require.Equal(t, "Cupboard", roomA.Occupants[1].LocationName)

	require.Empty(t, p.Zones[1].Occupants, "stale sighting excluded")
	require.NotNil(t, p.Zones[1].Occupants, "empty list, not null")

	require.Len(t, p.Unplaced, 1)
	require.Equal(t, 102, p.Unplaced[0].AssetID)

	// seen_within=0 means no limit: the stale Room B sighting comes back.
	all := buildPresence(m, tree, names, current, nil, 0, now)
	require.Len(t, all.Zones[1].Occupants, 1)
}
