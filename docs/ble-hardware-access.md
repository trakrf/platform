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

## The daemon holds the port, not the radio

**A running bridge does not block hand-testing. A connected client does.**

The Python bridge builds its transport *inside* the per-connection handler, so
no device is held until a client connects, and disconnecting releases it. Idle
expiry (`BLE_MCP_IDLE_TIMEOUT`, 600s) releases the command path too — and ends
the connection, never the process. Idle release is a lease on the command path,
not a process lifecycle.

So leaving the bridge up between runs is fine. To hand-test preview or prod
against a reader, close whatever is *connected* — you do not need to touch the
daemon.

This doc previously said the opposite, and asked to be re-verified after the
Python replatform. Verified 2026-08-29, three ways:

| check | result |
| -- | -- |
| ESPHome proxy slot accounting at release | `used=0 free=4 limit=4 allocated=[]` |
| live `get_connection_state` | `held:false · session:null · observer_count:0` |
| daemon log at DEBUG, 100k-line ring | silent 2h12m after the run ended |

**Cite the first one.** That is the proxy — the component that actually owns the
connection slots — reporting zero held. The other two are the daemon reporting
on itself, which is a weaker claim: "I sent a disconnect" is not "the slot is
free".

## Checking who holds the radio

One call, and it beats every process-grepping recipe this section used to carry:

```bash
# via the MCP tools, or over the control socket directly:
printf '{"op":"get_connection_state"}\n' | nc -U "${XDG_RUNTIME_DIR}/ble-bridge.sock"
```

```json
{"held": false, "session": null, "observer_count": 0, ...}
```

- **`held`** — someone owns the command path and can write to the device.
- **`observer_count`** — how many others are attached read-only.

### `held: false` is NECESSARY and NOT SUFFICIENT

**The reader is shared with the ble-mcp-test session** (reachable on cc2cc as
`bridge`), which runs its own hardware suites and holds the device for the length
of a publish or a soak.

A state query reads **state, not intent**. `held: false` is therefore equally
consistent with *"the other side has finished"* and *"the other side is between
two attempts"* — a retried publish, a spec that disconnects between reps, a suite
mid-restart. **The query cannot tell them apart, so it can never establish
clearance on its own.**

**Measured, 2026-08-31.** ble-mcp-test's first publish passed its 23-test hardware
gate and died at the last step on an expired OTP; it re-attempted 26 seconds later.
Platform polled inside that gap, read `held: false`, and connected. The flag was
telling the truth and the lock was genuinely enforced — the collision produced real
`DEVICE_BUSY` refusals naming the holder. The failure was that a **critical
section** (gate → OTP → retry) outlasted the **lock hold** protecting it. Cost:
8 of 23 e2e tests and a publish attempt.

## Take the reader by acquiring the lock, not by asking

`ble-mcp-test` ships `ble-radio-lock`, which takes an `flock(2)` on one fixed path
and execs your command. The kernel holds it for exactly as long as that command
lives, and takes it back when the process dies by any means — `Ctrl-C`, a killed
vitest, a crash. **The lock file is the contract, not the script:** anything that
flocks the same path participates, in any language.

```bash
pnpm exec ble-radio-lock path      # /tmp/ble-mcp-test.radio.lock -- both sides agree on this
```

**Our entry points are already wrapped**, so the ordinary case needs nothing from
you:

| command | holds the reader |
| -- | -- |
| `pnpm test:integration` | yes |
| `pnpm test:hardware` | yes |
| `pnpm test:e2e` (and `just frontend test-e2e`) | yes |
| `pnpm test:ui` | yes, for as long as the UI is open |
| `pnpm test:e2e:remote` (and `just frontend test-e2e-remote`) | **no** — `@hardware` is excluded, so it never reaches a reader |

A **hand-run vitest** is the case that is not wrapped for you, because there is no
entry point to wrap. Wrap it yourself:

```bash
pnpm exec ble-radio-lock --label platform-adhoc -- \
    pnpm vitest run tests/integration/cs108/locate.spec.ts
```

For several commands that are **one** operation — a build, then a run, then a
retry — take the hold once and stay inside it, or you release between the steps
and reproduce the 2026-08-31 defect exactly:

```bash
pnpm exec ble-radio-lock --label platform-session hold   # opens a shell holding the reader
# ...run as many commands as the operation needs...
exit                                                     # releases
```

The wrapper is **re-entrant for its own descendants**: a wrapped `pnpm test:e2e`
run inside that shell passes straight through rather than deadlocking on itself.

**Refusal is loud, immediate, and never queues.** A contended acquire exits **75**
(`EX_TEMPFAIL`) naming the holder, and the wrapped command does not run at all —
so exit 75 always means *"refused, never started"*, distinguishable from any code
your suite produces. Wait and re-run; do not poll for a gap.

**There is deliberately no way to ask whether the lock is free.** Acquisition is
the only interface, precisely because the 2026-08-31 collision was a poll whose
answer was true when read and false when acted on. You cannot poll what does not
exist — which is why the section above matters: `held` describes the device, and
the lock describes the operation.

