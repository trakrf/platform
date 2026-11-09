package hwrng

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingPool captures what would have gone to the kernel.
type recordingPool struct {
	mu    sync.Mutex
	calls int
	data  []byte
	bits  int
	err   error
}

func (p *recordingPool) AddEntropy(data []byte, bits int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.data = append([]byte(nil), data...)
	p.bits = bits
	return p.err
}

func (p *recordingPool) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func randomish(n int) []byte {
	b := make([]byte, n)
	x := uint32(2463534242)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}

func TestCreditPassesEveryByteAtEightBitsEach(t *testing.T) {
	src := randomish(512)
	pool := &recordingPool{}

	bits, err := Credit(bytes.NewReader(src), pool, 512)

	require.NoError(t, err)
	assert.Equal(t, 4096, bits)
	assert.Equal(t, 1, pool.Calls())
	assert.Equal(t, src, pool.data)
	assert.Equal(t, 4096, pool.bits)
}

func TestCreditRefusesAStuckSource(t *testing.T) {
	pool := &recordingPool{}

	_, err := Credit(bytes.NewReader(make([]byte, 512)), pool, 512)

	require.ErrorIs(t, err, ErrStuck)
	assert.Zero(t, pool.Calls(), "a stuck source must never be credited")
}

// A generator stuck in a cycle repeats with that cycle's period, whatever it
// is: aligned to nothing in particular, and as long as half the read.
func TestCreditRefusesARepeatingCycleOfAnyLength(t *testing.T) {
	for _, period := range []int{1, 2, 32, 33, 48, 64, 200, 256} {
		cycle := randomish(period)
		src := bytes.Repeat(cycle, 512/period+1)[:512]
		pool := &recordingPool{}

		_, err := Credit(bytes.NewReader(src), pool, 512)

		require.ErrorIs(t, err, ErrStuck, "period %d", period)
		assert.Zero(t, pool.Calls(), "period %d", period)
	}
}

func TestCreditRefusesAShortRead(t *testing.T) {
	pool := &recordingPool{}

	_, err := Credit(bytes.NewReader(randomish(100)), pool, 512)

	require.Error(t, err)
	assert.Zero(t, pool.Calls(), "a short read must never be credited")
}

func TestCreditReportsAPoolError(t *testing.T) {
	pool := &recordingPool{err: errors.New("EPERM")}

	_, err := Credit(bytes.NewReader(randomish(512)), pool, 512)

	require.Error(t, err)
}

// struct rand_pool_info in <linux/random.h>: int entropy_count (bits) at
// offset 0, int buf_size (bytes) at offset 4, the bytes from offset 8.
func TestPoolInfoMatchesTheKernelLayout(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	got := poolInfo(data, 64)

	require.Len(t, got, 8+len(data))
	assert.Equal(t, int32(64), int32(binary.NativeEndian.Uint32(got[0:4])))
	assert.Equal(t, int32(len(data)), int32(binary.NativeEndian.Uint32(got[4:8])))
	assert.Equal(t, data, got[8:])
}

// fakeSystem lays out the files Seed reads, in a temp dir.
type fakeSystem struct {
	cfg Config
}

func newFakeSystem(t *testing.T, current string, dev []byte) fakeSystem {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		HWRNG:        filepath.Join(dir, "hwrng"),
		Current:      filepath.Join(dir, "rng_current"),
		EntropyAvail: filepath.Join(dir, "entropy_avail"),
	}
	if current != "" {
		require.NoError(t, os.WriteFile(cfg.Current, []byte(current+"\n"), 0o600))
	}
	if dev != nil {
		require.NoError(t, os.WriteFile(cfg.HWRNG, dev, 0o600))
	}
	require.NoError(t, os.WriteFile(cfg.EntropyAvail, []byte("71\n"), 0o600))
	return fakeSystem{cfg: cfg}
}

func TestSeedCreditsTheCAAM(t *testing.T) {
	sys := newFakeSystem(t, "rng-caam", randomish(4096))
	pool := &recordingPool{}
	sys.cfg.Pool = pool

	res := Seed(sys.cfg)

	require.NoError(t, res.Err)
	assert.Empty(t, res.Skip)
	assert.Equal(t, "rng-caam", res.Source)
	assert.Equal(t, DefaultBytes*8, res.Bits)
	assert.Equal(t, 71, res.AvailBefore)
	assert.Equal(t, 1, pool.Calls())
}

func TestSeedSkipsAReaderWithNoHardwareRNG(t *testing.T) {
	sys := newFakeSystem(t, "rng-caam", nil) // no device node
	pool := &recordingPool{}
	sys.cfg.Pool = pool

	res := Seed(sys.cfg)

	assert.NotEmpty(t, res.Skip)
	require.NoError(t, res.Err)
	assert.Zero(t, pool.Calls())
}

// rng_current "none" means the device node exists but reading it fails with
// ENODEV: that is a machine with no generator, not a fault to warn about.
func TestSeedSkipsWhenTheKernelHasNoCurrentGenerator(t *testing.T) {
	for _, current := range []string{"none", ""} {
		sys := newFakeSystem(t, current, randomish(4096))
		pool := &recordingPool{}
		sys.cfg.Pool = pool

		res := Seed(sys.cfg)

		assert.NotEmpty(t, res.Skip, "rng_current %q", current)
		require.NoError(t, res.Err, "rng_current %q", current)
		assert.Zero(t, pool.Calls(), "rng_current %q", current)
	}
}

// Full credit is a claim about the generator. Only the one measured on the
// CS463 earns it; anything else is left to the kernel's own estimate.
func TestSeedCreditsNoGeneratorItDoesNotKnow(t *testing.T) {
	sys := newFakeSystem(t, "timeriomem_rng", randomish(4096))
	pool := &recordingPool{}
	sys.cfg.Pool = pool

	res := Seed(sys.cfg)

	assert.NotEmpty(t, res.Skip)
	assert.Equal(t, "timeriomem_rng", res.Source)
	require.NoError(t, res.Err)
	assert.Zero(t, pool.Calls())
}
