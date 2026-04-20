package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFakeClock_AdvanceMovesTimeForward(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	c := NewFakeClock(start)

	require.Equal(t, start, c.Now())

	c.Advance(30 * time.Second)
	require.Equal(t, start.Add(30*time.Second), c.Now())

	c.Advance(2 * time.Minute)
	require.Equal(t, start.Add(30*time.Second+2*time.Minute), c.Now())
}

func TestRealClock_ReturnsCurrentTime(t *testing.T) {
	c := RealClock{}
	before := time.Now()
	got := c.Now()
	after := time.Now()

	require.False(t, got.Before(before))
	require.False(t, got.After(after))
}
