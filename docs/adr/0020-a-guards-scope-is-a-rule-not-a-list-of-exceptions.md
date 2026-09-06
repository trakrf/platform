# ADR 0020 — A guard's scope is a rule, not a list of exceptions

Date: 2026-09-06
Status: Proposed
Tracking: TRA-1224 (this record, and the allowlist that prompted it), TRA-1226 (the
exclusion this extends), TRA-1186 (the same defect at four layers), ADR 0018 (the
adjacent decision about how a check reports, rather than what it covers)

## Context

Three separate mechanisms in this repo decide what they cover by enumerating it.
All three decayed, none of them said so, and the decay was invisible in every case
because a list that has fallen behind looks exactly like a list that is complete.

**The e2e console allowlist.** `DEFAULT_ALLOWED_ERRORS` and
`DEFAULT_CRITICAL_ERRORS` name console lines to suppress or to fail on. Measured
2026-09-06: 15 of 16 entries matched nothing anything under `src/` could print.
They were casualties of the ble-mcp-test replatform and the CS108 worker rebuild,
which reworded the emissions out from under strings nobody re-checked. The
critical half is the one that matters — eight dead entries there are a gate that
cannot fire, so an e2e pass asserted less than anyone reading it believed.

**The producer guard's own exclusion list.** `every-signal-needle-has-a-producer.test.ts`
exists to catch exactly that decay in the soak signals table. Its haystack must
exclude files that *contain* a needle without being able to *emit* it — the table
itself, and test files that build the line as a fixture — because otherwise the
check is satisfiable by the thing it checks. TRA-1226 added one such exclusion,
with a comment recording that it was found by deliberate break.

Six days later that guard had a hole. Three more fixture files carrying the same
literal in live code had been added, and nobody extended the list:

```
tests/config/soak-command-rejection-op-table.test.ts:69
tests/config/soak-command-timeout-op-table.test.ts:84
tests/config/soak-command-in-flight-count.test.ts:91,102
```

Measured on plain `main`: rewording the producer in `command.ts` left that suite
**green**. The guard against dead needles had itself gone dead, in precisely the
manner it exists to prevent — and it had a docblock explaining the danger.

**TRA-1186.** The 8080→25153 port sweep enumerated the places to change and
missed docs, scripts and examples, in both repos. The same defect at four layers.

The common shape is not carelessness. Each list was correct when written and
correct for its author's next several edits. What none of them had was a reason
for the *next* person — editing a different file, for an unrelated purpose — to
come back and extend it. The edit that kills the list is always somewhere else.

## Decision

**Where a guard must scope what it covers, express the scope as a property the
code can evaluate, not as an enumeration someone must maintain.**

A rule is checked afresh on every run against the tree as it actually is. A list
is checked once, by hand, by whoever wrote it.

Concretely, in the two forms this repo hit:

* **Exclusions.** `tests/config/` is excluded from the producer haystack as a
  *tree*, justified by a property that is true of everything in it: nothing there
  emits a product log line — it builds fixtures, parses captured logs, and
  asserts. A fifth fixture file cannot silently re-open the hole, because the
  rule never asked which files exist.

* **Inclusions.** The console allowlist's haystack is `src/` alone, justified by
  what the monitor actually watches: `ConsoleMonitor` reads the browser *page*
  console, and `src/` is the only tree the page loads. This is stricter than the
  wider search and that is the point — `connection.spec.ts` passes three of the
  guarded strings as its own inline options, so a search over `tests/` would let
  each consumer prove its own liveness, and the guard would go green on the very
  entries it was built to condemn.

Where an enumeration genuinely cannot be avoided, it must **fail closed**: an
unexplained entry is an error, and the only way to pass is to state the reason.
`EXTERNALLY_PRODUCED` does this — a needle may be declared external only with its
producer named, and "not found in src" is explicitly rejected as a reason,
because that is the finding rather than the exemption.

## Consequences

**A rule can over-exclude, and that is the acceptable direction.** Excluding all
of `tests/config/` gives up the ability to prove a needle whose only producer
lives there. Nothing is in that position, and if something ever is, the guard
goes red and names it — a false alarm that costs a conversation, against a false
green that costs a month.

**Every guard needs its red state demonstrated, not argued.** Both guards touched
here were verified by deliberate break in both directions: rename the emission in
`src/` and the guard fails naming the entry; restore it and it passes. The
TRA-1226 hole is the proof this is not ceremony — that exclusion was written *by*
someone doing a deliberate break, and still decayed, because nobody re-ran the
break afterwards. **A break demonstrated once is evidence about that day only.**

**An empty guarded list is a legitimate state and must be distinguishable from a
lost one.** Pruning the dead entries left the allowlist genuinely empty. The two
lists therefore get opposite emptiness rules, and the asymmetry is the reasoning,
not a special case: an empty *critical* list is a silent catastrophe, since
nothing can fail a run; an empty *allowlist* suppresses nothing and so fails
**more** runs. Requiring the allowlist to stay non-empty would be pressure to
invent a benign condition to list — which is how the dead entries came to exist.

**This is the scope counterpart to ADR 0018.** That record governs how a check
reports its result, requiring "could not evaluate" to be positively stated rather
than share an encoding with "all clear". This one governs what a check covers in
the first place. They fail the same way from a reader's seat — a green that was
never earned — and the fixes are independent, so both are needed.
