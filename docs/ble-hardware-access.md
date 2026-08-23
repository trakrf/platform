# BLE hardware access — who can hold the reader

Two things reach a CS108, and **only one can hold it at a time.** This document
explains which is which and what that costs you in practice.

## The bridge is test tooling only — never the product path

**The app has exactly one way to reach a CS108: direct Web Bluetooth**,
`navigator.bluetooth` in the browser. That is the path in prod, in preview, and in
any normal build. There is no bridge in the product path and there never should be.

`ble-mcp-test` exists solely so **tests** can drive real hardware from Node and
from headless browsers. It is injected **only** when
`VITE_BLE_BRIDGE_ENABLED === 'true'` — see `frontend/vite.config.ts:40`, which
returns early otherwise. Integration tests and `pnpm dev:bridge` set it; a preview
or prod build never does.

Do not describe these as "two paths to the reader". That phrasing invites someone
to imagine a supported bridge-in-production path, which must never exist.

## One connection at a time

A CS108 accepts **one connection**. Whatever holds the radio excludes everything
else:

- test tooling that is connected **blocks the real product path** to that reader
- a browser session **blocks the tests**

That is a property of the hardware and outlives any bridge implementation.

**The inverse is the one that misleads: an idle bridge port does not mean the
reader is free.** Someone may be holding it from a browser, and a browser never
appears as a bridge client. Conversely a page refresh releases the radio
immediately — browser-side release is implicit and cheap.

## Current implementation — expected to change

> Today's bridge (`rust-ble-test`) calls `transport.connect()` **once at process
> start** and holds the BLE link for its whole lifetime. A client disconnecting
> releases nothing. **Only `SIGTERM` frees the radio.**
>
> So today, to hand-test preview or prod against hardware you must **stop the
> bridge process** — closing the tests is not enough.
>
> `rust-ble-test` **goes away** in the replatform to `bleak`-based Python tooling,
> where the intent — *not yet a guarantee* — is to release the radio whenever no
> mock-to-bridge connection is active. If that lands, the rule relaxes from
> *"the bridge process must not be running"* to *"no test must be connected"*, and
> leaving the bridge up between runs stops being a problem.
>
> **Verify that behaviour before relying on it. Everything above this block holds
> either way.**

### Checking who holds the radio

`pgrep -f 'rust-bl[e]-test'` tells you whether the bridge process is up — but note
that `pgrep -f` can match **its own shell**, because the pattern appears in the
argv of the pipeline running it. Confirm with `ps -o pid,ppid,lstart,cmd -p <pid>`
or tie it to the socket with `ss -ltnp | grep 8080`; the listener cannot be faked
by a name match.

Remember that neither check answers "is the reader free" — only "is the bridge
holding it". A browser session is invisible to both.

## Bind address

The Rust bridge's **code default is `0.0.0.0`** (`config.rs:69`), i.e. LAN-wide,
on an endpoint with **no authentication** — no token, no origin check. Our
deployment narrows it to loopback with an explicit `BLE_MCP_WS_HOST=127.0.0.1`.
The safe binding here is deliberate configuration, not a safe default: an operator
who sets nothing gets a LAN-wide bind.

Loopback-only means the bridge is reachable **only from the machine it runs on**.
That is irrelevant to preview and prod, which never use the bridge, but it matters
for any test runner on another host.

## See also

- `reference_ble_bridge_restart` — how to start and stop the bridge
- `frontend/tests/integration/cs108/INTEGRATION-TEST-PRINCIPLES.md` — how the
  integration harness is allowed to talk to the worker
