# TRA-922 MQTT Topic Routing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Force an `{org_slug}/` prefix on new `publish_topic`s, and replace the static `MQTT_TOPIC` broker filter with data-driven per-topic subscriptions driven by a `topicroute` registry.

**Architecture:** A process-wide `topicroute.Registry` owns both the in-memory `publish_topic → ScanRoute` map (message routing) and the broker subscription set. It reconciles against a new `list_active_scan_topics()` SECURITY DEFINER function on boot, on scan-device CRUD, on a periodic ticker, and on every MQTT (re)connect. Scan-device create/edit enforce the `{org_slug}/` prefix at the handler layer (no DB constraint; existing rows grandfathered).

**Tech Stack:** Go (pgx, paho.mqtt.golang, chi, testify), TimescaleDB/Postgres, React/TypeScript (zustand, vitest).

**Spec:** `docs/superpowers/specs/2026-06-09-tra-922-mqtt-topic-routing-design.md`

---

### Task 1: `list_active_scan_topics()` DB function + `storage.ListScanTopics`

**Files:**
- Create: `backend/migrations/000021_list_active_scan_topics.up.sql`
- Create: `backend/migrations/000021_list_active_scan_topics.down.sql`
- Modify: `backend/internal/storage/ingest.go` (add `ListScanTopics`)
- Test: `backend/internal/storage/ingest_integration_test.go` (add `TestListScanTopics`)

- [ ] **Step 1: Write the migration up/down**

`000021_list_active_scan_topics.up.sql`:
```sql
-- TRA-922: full active-topic set for the ingest subscription registry.
-- SECURITY DEFINER so the RLS-enforced trakrf-app role can list every org's
-- topics at boot/reconcile with no org context (same pattern as resolve_scan_topic).
CREATE OR REPLACE FUNCTION trakrf.list_active_scan_topics()
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type, publish_topic text)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT d.org_id, d.id, d.type, d.publish_topic
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND d.transport = 'mqtt'
      AND d.publish_topic IS NOT NULL;
$$;
```
`000021_list_active_scan_topics.down.sql`:
```sql
DROP FUNCTION IF EXISTS trakrf.list_active_scan_topics();
```

- [ ] **Step 2: Add `ListScanTopics` to storage** (in `ingest.go`, after `ResolveScanTopic`)

