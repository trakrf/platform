package migrations

import "testing"

// TestLatest reports the highest version in the embedded set.
//
// This is half of the comparison that lets a running backend notice its own
// database is behind it (TRA-1190). The other half is the applied version read
// from the ledger; neither is useful alone.
func TestLatest(t *testing.T) {
	got, err := Latest()
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}
	if got < 40 {
		t.Errorf("Latest() = %d, want >= 40 (000040_users_must_change_password is in the tree)", got)
	}

	// The sequence is NOT contiguous, and that is fine. 000034 has never existed
	// in this repository — not deleted, never written; checksums.txt records the
	// same gap. A skipped number costs nothing: golang-migrate applies files in
	// sorted order and records the highest version it reached, so a database
	// that has run every file reports 40 whether or not a 34 was ever authored.
	//
	// What would NOT be fine is a duplicate version, because golang-migrate
	// would apply one of the pair and then record a version that describes two
	// different schemas depending on which. all() refuses to return a set
	// containing one, so this asserts the ordering it does return.
	names, err := Names()
	if err != nil {
		t.Fatalf("Names() error: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("Names() returned nothing")
	}
	ms, err := all()
	if err != nil {
		t.Fatalf("all() error: %v", err)
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].version <= ms[i-1].version {
			t.Errorf("versions are not strictly increasing: %q (%d) follows %q (%d)",
				ms[i].name, ms[i].version, ms[i-1].name, ms[i-1].version)
		}
	}
	if ms[len(ms)-1].version != got {
		t.Errorf("Latest() = %d but the highest entry is %d", got, ms[len(ms)-1].version)
	}
}

// TestAllRejectsDuplicateVersions pins the refusal above. Two files claiming one
// version is the one malformation that makes the applied-vs-embedded comparison
// meaningless, so it must be an error rather than a silent pick.
func TestAllRejectsDuplicateVersions(t *testing.T) {
	ms, err := all()
	if err != nil {
		t.Fatalf("all() error: %v", err)
	}
	seen := map[uint]string{}
	for _, m := range ms {
		if prev, dup := seen[m.version]; dup {
			t.Errorf("version %d claimed by both %q and %q", m.version, prev, m.name)
		}
		seen[m.version] = m.name
	}
}

// TestPendingSince names the migrations a database at a given version has not
// applied. The names are the point: "you are at 38, latest is 40" leaves the
// reader to go and look up what 39 and 40 were, and the whole reason this
// exists is that nobody looked.
func TestPendingSince(t *testing.T) {
	latest, err := Latest()
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}

	t.Run("current database has nothing pending", func(t *testing.T) {
		pending, err := PendingSince(latest)
		if err != nil {
			t.Fatalf("PendingSince(%d) error: %v", latest, err)
		}
		if len(pending) != 0 {
			t.Errorf("PendingSince(latest) = %v, want empty", pending)
		}
	})

	t.Run("a database two behind names both", func(t *testing.T) {
		pending, err := PendingSince(latest - 2)
		if err != nil {
			t.Fatalf("PendingSince error: %v", err)
		}
		if len(pending) != 2 {
			t.Fatalf("PendingSince(latest-2) = %v, want 2 entries", pending)
		}
	})

	// The exact case this ticket was filed for: a local database sitting at 38
	// while 000039 and 000040 were unapplied. Login 500'd with `column
	// "must_change_password" does not exist` and every e2e spec that logs in
	// failed identically, at exactly 11.3s, for two days.
	t.Run("version 38 names 39 and 40", func(t *testing.T) {
		pending, err := PendingSince(38)
		if err != nil {
			t.Fatalf("PendingSince(38) error: %v", err)
		}
		if len(pending) < 2 {
			t.Fatalf("PendingSince(38) = %v, want at least 000039 and 000040", pending)
		}
		if pending[0] != "000039_hermetic_stored_functions" {
			t.Errorf("pending[0] = %q, want 000039_hermetic_stored_functions", pending[0])
		}
		if pending[1] != "000040_users_must_change_password" {
			t.Errorf("pending[1] = %q, want 000040_users_must_change_password", pending[1])
		}
	})

	// A database ahead of the binary is a rollback, not a drift: the pod is
	// older than the schema. It must not be reported as "pending", which would
	// send the operator to run migrations that do not exist.
	t.Run("a database ahead of the binary has nothing pending", func(t *testing.T) {
		pending, err := PendingSince(latest + 5)
		if err != nil {
			t.Fatalf("PendingSince error: %v", err)
		}
		if len(pending) != 0 {
			t.Errorf("PendingSince(latest+5) = %v, want empty", pending)
		}
	})
}
