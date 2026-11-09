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

// read takes n bytes from src. Output with any 16-byte window that appears
// twice is refused and never credited: a hardware RNG that fails usually fails
// by going constant, cycling, or stalling into zeros partway through, and each
// of those repeats a window.
func read(src io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(src, buf); err != nil {
		return nil, fmt.Errorf("read hardware RNG: %w", err)
	}
	if repeats(buf) {
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

// window is long enough that genuinely random output almost never repeats one:
// at 512 bytes there are 497 windows, and the chance that any two of the
// 123k pairs match is about 4e-34. An 8-byte window would be about 7e-15, a
// 4-byte one 3e-5.
const window = 16

// repeats reports whether any window-byte run of buf appears twice, at any two
// offsets. That catches a cycle of any period up to len(buf)-window wherever it
// starts, constant output (period 1), and a stuck tail of a repeated byte.
func repeats(buf []byte) bool {
	if len(buf) == 0 {
		return true
	}
	seen := make(map[[window]byte]struct{}, len(buf))
	for i := 0; i+window <= len(buf); i++ {
		w := [window]byte(buf[i : i+window])
		if _, ok := seen[w]; ok {
			return true
		}
		seen[w] = struct{}{}
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
	Bits        int    // offered to the kernel; entropy_avail shows what it kept
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

	type answer struct {
		buf []byte
		err error
	}
	// The goroutine only opens and reads; crediting happens here, and only for an
	// answer that beat the timer, so a late answer is never credited. An open or
	// read that never returns strands that goroutine, not the daemon.
	done := make(chan answer, 1)
	go func() {
		src, err := os.Open(cfg.HWRNG)
		if err != nil {
			done <- answer{err: fmt.Errorf("open hardware RNG: %w", err)}
			return
		}
		defer src.Close()
		buf, err := read(src, cfg.Bytes)
		done <- answer{buf, err}
	}()

	timer := time.NewTimer(cfg.Timeout)
	defer timer.Stop()
	select {
	case a := <-done:
		if a.err != nil {
			res.Err = a.err
		} else {
			res.Bits, res.Err = credit(cfg.Pool, a.buf)
		}
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
