# ADR 0009 — An instrument records the conditions its measurement depends on, at the time; a check of the repo is not a check of the running process

Date: 2026-08-29
Status: Proposed
Tracking: TRA-1200 (this change, and the arm that paid for it), TRA-1211 (the bridge-side half), TRA-1150 (the wedge under measurement), TRA-1189 (the campaign's method rules)

## Context

TRA-1200's CPU-swap arm ran 150 repetitions against real hardware over 68
minutes. Every instrument gate passed: the watchdog stayed silent and exited 0,
the bridge held one identity throughout with `NRestarts=0`, all 150 captures were
valid, the deliberate-break validation was performed on the run that produced the
data. The result was clean and large — 0/150 against a reference of 33/407.

Two conditions the comparison depended on were nevertheless wrong, and **neither
was visible from inside the run**. Both were found afterwards, by hand, from
archived logs.

**One: the field was not the reference field.** The arm was ~17% short on
unique tags, because the reader had been pulled back from the tag stack to gun a
barcode. The comparison's binding rule is *only one variable moves*; a second one
had moved, in the direction that favoured the observed result. This was
discovered by parsing 150 output logs and untarring the 2026-08-23 reference
archive — after the number had already been reported.

It had happened before. Cell A was **halted** on 2026-08-23 for the same
shortfall (~19% unique, ~41% reads), diagnosed the same way, after the fact.
Twice is not bad luck; it is a missing instrument.

**Two: the browser was not running the mock the tree described.** All 150 reps
loaded mock 0.12.0 against bridge 0.13.0. Every repo-level check was *correct*:
`git status` clean, `pnpm-lock.yaml` pinning 0.13.0 exactly, the
`node_modules/ble-mcp-test` symlink pointing at 0.13.0. The dev server had been
up for 22 hours and had resolved the bundle 8 hours before the dependency moved.

The mechanism matters because the obvious fix does not work: `require.resolve`
does not return the symlink, it resolves *through* it to a version-pinned pnpm
store path, and Node caches that resolution for the life of the process. pnpm
retains every previously installed version, so the cached path stayed present and
readable. **The stale read succeeded, silently, indefinitely.** Reading the
version from `package.json` or the lockfile — the obvious guard — would have
reported 0.13.0 and confirmed the error.

The existing guard could not catch it either, and that is not a defect in it.
TRA-1177 hardened that path to *throw* rather than serve a page without
`navigator.bluetooth`, on the correct reasoning that a missing mock presents as a
dead reader rather than a packaging error. It protects against a path that is
**missing**. This path was present the whole time.

The only party that saw the truth was the bridge, which logged
`mock version mismatch: expected 0.13.0, got 0.12.0` — once per connection, 150
times, into a journal nothing in this repo reads.

## Decision

**An instrument records the conditions its measurement depends on, in the record,
at the time the measurement is taken.** Not in a runbook, not in an operator's
memory, and not left to reconstruction.

Three rules follow, and each is the general form of one of the failures above.

**1. A condition that can invalidate a comparison is part of the record.** If a
number can be wrong because of it, the instrument writes it down per repetition.
Field density is now recorded on every e2e rep and printed against the reference
baseline, because a distribution with nothing to compare against is what let a
17% shortfall pass twice.

**2. Verify the running process, not the repository.** A clean tree, a correct
lockfile and a correct symlink are claims about the *tree*. A long-lived process
can hold a resolution, a bundle, or a configuration that the tree stopped
describing hours ago. Where a process outlives the state it read, something must
report what it is *actually* running. The bridge already does this correctly —
`instance_id` and `code_fingerprint` exist precisely so a consumer can ask *is
this the code I think it is* — and the dev server was the one component in the
measurement path that could not answer.

**3. "Cannot check" is its own answer, distinct from "checked and clean".** An
absent signal must never be recorded as a zero or a pass. This repo already
learned it once for soak signals (TRA-1206, where a structurally absent needle
read as 0 and disarmed an abort). It is the same rule for a boolean: a
`mock_version_match` that cannot distinguish *mismatch* from *unknown* leaves us
exactly where we started.

A corollary on where enforcement belongs: **the party that can observe a fact
reports it; the party with the requirement decides what is fatal.** The bridge
can see that two versions differ but cannot know whether the difference matters —
that is semantic, and depends on what changed. A measurement harness *does* know
its own requirement (exact match, or the arm is void). So the bridge exposes the
fact and keeps warning; the harness aborts. Pushing the strictness into the
shared tool would bake one consumer's policy into everyone's dev loop.

## Consequences

Two detectors now cover the mock-version case, deliberately not one. The
client-side check reads what was loaded from disk; the watchdog reads what
arrived over the wire. They fail for different reasons and neither is a fallback
for the other — the client-side one is blind when the path carries no version,
and the bridge-side one is blind against a bridge older than ble-mcp-test 0.14.0.

Density recording is additive and reconstructible: `resolveReadCycles` recomputes
from retained logs, so archived runs — including the 150-rep arm that motivated
this — become analysable without being re-run. Back-tested, it reproduces the
hand-derived figures exactly.

**This does not retroactively validate TRA-1200's arm.** That result stands as
reported, with both conditions named in its caveats: 0/150 on a field ~17%
sparser than the reference, using a mock the lockfile did not describe. The
instrument exists so the *next* arm does not need the same footnotes.

The cost is that every new recorded condition is a new thing that can be wrong,
and a record nobody reads is not free. The mitigation is that both additions
print against a reference or abort — neither is a value filed away for a future
reader to interpret.

Not adopted: adding a "restart the dev server" step to a runbook. A discipline
fix fails silently on exactly the runs that matter, and this one already survived
a clean tree, a correct lockfile, and a hardware-verified instrument pass.

Related: ADR 0008 (observe a peer through its contract, not its supervisor) is the
same family — a check that succeeded against the wrong subject. That one is about
*which* subject to interrogate; this one is about recording the answer at the
moment it is still true.