### What the lock does not cover

`flock` is same-kernel, and it only binds processes that take it. Two cases sit
outside it, both by construction:

- **A browser hand-test.** Someone driving a reader from a real
  `navigator.bluetooth` tab takes no lock and appears in no bridge state. Holding
  the lock tells you no other **mssb command** is running; it says nothing about
  the reader itself. The one honest observable is advertising: a connected
  peripheral stops advertising, so *heard advertising* is real evidence nobody
  holds it, while *not heard* means held **or** powered off **or** out of range.
  Use it to decide whether to ask, never as permission to proceed.
- **Arm B on knuckles.** A lock held on mssb excludes nothing on another host, so
  that recipe is deliberately left unwrapped rather than wrapped for appearances.
  Co-ordinate it by hand.

**The gap between a hand test's reps is conceded, not co-ordinated.** If the device
is advertising and an automated run takes it between someone's reps, that is
accepted — the reciprocal being that a hand test which reconnects and cannot find
the device has its own tell, and checks bridge status. Each side's loss is visible
to the side that suffers it, so neither has to warn the other about that window.

The mechanism, and what each design decision rejected, are in the ble-mcp-test
checkout: `docs/radio-lock.md` and `docs/design/2026-09-06-radio-lock.md`
(TRA-1241).

**`observer_count > 0` is the hazard worth naming.** It is most often a leftover
mock-injected browser tab, which **appears in no process listing and in no log**
— so every `ps`/`pgrep`/`ss` recipe reports a clear field while a tab quietly
holds the command path. That is not hypothetical: on 2026-08-26 contention of
exactly this kind invalidated two hardware runs inside ten minutes.

**Do not identify the daemon by name.** Three name-based checks have been wrong
here in a row, each silently: one named a Rust binary deleted in the replatform,
one used the Python *module* name (which never appears in a cmdline — the
console script spells it with a hyphen), and `pgrep -f` matches *its own shell's
argv*, which produced a false abort during TRA-1189. If you need the process,
ask the socket which pid is serving the port — whatever is serving it **is** the
bridge, whatever it is called.

## Running it — a supervised user unit

The bridge runs as a systemd unit owned by ble-mcp-test (their TRA-1202), so
start/stop is `systemctl`, not a hand-rolled `nohup`:

```bash
systemctl --user status ble-bridge          # is it up?
systemctl --user start|stop ble-bridge      # start / stop
journalctl --user -u ble-bridge -f          # follow the log
just bridge-restart                         # after anything under bridge/ changes
```

`just bridge-restart` is a recipe **in the ble-mcp-test checkout**, not in this
repo — run it from there.

⚠ **`pkill` is the wrong tool now.** `Restart=always` brings the daemon back
within 5s, so a kill presents as *"the kill didn't work"* rather than *"wrong
tool"*. And you almost never want it anyway: the daemon holding the port is not
what blocks you.

⚠ **`--user` is load-bearing, not a style preference.** The MCP control socket
lives under `/run/user/<uid>`, which **does not exist for a system unit**. A
system-scope install therefore comes up looking perfectly healthy with the
entire MCP surface silently gone — and presents as *"the MCP tools are broken"*,
never as *"the unit is installed wrong"*. Do not promote it to
`/etc/systemd/system`.

A **permanent** failure — a bad or missing env file — reaches `failed` after
five attempts in about 25 seconds rather than looping forever, and `failed` is
sticky: it needs `systemctl --user reset-failed` before it will start again.
A start failure is always a configuration failure, so a restart storm is never
transient.

**Do not use `status.version` to tell whether the daemon is running current
code.** A release number cannot answer that question even when it is accurate:
two daemons at the same released version can be serving different code, because
a version moves on release and code moves on merge. For most of the replatform
it could not answer it at all — it sat frozen at `0.1.0` while everything
underneath it changed, so it was a confident wrong answer rather than a stale
one. It is being synced to the released version, which stops the frozen value
being a lie without making it a currency check.

Code currency is answered by ble-mcp-test's own staleness guard in its
`pretest`, which compares the running daemon against the checkout it was
started from.

## Bind address

The bridge binds **`127.0.0.1:25153`** — loopback only, reachable solely from
the machine it runs on. That is irrelevant to preview and prod, which never use
the bridge, but it matters for any test runner on another host.

The endpoint has **no authentication** — no token, no origin check. Loopback is
the whole authorization story, which is why the bind address is not a detail to
relax casually. The MCP control socket alongside it is mode `0600`, owner-only,
for the same reason.

## See also

- `frontend/scripts/watch-soak-abort-criteria.mjs` — the soak watchdog, and why
  it detects a bridge restart through `status.uptime_seconds` rather than
  through systemd
- `docs/bridge-service.md` in the ble-mcp-test checkout — the unit itself
- `frontend/tests/integration/cs108/INTEGRATION-TEST-PRINCIPLES.md` — how the
  integration harness is allowed to talk to the worker
