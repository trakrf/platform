package locationmaps

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/locationmap"
)

func ptr(i int) *int { return &i }

// tree: site(1) > floor(2) > roomA(3) > cupboard(5), and floor(2) > roomB(4).
// Location 9 is outside the map's subtree.
var tree = map[int]*int{1: nil, 2: ptr(1), 3: ptr(2), 4: ptr(2), 5: ptr(3)}

func TestRollUp(t *testing.T) {
	pinned := map[int]bool{3: true, 4: true}

	cases := []struct {
		name   string
		loc    int
		wantID int
		wantOK bool
	}{
		{"pinned location itself", 3, 3, true},
		{"grandchild rolls up to its pinned ancestor", 5, 3, true},
		{"unpinned floor is unplaced", 2, 0, false},
		{"the map location itself is unplaced", 1, 0, false},
		{"outside the subtree", 9, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := rollUp(1, tree, pinned, c.loc)
			require.Equal(t, c.wantOK, ok)
			require.Equal(t, c.wantID, id)
		})
	}
}

func TestRollUp_CycleTerminates(t *testing.T) {
	cyc := map[int]*int{1: nil, 2: ptr(3), 3: ptr(2)}
	_, ok := rollUp(1, cyc, map[int]bool{}, 2)
	require.False(t, ok)
}

func TestSniffMapImage(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n0000")
	jpeg := []byte("\xff\xd8\xff\xe00000")
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")

	for name, c := range map[string]struct {
		in   []byte
		want string
	}{"png": {png, "image/png"}, "jpeg": {jpeg, "image/jpeg"}, "webp": {webp, "image/webp"}} {
		t.Run(name, func(t *testing.T) {
			got, err := sniffMapImage(c.in)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}

	_, err := sniffMapImage([]byte("hello, plain text"))
	require.ErrorIs(t, err, errUnsupportedImage)
	_, err = sniffMapImage([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	require.ErrorIs(t, err, errUnsupportedImage)
	_, err = sniffMapImage(nil)
	require.ErrorIs(t, err, errEmptyImage)

	big := append(append([]byte{}, png...), bytes.Repeat([]byte{0}, locationmap.MaxImageBytes)...)
	_, err = sniffMapImage(big)
	require.True(t, errors.Is(err, errImageTooLarge))
}

func TestValidatePins(t *testing.T) {
	require.NoError(t, validatePins(1, []locationmap.Pin{{LocationID: 3, XPct: 0, YPct: 100}}))
	require.Error(t, validatePins(1, []locationmap.Pin{{LocationID: 3, XPct: -0.1, YPct: 5}}))
	require.Error(t, validatePins(1, []locationmap.Pin{{LocationID: 3, XPct: 5, YPct: 100.1}}))
	require.Error(t, validatePins(1, []locationmap.Pin{{LocationID: 3}, {LocationID: 3}}), "duplicate location")
	require.Error(t, validatePins(1, []locationmap.Pin{{LocationID: 1}}), "map location pinned on itself")
}
