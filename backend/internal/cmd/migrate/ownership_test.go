package migrate

import (
	"strings"
	"testing"
)

// The message is the whole deliverable of this preflight. Whoever reads it is
// looking at a failed migrate Job with no pod logs (they are auto-cleaned), so it
// has to carry the offending object, why the migration cannot fix itself, and the
// exact statement that repairs it.

func TestOwnershipDriftError_NamesObjectOwnerAndRepair(t *testing.T) {
	err := ownershipDriftError([]ownershipDrift{{
		kind:  "function",
		name:  "trakrf.normalize_tag_value(text)",
		owner: "postgres",
	}}, "trakrf-migrate")

	if err == nil {
		t.Fatal("expected an error for a drifted object, got nil")
	}
	msg := err.Error()

	for _, want := range []string{
		"trakrf.normalize_tag_value(text)",
		"postgres",
		"trakrf-migrate",
		`ALTER FUNCTION trakrf.normalize_tag_value(text) OWNER TO "trakrf-migrate";`,
		"TRA-1104",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\ngot:\n%s", want, msg)
		}
	}
}

// A repair line is only useful if it is valid DDL for that object kind. ALTER
// TABLE against a view is a syntax error, which turns the one actionable line in
// the message into a second problem to debug.
func TestOwnershipDriftError_RepairVerbMatchesKind(t *testing.T) {
	cases := []struct {
		kind string
		name string
		want string
	}{
		{"table", "trakrf.assets", `ALTER TABLE trakrf.assets OWNER TO "trakrf-migrate";`},
		{"view", "trakrf.asset_scan_latest", `ALTER VIEW trakrf.asset_scan_latest OWNER TO "trakrf-migrate";`},
		{"materialized view", "trakrf.rollup", `ALTER MATERIALIZED VIEW trakrf.rollup OWNER TO "trakrf-migrate";`},
		{"sequence", "trakrf.assets_seq", `ALTER SEQUENCE trakrf.assets_seq OWNER TO "trakrf-migrate";`},
		{"function", "trakrf.f(text)", `ALTER FUNCTION trakrf.f(text) OWNER TO "trakrf-migrate";`},
		{"procedure", "trakrf.p()", `ALTER PROCEDURE trakrf.p() OWNER TO "trakrf-migrate";`},
		{"type", "trakrf.mood", `ALTER TYPE trakrf.mood OWNER TO "trakrf-migrate";`},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			err := ownershipDriftError(
				[]ownershipDrift{{kind: tc.kind, name: tc.name, owner: "postgres"}},
				"trakrf-migrate")
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected repair line %q\ngot:\n%s", tc.want, err.Error())
			}
		})
	}
}

// Every drifted object needs its own repair line. Reporting only the first sends
// the operator round the loop once per object, each time waiting for a full
// deploy to discover the next one.
func TestOwnershipDriftError_ReportsEveryObject(t *testing.T) {
	err := ownershipDriftError([]ownershipDrift{
		{kind: "function", name: "trakrf.a(text)", owner: "postgres"},
		{kind: "table", name: "trakrf.b", owner: "someone_else"},
	}, "trakrf-migrate")

	msg := err.Error()
	for _, want := range []string{
		`ALTER FUNCTION trakrf.a(text) OWNER TO "trakrf-migrate";`,
		`ALTER TABLE trakrf.b OWNER TO "trakrf-migrate";`,
		"someone_else",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\ngot:\n%s", want, msg)
		}
	}
}

// The repair needs rights the migrating role does not have — that is the whole
// trap. Saying so stops the reader trying the ALTER as the migrate role, getting
// the same "must be owner" error, and concluding the message is wrong.
func TestOwnershipDriftError_SaysRepairNeedsSuperuser(t *testing.T) {
	err := ownershipDriftError(
		[]ownershipDrift{{kind: "function", name: "trakrf.f(text)", owner: "postgres"}},
		"trakrf-migrate")

	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "superuser") && !strings.Contains(msg, "owner") {
		t.Errorf("message should say the repair needs superuser/owner rights, got:\n%s", err.Error())
	}
}

// An unknown relkind must still produce a usable line rather than a malformed
// one. Postgres gains object kinds; this preflight should degrade to "tell the
// human what and who" rather than emit invalid DDL.
func TestOwnershipDriftError_UnknownKindStillReportsObject(t *testing.T) {
	err := ownershipDriftError(
		[]ownershipDrift{{kind: "widget", name: "trakrf.thing", owner: "postgres"}},
		"trakrf-migrate")

	msg := err.Error()
	if !strings.Contains(msg, "trakrf.thing") || !strings.Contains(msg, "postgres") {
		t.Errorf("unknown kind should still name object and owner, got:\n%s", msg)
	}
	if strings.Contains(msg, "ALTER WIDGET") {
		t.Errorf("should not invent DDL for an unknown kind, got:\n%s", msg)
	}
}
