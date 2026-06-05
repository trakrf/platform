# TRA-924: Live Reads via backend SSE proxy (org-enforced) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the direct-browser MQTT Live Reads feed with an authenticated, org-filtered backend SSE endpoint that taps the existing in-process ingest read stream, and remove all broker creds/config from the browser.

**Architecture:** A new in-process `readstream.Broadcaster` (pub/sub keyed by org) is fed by the TRA-900 MQTT subscriber: right after `Parse()` produces `[]scanread.Read`, the subscriber publishes those reads under `route.OrgID`. An authenticated SSE handler (`GET /api/v1/reads/stream`) subscribes the caller to their own org's channel and streams JSON read events. The frontend swaps `useReaderFeed`'s mqtt.js client for a `fetch()`-based SSE reader that carries the existing JWT bearer (native `EventSource` can't set `Authorization`), feeding the unchanged `store`/`age` modules. The `window.__APP_CONFIG__.readerFeed` runtime config and the `mqtt` dependency are removed.

**Tech Stack:** Go (chi router, Eclipse Paho MQTT, zerolog), React/TypeScript (Vitest), SSE over `fetch` + `ReadableStream`.

**Out of scope (handed off / deferred):**
- Broker hardening (cluster-internal WSS listener, retire `frontend-readonly` user) → trakrf/infra, handed off via Linear comment (no cross-repo edits).
- Multi-replica fan-out → deferred, aligned to TRA-907; single-replica in-process fan-out is correct for today. Documented in code + PR.

**Key design decisions:**
1. **Tap parsed reads, not `res.Resolved`.** The Live Reads tab is a *coverage diagnostic* — it shows ALL reads (incl. unknown EPCs), so we tap the `[]scanread.Read` from `Parse()` (subscriber.go), not the membership-filtered `PersistReads` output.
2. **fetch-based SSE, not native EventSource.** Ticket says SSE "rides the existing JWT bearer via the API client"; native `EventSource` cannot set headers. Use `fetch()` streaming.
3. **Clear the per-request write deadline.** `http.Server.WriteTimeout` is 10s; long-lived SSE must clear it via `http.ResponseController.SetWriteDeadline(time.Time{})`.
4. **Non-blocking fan-out.** Per-client buffered channel; drop on full buffer so a slow browser never stalls ingestion.

---

## File Structure

**Backend (create):**
- `backend/internal/services/readstream/broadcaster.go` — pub/sub broadcaster + `ReadEvent` DTO + `readerKeyFromTopic`.
- `backend/internal/services/readstream/broadcaster_test.go`
- `backend/internal/handlers/readstream/readstream.go` — SSE handler + route registration.
- `backend/internal/handlers/readstream/readstream_test.go`

**Backend (modify):**
- `backend/internal/ingest/subscriber.go` — add `ReadPublisher` interface + `feed` field; publish parsed reads.
- `backend/internal/cmd/serve/serve.go` — construct broadcaster, wire to subscriber + handler; drop `READER_FEED_MQTT_*` env wiring.
- `backend/internal/cmd/serve/router.go` — register SSE route in the session-auth group.
- `backend/internal/handlers/frontend/frontend.go` — drop `ReaderFeedConfig` from injected app config.
- `backend/internal/handlers/frontend/frontend_test.go` — drop readerFeed assertions (if present).

**Frontend (create):**
- `frontend/src/lib/readerfeed/stream.ts` — fetch-based SSE reader → `ParsedRead[]`.
- `frontend/src/lib/readerfeed/stream.test.ts`

**Frontend (modify):**
- `frontend/src/hooks/readerfeed/useReaderFeed.ts` — consume SSE instead of MQTT.
- `frontend/src/lib/readerfeed/index.ts` — drop CS463/MQTT parse exports if removed.
- `frontend/src/lib/appConfig.ts` — drop `readerFeed` from `__APP_CONFIG__` type.
- `frontend/src/components/LiveReadsScreen.tsx` — adapt status/topic display.
- `frontend/package.json` — remove `mqtt` dependency.

**Frontend (delete):**
- `frontend/src/lib/readerfeed/config.ts` + its test — runtime broker config gone.
- `frontend/src/lib/readerfeed/parse.ts` + its test — CS463 parsing now server-side; readerKey arrives in the event.

---

## Task 1: Backend — `readstream.Broadcaster`

**Files:**
- Create: `backend/internal/services/readstream/broadcaster.go`
- Test: `backend/internal/services/readstream/broadcaster_test.go`

- [ ] **Step 1: Write failing tests** — org isolation, drop-on-full, unsubscribe stops delivery.

```go
package readstream

import (
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

func reads(epcs ...string) []scanread.Read {
	out := make([]scanread.Read, 0, len(epcs))
	for _, e := range epcs {
		out = append(out, scanread.Read{EPC: e, CapturePointName: "dock", AntennaPort: 1, RSSI: -55, ReaderTimestamp: time.UnixMilli(1_700_000_000_000)})
	}
	return out
}

func TestBroadcaster_OrgIsolation(t *testing.T) {
	b := New()
	chA, cancelA := b.Subscribe(1)
	defer cancelA()
	chB, cancelB := b.Subscribe(2)
	defer cancelB()

	b.Publish(1, "trakrf.id/dock-1/reads", reads("EPC-A"))

	select {
	case ev := <-chA:
		if ev.EPC != "EPC-A" || ev.ReaderKey != "dock-1" {
			t.Fatalf("org 1 got wrong event: %+v", ev)
		}
		if ev.ReaderTimestampMs != 1_700_000_000_000 {
			t.Fatalf("bad ts ms: %d", ev.ReaderTimestampMs)
		}
	case <-time.After(time.Second):
		t.Fatal("org 1 did not receive its event")
	}

	select {
	case ev := <-chB:
		t.Fatalf("org 2 leaked an event: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBroadcaster_DropsWhenFull(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe(1) // never drained
	defer cancel()
	// Far more than the buffer; must not block/panic.
	for i := 0; i < clientBuffer*4; i++ {
		b.Publish(1, "trakrf.id/r/reads", reads("E"))
	}
}

func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe(1)
	cancel()
	b.Publish(1, "trakrf.id/r/reads", reads("E"))
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("received after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestReaderKeyFromTopic(t *testing.T) {
	if got := readerKeyFromTopic("trakrf.id/dock-7/reads"); got != "dock-7" {
		t.Fatalf("got %q", got)
	}
	if got := readerKeyFromTopic("weird/topic"); got != "weird/topic" {
		t.Fatalf("fallback got %q", got)
	}
}
```

- [ ] **Step 2: Run, verify fail** — `just backend test ./internal/services/readstream/...` → FAIL (undefined `New`, etc.)

- [ ] **Step 3: Implement**

```go
// Package readstream fans out parsed MQTT reads to per-org SSE subscribers,
// in-process. Single-replica only: multi-replica fan-out needs a shared
// pub/sub or sticky sessions (deferred, aligned to TRA-907).
package readstream

import (
	"regexp"
	"sync"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// clientBuffer bounds per-subscriber queue depth. A browser that can't keep up
// drops reads rather than stalling ingestion — the feed is a live diagnostic,
// not a durable log.
const clientBuffer = 256

// ReadEvent is the wire shape streamed to the browser. JSON tags match the
// frontend ParsedRead interface so the client needs no field remapping.
type ReadEvent struct {
	EPC               string `json:"epc"`
	ReaderKey         string `json:"readerKey"`
	CapturePointName  string `json:"capturePointName"`
	AntennaPort       int    `json:"antennaPort"`
	RSSI              int    `json:"rssi"`
	ReaderTimestampMs int64  `json:"readerTimestampMs"`
}

type subscriber struct {
	ch chan ReadEvent
}

// Broadcaster is a concurrency-safe per-org pub/sub hub.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[int]map[*subscriber]struct{}
}

func New() *Broadcaster {
	return &Broadcaster{subs: make(map[int]map[*subscriber]struct{})}
}

// Subscribe registers a client for an org's read stream. The returned cancel
// func unsubscribes and closes the channel; it is safe to call once.
func (b *Broadcaster) Subscribe(orgID int) (<-chan ReadEvent, func()) {
	s := &subscriber{ch: make(chan ReadEvent, clientBuffer)}
	b.mu.Lock()
	if b.subs[orgID] == nil {
		b.subs[orgID] = make(map[*subscriber]struct{})
	}
	b.subs[orgID][s] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if set := b.subs[orgID]; set != nil {
				delete(set, s)
				if len(set) == 0 {
					delete(b.subs, orgID)
				}
			}
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Publish converts parsed reads to events and non-blocking-sends them to every
// subscriber of orgID. Implements ingest.ReadPublisher.
func (b *Broadcaster) Publish(orgID int, topic string, rs []scanread.Read) {
	b.mu.Lock()
	set := b.subs[orgID]
	if len(set) == 0 {
		b.mu.Unlock()
		return
	}
	targets := make([]*subscriber, 0, len(set))
	for s := range set {
		targets = append(targets, s)
	}
	b.mu.Unlock()

	key := readerKeyFromTopic(topic)
	for _, r := range rs {
		ev := ReadEvent{
			EPC:               r.EPC,
			ReaderKey:         key,
			CapturePointName:  r.CapturePointName,
			AntennaPort:       r.AntennaPort,
			RSSI:              r.RSSI,
			ReaderTimestampMs: r.ReaderTimestamp.UnixMilli(),
		}
		for _, s := range targets {
			select {
			case s.ch <- ev:
			default: // slow client; drop
			}
		}
	}
}

var topicRe = regexp.MustCompile(`^trakrf\.id/([^/]+)/reads$`)

func readerKeyFromTopic(topic string) string {
	if m := topicRe.FindStringSubmatch(topic); m != nil {
		return m[1]
	}
	return topic
}
```

- [ ] **Step 4: Run, verify pass** — `just backend test ./internal/services/readstream/...` → PASS
- [ ] **Step 5: Commit** — `feat(tra-924): in-process per-org read broadcaster`

---

## Task 2: Backend — tap the ingest subscriber

**Files:**
- Modify: `backend/internal/ingest/subscriber.go`
- Test: `backend/internal/ingest/subscriber_publish_test.go` (create)

- [ ] **Step 1: Write failing test** — a fake publisher captures the parsed reads + org for a handled CS463 message. (Reuse whatever test seam exists; if `handleMessage` needs a resolvable topic, assert via the publisher with a stubbed store. If store stubbing is impractical in a unit test, instead assert the smaller pure contract: `NewSubscriber` accepts a `ReadPublisher` and stores it. Prefer the behavioral test if a store fake already exists in the package.)

```go
package ingest

import (
	"sync"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

type fakePublisher struct {
	mu    sync.Mutex
	calls []pubCall
}
type pubCall struct {
	orgID int
	topic string
	reads []scanread.Read
}

func (f *fakePublisher) Publish(orgID int, topic string, rs []scanread.Read) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pubCall{orgID, topic, rs})
}
```

Add a test `TestSubscriber_PublishesParsedReads` only if the package already has a store fake usable by `handleMessage`. Otherwise keep `fakePublisher` as the compile-time proof that the interface is satisfied and rely on Task 4's handler integration test for end-to-end coverage. Document the choice in the test file header.

- [ ] **Step 2: Run, verify fail/compile-error**

- [ ] **Step 3: Implement** — add interface + field + call.

In `subscriber.go`, near the existing `ReadEvaluator` interface:

```go
// ReadPublisher receives every parsed read (pre-membership-filter) for live
// fan-out, e.g. the org-scoped SSE feed. Optional; nil disables fan-out.
type ReadPublisher interface {
	Publish(orgID int, topic string, reads []scanread.Read)
}
```

Add `feed ReadPublisher` to the `Subscriber` struct, and a param to `NewSubscriber`:

```go
func NewSubscriber(cfg Config, store *storage.Storage, eval ReadEvaluator, feed ReadPublisher, log *zerolog.Logger) *Subscriber {
	return &Subscriber{cfg: cfg, store: store, eval: eval, feed: feed, log: log.With().Str("component", "ingest").Logger()}
}
```

In `handleMessage`, immediately after `reads, err := Parse(route.DeviceType, payload)` succeeds and before `PersistReads`, publish the full parsed set (coverage diagnostic — all reads, not just membership-passing):

```go
	// Live-feed fan-out (TRA-924): all parsed reads, org-scoped, best-effort.
	if s.feed != nil && len(reads) > 0 {
		s.feed.Publish(route.OrgID, topic, reads)
	}
```

- [ ] **Step 4: Run, verify pass** — `just backend test ./internal/ingest/...` → PASS
- [ ] **Step 5: Commit** — `feat(tra-924): tap parsed read stream for live feed`

---

## Task 3: Backend — SSE handler

**Files:**
- Create: `backend/internal/handlers/readstream/readstream.go`
- Test: `backend/internal/handlers/readstream/readstream_test.go`

- [ ] **Step 1: Write failing tests** — (a) org filtering: a request with org-context X receives X's events and not Y's; (b) SSE framing: response has `Content-Type: text/event-stream` and emits `data: {json}\n\n`; (c) connection survives past a low server `WriteTimeout` (heartbeat keeps it alive / write deadline cleared).

```go
package readstream_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/handlers/readstream"
	"github.com/trakrf/platform/backend/internal/middleware"
	rs "github.com/trakrf/platform/backend/internal/services/readstream"
)

// withOrg injects an org id the same way middleware.GetRequestOrgID reads it.
// (Use whatever context key the project's middleware exposes; see note below.)
func withOrg(r *http.Request, orgID int) *http.Request { /* see Step 3 note */ return r }

func TestStream_OrgFilteredFraming(t *testing.T) {
	b := rs.New()
	h := readstream.NewHandler(b)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Stream(w, withOrg(r, 1))
	}))
	srv.Config.WriteTimeout = 500 * time.Millisecond // prove we clear the deadline
	srv.Start()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	// Give the handler time to subscribe, then publish past the WriteTimeout window.
	time.Sleep(700 * time.Millisecond)
	b.Publish(2, "trakrf.id/x/reads", []scanreadRead("OTHER")) // wrong org
	b.Publish(1, "trakrf.id/dock-9/reads", []scanreadRead("EPC-1"))

	sc := bufio.NewScanner(resp.Body)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("did not receive org-1 data frame")
		default:
		}
		if !sc.Scan() {
			t.Fatalf("stream closed early: %v", sc.Err())
		}
		line := sc.Text()
		if strings.Contains(line, "OTHER") {
			t.Fatal("received another org's read")
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, "EPC-1") {
			if !strings.Contains(line, `"readerKey":"dock-9"`) {
				t.Fatalf("frame missing readerKey: %s", line)
			}
			return // success: survived past WriteTimeout AND org-filtered
		}
	}
}
```

> Helper `scanreadRead` builds a `scanread.Read` with the given EPC (define in the test). `withOrg` must set the org the way the real middleware does — see Step 3 note for the exact context key. If the middleware org key isn't exported for tests, the handler test injects org via an exported test seam OR the test exercises the route through the real `middleware.Auth` chain in Task 5's router test instead; in that case keep this test focused on framing + WriteTimeout survival with a fixed org.

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

```go
// Package readstream serves the org-scoped live-reads SSE endpoint.
package readstream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/httputil"
	"github.com/trakrf/platform/backend/internal/middleware"
	rs "github.com/trakrf/platform/backend/internal/services/readstream"
)

const heartbeatInterval = 20 * time.Second

type Handler struct {
	broadcaster *rs.Broadcaster
}

func NewHandler(b *rs.Broadcaster) *Handler { return &Handler{broadcaster: b} }

// RegisterRoutes mounts the SSE endpoint. Caller must apply session auth.
func (h *Handler) RegisterRoutes(r chiRouter) {
	r.Get("/api/v1/reads/stream", h.Stream)
}

// chiRouter is the minimal surface we need from chi.Router.
type chiRouter interface {
	Get(pattern string, h http.HandlerFunc)
}

// Stream holds an SSE connection open, forwarding the caller's org reads.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	// Long-lived stream: clear the server WriteTimeout for this connection.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch, cancel := h.broadcaster.Subscribe(orgID)
	defer cancel()

	hb := time.NewTicker(heartbeatInterval)
	defer hb.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hb.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
```

> **Step 3 note (org context in test):** Inspect `middleware/orgresolver.go` for how `GetRequestOrgID` reads the org (session claims vs api-key principal) and the context key/helper to set it. Use the exported setter if one exists (e.g. a `middleware.WithUserClaims`-style helper or `context.WithValue` with the exported key). If nothing is exported, drive the org-filtering assertion through Task 5's router test (real `middleware.Auth` + a minted session JWT) and keep this handler test to framing + WriteTimeout survival using a fixed org via a tiny exported test hook. Pick the lowest-friction option discovered at implementation time.

- [ ] **Step 4: Run, verify pass**
- [ ] **Step 5: Commit** — `feat(tra-924): org-scoped live-reads SSE handler`

---

## Task 4: Backend — wire into serve + router

**Files:**
- Modify: `backend/internal/cmd/serve/serve.go`, `backend/internal/cmd/serve/router.go`

- [ ] **Step 1: Construct broadcaster + handler** in `serve.go` before subscriber/handlers:

```go
	readBroadcaster := readstream.New()
```

Pass it into the subscriber (replaces the geofence-only construction):

```go
	subscriber := ingest.NewSubscriber(mqttCfg, store, geofenceEngine, readBroadcaster, log)
```

(If MQTT is disabled, the broadcaster simply has no publisher — the endpoint still serves an empty/heartbeat-only stream.)

Construct the handler with the other handlers:

```go
	readstreamHandler := readstreamhandler.NewHandler(readBroadcaster)
```

Add imports: `readstream` service pkg and `readstreamhandler "github.com/trakrf/platform/backend/internal/handlers/readstream"`.

- [ ] **Step 2: Register the route** in the session-auth group in `router.go` (alongside `assetsHandler.RegisterRoutes(r)` etc.), and thread the handler through `setupRouter`'s signature:

```go
	readstreamHandler.RegisterRoutes(r)
```

> If `ContentType` middleware in the session group rejects GET-with-no-body or sets a JSON content-type expectation that conflicts with SSE, register the SSE route in a nested `r.Group` that applies only `middleware.Auth` (+ `SentryContext`) and skips `ContentType`. Verify by reading `middleware.ContentType` behavior for GET.

- [ ] **Step 3: Build** — `just backend build` → success
- [ ] **Step 4: Smoke** — `just backend test ./internal/cmd/serve/... ./internal/handlers/readstream/...` → PASS
- [ ] **Step 5: Commit** — `feat(tra-924): wire live-reads SSE into server`

---

## Task 5: Backend — remove readerFeed app-config injection

**Files:**
- Modify: `backend/internal/handlers/frontend/frontend.go` (+ `frontend_test.go`)
- Modify: `backend/internal/cmd/serve/serve.go`

- [ ] **Step 1: Update/relax test** — drop assertions that the injected config contains `readerFeed`; assert it still contains `environmentLabel` and no longer contains `readerFeed`/broker creds.
- [ ] **Step 2: Implement** — delete `ReaderFeedConfig` type, the `readerFeed` field in `appConfig`, the param on `NewHandler`, and the fallback JSON literal's `readerFeed` key in `buildAppConfigScript`. In `serve.go`, drop the `READER_FEED_MQTT_*` `os.Getenv` block; `NewHandler(frontendFS, "frontend/dist", os.Getenv("ENVIRONMENT_LABEL"))`.
- [ ] **Step 3: Build + test** — `just backend build` and `just backend test ./internal/handlers/frontend/...` → PASS
- [ ] **Step 4: Commit** — `chore(tra-924): drop reader-feed broker config from app injection`

---

## Task 6: Frontend — SSE stream reader

**Files:**
- Create: `frontend/src/lib/readerfeed/stream.ts`
- Test: `frontend/src/lib/readerfeed/stream.test.ts`

- [ ] **Step 1: Write failing tests** — frame parser turns an SSE buffer into `ParsedRead[]`, ignoring comments/heartbeats and partial frames.

```ts
import { describe, it, expect } from 'vitest';
import { parseSSEChunk, type SSEParseState } from './stream';

describe('parseSSEChunk', () => {
  it('parses complete data frames into reads', () => {
    const st: SSEParseState = { buffer: '' };
    const ev = { epc: 'E1', readerKey: 'dock', capturePointName: 'cp', antennaPort: 1, rssi: -50, readerTimestampMs: 10 };
    const reads = parseSSEChunk(st, `data: ${JSON.stringify(ev)}\n\n`);
    expect(reads).toHaveLength(1);
    expect(reads[0]).toMatchObject(ev);
  });

  it('ignores comments/heartbeats', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, ': ping\n\n')).toHaveLength(0);
    expect(parseSSEChunk(st, ': connected\n\n')).toHaveLength(0);
  });

  it('buffers a split frame across chunks', () => {
    const st: SSEParseState = { buffer: '' };
    const ev = { epc: 'E2', readerKey: 'd', capturePointName: 'c', antennaPort: 2, rssi: -40, readerTimestampMs: 5 };
    const full = `data: ${JSON.stringify(ev)}\n\n`;
    const mid = Math.floor(full.length / 2);
    expect(parseSSEChunk(st, full.slice(0, mid))).toHaveLength(0);
    const reads = parseSSEChunk(st, full.slice(mid));
    expect(reads).toHaveLength(1);
    expect(reads[0].epc).toBe('E2');
  });

  it('drops malformed JSON without throwing', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, 'data: not-json\n\n')).toHaveLength(0);
  });
});
```

- [ ] **Step 2: Run, verify fail** — `just frontend test stream` → FAIL
- [ ] **Step 3: Implement**

```ts
import type { ParsedRead } from './index';

export const READ_STREAM_PATH = '/api/v1/reads/stream';

export interface SSEParseState {
  buffer: string;
}

/**
 * Feed raw SSE text; returns any complete `data:` frames parsed into reads.
 * Comments (`:` lines) and heartbeats are ignored. Incomplete trailing frames
 * stay buffered in `state` for the next chunk.
 */
export function parseSSEChunk(state: SSEParseState, chunk: string): ParsedRead[] {
  state.buffer += chunk;
  const reads: ParsedRead[] = [];
  let sep: number;
  while ((sep = state.buffer.indexOf('\n\n')) !== -1) {
    const frame = state.buffer.slice(0, sep);
    state.buffer = state.buffer.slice(sep + 2);
    for (const line of frame.split('\n')) {
      if (!line.startsWith('data:')) continue;
      const json = line.slice(5).trim();
      if (!json) continue;
      try {
        const r = JSON.parse(json) as ParsedRead;
        if (typeof r.epc === 'string') reads.push(r);
      } catch {
        // malformed frame; skip
      }
    }
  }
  return reads;
}

export interface ReadStreamCallbacks {
  onReads: (reads: ParsedRead[]) => void;
  onOpen: () => void;
  onError: (err: unknown) => void;
}

export interface ReadStreamHandle {
  close: () => void;
}

/**
 * Open the org-scoped live-reads SSE stream over fetch (carries the JWT bearer;
 * native EventSource cannot set Authorization). Auto-reconnects with backoff.
 * `getToken` returns the current access token; `onUnauthorized` should refresh
 * it and resolve true to retry.
 */
export function openReadStream(opts: {
  baseURL: string;
  getToken: () => string | null;
  onUnauthorized: () => Promise<boolean>;
  callbacks: ReadStreamCallbacks;
}): ReadStreamHandle {
  let closed = false;
  let controller: AbortController | null = null;
  let backoff = 1000;

  const run = async () => {
    while (!closed) {
      controller = new AbortController();
      const state: SSEParseState = { buffer: '' };
      try {
        const token = opts.getToken();
        const resp = await fetch(opts.baseURL + READ_STREAM_PATH, {
          headers: {
            Accept: 'text/event-stream',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          signal: controller.signal,
        });
        if (resp.status === 401) {
          const ok = await opts.onUnauthorized();
          if (!ok) throw new Error('unauthorized');
          continue; // retry immediately with refreshed token
        }
        if (!resp.ok || !resp.body) throw new Error(`stream HTTP ${resp.status}`);
        opts.callbacks.onOpen();
        backoff = 1000;
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          const reads = parseSSEChunk(state, decoder.decode(value, { stream: true }));
          if (reads.length) opts.callbacks.onReads(reads);
        }
      } catch (err) {
        if (closed) return;
        opts.callbacks.onError(err);
      }
      if (closed) return;
      await new Promise((r) => setTimeout(r, backoff));
      backoff = Math.min(backoff * 2, 15000);
    }
  };
  void run();

  return {
    close: () => {
      closed = true;
      controller?.abort();
    },
  };
}
```

- [ ] **Step 4: Run, verify pass** — `just frontend test stream` → PASS
- [ ] **Step 5: Commit** — `feat(tra-924): fetch-based SSE read stream client`

---

## Task 7: Frontend — rewrite `useReaderFeed` for SSE

**Files:**
- Modify: `frontend/src/hooks/readerfeed/useReaderFeed.ts`

- [ ] **Step 1: Implement** — replace mqtt.js usage with `openReadStream`; keep the `expireReads` interval and `mergeReads` pipeline unchanged. Source the base URL and token from the existing API/auth config (`API_BASE_URL` and `useAuthStore`). Map stream lifecycle to `ReaderFeedStatus`.

```ts
import { useEffect, useRef, useState } from 'react';
import { mergeReads, expireReads, READ_TTL_SECONDS } from '@/lib/readerfeed';
import type { LiveRead, ReaderFeedStatus } from '@/lib/readerfeed';
import { openReadStream } from '@/lib/readerfeed/stream';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';

const EXPIRY_TICK_MS = 1000;

export interface ReaderFeedState {
  reads: LiveRead[];
  status: ReaderFeedStatus;
  error: string | null;
  readerCount: number;
  topic: string;
  configured: boolean;
}

export function useReaderFeed(): ReaderFeedState {
  const [readsMap, setReadsMap] = useState<Map<string, LiveRead>>(new Map());
  const [status, setStatus] = useState<ReaderFeedStatus>('connecting');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const handle = openReadStream({
      baseURL: API_BASE_URL,
      getToken: () => useAuthStore.getState().token ?? null,
      onUnauthorized: async () => {
        try {
          await useAuthStore.getState().refresh();
          return !!useAuthStore.getState().token;
        } catch {
          return false;
        }
      },
      callbacks: {
        onOpen: () => {
          setStatus('connected');
          setError(null);
        },
        onReads: (reads) => setReadsMap((prev) => mergeReads(prev, reads, Date.now())),
        onError: (err) => {
          setStatus('error');
          setError(err instanceof Error ? err.message : 'stream error');
        },
      },
    });
    return () => handle.close();
  }, []);

  useEffect(() => {
    const id = setInterval(() => {
      setReadsMap((prev) => expireReads(prev, Date.now(), READ_TTL_SECONDS));
    }, EXPIRY_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const reads = Array.from(readsMap.values());
  const readerCount = new Set(reads.map((r) => r.readerKey)).size;
  return { reads, status, error, readerCount, topic: 'live reads', configured: true };
}
```

> Verify `API_BASE_URL` and `useAuthStore`'s `token`/`refresh` names against the real files (`lib/api/client.ts`, `stores/authStore.ts`) and adjust imports/casing. If `API_BASE_URL` isn't exported, export it or derive it from the same env the axios client uses.

- [ ] **Step 2: Typecheck** — `just frontend typecheck` → PASS
- [ ] **Step 3: Commit** — `feat(tra-924): live reads hook consumes backend SSE`

---

## Task 8: Frontend — remove broker config + mqtt dependency + dead parse module

**Files:**
- Delete: `frontend/src/lib/readerfeed/config.ts` (+ test), `frontend/src/lib/readerfeed/parse.ts` (+ test)
- Modify: `frontend/src/lib/readerfeed/index.ts`, `frontend/src/lib/appConfig.ts`, `frontend/src/components/LiveReadsScreen.tsx`, `frontend/package.json`

- [ ] **Step 1:** Remove `readerFeed` from the `Window.__APP_CONFIG__` type in `appConfig.ts`.
- [ ] **Step 2:** Delete `config.ts`/`parse.ts` and their tests; drop their re-exports from `index.ts` (keep `ParsedRead`, `LiveRead`, `ReaderFeedStatus`, store/age exports). If `ParsedRead` was defined in `parse.ts`, move the interface into `index.ts`.
- [ ] **Step 3:** In `LiveReadsScreen.tsx`, since `configured` is now always true and there's no broker topic, simplify the "Not configured" branch (it becomes unreachable — remove it) and replace the topic caption with a neutral label. Keep status dot/labels for connecting/connected/error.
- [ ] **Step 4:** Remove `"mqtt": "^5.15.1"` from `package.json`; run `pnpm install` to update the lockfile.
- [ ] **Step 5: Verify** — `just frontend typecheck` and `just frontend test` → PASS; grep confirms no remaining `mqtt`, `getReaderFeedConfig`, `__APP_CONFIG__.readerFeed`, `READER_FEED_MQTT` references in `src/`.
- [ ] **Step 6: Commit** — `chore(tra-924): remove browser broker creds, config, and mqtt dep`

---

## Task 9: Full validation + handoff

- [ ] **Step 1:** `just backend build && just backend test ./...` → PASS (note any pre-existing integration-test gaps that need a DB).
- [ ] **Step 2:** `just frontend validate` → PASS.
- [ ] **Step 3:** `just lint` → clean.
- [ ] **Step 4:** Regenerate OpenAPI if the SSE route needs annotations — the SSE endpoint is not a JSON REST resource; confirm whether `api-spec` drift check trips on an unannotated route. If it does, add a minimal annotation or exclude per existing convention.
- [ ] **Step 5:** Push branch, open PR (base `main`), body summarizing: SSE proxy, org enforcement, removed browser creds, single-replica caveat (TRA-907), infra follow-up.
- [ ] **Step 6:** Post a Linear comment on TRA-924 handing off the broker-hardening items to infra (cluster-internal WSS listener, retire `frontend-readonly` user + ACL; overlaps TRA-857) and recording the multi-replica decision (gate Live Reads on single-replica until TRA-907).
- [ ] **Step 7:** **HOLD for Mike's diff review before merge** (per project rule: never merge without explicit approval).

---

## Self-Review

- **Spec coverage:** SSE endpoint + org filter (Tasks 1–4) ✓; frontend SSE swap (Tasks 6–7) ✓; remove `readerFeed` runtime config + browser creds (Tasks 5, 8) ✓; broker hardening (handoff, Task 9.6) ✓; multi-replica decision (documented, Task 9.6) ✓.
- **Type consistency:** `ReadEvent` JSON tags ↔ `ParsedRead` fields (epc/readerKey/capturePointName/antennaPort/rssi/readerTimestampMs) ✓; `Publish(orgID, topic, reads)` identical in interface, broadcaster, fake, and call site ✓; `parseSSEChunk(state, chunk)` signature consistent across test + impl + caller ✓.
- **Placeholders:** none — every code step is concrete. Two explicit "verify against real file" notes (org context key in Task 3; auth-store/api-base names in Task 7) are runtime confirmations, not deferred work.
