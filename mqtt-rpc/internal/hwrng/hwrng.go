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
	"sync"
	"time"
)

const (
	// DefaultBytes is one 4.14 input pool's worth (4096 bits).
	DefaultBytes = 512
	// DefaultTimeout bounds the open and the read: the CAAM answers 512 bytes in
	// ~1 ms, and the daemon's start must never wait on a generator that has
	// stopped.
	DefaultTimeout = 2 * time.Second

	defaultHWRNG        = "/dev/hwrng"
	defaultCurrent      = "/sys/class/misc/hw_random/rng_current"
	defaultEntropyAvail = "/proc/sys/kernel/random/entropy_avail"
)

// trusted names the generators that earn full credit, 8 bits a byte. Crediting
// is a claim about the generator, so it is made only for the one measured on
// the CS463: the i.MX6 CAAM's RNG4, a DRBG seeded from its own TRNG. Any other
// generator is left to the kernel's own estimate.
var trusted = map[string]bool{"rng-caam": true}

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
// Output that repeats with any period up to half its length (constant output is
// period 1) is refused and never credited: a hardware RNG that fails usually
// fails that way.
func Credit(src io.Reader, pool Pool, n int) (int, error) {
	buf, err := read(src, n)
	if err != nil {
		return 0, err
	}
	return credit(pool, buf)
}

func read(src io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(src, buf); err != nil {
		return nil, fmt.Errorf("read hardware RNG: %w", err)
	}
	if periodic(buf) {
		return nil, ErrStuck
	}
	return buf, nil
}

func credit(pool Pool, buf []byte) (int, error) {
	bits := len(buf) * 8
	if err := pool.AddEntropy(buf, bits); err != nil {
		return 0, fmt.Errorf("credit entropy: %w", err)
	}
	return bits, nil
}

// periodic reports whether buf repeats with some period p <= len(buf)/2.
// Quadratic in the worst case, which at 512 bytes is nothing.
func periodic(buf []byte) bool {
	if len(buf) == 0 {
		return true
	}
	for p := 1; p <= len(buf)/2; p++ {
		if bytes.Equal(buf[:len(buf)-p], buf[p:]) {
			return true
		}
	}
	return false
}

// Config says where to read from and where to credit to. The zero value is the
// reader's real devices.
type Config struct {
	HWRNG        string        // default /dev/hwrng
	Current      string        // the generator's name; default /sys/class/misc/hw_random/rng_current
	EntropyAvail string        // default /proc/sys/kernel/random/entropy_avail
	Pool         Pool          // default: the kernel, through /dev/random
	Bytes        int           // default DefaultBytes
	Timeout      time.Duration // default DefaultTimeout
}

// Result is what Seed did, for one log line.
type Result struct {
	Skip        string // why nothing was credited, when that is not a fault
	Source      string // the kernel's current generator, e.g. "rng-caam"
	Bits        int    // credited
	AvailBefore int    // entropy_avail before, -1 if unreadable
	AvailAfter  int    // entropy_avail after, -1 if unreadable
	Err         error
}

// Seed credits the hardware RNG to the kernel once. It never blocks longer than
// the timeout. A machine with no generator, or one it does not trust, is
// skipped, not an error.
func Seed(cfg Config) Result {
	cfg = withDefaults(cfg)
	res := Result{
		Source:      readName(cfg.Current),
		AvailBefore: readInt(cfg.EntropyAvail),
		AvailAfter:  -1,
	}
	switch {
	case res.Source == "" || res.Source == "none":
		res.Skip = "the kernel has no hardware RNG"
		return res
	case !trusted[res.Source]:
		res.Skip = "not a hardware RNG this daemon credits"
		return res
	}
	if _, err := os.Stat(cfg.HWRNG); errors.Is(err, fs.ErrNotExist) {
		res.Skip = "no hardware RNG device"
		return res
	}

	type outcome struct {
		bits int
		err  error
	}
	done := make(chan outcome, 1)
	// Once Seed has given up, a late answer is not credited: Seed has already
	// reported the timeout, and the log line must stay true.
	var mu sync.Mutex
	abandoned := false
	go func() {
		// An open or read that never returns strands this goroutine, not the
		// daemon.
		bits, err := func() (int, error) {
			src, err := os.Open(cfg.HWRNG)
			if err != nil {
				return 0, fmt.Errorf("open hardware RNG: %w", err)
			}
			defer src.Close()
			buf, err := read(src, cfg.Bytes)
			if err != nil {
				return 0, err
			}
			mu.Lock()
			defer mu.Unlock()
			if abandoned {
				return 0, ErrTimeout
			}
			return credit(cfg.Pool, buf)
		}()
		done <- outcome{bits, err}
	}()

	timer := time.NewTimer(cfg.Timeout)
	defer timer.Stop()
	var o outcome
	select {
	case o = <-done:
	case <-timer.C:
		mu.Lock()
		abandoned = true
		mu.Unlock()
		// The credit may have finished between the timer and the lock.
		select {
		case o = <-done:
		default:
			o.err = ErrTimeout
		}
	}
	res.Bits, res.Err = o.bits, o.err
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
