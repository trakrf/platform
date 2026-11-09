package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const (
	defaultMaxBytes = 64 << 20 // the armv7 binary is ~10 MB
	downloadTimeout = 10 * time.Minute
	keepVersions    = 3
	// staleDownloadAge: a temp file untouched this long belongs to no live
	// download — ours is bounded by downloadTimeout, the guard's curl by 300 s.
	staleDownloadAge = downloadTimeout + 5*time.Minute
)

// ErrAlreadyCommitted: this process already committed its update. A reply that
// raced the committing one gets this, and it is not a failure.
var ErrAlreadyCommitted = errors.New("update: already committed")

// Updater is the daemon side of a remote update. It never executes the binary it
// downloads: the signature and hash are the whole check, and systemd starts it.
type Updater struct {
	Layout         Layout
	Keys           []TrustedKey
	Running        string // this process's version
	HTTP           *http.Client
	Restart        func() // asks the daemon to exit so systemd restarts it on the new link
	Log            zerolog.Logger
	MaxBytes       int64
	GuardTimerUnit string

	swap func(Layout, string) error // SwapLink; a seam for failure tests

	mu        sync.Mutex
	busy      bool
	attempt   *readerrpc.UpdateInfo // this process's own attempt, if any
	probation bool
	committed bool // this process ended its own probation
}

// New builds an Updater with production defaults.
func New(l Layout, running string, restart func(), log zerolog.Logger) *Updater {
	return &Updater{
		Layout:         l,
		Keys:           TrustedKeys,
		Running:        running,
		HTTP:           &http.Client{Timeout: downloadTimeout, Transport: downloadTransport()},
		Restart:        restart,
		Log:            log.With().Str("component", "update").Logger(),
		MaxBytes:       defaultMaxBytes,
		GuardTimerUnit: DefaultGuardTimerUnit,
		swap:           SwapLink,
	}
}

func refuse(format string, a ...any) error {
	return &readerrpc.UpdateRefusedError{Reason: fmt.Sprintf(format, a...)}
}

// CheckPending runs once at startup: it decides whether this process is an update
// on probation (see the package-level CheckPending), and clears temp files left
// by a download that a kill or power cut interrupted.
func (u *Updater) CheckPending() error {
	if err := CleanStale(u.Layout, staleDownloadAge); err != nil {
		u.Log.Warn().Err(err).Msg("clean abandoned downloads")
	}
	on, err := CheckPending(u.Layout, u.Running)
	u.mu.Lock()
	u.probation = on
	u.mu.Unlock()
	return err
}

// OnProbation reports whether this process still has to commit.
func (u *Updater) OnProbation() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.probation
}

// Preflight checks everything a future update from this build will need. It runs
// before every update and again before a commit, so a build whose own updater is
// broken never commits — the guard rolls it back instead.
func (u *Updater) Preflight() error {
	if len(u.Keys) == 0 {
		return errors.New("no trusted release keys compiled in")
	}
	if fi, err := os.Stat(u.Layout.Guard()); err != nil || fi.Mode()&0o111 == 0 {
		return fmt.Errorf("rollback guard %s is not installed (re-run install.sh)", u.Layout.Guard())
	}
	if _, err := os.Stat(u.GuardTimerUnit); err != nil {
		return fmt.Errorf("rollback guard timer %s is not installed (re-run install.sh)", u.GuardTimerUnit)
	}
	// Installed is not enough: a disabled timer never ticks, so a build that
	// never reaches the broker would never be rolled back. `systemctl enable`
	// records itself as this link, which needs no systemctl call to read.
	if _, err := os.Stat(guardTimerWants(u.GuardTimerUnit)); err != nil {
		return fmt.Errorf("rollback guard timer %s is not enabled (re-run install.sh)", u.GuardTimerUnit)
	}
	active, err := ActiveVersion(u.Layout)
	if err != nil {
		return err
	}
	if active != u.Running {
		return fmt.Errorf("active link points at %s but %s is running", active, u.Running)
	}
	probe, err := os.CreateTemp(u.Layout.VersionsDir(), ".preflight-*")
	if err != nil {
		return fmt.Errorf("versions dir is not writable: %w", err)
	}
	probe.Close()
	return os.Remove(probe.Name())
}

// Start verifies p and, if it is acceptable, starts the update in the background.
// Everything that can be checked without the download is checked here, so a bad
// request is refused in the RPC reply with nothing touched.
func (u *Updater) Start(p readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	switch {
	case u.busy:
		return readerrpc.DaemonUpdateResult{}, refuse("an update is already in progress")
	case u.probation:
		return readerrpc.DaemonUpdateResult{}, refuse("this build (%s) has not committed yet", u.Running)
	}
	if !ValidVersion(p.Version) {
		return readerrpc.DaemonUpdateResult{}, refuse("invalid version %q", p.Version)
	}
	if pu, err := url.Parse(p.URL); err != nil || pu.Scheme != "https" || pu.Host == "" {
		return readerrpc.DaemonUpdateResult{}, refuse("url must be https")
	}
	if p.Version == u.Running {
		return readerrpc.DaemonUpdateResult{}, refuse("%s is already running", p.Version)
	}
	if err := Verify(u.Keys, p); err != nil {
		return readerrpc.DaemonUpdateResult{}, refuse("%v", err)
	}
	if err := u.Preflight(); err != nil {
		return readerrpc.DaemonUpdateResult{}, refuse("%v", err)
	}
	u.busy = true
	u.attempt = &readerrpc.UpdateInfo{State: readerrpc.UpdateStateDownloading, Target: p.Version, At: nowStamp()}
	u.Log.Info().Str("from", u.Running).Str("to", p.Version).Str("url", p.URL).Msg("update accepted")
	go u.run(p)
	return readerrpc.DaemonUpdateResult{Accepted: true, From: u.Running, To: p.Version}, nil
}

