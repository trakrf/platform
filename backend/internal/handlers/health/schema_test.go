package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/migrations"
)

// A backend serving a schema older than the migrations it was built with is the
// failure TRA-1190 was filed for. It presented as 89 identical e2e failures:
// the database was at 38, `000040_users_must_change_password` was unapplied,
// login 500'd on `column "must_change_password" does not exist`, and every spec
// that logs in timed out at exactly 11.3s.
//
// Every cheap check passed while that was true — /health 200, signup 201 —
// because nothing else touched the new column. So the tell has to be something
// a suite can read before it starts, which is what these tests pin.

func newTestHandler(t *testing.T, read SchemaReader) *Handler {
	t.Helper()
	h := NewHandler(nil, buildinfo.Info{Version: "test"}, time.Now())
	h.readSchema = read
	return h
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
	}
	return resp
}

func get(t *testing.T, h *Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	return rec
}

// deref fails the test rather than panicking, so a version that went missing
// reports as the assertion it broke instead of a nil dereference three frames up.
func deref(t *testing.T, label string, v *uint) uint {
	t.Helper()
	if v == nil {
		t.Fatalf("%s is absent, want a version", label)
	}
	return *v
}

func TestHealth_SchemaBehindFails(t *testing.T) {
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatalf("migrations.Latest(): %v", err)
	}

	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		return latest - 2, false, nil
	})
	rec := get(t, h)

	// 503, not 200. "Refuse to look healthy" is the whole point: a 200 here is
	// what let a stale database survive two full characterisation runs.
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	resp := decode(t, rec)
	if resp.Status != "schema_behind" {
		t.Errorf("status = %q, want schema_behind", resp.Status)
	}
	if resp.Schema == nil {
		t.Fatal("schema block missing — the versions are the actionable part")
	}
	if !resp.Schema.Readable {
		t.Error("readable = false, want true — the ledger was read successfully")
	}
	applied := deref(t, "applied", resp.Schema.Applied)
	expected := deref(t, "expected", resp.Schema.Expected)
	if applied != latest-2 || expected != latest {
		t.Errorf("schema = %d/%d, want %d/%d", applied, expected, latest-2, latest)
	}
	// Naming the pending migrations is deliberate. "38 vs 40" makes the reader
	// go and look up what 39 and 40 were, and nobody did.
	if len(resp.Schema.Pending) != 2 {
		t.Errorf("pending = %v, want 2 entries", resp.Schema.Pending)
	}
}

func TestHealth_SchemaCurrentPasses(t *testing.T) {
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatalf("migrations.Latest(): %v", err)
	}

	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		return latest, false, nil
	})
	rec := get(t, h)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	resp := decode(t, rec)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Schema == nil {
		t.Fatalf("schema block missing, want applied=%d", latest)
	}
	if !resp.Schema.Readable {
		t.Error("readable = false, want true")
	}
	if applied := deref(t, "applied", resp.Schema.Applied); applied != latest {
		t.Errorf("applied = %d, want %d", applied, latest)
	}
	if len(resp.Schema.Pending) != 0 {
		t.Errorf("pending = %v, want empty", resp.Schema.Pending)
	}
	if resp.Schema.Reason != "" {
		t.Errorf("reason = %q, want empty on a healthy read", resp.Schema.Reason)
	}
}

// A pod older than the schema is a normal rolling deploy, not a fault: helm runs
// the migrate Job as a pre-upgrade hook, so every old replica serves an ahead
// schema for the length of the rollout. Failing here would turn each deploy into
// an outage.
func TestHealth_SchemaAheadStaysHealthy(t *testing.T) {
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatalf("migrations.Latest(): %v", err)
	}

	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		return latest + 3, false, nil
	})
	rec := get(t, h)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (a rollout is not a fault)", rec.Code)
	}
	if resp := decode(t, rec); resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

