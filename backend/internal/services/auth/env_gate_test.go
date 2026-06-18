package auth

import "testing"

func TestSignupAllowedInEnv(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		// Allowed: prod (real customers), prod alias, local dev (unset), and CI.
		{"", true},
		{"prod", true},
		{"production", true},
		{"test", true},
		// Blocked: every non-prod DEPLOYED env, and anything unrecognized
		// (fail toward blocking so a stray env can't leak self-service signup).
		{"preview", false},
		{"demo", false},
		{"staging", false},
		{"dev", false},
		{"something-else", false},
	}
	for _, c := range cases {
		if got := signupAllowedInEnv(c.env); got != c.want {
			t.Errorf("signupAllowedInEnv(%q) = %v, want %v", c.env, got, c.want)
		}
	}
}
