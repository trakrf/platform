package testutil

import "testing"

// TRA-1085: the default has to be "fail", because the failure this guards
// against is a pre-release check reporting success by not running.
func TestAllowDBSkipDefaultsToFalse(t *testing.T) {
	t.Setenv("TRAKRF_ALLOW_DB_SKIP", "")

	if AllowDBSkip() {
		t.Fatal("AllowDBSkip() = true with the env var unset; an unreachable database must fail by default")
	}
}

func TestAllowDBSkipHonoursExplicitOptOut(t *testing.T) {
	t.Setenv("TRAKRF_ALLOW_DB_SKIP", "1")

	if !AllowDBSkip() {
		t.Fatal("AllowDBSkip() = false with TRAKRF_ALLOW_DB_SKIP=1")
	}
}

// Opting out is deliberately narrow: a truthy-looking value someone guessed at
// must not silently disable the guard.
func TestAllowDBSkipRejectsOtherValues(t *testing.T) {
	for _, v := range []string{"0", "true", "TRUE", "yes", "on", " 1", "1 "} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("TRAKRF_ALLOW_DB_SKIP", v)

			if AllowDBSkip() {
				t.Fatalf("AllowDBSkip() = true for %q; only \"1\" opts out", v)
			}
		})
	}
}
