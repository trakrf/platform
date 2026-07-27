// Package capability holds the code-owned capability vocabulary (ADR 0002).
//
// A capability is a named grant that unlocks a use-case surface (routes + UI)
// for an org. A customer one-off is not an architectural category — it is a
// capability with a grant count of one — so nothing here distinguishes the two;
// the difference lives entirely in grant data.
//
// This registry is the ONLY place capability names are minted. The seeded
// `trakrf.capabilities` lookup table mirrors it, and a test pins the two in
// sync (capability_registry_integration_test.go in this package) so code and
// DDL cannot drift silently.
//
// Names describe workflows, never customers: `wip_tracking`, not `acmecorp`.
// Promotion from one-off to standard is then a grant-data change rather than a
// rename through DB values, JSON fields, logs, and error envelopes.
//
// There is deliberately no policy/presentation field. How an ungated capability
// presents — absent vs. locked — is a build-time frontend and spec concern
// (ADR 0002 §"Frontend"), not something the backend gets an opinion about.
// Asset management is the always-on base and is NOT listed here: it is never a
// grant, so a provisioning bug can degrade an org but never brick it.
package capability

const (
	// Geofence is the zone / enter-exit-dwell rule engine plus its alarm
	// output configuration — the customer-facing geofence surface. The
	// engine itself is core-as-infrastructure (mustering runs on it) and
	// the ingestion path it feeds from is never gated.
	Geofence = "geofence"

	// Inventory is expected-vs-observed reconciliation and count sessions
	// over fungible stock — quantity-of-class-at-location, not the Scan
	// tab, which is part of the ungated asset-management base. No routes
	// exist for it yet.
	Inventory = "inventory"

	// Mustering is the roster / muster-mode / presence-rollup surface built
	// on the geofence engine, with a distinct buyer persona and data
	// sensitivity.
	Mustering = "mustering"
)

// All is the complete capability vocabulary, sorted, matching the rows seeded
// into trakrf.capabilities by migration 000036.
var All = []string{Geofence, Inventory, Mustering}

// IsValid reports whether name is a known capability. Grants are additionally
// constrained by the lookup-table FK, so this is a convenience for callers that
// want to reject a bad name before touching the database — not the integrity
// guarantee.
func IsValid(name string) bool {
	for _, c := range All {
		if c == name {
			return true
		}
	}
	return false
}
