package serve

import "testing"

// TestTestAffordancesAllowed pins the fail-closed gate for developer/test-only
// routes (/test/* handler + schemathesis rate-limit bypass). Production —
// APP_ENV="prod" (the deploy chart's prod key) or "production" — and any
// unrecognized env must be DENIED; only explicit dev/test/preview envs allowed.
func TestTestAffordancesAllowed(t *testing.T) {
	allow := []string{"", "test", "preview", "development", "dev", "local"}
	deny := []string{"prod", "production", "staging", "prod-eu", "Production", "preprod"}

	for _, e := range allow {
		if !testAffordancesAllowed(e) {
			t.Errorf("APP_ENV=%q: test affordances should be ALLOWED", e)
		}
	}
	for _, e := range deny {
		if testAffordancesAllowed(e) {
			t.Errorf("APP_ENV=%q: test affordances must be DENIED (fail-closed)", e)
		}
	}
}
