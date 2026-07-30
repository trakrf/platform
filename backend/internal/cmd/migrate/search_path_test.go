package migrate

import "testing"

// TestBuildPoolConfigImposesDDLSearchPath is the guard for ADR 0003: the runner,
// not the caller, decides the search_path that unqualified DDL resolves against.
//
// This is a unit test rather than an end-to-end placement assertion on purpose.
// Every migration 000001-000038 already carries its own
// `SET search_path = trakrf, public` header, so an end-to-end check of "did the
// tables land in trakrf" passes whether or not this override exists — it would be
// tautological. What actually needs guarding is that new, header-less migrations
// cannot have their DDL target decided by a role default or a DSN parameter.
func TestBuildPoolConfigImposesDDLSearchPath(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "DSN asks for a different path",
			url:  "postgres://u:p@localhost:5432/db?sslmode=disable&options=-c%20search_path%3Dpublic",
		},
		{
			name: "DSN asks for the same path",
			url:  "postgres://u:p@localhost:5432/db?sslmode=disable&options=-c%20search_path%3Dtrakrf,public",
		},
		{
			name: "DSN says nothing about search_path",
			url:  "postgres://u:p@localhost:5432/db?sslmode=disable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config, err := buildPoolConfig(tc.url)
			if err != nil {
				t.Fatalf("buildPoolConfig: %v", err)
			}

			got := config.ConnConfig.RuntimeParams["search_path"]
			if got != ddlSearchPath {
				t.Errorf("search_path = %q, want %q — the caller's connection string "+
					"must not decide where a migration's DDL lands", got, ddlSearchPath)
			}
		})
	}
}

// TestDDLSearchPathLeadsWithTrakrf guards the ordering. Unqualified CREATE
// resolves to the first *existing* schema on the path, so a path that led with
// public would put the schema's own tables in public.
func TestDDLSearchPathLeadsWithTrakrf(t *testing.T) {
	if ddlSearchPath != "trakrf, public" {
		t.Fatalf("ddlSearchPath = %q; trakrf must lead so unqualified DDL "+
			"lands in the application schema", ddlSearchPath)
	}
}

// TestLedgerSchemaIsPinned guards against the ledger schema being emptied out,
// which would return golang-migrate to CURRENT_SCHEMA() resolution and
// reintroduce TRA-1069's split history.
func TestLedgerSchemaIsPinned(t *testing.T) {
	if ledgerSchema == "" {
		t.Fatal("ledgerSchema is empty: golang-migrate would fall back to " +
			"CURRENT_SCHEMA() and the ledger could relocate between runs (TRA-1069)")
	}
}