// A dirty ledger means a migration aborted partway: the schema is in neither the
// old shape nor the new one. That is the least healthy state of the three.
func TestHealth_DirtyLedgerFails(t *testing.T) {
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatalf("migrations.Latest(): %v", err)
	}

	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		return latest, true, nil
	})
	rec := get(t, h)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	resp := decode(t, rec)
	if resp.Status != "schema_dirty" {
		t.Errorf("status = %q, want schema_dirty", resp.Status)
	}
	if resp.Schema == nil || !resp.Schema.Dirty {
		t.Errorf("schema = %+v, want dirty=true", resp.Schema)
	}
	if !resp.Schema.Readable {
		t.Error("readable = false, want true — a dirty ledger is one that WAS read")
	}
}

// Not being able to READ the ledger is not evidence that the schema is behind,
// and must not be reported as though it were — that would turn every transient
// database blip into a false "run your migrations". The database field already
// carries connectivity.
//
// But it must still be SAID. Omitting the block made "I cannot tell" byte-for-byte
// identical to "everything is fine", and that is how TRA-1190's check ran inert in
// preview and prod for a month: `trakrf-app` had no SELECT on
// `trakrf.schema_migrations`, every read errored, and /health answered 200 with no
// schema key — exactly what a healthy backend answers (TRA-1218). The 200 was right;
// the silence was not.
func TestHealth_UnreadableLedgerIsReportedNotOmitted(t *testing.T) {
	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		// The error preview and prod actually produced, not a stand-in.
		return 0, false, errors.New(
			`ERROR: permission denied for table schema_migrations (SQLSTATE 42501)`)
	})
	rec := get(t, h)

	// Still 200: an unreadable ledger is not evidence of drift.
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	resp := decode(t, rec)
	if resp.Status == "schema_behind" {
		t.Error("an unreadable ledger was reported as schema_behind")
	}

	// The assertion that would have caught TRA-1218. Everything above this line
	// passed while the check was inert.
	if resp.Schema == nil {
		t.Fatal("schema block omitted — that is indistinguishable from a healthy " +
			"response, which is the whole defect")
	}
	if resp.Schema.Readable {
		t.Error("readable = true, want false")
	}
	if resp.Schema.Reason == "" {
		t.Error("reason is empty — \"readable: false\" alone does not say whether the " +
			"ledger is missing or the grant is")
	}
	// No invented versions. A zero here would read as "applied 0 of N", which is a
	// claim about the database rather than an admission about the read.
	if resp.Schema.Applied != nil || resp.Schema.Expected != nil {
		t.Errorf("applied/expected = %v/%v, want both absent when the ledger was not read",
			resp.Schema.Applied, resp.Schema.Expected)
	}
}

// The unit-test path: no pool at all. Must not panic and must not invent a
// verdict about a database it never spoke to.
//
// This one keeps omitting the block rather than reporting readable:false, and the
// distinction is the point: there is no database here to be unable to read, so
// "the ledger is unreadable" would be a claim about a read that was never
// attempted. A real server always passes a live pool.
func TestHealth_NoPoolOmitsSchema(t *testing.T) {
	h := NewHandler(nil, buildinfo.Info{Version: "test"}, time.Now())
	rec := get(t, h)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if resp := decode(t, rec); resp.Schema != nil {
		t.Errorf("schema = %+v, want omitted with no pool", resp.Schema)
	}
}

// /healthz and /readyz are the k8s liveness and readiness probes. They are
// deliberately NOT affected: a behind schema must not evict pods or pull them
// out of the load balancer, because the repair for it is a migration, and a
// pod that has been killed cannot serve while that runs.
func TestProbes_UnaffectedBySchemaState(t *testing.T) {
	h := newTestHandler(t, func(context.Context) (uint, bool, error) {
		return 1, true, nil // behind AND dirty
	})

	rec := httptest.NewRecorder()
	h.Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200 — liveness must not follow schema state", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Errorf("/healthz body = %q, want \"ok\"", got)
	}
}
