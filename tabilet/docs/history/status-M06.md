# Retired milestone M06 - Playwright Adapter Spike

**Milestone.** M06
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M06.md
**Source specification.** tabilet/memory-bank/milestone.md#m06-playwright-adapter-spike
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Default tests do not launch a browser. - [x] Candidate locators are accessibility-first. - [x] Adapter output can generate a draft profile.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M06 Playwright Adapter Spike

**Goal.** Import Playwright-generated evidence into the normalized evidence
model.

Acceptance:

- Adapter consumes saved accessibility snapshot / action probe fixtures.
- No live browser is required for default tests.
- Candidate locators use accessibility roles/names and carry ambiguity notes.
- Adapter output can feed M04 draft generation.

````

## Status record

````markdown
# Status M06: Playwright Adapter Spike

## Goal

Import Playwright-generated evidence into the normalized evidence model.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define saved fixture format | `[+]` | JSON: url, observedAt, actionHint, snapshot tree with role/name/children. `url` and `observedAt` are required; fixture origin must match `opts.Origin`. |
| Implement adapter | `[+]` | `adapter/playwright/`: walkSnapshot collects interactive roles; maxSnapshotDepth=64 prevents stack overflow. No live browser dependency. |
| Map locators | `[+]` | Only nodes with role in interactiveRoles enum become CandidateLocators. |
| Feed draft builder | `[+]` | Records feed draft.Build via evidence.Record.CandidateLocators. |

## Acceptance

- [x] Default tests do not launch a browser.
- [x] Candidate locators are accessibility-first.
- [x] Adapter output can generate a draft profile.
````
