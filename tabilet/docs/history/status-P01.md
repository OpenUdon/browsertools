# Retired milestone P01 - Strict Draft And Unified Promotion

**Milestone.** P01
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-P01.md
**Source specification.** tabilet/memory-bank/milestone.md#p01-strict-draft-and-unified-promotion
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Evidence cannot invent action semantics. - [x] Review and revalidation cannot disagree about promotion. - [x] Modified or stale handoff artifacts fail verification.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### P01 Strict Draft And Unified Promotion

**Goal.** Prevent evidence-only action invention and make review use the exact
same fail-closed gate as revalidation.

Acceptance:

- `draft.Spec` explicitly declares every sequence and safety policy.
- Drafting never inserts placeholder navigation, guesses clicks, or assumes
  `read_only`.
- Review bundles preserve decisions and bind exact profiles/evidence with
  canonical SHA-256 digests.
- `Bundle.Promotable` and `review.Verify` are the only promotion/handoff gate.

````

## Status record

````markdown
# Status P01 - Strict Draft And Unified Promotion

## Goal

Require explicit action intent and make review bundles use the same gate as
fixture revalidation.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Replace loose draft options | `[+]` | `draft.Spec` and `ActionSpec` require explicit sequences and safety policies. |
| Remove unsafe inference | `[+]` | No automatic `/` navigation, arbitrary click, or `read_only` default remains. |
| Unify review and revalidation | `[+]` | `review.Build` embeds the exact `revalidate.CheckAt` result and gaps. |
| Bind handoff artifacts | `[+]` | Bundles carry typed profile snapshots, decisions, assessment time, and canonical profile/evidence SHA-256 digests. |
| Add current verification | `[+]` | `Bundle.Promotable` handles the assessment snapshot; `review.Verify` rechecks digests and freshness. |

## Acceptance

- [x] Evidence cannot invent action semantics.
- [x] Review and revalidation cannot disagree about promotion.
- [x] Modified or stale handoff artifacts fail verification.
````
