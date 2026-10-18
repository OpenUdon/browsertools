# Retired milestone E01 - Deterministic Evidence Matching And Revalidation

**Milestone.** E01
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-E01.md
**Source specification.** tabilet/memory-bank/milestone.md#e01-deterministic-evidence-matching-and-revalidation
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Missing action evidence and declared locator drift fail closed. - [x] Navigate-only actions do not require unrelated locator evidence. - [x] Ambiguity needs a persisted rationale. - [x] Revalidation remains offline and side-effect-free.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### E01 Deterministic Evidence Matching And Revalidation

**Goal.** Make fixture-only revalidation match exact declared profile evidence
and carry explicit ambiguity decisions.

Acceptance:

- Every action has evidence and every declared sequence/a11y-output locator has
  a matching saved candidate.
- Ambiguity is resolved only by an action/locator decision with rationale.
- Origins, evidence normalization, expiry, and post-action completion waits are
  checked at an explicit time.
- No live browser interface or stub remains in the package.

````

## Status record

````markdown
# Status E01 - Deterministic Evidence Matching And Revalidation

## Goal

Make fixture revalidation exact, deterministic, and decision-aware without
launching a browser.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Add explicit locator decisions | `[+]` | Evidence decisions bind action plus role/name/text/value and require rationale. |
| Match declared locators | `[+]` | Sequence, wait, and a11y-output locators require matching saved candidates. |
| Unify deterministic safety checks | `[+]` | Checks cover evidence validity, canonical origin, inclusive expiry at an explicit time, and final-action completion waits. |
| Remove live revalidator surface | `[+]` | Deleted the stub/interface; package behavior is fixture-only. |

## Acceptance

- [x] Missing action evidence and declared locator drift fail closed.
- [x] Navigate-only actions do not require unrelated locator evidence.
- [x] Ambiguity needs a persisted rationale.
- [x] Revalidation remains offline and side-effect-free.
````
