package readerd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

// Host is what the health monitor does to the reader itself when the vendor app
// is wedged.
type Host interface {
	// RestartReaderApp restarts the vendor web app (EmbeddedGlassFish) and
	// returns once it has been started again. It does not wait for /API to answer.
	RestartReaderApp(ctx context.Context) error
	// Reboot asks systemd for a clean reboot. It returns once the job is queued.
	Reboot() error
}

const (
	readerAppUnit = "embeddedglassfish.service"
	// appTermWait and appKillWait bound how long the old app gets to exit.
	appTermWait = 30 * time.Second
	appKillWait = 10 * time.Second
	appPoll     = time.Second
)

// SystemdHost implements Host on the CS463 (systemd 237, cgroup v1).
//
// systemctl restart alone is not a restart there. The vendor init script's stop
// kills only the pid in a pidfile that is not always written, and the
// generated unit is KillMode=process with no main pid. So systemd kills nothing,
// and start launches a second java beside the first. RestartReaderApp therefore
// signals whatever is left in the unit's cgroup itself. If anything survives
// SIGKILL it refuses to start, and the caller escalates to a reboot.
type SystemdHost struct {
	log zerolog.Logger

	cgroupCandidates []string
	run              func(ctx context.Context, name string, args ...string) error
	readProcs        func(path string) ([]int, error)
	kill             func(pid int, sig syscall.Signal) error
	sync             func()
	sleep            func(time.Duration)
}

// NewSystemdHost builds the real host.
func NewSystemdHost(log *zerolog.Logger) *SystemdHost {
	cg := "system.slice/" + readerAppUnit + "/cgroup.procs"
	return &SystemdHost{
		log: log.With().Str("component", "host").Logger(),
		cgroupCandidates: []string{
			"/sys/fs/cgroup/systemd/" + cg, // cgroup v1 (the CS463)
			"/sys/fs/cgroup/unified/" + cg, // hybrid
			"/sys/fs/cgroup/" + cg,         // v2
		},
		run:       runCommand,
		readProcs: readCgroupProcs,
		kill:      syscall.Kill,
		sync:      syscall.Sync,
		sleep:     time.Sleep,
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readCgroupProcs lists the pids in a cgroup.procs file. A cgroup that no longer
// exists has no processes.
func readCgroupProcs(path string) ([]int, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, f := range strings.Fields(string(b)) {
		pid, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("readerd: bad pid %q in %s", f, path)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// procsFile finds the unit's cgroup.procs. Call it while the unit is up: its
// cgroup directory is removed once it is empty.
func (h *SystemdHost) procsFile() (string, bool) {
	for _, p := range h.cgroupCandidates {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// RestartReaderApp stops the app and kills whatever outlives the stop, then
// starts it again.
func (h *SystemdHost) RestartReaderApp(ctx context.Context) error {
	procs, ok := h.procsFile()
	if !ok {
		// Without the cgroup there is no way to tell whether the old java has
		// exited, so a start could create a second instance.
		return fmt.Errorf("readerd: cannot find the %s cgroup; not restarting the app blind", readerAppUnit)
	}
	if err := h.run(ctx, "systemctl", "stop", readerAppUnit); err != nil {
		h.log.Warn().Err(err).Msg("stopping the reader app failed; killing its processes directly")
	}
	for _, step := range []struct {
		sig  syscall.Signal
		wait time.Duration
	}{{syscall.SIGTERM, appTermWait}, {syscall.SIGKILL, appKillWait}} {
		left, err := h.signalAndWait(procs, step.sig, step.wait)
		if err != nil {
			return err
		}
		if len(left) == 0 {
			return h.run(ctx, "systemctl", "start", readerAppUnit)
		}
		h.log.Warn().Ints("pids", left).Str("signal", step.sig.String()).Msg("reader app processes still running")
	}
	return fmt.Errorf("readerd: reader app processes survived SIGKILL; not starting a second instance")
}

// signalAndWait signals every pid in the cgroup, then polls until it is empty or
// wait elapses. It returns the pids still present.
func (h *SystemdHost) signalAndWait(procsPath string, sig syscall.Signal, wait time.Duration) ([]int, error) {
	pids, err := h.readProcs(procsPath)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil
	}
	for _, pid := range pids {
		if err := h.kill(pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			h.log.Warn().Err(err).Int("pid", pid).Msg("signal reader app process")
		}
	}
	for waited := time.Duration(0); waited < wait; waited += appPoll {
		if pids, err = h.readProcs(procsPath); err != nil || len(pids) == 0 {
			return pids, err
		}
		h.sleep(appPoll)
	}
	return h.readProcs(procsPath)
}

// Reboot flushes the filesystems and asks systemd for a clean reboot, which
// stops the vendor app the orderly way. It never power-resets.
func (h *SystemdHost) Reboot() error {
	h.sync()
	return h.run(context.Background(), "systemctl", "reboot")
}
