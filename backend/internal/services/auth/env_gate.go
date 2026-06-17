package auth

import "errors"

// ErrSignupNotAllowed is returned by Signup when self-service (non-invitation)
// signup is blocked on the current environment (TRA-970). The handler maps it to
// a 403 "go to production" response. Invitation-based signup is never gated by it.
var ErrSignupNotAllowed = errors.New("signup_not_allowed")

// signupAllowedEnvs is an allowlist of APP_ENV values where self-service signup
// is permitted: production (the real funnel), its "production" alias, local dev
// (APP_ENV unset → ""), and the test/CI harness (APP_ENV="test"). Every other
// value — preview, demo, staging, or anything unrecognized — is blocked so that
// random visitors cannot spin up orgs on non-prod sites (TRA-970). Mirrors the
// prod detection in services/email/resend.go and fails toward blocking.
var signupAllowedEnvs = map[string]bool{
	"":           true,
	"prod":       true,
	"production": true,
	"test":       true,
}

// signupAllowedInEnv reports whether self-service signup is allowed for the given
// APP_ENV value.
func signupAllowedInEnv(appEnv string) bool {
	return signupAllowedEnvs[appEnv]
}