func (u *Updater) setState(state, errMsg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.attempt.State = state
	u.attempt.Error = errMsg
	u.attempt.At = nowStamp()
	if state == readerrpc.UpdateStateFailed {
		u.busy = false
	}
}

func (u *Updater) run(p readerrpc.DaemonUpdateParams) {
	tmp := filepath.Join(u.Layout.VersionsDir(), ".mqtt-rpcd-"+p.Version+".download")
	fail := func(err error) {
		_ = os.Remove(tmp)
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		u.Log.Error().Err(err).Str("to", p.Version).Msg("update failed; nothing replaced")
		// Persist before publishing the state: anyone who sees "failed" must be
		// able to rely on it surviving a daemon restart.
		_ = writeLast(u.Layout, readerrpc.UpdateStateFailed, p.Version, msg)
		u.setState(readerrpc.UpdateStateFailed, msg)
	}

	if err := CleanStale(u.Layout, staleDownloadAge); err != nil {
		u.Log.Warn().Err(err).Msg("clean abandoned downloads")
	}
	if err := u.download(p.URL, tmp, p.SHA256); err != nil {
		fail(err)
		return
	}
	if err := os.Rename(tmp, u.Layout.VersionPath(p.Version)); err != nil {
		fail(err)
		return
	}
	// The rename must be durable before pending names it: otherwise a power cut
	// could leave a guard armed for a binary whose directory entry was lost.
	if err := syncDir(u.Layout.VersionsDir()); err != nil {
		fail(err)
		return
	}
	u.setState(readerrpc.UpdateStateInstalling, "")

	// Arm the guard BEFORE swapping: a crash between the two leaves pending
	// naming a version that is not running, which CheckPending clears. The
	// reverse order could leave a new binary running with no rollback armed.
	if err := WritePending(u.Layout, Pending{Target: p.Version, From: u.Running}); err != nil {
		fail(err)
		return
	}
	if err := u.swap(u.Layout, p.Version); err != nil {
		// The rename may have landed even though the call failed (e.g. the
		// directory fsync). If the link already points at the new build, it is
		// the build on probation: keep the guard armed and restart onto it.
		if active, aerr := ActiveVersion(u.Layout); aerr == nil && active == p.Version {
			u.Log.Warn().Err(err).Str("to", p.Version).Msg("link swap reported an error but the link moved; restarting under probation")
			u.Restart()
			return
		}
		_ = os.Remove(u.Layout.Pending())
		fail(err)
		return
	}
	if err := Prune(u.Layout, keepVersions, p.Version, u.Running); err != nil {
		u.Log.Warn().Err(err).Msg("prune old versions")
	}
	u.Log.Info().Str("from", u.Running).Str("to", p.Version).Msg("update installed; restarting onto the new version")
	u.Restart()
}

// download streams url into dst, hashing as it goes, and fails unless the bytes
// match wantSHA. Redirects are followed only to https.
func (u *Updater) download(rawURL, dst, wantSHA string) error {
	client := *u.HTTP
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to non-https url %s refused", req.URL.Redacted())
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: status %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, u.MaxBytes+1))
	if err == nil && n > u.MaxBytes {
		err = fmt.Errorf("download exceeds %d bytes", u.MaxBytes)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("sha256 mismatch: got %s", got)
	}
	return os.Chmod(dst, 0o755)
}

// Commit ends this build's probation, after checking that its own updater is
// usable. A commit that loses the race to the guard returns ErrNotPending; one
// that loses to this process's own earlier commit returns ErrAlreadyCommitted.
func (u *Updater) Commit() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.committed {
		return ErrAlreadyCommitted
	}
	if !u.probation {
		return ErrNotPending
	}
	if err := u.Preflight(); err != nil {
		return fmt.Errorf("update: not committing, updater preflight failed: %w", err)
	}
	if err := Commit(u.Layout, u.Running); err != nil {
		if errors.Is(err, ErrNotPending) {
			u.probation = false
		}
		return err
	}
	u.probation = false
	u.committed = true
	return nil
}

// Info reports this daemon's version and the latest update outcome.
func (u *Updater) Info() readerrpc.DaemonInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	info := readerrpc.DaemonInfo{Version: u.Running}
	switch {
	case u.attempt != nil:
		a := *u.attempt
		info.Update = &a
	case u.probation:
		info.Update = &readerrpc.UpdateInfo{State: readerrpc.UpdateStatePending, Target: u.Running}
	default:
		if last, ok := ReadLast(u.Layout); ok {
			info.Update = &last
		}
	}
	return info
}
