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
run blocks.** Enforced as steps in the `lint-test` job — 182 tests, ~5 minutes,
`--grep-invert @hardware`, against a database and backend stood up in the job.

It is a step in an existing job rather than a job of its own, and that is
load-bearing: the main-branch-protection ruleset requires the exact contexts
`build`, `lint-test`, `api-spec` and `main contract-tests must be green`, so a
new job would have been **advisory until somebody edited that ruleset**. Shipping
a gate that gates nothing is the precise failure this record exists to rule out,
so it goes where enforcement already is. The changelog gate lives inside
`lint-test` for the same reason.

⚠ The first draft of this record proposed gating against the PR's own preview
deployment. **There is no such thing.** `sync-preview.yml` resets `preview` to
`main` and merges *every* open non-draft PR into it, so a failure there can be
caused by somebody else's branch. Preview is a composition, and a composition
cannot gate an individual PR — it is the continuous integration signal that
replaced "require branches to be up to date" (TRA-1094), which is a different
job from blocking a merge.

**2. The `@hardware` subset is advisory and says so.** It cannot run in CI —
there is one physical CS108 behind one bridge — so calling it a gate would be
a claim nothing can enforce.

There is a second reason to keep it out, and it is easier to forget: **a gate
must not flake.** TRA-1259 is a live intermittent in `hold-sweep`. Gating a set
that contains a known random failure teaches people to ignore the gate, which is
the same end state as having no gate at all.

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
**419** further errors (TRA-1258), which is a real backlog and a separate piece of work,
not something to fold into this one.

**Turning retries on found two flakes the bench never could.** CI sets
`retries: 2`; local runs set `0`. So the first CI-shaped arm was also the first
run in this ticket's history where `flaky` could be non-zero at all — and it
immediately surfaced two tests in the gated subset that no amount of local
running would have shown:

  `hamburger-menu` closed the drawer with a bare positional click on `<body>`,
  racing the open animation. Fixed by clicking the overlay, which is the pattern
  the sibling test in the same file already used.

  `auth` asserted on the transient `"Logging in..."` button text. Against a warm
  local backend the login fails before the assertion runs, so it failed through
  all three attempts. The assertion was incidental — that test is named for the
  error message — and was removed rather than stabilised by slowing the server
  down, which would test the harness rather than the app.

Neither was introduced here; both were pre-existing and invisible. That is the
argument for the gate in miniature: a suite nothing runs under retry pressure
cannot tell you which of its greens are luck.

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

Finally, `console-utils.ts` was **inert as a gate**: nothing called
`assertNoErrors`, `getErrors()` or `generateReport()`, so its critical/allowed
lists could not fail a run however carefully they were maintained. Repairing a
list is not the same as restoring a gate.

**Deleted 2026-09-07**, authorised after this record was first written. Two
things came out of that which are worth keeping here, because both argue the
decision was not merely tidiness:

Its `logAllMessages: true` registered a **second** `page.on('console')` listener
on the same page as the forwarder, so every browser line during
`connection.spec.ts` reached the captured log **twice** — 56 `[ble-timing]
write-ack` timestamps appearing exactly twice on the 2026-09-07 arm. That is a
double-count in `ackSamples` and `connectSamples`, so the monitor was not inert
after all: it was silently corrupting two measured signals.

Deleting it also retired
`every-console-allowlist-entry-has-a-producer.test.ts`, which had shipped only
days earlier to guard the two lists per entry. That guard was correct and did
its job; it simply has nothing left to guard once the lists are gone. Removing a
guard alongside the thing it guards is the honest move — the alternative is a
passing test over an empty list, which is the same false comfort this whole
record is about. The surviving pattern is the one 9 of 35 specs already reached
independently: a narrow `page.on('console')` next to the thing that provokes it.
