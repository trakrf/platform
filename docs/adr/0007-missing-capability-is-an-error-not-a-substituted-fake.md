# ADR 0007 — On the product path, a missing capability is an error; never substitute a fake

Date: 2026-08-26
Status: Proposed
Tracking: TRA-1177 (this change), TRA-1155 (the replatform whose window made the cleanup cheap), TRA-1161 (which deleted the HTTP surface the dead config pointed at)

## Context

TRA-1177 was scoped as tidying: resolve a bundle through an exports map, decide
the fate of some inherited scaffolding. Two of the things it found were the same
defect wearing different clothes, and neither was visible as a defect from the
code alone.

**Instance one — the transport factory.** `createAutoTransport` ended in
`return new MockTransport(config.mock)`. `MockTransport` reports an 85% battery
level and streams three hardcoded EPCs at 100 ms intervals. It was reached
whenever `'bluetooth' in navigator` was false — every Firefox, every desktop
Safari, every iOS browser, since Apple forbids non-WebKit engines.

The ticket assumed this was unreachable because `useBluetoothSupport` gates the
UI. That assumption held for three of the four places that call `connect()`:
`Header.tsx` and `SettingsScreen.tsx` both early-return on `!isBrowserSupported`,
and `BrowserSupportBanner`'s button is hardcoded `disabled` with no handler.
`ScanControls.tsx` — the kits flow — had `disabled={isReconnecting}` and no
support check, and `DeviceManager.create` performs no Bluetooth precheck of its
own. So a real user on an unsupported browser clicked Connect, the connect
resolved, and the app showed them a healthy scanner with a battery level and a
stream of tags. None of it existed.

**Instance two — the mock bundle.** `vite.config.ts` read the ble-mcp-test
browser bundle by literal path through a symlink into that package's `dist/`. On
a read failure it logged and `return html` — serving the page with no
`navigator.bluetooth` at all. The symptom is a reader that will not connect, on
a machine where the reader is fine.

Both are the same shape: **something was unavailable, and the code substituted
something that behaved enough like it to pass for working.** Neither throws.
Neither logs anything a user sees. Both produce a system that is confidently
wrong, and both were introduced by people trying to be helpful — a fallback
transport so tests need no hardware, a caught exception so a build does not fail
on a missing dev dependency.

That is what makes this worth an ADR rather than a code comment. The defect is
not in either file. It is in a habit that reads as defensive programming, and
both instances survived review at the time.

## Decision

**On any path a user can reach, an unavailable capability raises an error. It is
never silently replaced by a substitute that fabricates its outputs.**

Concretely, from this change:

- `TransportFactory.create` throws when `navigator.bluetooth` is absent, rather
  than returning a transport that invents device data. `MockTransport` and
  `BridgeTransport` are deleted rather than gated, because a fake that exists is
  a fake something will eventually reach.
- The vite bridge plugin throws when the bundle cannot be resolved, rather than
  serving a page without the mock it was asked to inject.

Two corollaries worth stating, because both were tempting here:

1. **A fake that is only for tests must be unreachable from production code, not
   merely unlikely.** "Nothing sets that flag" is a property of today's tree, not
   a guarantee. If the only thing standing between a user and fabricated data is
   that no caller currently passes a config field, that is not a gate.

2. **Guarding at the UI is not the same as guarding at the source.** Three of
   four call sites checked browser support and the fourth did not, which is the
   expected outcome for a rule enforced by convention across call sites. The
   check belongs where the capability is acquired; UI gates are for telling the
   user why, not for preventing the substitution.

## Consequences

**A browser without Web Bluetooth now surfaces an error instead of a working-looking
app.** That is the intent. `useBluetoothSupport` already explains what to do
about it — the banner, the Help copy and the per-platform recommendations all
predate this and are unaffected.

**The mocked e2e path is unaffected**, and it is worth recording why, since it
looks like it should break: ble-mcp-test's injected mock assigns
`navigator.bluetooth` directly (verified against the installed 0.7.3 bundle). The
mock therefore takes the same branch real hardware does. This is also what
CLAUDE.md means by *"the app reaches a CS108 solely via browser
`navigator.bluetooth`"* — there is one path, and the mock substitutes the
browser API rather than the application's transport.

**Deleting a fallback removes a failure mode and adds a failure.** Builds now
break when the mock bundle is missing, where before they produced a page. This
is the trade being made deliberately: a build that fails names its cause in one
line, and a page with no `navigator.bluetooth` costs somebody an afternoon on
the wrong hypothesis. The asymmetry is the whole argument.

**This does not forbid fallbacks generally.** Falling back to a cached value, a
lower-resolution source, or a degraded-but-honest mode is fine — the rule is
about *fabrication*. The test is whether the substitute can be distinguished
from the real thing by its output. An 85% battery reading from a scanner that is
not there cannot be.
