package testutil

import "os"

// AllowDBSkipEnv is the one escape hatch for AllowDBSkip. Exported so failure
// messages can name it without the string drifting out of sync.
const AllowDBSkipEnv = "TRAKRF_ALLOW_DB_SKIP"

// AllowDBSkip reports whether a test may skip itself when no database is
// reachable. It may not, by default.
//
// TRA-1085: `go test -tags=integration ./internal/cmd/migrate/` once printed
// "ok ... 0.009s" and exited 0 while every test in it SKIPped on a bad
// password — only -v revealed it. That suite is part of the pre-release check,
// and a check that can pass by not running is worse than no check at all.
//
// Set TRAKRF_ALLOW_DB_SKIP=1 to opt back into skipping, for the case where
// running a suite without a database really is what you want. Exactly "1" —
// nothing else counts, so a guessed-at truthy value cannot quietly disable the
// guard for everyone.
func AllowDBSkip() bool {
	return os.Getenv(AllowDBSkipEnv) == "1"
}