```go
// ListScanTopics returns every live mqtt device's publish_topic mapped to its
// route, for the subscription registry (TRA-922). SECURITY DEFINER, so no org
// context is needed.
func (s *Storage) ListScanTopics(ctx context.Context) (map[string]ScanRoute, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT org_id, scan_device_id, device_type, publish_topic FROM trakrf.list_active_scan_topics()`)
	if err != nil {
		return nil, fmt.Errorf("list scan topics: %w", err)
	}
	defer rows.Close()
	out := map[string]ScanRoute{}
	for rows.Next() {
		var r ScanRoute
		var topic string
		if err := rows.Scan(&r.OrgID, &r.ScanDeviceID, &r.DeviceType, &topic); err != nil {
			return nil, fmt.Errorf("scan scan topic row: %w", err)
		}
		out[topic] = r
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Write integration test** (`TestListScanTopics`) — register two mqtt devices (distinct topics) + one web_ble device (no topic); assert the map has exactly the two mqtt topics with correct org/device/type, and excludes the web_ble device. Soft-delete one mqtt device; assert it drops out. Follow the existing `registerDevice`/helpers used by `TestResolveScanTopic_ByPublishTopic`.

- [ ] **Step 4: Run** `just backend test-integration` (or the integration target) for `TestListScanTopics`. Expected: PASS.

- [ ] **Step 5: Commit** `feat(tra-922): list_active_scan_topics fn + storage.ListScanTopics`

---

### Task 2: `topicroute.Registry` package

**Files:**
- Create: `backend/internal/services/topicroute/registry.go`
- Test: `backend/internal/services/topicroute/registry_test.go`

- [ ] **Step 1: Write failing unit tests** (`registry_test.go`) with a fake lister + fake manager:

```go
package topicroute

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeLister struct{ m map[string]storage.ScanRoute; err error }
func (f *fakeLister) ListScanTopics(context.Context) (map[string]storage.ScanRoute, error) { return f.m, f.err }

type fakeMgr struct{ subs, unsubs []string }
func (f *fakeMgr) Subscribe(t string)   { f.subs = append(f.subs, t) }
func (f *fakeMgr) Unsubscribe(t string) { f.unsubs = append(f.unsubs, t) }

func TestReconcile_AddsAndLooksUp(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"org-a/dock-1/reads": {OrgID: 1, ScanDeviceID: 10, DeviceType: "csl_cs463"}}}
	mgr := &fakeMgr{}
	r := NewRegistry(l, testLogger())
	r.SetManager(mgr)
	assert.NoError(t, r.Reconcile(context.Background()))
	got, ok := r.Lookup("org-a/dock-1/reads")
	assert.True(t, ok)
	assert.Equal(t, 10, got.ScanDeviceID)
	assert.Equal(t, []string{"org-a/dock-1/reads"}, mgr.subs)
}

func TestReconcile_RemovesGoneTopics(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"org-a/d/reads": {OrgID: 1, ScanDeviceID: 1}}}
	mgr := &fakeMgr{}
	r := NewRegistry(l, testLogger())
	r.SetManager(mgr)
	_ = r.Reconcile(context.Background())
	l.m = map[string]storage.ScanRoute{} // device deleted
	_ = r.Reconcile(context.Background())
	_, ok := r.Lookup("org-a/d/reads")
	assert.False(t, ok)
	assert.Equal(t, []string{"org-a/d/reads"}, mgr.unsubs)
}

func TestReconcile_NoManagerIsMapOnly(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"o/d/reads": {ScanDeviceID: 5}}}
	r := NewRegistry(l, testLogger()) // no SetManager
	assert.NoError(t, r.Reconcile(context.Background()))
	_, ok := r.Lookup("o/d/reads")
	assert.True(t, ok)
}

func TestTopicsSnapshot(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"a/x/reads": {}, "b/y/reads": {}}}
	r := NewRegistry(l, testLogger())
	_ = r.Reconcile(context.Background())
	assert.ElementsMatch(t, []string{"a/x/reads", "b/y/reads"}, r.Topics())
}
```
(`testLogger()` returns a discard `zerolog.Logger`; define a small helper in the test file.)

- [ ] **Step 2: Run** `go test ./internal/services/topicroute/...` Expected: FAIL (package missing).

- [ ] **Step 3: Implement `registry.go`**

```go
// Package topicroute owns the in-memory publish_topic -> ScanRoute map used to
// route incoming MQTT reads, AND the broker subscription set those topics imply.
// One structure, two jobs: the set of map keys is exactly the set of topics the
// subscriber subscribes to. Reconcile() re-derives both from the DB (TRA-922).
package topicroute

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/storage"
)

// TopicLister is the storage dependency (satisfied by *storage.Storage).
type TopicLister interface {
	ListScanTopics(ctx context.Context) (map[string]storage.ScanRoute, error)
}

// SubscriptionManager applies subscription deltas to the live broker client.
// Implemented by *ingest.Subscriber; nil until a subscriber attaches (MQTT off).
type SubscriptionManager interface {
	Subscribe(topic string)
	Unsubscribe(topic string)
}

type Registry struct {
	lister TopicLister
	log    zerolog.Logger
	mu     sync.RWMutex
	routes map[string]storage.ScanRoute
	mgr    SubscriptionManager
}

func NewRegistry(lister TopicLister, log zerolog.Logger) *Registry {
	return &Registry{lister: lister, log: log.With().Str("component", "topicroute").Logger(), routes: map[string]storage.ScanRoute{}}
}

func (r *Registry) SetManager(m SubscriptionManager) {
	r.mu.Lock()
	r.mgr = m
	r.mu.Unlock()
}

// Lookup returns the route for a topic from the in-memory map (message path).
func (r *Registry) Lookup(topic string) (storage.ScanRoute, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.routes[topic]
	return rt, ok
}

// Topics returns a snapshot of all known topics (for OnConnect bulk-subscribe).
func (r *Registry) Topics() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.routes))
	for t := range r.routes {
		out = append(out, t)
	}
	return out
}

// Reconcile re-derives the map from the DB and applies the add/remove deltas to
// the subscription manager (if attached). Safe to call on boot (no manager =>
// map-only), on CRUD, and on a ticker.
func (r *Registry) Reconcile(ctx context.Context) error {
	fresh, err := r.lister.ListScanTopics(ctx)
	if err != nil {
		return err
	}
	var toSub, toUnsub []string
	r.mu.Lock()
	for topic := range r.routes {
		if _, ok := fresh[topic]; !ok {
			delete(r.routes, topic)
			toUnsub = append(toUnsub, topic)
		}
	}
	for topic, route := range fresh {
		if _, ok := r.routes[topic]; !ok {
			toSub = append(toSub, topic)
		}
		r.routes[topic] = route // refresh route even if topic unchanged
	}
	mgr := r.mgr
	r.mu.Unlock()
	if mgr != nil {
		for _, t := range toSub {
			mgr.Subscribe(t)
		}
		for _, t := range toUnsub {
			mgr.Unsubscribe(t)
		}
	}
	if len(toSub) > 0 || len(toUnsub) > 0 {
		r.log.Info().Int("added", len(toSub)).Int("removed", len(toUnsub)).Msg("topic registry reconciled")
	}
	return nil
}
```

- [ ] **Step 4: Run** `go test ./internal/services/topicroute/...` Expected: PASS.
- [ ] **Step 5: Commit** `feat(tra-922): topicroute registry (routing map + subscription set)`

---

### Task 3: Subscriber → data-driven subscriptions; retire `MQTT_TOPIC`

**Files:**
- Modify: `backend/internal/ingest/subscriber.go`
- Modify: `backend/internal/ingest/config.go`
- Modify: `backend/internal/ingest/config_test.go`
- Modify: `backend/internal/cmd/serve/serve.go` (construct registry + ticker; pass registry to subscriber)
- Modify: `deploy/edge/.env.example`

- [ ] **Step 1: Update `config.go`** — remove the `Topic` field, its env read, and the `trakrf.id/#` default. Keep `URL`, `ClientID`. Update the doc comment to note `MQTT_TOPIC` is retired (subscriptions are data-driven, TRA-922).

- [ ] **Step 2: Update `config_test.go`** — drop `MQTT_TOPIC` assertions/overrides; keep URL + ClientID default/override tests. Run `go test ./internal/ingest/ -run TestConfig` after Step 6.

- [ ] **Step 3: Rework `subscriber.go`:**
  - `NewSubscriber(cfg Config, store *storage.Storage, registry *topicroute.Registry, eval ReadEvaluator, feed ReadPublisher, log *zerolog.Logger)` — store `registry` on the struct and call `registry.SetManager(s)`.
  - Add methods implementing `SubscriptionManager`:
    ```go
    func (s *Subscriber) Subscribe(topic string) {
        if s.client == nil || !s.client.IsConnected() {
            return // OnConnect will (re)subscribe the full set from the registry
        }
        if tok := s.client.Subscribe(topic, 1, s.handleMessage); tok.Wait() && tok.Error() != nil {
            s.log.Error().Err(tok.Error()).Str("topic", topic).Msg("subscribe failed")
            return
        }
        s.log.Info().Str("topic", topic).Msg("subscribed")
    }
    func (s *Subscriber) Unsubscribe(topic string) {
        if s.client == nil || !s.client.IsConnected() {
            return
        }
        if tok := s.client.Unsubscribe(topic); tok.Wait() && tok.Error() != nil {
            s.log.Error().Err(tok.Error()).Str("topic", topic).Msg("unsubscribe failed")
        }
    }
    ```
  - Replace the `OnConnect` body: bulk-subscribe `s.registry.Topics()` via `SubscribeMultiple`:
    ```go
    SetOnConnectHandler(func(c mqtt.Client) {
        topics := s.registry.Topics()
        if len(topics) == 0 {
            s.log.Info().Msg("connected; no registered topics to subscribe")
            return
        }
        filters := make(map[string]byte, len(topics))
        for _, t := range topics {
            filters[t] = 1
        }
        if tok := c.SubscribeMultiple(filters, s.handleMessage); tok.Wait() && tok.Error() != nil {
            s.log.Error().Err(tok.Error()).Int("count", len(topics)).Msg("bulk subscribe failed")
            return
        }
        s.log.Info().Int("count", len(topics)).Msg("subscribed to registered topics")
    }).
    ```
  - In `handleMessage`, replace the `store.ResolveScanTopic` call with a registry lookup + fallback:
    ```go
    route, ok := s.registry.Lookup(topic)
    if !ok {
        var found bool
        route, found, err = s.store.ResolveScanTopic(ctx, topic) // defensive fallback
        if err != nil { /* existing error handling */ }
        if !found { /* existing unregistered-topic handling (log + return) */ }
    }
    ```
    (Adapt to the exact existing variable names/flow at `subscriber.go:124`.)
  - Remove logging of `s.cfg.Topic` in `Start()` (no longer a single topic).

- [ ] **Step 4: Update `serve.go`** — before the `mqttCfg` block, construct the registry unconditionally and do an initial load:
  ```go
  topicRegistry := topicroute.NewRegistry(store, log)
  if err := topicRegistry.Reconcile(ctx); err != nil {
      log.Warn().Err(err).Msg("initial topic registry load failed; will retry on ticker")
  }
  ```
  Inside `if mqttCfg.Enabled()`: pass `topicRegistry` to `ingest.NewSubscriber`, and start a reconcile ticker:
  ```go
  reconcileStop := make(chan struct{})
  go func() {
      t := time.NewTicker(5 * time.Minute)
      defer t.Stop()
      for {
          select {
          case <-reconcileStop:
              return
          case <-t.C:
              if err := topicRegistry.Reconcile(ctx); err != nil {
                  log.Warn().Err(err).Msg("topic registry reconcile failed")
              }
          }
      }
  }()
  defer close(reconcileStop)
  ```
  Keep `topicRegistry` in scope to pass to the scandevices handler in Task 4.

- [ ] **Step 5: Update `deploy/edge/.env.example`** — remove the `MQTT_TOPIC=trakrf.id/#` line; add a comment: `# MQTT_TOPIC retired (TRA-922): subscriptions are data-driven from registered publish_topics.`

- [ ] **Step 6: Run** `just backend build` (compiles serve + ingest) and `go test ./internal/ingest/...`. Expected: build OK, config tests PASS.

- [ ] **Step 7: Commit** `feat(tra-922): data-driven MQTT subscriptions; retire MQTT_TOPIC filter`

---

### Task 4: Prefix validation + registry reconcile on scan-device CRUD

**Files:**
- Modify: `backend/internal/handlers/scandevices/scandevices.go` (Handler gets registry; validate prefix; reconcile after mutation)
- Modify: `backend/internal/cmd/serve/serve.go` (`NewHandler(store, topicRegistry)`)
- Test: `backend/internal/handlers/scandevices/scandevices_prefix_integration_test.go` (new)

- [ ] **Step 1: Write failing integration tests** covering: (a) create mqtt device with `publish_topic` not starting `{slug}/` → 422; (b) create with correct prefix → 201; (c) create mqtt device for an org whose `identifier` is empty → 422; (d) PATCH a grandfathered device WITHOUT changing `publish_topic` → 200 (no prefix check); (e) PATCH changing `publish_topic` to a non-prefixed value → 422; (f) create web_ble device with no topic → 201 (no prefix check). Use the existing scandevices integration test harness/org fixtures.

- [ ] **Step 2: Run** the new test. Expected: FAIL (validation not implemented).

- [ ] **Step 3: Implement** in `scandevices.go`:
  - `Handler` struct gains `registry *topicroute.Registry`; `NewHandler(storage *storage.Storage, registry *topicroute.Registry) *Handler`.
  - Add a helper:
    ```go
    // requireTopicPrefix enforces the {org_slug}/ prefix on a publish_topic
    // (TRA-922). Empty topic => allowed (no-topic / grandfather). Returns a
    // user-facing message on violation.
    func (h *Handler) validateTopicPrefix(ctx context.Context, orgID int, transport, topic string) (string, bool) {
        if topic == "" || transport == "web_ble" {
            return "", true
        }
        org, err := h.storage.GetOrganizationByID(ctx, orgID)
        if err != nil || org == nil || org.Identifier == "" {
            return "organization has no identifier; cannot set a publish_topic", false
        }
        if !strings.HasPrefix(topic, org.Identifier+"/") {
            return "publish_topic must start with \"" + org.Identifier + "/\"", false
        }
        return "", true
    }
    ```
  - In `Create`: after `validate.Struct`, resolve transport (`req.Transport`, "" treated as mqtt → not web_ble) and topic (`deref(req.PublishTopic)`); call `validateTopicPrefix`; on failure `httputil.WriteJSONError(w, r, http.StatusUnprocessableEntity, modelerrors.ErrValidation, msg, reqID)`. After a successful create, call `h.reconcile(r.Context())` (best-effort; see Step 4).
  - In `Update`: only when `req.PublishTopic != nil` (topic is being changed), validate the new value (transport from `req.Transport` if set, else fetch the existing device's transport via `GetScanDeviceByID`). On failure → 422. After a successful update, `h.reconcile`.
  - In `Delete`: after success, `h.reconcile`.
    ```go
    func (h *Handler) reconcile(ctx context.Context) {
        if h.registry == nil {
            return
        }
        if err := h.registry.Reconcile(ctx); err != nil {
            // best-effort: the periodic ticker backstops; the mutation already succeeded
            _ = err
        }
    }
    ```
  - Confirm `modelerrors.ErrValidation` exists (else use the validation error constant already used by `RespondValidationError`).

- [ ] **Step 4: Update `serve.go`** — `scanDevicesHandler := scandeviceshandler.NewHandler(store, topicRegistry)`.

- [ ] **Step 5: Run** `just backend build` + the new integration test + existing `TestScanDevice_*`. Expected: PASS.

- [ ] **Step 6: Commit** `feat(tra-922): enforce {org_slug}/ publish_topic prefix; reconnect registry on CRUD`

---

### Task 5: Expose current org `identifier` on `/users/me`

**Files:**
- Modify: `backend/internal/models/organization/organization.go` (`UserOrgWithRole.Identifier`)
- Modify: `backend/internal/services/orgs/service.go` (populate it)
- Test: `backend/internal/services/orgs/service_test.go` (assert identifier present) OR a handler test that already exercises `/users/me`.

- [ ] **Step 1: Write/extend a failing test** asserting `profile.CurrentOrg.Identifier` equals the org's identifier.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** — add `Identifier string \`json:"identifier"\`` to `UserOrgWithRole`. In `service.go` where `CurrentOrg` is built (around line 139), fetch the identifier: call `s.storage.GetOrganizationByID(ctx, currentOrgID)` and set `Identifier: org.Identifier` (the `org` in the loop is a `UserOrg` with only id/name, so use the storage fetch). Guard nil.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat(tra-922): include org identifier in /users/me current_org`

---

### Task 6: Backend `readerKeyFromTopic` — relax the fixed root

**Files:**
- Modify: `backend/internal/services/readstream/tracker.go:340`
- Test: `backend/internal/services/readstream/tracker_test.go`

- [ ] **Step 1: Add a failing test** asserting `readerKeyFromTopic("organized-chaos/dock-1/reads") == "dock-1"` and the grandfathered `readerKeyFromTopic("trakrf.id/dock-1/reads") == "dock-1"` still holds. (`readerKeyFromTopic` is unexported — add the test in-package.)
- [ ] **Step 2: Run** → FAIL on the new-scheme case.
- [ ] **Step 3: Change** the regex `^trakrf\.id/([^/]+)/reads$` → `^[^/]+/([^/]+)/reads$`. Update the doc comment to "extracts the reader key from a `{prefix}/{key}/reads` topic".
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat(tra-922): readerKeyFromTopic accepts any prefix segment`

---

### Task 7: Frontend — prefill + relaxed topic parsing

**Files:**
- Modify: `frontend/src/types/org/index.ts` (`UserOrgWithRole.identifier`)
- Modify: `frontend/src/lib/scandevices/deviceProfile.ts` (`TOPIC_RE`)
- Modify: `frontend/src/lib/scandevices/deviceProfile.test.ts`
- Modify: `frontend/src/components/scandevices/ScanDeviceForm.tsx` (prefill + help text)

- [ ] **Step 1: Add `identifier: string` to `UserOrgWithRole`** in `types/org/index.ts`.
- [ ] **Step 2: Relax `TOPIC_RE`** to `/^[^/]+\/([^/]+)\/reads$/`; update the doc comment. Add/extend `deviceProfile.test.ts`: `readerKeyFromTopic('organized-chaos/dock-1/reads')` → `'dock-1'`; grandfathered `trakrf.id/dock-1/reads` → `'dock-1'`. Run `pnpm test deviceProfile`.
- [ ] **Step 3: Prefill in `ScanDeviceForm.tsx`** — read the current org slug from the store (`useOrgStore((s) => s.currentOrg?.identifier)`). On create mode (no `device`), when transport is `mqtt` and `publish_topic` is empty, initialize it to `${slug}/`. Update the placeholder to `e.g., ${slug}/dock-reader-1/reads` (fallback to the generic example if slug is unavailable) and the help text to mention the required `{org}/` prefix. Do NOT prefill in edit mode (grandfathered values must be shown verbatim).
- [ ] **Step 4: Run** `just frontend test` (deviceProfile + any ScanDeviceForm tests) and `just frontend lint`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(tra-922): prefill {org_slug}/ in scan-device form; relaxed topic parse`

---

### Task 8: Full validation, Linear, PR

- [ ] **Step 1:** `just lint` (backend + frontend). Fix any findings.
- [ ] **Step 2:** `just test` (backend unit + frontend) and the integration suite. Confirm green; capture output.
- [ ] **Step 3:** If the OpenAPI spec is generated and `/users/me` schema changed, run `just backend api-spec` and commit drift.
- [ ] **Step 4:** Push the branch; open a PR (merge-commit, not squash) titled `feat(tra-922): MQTT topic routing — {org_slug}/ prefix + data-driven subscriptions`. Body: summary, the four decisions, test evidence, and an explicit **infra/ops note**: the live edge box `.env` should drop `MQTT_TOPIC` (subscriptions are now data-driven); grandfathered demo topics keep working unchanged.
- [ ] **Step 5:** Comment on TRA-922 in Linear with the PR link and the grandfather/edge-env note. **Hold for review — do not merge.**

---

## Self-review notes
- **Spec coverage:** D1 prefix → Task 4 (+ frontend prefill Task 7, slug exposure Task 5); D2 immutability → no code (documented); D3 grandfather → Task 4 (validate only on change) + no data migration; D4 data-driven subs → Tasks 1–3; readerKey consequence → Tasks 6–7; performance/cache → the registry map (Task 2), no negative cache needed.
- **Type consistency:** `storage.ScanRoute{OrgID,ScanDeviceID,DeviceType}` reused throughout (ints, matches `ingest.go`). Registry depends on `TopicLister` (interface) + `SubscriptionManager` (interface, implemented by `*ingest.Subscriber`). No `topicroute → ingest` import (no cycle).
- **Build order:** signature changes (`NewSubscriber`, `NewHandler`) land with their `serve.go` updates in the same task to keep the tree compiling.
