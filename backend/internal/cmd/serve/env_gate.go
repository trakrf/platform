package serve

// testAffordanceEnvs lists the APP_ENV values where developer/test-only routes
// may be mounted: the /test/* handler (incl. POST /test/apikeys, which mints
// API keys) and the schemathesis rate-limit bypass.
//
// Fail-CLOSED (TRA-861): production — APP_ENV="prod" (the deploy chart's prod
// env key) or "production" — and any UNRECOGNIZED env get NO test affordances.
// The previous `APP_ENV != "production"` gate was fail-open and, because the
// chart emits APP_ENV="prod" (not "production"), left /test/apikeys mounted on
// the public prod host — a key-minting hole.
//
// COUPLING TO INFRA ENV KEYS (argocd/root/values.yaml): this set must include
// every infra env KEY that should expose /test — currently only "preview".
// "prod" must NEVER be added. A new public/non-prod env key defaults to NO test
// affordances; coordinate with infra before adding one here.
var testAffordanceEnvs = map[string]bool{
	"":            true, // local development (APP_ENV unset)
	"test":        true, // CI / contract-test harness
	"preview":     true, // preview proving ground (infra env key)
	"development": true,
	"dev":         true,
	"local":       true,
}

// testAffordancesAllowed reports whether developer/test-only routes may mount in
// the given APP_ENV. Fail-closed: production and any unrecognized env → false.
func testAffordancesAllowed(appEnv string) bool {
	return testAffordanceEnvs[appEnv]
}
