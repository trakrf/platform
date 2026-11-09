package hwrng

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingPool captures what would have gone to the kernel.
type recordingPool struct {
	calls int
	data  []byte
	bits  int
	err   error
}

func (p *recordingPool) AddEntropy(data []byte, bits int) error {
	p.calls++
	p.data = append([]byte(nil), data...)
	p.bits = bits
	return p.err
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
	assert.Equal(t, 1, pool.calls)
	assert.Equal(t, src, pool.data)
	assert.Equal(t, 4096, pool.bits)
}

func TestCreditRefusesAStuckSource(t *testing.T) {
	pool := &recordingPool{}

	_, err := Credit(bytes.NewReader(make([]byte, 512)), pool, 512)

	require.ErrorIs(t, err, ErrStuck)
	assert.Zero(t, pool.calls, "a stuck source must never be credited")
}

func TestCreditRefusesARepeatingBlock(t *testing.T) {
	block := randomish(blockSize)
	src := bytes.Repeat(block, 512/blockSize)
	pool := &recordingPool{}

	_, err := Credit(bytes.NewReader(src), pool, 512)

	require.ErrorIs(t, err, ErrStuck)
	assert.Zero(t, pool.calls)
}

func TestCreditRefusesAShortRead(t *testing.T) {
	pool := &recordingPool{}

	_, err := Credit(bytes.NewReader(randomish(100)), pool, 512)

	require.Error(t, err)
	assert.Zero(t, pool.calls, "a short read must never be credited")
}

func TestCreditReportsAPoolError(t *testing.T) {
	pool := &recordingPool{err: errors.New("EPERM")}

	_, err := Credit(bytes.NewReader(randomish(512)), pool, 512)

	require.Error(t, err)
}

// poolInfo is struct rand_pool_info from <linux/random.h>:
// int entropy_count (bits); int buf_size (bytes); __u32 buf[].
func TestPoolInfoMatchesTheKernelLayout(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	got := poolInfo(data, 64)

	require.Len(t, got, 8+len(data))
	assert.Equal(t, int32(64), int32(binary.NativeEndian.Uint32(got[0:4])))
	assert.Equal(t, int32(len(data)), int32(binary.NativeEndian.Uint32(got[4:8])))
	assert.Equal(t, data, got[8:])
}

func TestRNDADDENTROPYIsTheKernelValue(t *testing.T) {
	// _IOW('R', 0x03, int[2]) — the same on arm and amd64.
	assert.Equal(t, uintptr(0x40085203), uintptr(rndAddEntropy))
}

func TestSeedSkipsAReaderWithNoHardwareRNG(t *testing.T) {
	dir := t.TempDir()
	pool := &recordingPool{}

	res := Seed(Config{
		HWRNG: filepath.Join(dir, "hwrng"), // absent
		Pool:  pool,
	})

	assert.True(t, res.Skipped)
	require.NoError(t, res.Err)
	assert.Zero(t, pool.calls)
}

func TestSeedCreditsFromTheHardwareRNG(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "hwrng")
	require.NoError(t, os.WriteFile(dev, randomish(4096), 0o600))
	name := filepath.Join(dir, "rng_current")
	require.NoError(t, os.WriteFile(name, []byte("rng-caam\n"), 0o600))
	avail := filepath.Join(dir, "entropy_avail")
	require.NoError(t, os.WriteFile(avail, []byte("71\n"), 0o600))
	pool := &recordingPool{}

	res := Seed(Config{HWRNG: dev, Current: name, EntropyAvail: avail, Pool: pool})

	require.NoError(t, res.Err)
	assert.False(t, res.Skipped)
	assert.Equal(t, "rng-caam", res.Source)
	assert.Equal(t, DefaultBytes*8, res.Bits)
	assert.Equal(t, 71, res.AvailBefore)
	assert.Equal(t, 1, pool.calls)
}

// blockingReader never returns, like a hardware RNG that has stopped answering.
type blockingReader struct{ release chan struct{} }

func (b blockingReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

func TestSeedGivesUpOnASourceThatNeverAnswers(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	pool := &recordingPool{}

	start := time.Now()
	res := Seed(Config{
		Open:    func() (io.ReadCloser, error) { return io.NopCloser(blockingReader{release}), nil },
		Pool:    pool,
		Timeout: 50 * time.Millisecond,
	})

	require.ErrorIs(t, res.Err, ErrTimeout)
	assert.Less(t, time.Since(start), 2*time.Second, "the daemon's start must not wait on the RNG")
	assert.Zero(t, pool.calls)
}
