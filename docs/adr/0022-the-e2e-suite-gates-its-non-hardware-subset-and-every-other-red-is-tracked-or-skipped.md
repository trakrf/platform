# ADR 0022 — The e2e suite gates its non-hardware subset; every other red is either tracked or skipped with its reason

Date: 2026-09-07
Status: Proposed
Tracking: TRA-1253 (this record), TRA-1224 (the question, deferred to here), TRA-1246 / TRA-1245 (the parts already closed), ADR 0018 (a check that cannot run says so), ADR 0020 (a guard's scope is a rule, not a list of exceptions)

## Context

The Playwright suite has been red, manual and unexplained for months. TRA-1224
deliberately did not answer "what is this suite FOR", on the grounds that the
call is only honest with a current failure inventory in hand. This record is
that answer, written against a measured inventory rather than an aspiration.

### What the measurement found

Three full arms on the CS108 bench, 223 tests each, `main` first and the branch
after. The method matters more than the numbers: **diff the failure SETS, never
compare counts.** A fixed failure and a new one cancel out in a count.

The ticket carried a four-day-old table of 12 expected failures. The measured
figure was 9, and the difference was not drift in one direction — four had been
fixed since (`org-members` ×3, `inventory-save` ×1) and one was new
(`hold-sweep`). A count would have shown "12 → 9" and hidden both.

Diffing arm 1 against arm 2 also settled a question no single run can answer:
`hold-sweep` failed in one arm and passed in the other. It is **intermittent**,
and it had been sitting inside a characterisation that called the failures
deterministic.

That characterisation cited `flaky: 0` as its evidence. It is not evidence:
`playwright.config.ts` sets `retries: process.env.CI ? 2 : 0`, and Playwright
can only report a test as flaky if it fails and then passes **on a retry**.
With retries off, `flaky: 0` is a tautology. What actually established
determinism in TRA-1224 was the other half of its method — "measured twice" —
and that is the half worth keeping.

### None of the stable red was a product defect

All eight stable failures were faults in the instruments, and each one produced
a result that could not be told apart from a real finding:

| spec | n | what it actually was |
| -- | -- | -- |
| `hamburger-menu` | 2 | looked for a tab labelled "Inventory"; renamed to "Scan" in TRA-1029 |
| `members` | 2 | asserted `not.toContain('TypeError')` over the whole HTML document — which under `dev:bridge` inlines the ble-mcp-test mock, whose source contains three literal `throw new TypeError`. It was matching its own harness. |
| `lazy-chunk-recovery` | 4 | a preview-targeted spec run against `localhost`; its guard checked that `API_TEST_LOGIN` was **set**, not that it **applied** to the target |

The `members` case is the one to remember. Its first assertion —
`not.toContain('Cannot read properties of null')`, the actual TRA-181 symptom —
**passed on every run**. The bug it exists to catch was absent the whole time,
and the spec was red anyway, on a substring that only ever appeared in the test
instrument. Anyone triaging that failure from its name would have gone looking
at the members screen.

### The same shape, five times over

Every defect this ticket touched has one form: an instrument reported a number
that could not distinguish *"the condition did not occur"* from *"I could not
see it"*.

1. A `logger.warn` line carried no KEEP token, so the forwarder dropped it and
   the needle read `0`.
2. A log line interpolated `readerState` into the matched region, so the same
   event was visible in `Connecting` and invisible in `Busy`.
3. `flaky: 0` with retries disabled, read as proof of determinism.
4. Helpers reading `tagStore.inventoryRunning`, a field that has never existed:
   `expect(undefined || false).toBe(false)` is a test that cannot fail.
5. An assertion broad enough to match the source of the mock injected to run it.

Numbers 1, 2 and 4 were invisible because `tsconfig.json` excluded `tests/**`,
so `tsc` never looked at the tree that produces every e2e verdict.

## Decision

**1. The non-hardware subset is a gate. It is expected to be green, and a red
run blocks.** It is now green on the bench (213 passed, 10 skipped, 0 failed),
so this is a statement about a state that exists rather than one to work
towards. Wiring it into CI against the PR's own preview deployment is a
follow-up; this record fixes the intent so that work has something to implement.

**2. The `@hardware` subset is advisory and says so.** It cannot run in CI —
there is one physical CS108 behind one bridge — so calling it a gate would be
a claim nothing can enforce.

**3. A spec that cannot pass against the configured target SKIPS, naming the
target it needs.** It does not fail. `lazy-chunk-recovery` failing four times
against `localhost` was one unmet precondition wearing four tests' clothing,
and it consumed a whole spec's worth of triage in this ticket's own inventory.
This is ADR 0018 applied to a target rather than to a dependency.

**4. Red is tracked or it is fixed. There is no third state.** An untracked red
is what teaches people to ignore the suite, and it is what made a four-day-old
failure table wrong in both directions.

**5. The tracked-baseline discipline is the method: run the branch, run `main`,
diff the failure SETS.** Two arms, because one arm cannot separate an
intermittent failure from a deterministic one — and do not quote `flaky: 0` as
evidence of anything while `retries: 0`.

## Consequences

`tests/**` is now inside `tsc --noEmit`, so a helper reading a field the store
does not have is a build error rather than a silently-`undefined` assertion.
The `**/*.test.ts` and `**/*.spec.ts` exclusions remain: removing those surfaces
**453** further errors, which is a real backlog and a separate piece of work,
not something to fold into this one.

Two costs are worth naming rather than discovering later.

The forwarder now keeps three additional strings, and that is a **measurement
change** — every extra line lands in the log the soak signal counts are computed
from. It was sized before it was adopted, and the sizing overturned the obvious
implementation: matching the whole `[CommandManager]` prefix took the count from
49 to 94, of which 90 were routine `logger.debug` chatter. Matching the three
needle texts costs **+1 line** and captures the same signals.

`powerOffTimeouts`, `toleratedPowerOffs` and `commandInFlight` stay `null` on an
e2e arm, and **the reason has changed** — which matters more than the value. It
used to be "the forwarder cannot pass these lines"; it is now "the forwarder
passes them, but no arm has yet observed one", both runs having been clean. They
move into `E2E_SIGNALS` when an arm actually catches an occurrence, on the same
measure-don't-assume rule that governed the widening.

Finally, `console-utils.ts` remains in the tree and remains **inert as a gate**:
nothing calls `assertNoErrors`, `getErrors()` or `generateReport()`, so its
critical/allowed lists cannot fail a run however carefully they are maintained.
Repairing a list is not the same as restoring a gate. Deleting it is the likely
right answer — 9 of 35 specs have already grown their own narrow
`page.on('console')` next to the thing they provoke — but it is a deletion, and
it is left to a human to authorise. Its measured contribution to a captured run
is about 73 lines.
