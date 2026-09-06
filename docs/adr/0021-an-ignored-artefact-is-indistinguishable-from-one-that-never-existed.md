# ADR 0021 — An ignored artefact is indistinguishable from one that never existed, so an exclusion justified by a missing input must record how the absence was established

Date: 2026-09-06
Status: Proposed
Tracking: TRA-1215 (this record), TRA-1213 / TRA-1214 (folded into it), TRA-1254 (the tests this uncovered), ADR 0018 (the sibling: a check that cannot run says so), ADR 0020 (a guard's scope is a rule, not a list of exceptions)

## Context

`frontend/tests/data` held 2.2MB of hardware captures — including
`vendor-app-packet-cap`, the only recording we have of the CSL vendor
application's dispatch engine, which is the evidence ADR 0011's
vendor-behaviour section rests on. It also held the loader modules three test
files import.

None of it was in version control, and had never been.

`.gitignore` carried a bare `data/` in its `# Database` block, beside
`postgres-data/` and `timescaledb-data/`. Read in place, its intent is
unmistakable: a local database volume at the repo root. But a bare directory
name in gitignore matches **at any depth**, and `frontend/tests/data` was the
only directory named `data` in the tree. One rule, aimed at one thing, silently
holding an irreplaceable artefact outside the repository.

That much is an ordinary mistake. What makes it worth recording is what grew on
top of it.

`vitest.config.ts` excluded two spec files:

```ts
// Tests with missing test data files (tests/data/ never created)
'**/src/worker/cs108/rfid/parser.test.ts',
'**/src/worker/cs108/rfid/inventory/handler.test.ts',
```

**That comment is true, and it is caused by the very rule it is reasoning
about.** From inside a fresh clone the directory genuinely is not there, and
"never created" is the natural reading of that. Nobody was careless. The
explanation is what someone reaches for when the artefact is absent and the
reason for its absence is invisible — and once written down, it reads as a
perfectly good reason right up until someone asks *why* it was never created.

The cost was 173 tests that ran nowhere while looking like a known, benign gap.
Re-enabling them recovered all of `parser.test.ts` and surfaced two genuine
Locate-path failures in `handler.test.ts` that had been dark for months
(TRA-1254).

Then the same shape recurred inside the fix. Having rescued the raw
`btsnoop_hci.log` out of the ignored `scratchpad/` and written a README
pointing at it, the file did not stage. `*.log`, a rule aimed squarely at run
output, had swallowed a 233KB capture that cannot be regenerated without the
handset and the vendor's app. Twice in one ticket, both invisible, and the
second one would have shipped a document referring to a file that was not
there.

### The tell

**A justification that quotes the symptom of its own cause.** "The data was
never created" describes exactly what the ignore rule produces, offered as the
reason the ignore rule's effect is acceptable. The statement is locally
verifiable and globally wrong, which is why review does not catch it: checking
it confirms it.

This is the same family as ADR 0018, where "I could not evaluate this" shared
an encoding with "all clear". Here, *"the input was never produced"* shares an
encoding with *"the input is excluded from your view"*. In both cases a
not-measurable state is silently rendered as a benign one, and downstream code
then reasons confidently from the benign reading.

It is also ADR 0020's principle applied to a different kind of guard. A bare
`data/` is a rule whose *stated* scope (a database volume) and *actual* scope
(any directory of that name, anywhere) differ — and nothing about reading it
reveals the gap.

## Decision

**1. Ignore patterns intended for one location are anchored to it.** `/data/`,
not `data/`. A pattern that is deliberately depth-independent stays that way,
but the choice is made rather than inherited from the terse form.

**2. An exclusion justified by a missing input records how the absence was
established.** "Not created" and "not visible from here" are different claims
and only one of them is checkable from inside the repo. A comment that asserts
the first without saying how it was distinguished from the second is not a
justification; the honest form names the check that was run.

**3. An artefact that is evidence is tracked, or its absence is explicit.**
Captures are not run output. A recording that cannot be reproduced without
hardware, a handset, or someone else's application is evidence, and the default
for evidence is version control. Where something is deliberately excluded — a
2.1MB bench photograph, here — the exclusion is named in `.gitignore` with its
reason, so the next reader sees a decision rather than an accident.

**4. Adding a load-bearing capture means checking it actually staged.** Broad
rules are exactly the ones nobody remembers, and a file that does not stage
produces no error. `git check-ignore -v <path>` answers it in one line.

## Consequences

`.gitignore` grows a small number of explanatory comments and one negation. That
is the intended trade: the rules are longer to read and no longer able to
silently eat something irreplaceable.

This does not argue for tracking large binaries by default, and it is not a
licence to un-ignore build output. The distinction it draws is between
**artefacts that can be regenerated** and **artefacts that cannot**. Only the
second class earns the treatment above.

It also does not claim the original rules were wrong when written. A bare
`data/` in a repo with no `tests/data` is harmless, and stays harmless until
someone adds a directory that happens to share the name — which is the point.
The rule's blast radius is a property of the tree, not of the rule, so it
changes silently as the tree grows, and nothing re-examines it.

**What would reopen this:** a case where anchoring a pattern caused something
genuinely disposable to be committed. The rules above bias toward keeping
evidence, and if that bias starts adding weight to the repository without adding
recoverable information, the third point is the one to revisit.
