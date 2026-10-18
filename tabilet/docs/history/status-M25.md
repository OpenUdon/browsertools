# Retired milestone M25 - Author-Session Containment Remediation

**Milestone.** M25
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M25.md
**Source specification.** tabilet/memory-bank/milestone.md#m25-author-session-containment-remediation
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M25 Author-Session Containment Remediation

**Goal.** Close the reviewed frame-disclosure, MFA-compatibility, and
interactive-lifecycle gaps without changing the author-session v2 wire.

Acceptance:

- Frame-context names cross the canonical accessibility-label reduction
  boundary before prompt or safe-review use.
- MFA checkpoints preserve an exact compatible backend subset; legacy
  backends retain the bounded same-family fallback.
- The total timeout charges active browser operations only, excluding human
  credential entry, MFA retrieval, and output review.
- Cancellation and teardown remain fail closed, and focused, standalone, vet,
  formatting, and diff gates pass.

Status: complete; see [status-M25.md](status-M25.md).

````

## Status record

````markdown
# Status M25 - Author-Session Containment Remediation

## Goal

Close the reviewed frame-disclosure, MFA-compatibility, and interactive
lifecycle gaps without changing the strict author-session v2 wire.

## State

Complete. The authoring boundary preserves narrow MFA compatibility, reduces
frame identities consistently, and excludes human waits from active browser
time.

| Item | State | Notes |
|---|---|---|
| Frame-name disclosure boundary | `[+]` | Playwright capture and the protocol server independently require exact canonical `ReduceAccessibilityLabel` output for non-empty frame names. |
| Exact MFA compatibility subsets | `[+]` | Candidate facts can carry a unique same-family subset; checkpoints reject cross-family, duplicate, or non-MFA inventories. |
| Active-operation timeout | `[+]` | Open, observe, focus, execute, and completion re-observation share the finite active budget; operator waits are excluded. |
| Regression coverage | `[+]` | Unsafe frame names, narrowed OTP/WebAuthn families, invalid challenge inventories, and human-idle behavior are covered. |
| Acceptance gates | `[+]` | Focused tests, standalone tests, vet, formatting, and diff checks pass. |

Implementation commit: Browsertools `6c3deeb`.
````
