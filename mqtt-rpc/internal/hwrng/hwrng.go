// Package hwrng feeds the reader's hardware random number generator into the
// kernel's entropy pool, so TLS can start seconds after a boot, not minutes.
//
// The CS463 (i.MX6, kernel 4.14) has a CAAM hardware RNG that the kernel
// registers as /dev/hwrng but never credits: rng_core's quality is 0, so the
// kernel's hwrng thread never runs. With no rngd or haveged on the image, the
// pool fills only from interrupts, and `random: crng init done` lands 90–390 s
// after a boot. Until then getrandom(2) blocks, which stalls Go's crypto/rand
// (mqtt-rpcd's broker handshake) and the vendor app's own CloudServer MQTT
// client. Reading the CAAM once and crediting it with RNDADDENTROPY ends the
// wait at once.
//
// It needs no saved state, so it works on the first boot after a reflash and
// after a power pull. A saved seed would not, and crediting the same seed on two
// boots is unsafe.
package hwrng

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBytes is one 4.14 input pool's worth (4096 bits).
	DefaultBytes = 512
	// DefaultTimeout bounds the read: the CAAM answers 512 bytes in ~1 ms, and
	// the daemon's start must never wait on a generator that has stopped.
	DefaultTimeout = 2 * time.Second

	defaultHWRNG        = "/dev/hwrng"
	defaultCurrent      = "/sys/class/misc/hw_random/rng_current"
	defaultEntropyAvail = "/proc/sys/kernel/random/entropy_avail"

	// blockSize is the unit of the repeated-output check.
	blockSize = 32
)

var (
	// ErrStuck means the generator returned output no healthy RNG produces.
	ErrStuck = errors.New("hardware RNG output is stuck")
	// ErrTimeout means the generator did not answer in time.
	ErrTimeout = errors.New("hardware RNG did not answer in time")
)

// Pool is where credited entropy goes: the kernel, outside tests.
type Pool interface {
	AddEntropy(data []byte, bits int) error
}

// Credit reads n bytes from src and credits them to pool at 8 bits a byte.
// Output that is constant, or that repeats a block back to back, is refused and
// never credited: a hardware RNG that fails usually fails that way.
func Credit(src io.Reader, pool Pool, n int) (int, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(src, buf); err != nil {
		return 0, fmt.Errorf("read hardware RNG: %w", err)
	}
	if stuck(buf) {
		return 0, ErrStuck
	}
	bits := n * 8
	if err := pool.AddEntropy(buf, bits); err != nil {
		return 0, fmt.Errorf("credit entropy: %w", err)
	}
	return bits, nil
}

func stuck(buf []byte) bool {
	if len(buf) == 0 || bytes.Count(buf, buf[:1]) == len(buf) {
		return true
	}
	for i := blockSize; i+blockSize <= len(buf); i += blockSize {
		if bytes.Equal(buf[i-blockSize:i], buf[i:i+blockSize]) {
			return true
		}
	}
	return false
}

// Config says where to read from and where to credit to. The zero value is the
// reader's real devices.
type Config struct {
	HWRNG        string // default /dev/hwrng
	Current      string // the generator's name; default /sys/class/misc/hw_random/rng_current
	EntropyAvail string // default /proc/sys/kernel/random/entropy_avail
	// Open overrides opening HWRNG (tests).
	Open    func() (io.ReadCloser, error)
	Pool    Pool          // default: the kernel, through /dev/random
	Bytes   int           // default DefaultBytes
	Timeout time.Duration // default DefaultTimeout
}

// Result is what Seed did, for one log line.
type Result struct {
	Skipped     bool   // no hardware RNG on this machine
	Source      string // e.g. "rng-caam"
	Bits        int    // credited
	AvailBefore int    // entropy_avail before, -1 if unreadable
	AvailAfter  int    // entropy_avail after, -1 if unreadable
	Err         error
}

// Seed credits the hardware RNG to the kernel once. It never blocks longer than
// the timeout, and a machine with no hardware RNG is skipped, not an error.
func Seed(cfg Config) Result {
	cfg = withDefaults(cfg)
	res := Result{
		Source:      readName(cfg.Current),
		AvailBefore: readInt(cfg.EntropyAvail),
		AvailAfter:  -1,
	}

	src, err := cfg.Open()
	if errors.Is(err, fs.ErrNotExist) {
		res.Skipped = true
		return res
	}
	if err != nil {
		res.Err = fmt.Errorf("open hardware RNG: %w", err)
		return res
	}

	type outcome struct {
		bits int
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		// A read that never returns strands this goroutine, not the daemon.
		defer src.Close()
		bits, err := Credit(src, cfg.Pool, cfg.Bytes)
		done <- outcome{bits, err}
	}()

	timer := time.NewTimer(cfg.Timeout)
	defer timer.Stop()
	select {
	case o := <-done:
		res.Bits, res.Err = o.bits, o.err
	case <-timer.C:
		res.Err = ErrTimeout
	}
	res.AvailAfter = readInt(cfg.EntropyAvail)
	return res
}

func withDefaults(cfg Config) Config {
	if cfg.HWRNG == "" {
		cfg.HWRNG = defaultHWRNG
	}
	if cfg.Current == "" {
		cfg.Current = defaultCurrent
	}
	if cfg.EntropyAvail == "" {
		cfg.EntropyAvail = defaultEntropyAvail
	}
	if cfg.Open == nil {
		path := cfg.HWRNG
		cfg.Open = func() (io.ReadCloser, error) { return os.Open(path) }
	}
	if cfg.Pool == nil {
		cfg.Pool = KernelPool{}
	}
	if cfg.Bytes <= 0 {
		cfg.Bytes = DefaultBytes
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return cfg
}

func readName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readInt(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return n
}
