# Status M05: Review Bundle And Safety Reports

## Goal

Produce review artifacts for profile candidates.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define review bundle shape | `[+]` | `review.Bundle`: Profile, ValidationReport, EvidenceSummary, Gaps, ConfidenceRationale, ExpiryNote, OriginSummary, SideEffectSummary. |
| Add safety report | `[+]` | `collectGaps` detects: expired evidence (lastVerifiedAt+expiresAfter), missing confirmation on write actions, CSS fallback reason missing, ambiguous locators in evidence. |
| Add deterministic renderer | `[+]` | `buildSideEffectSummary`, `buildOriginSummary`, `buildEvidenceSummary` all sort their outputs; gaps sorted by (Kind, Field). |
| Add tests | `[+]` | Valid bundle, missing confirmation, expired evidence, CSS fallback gap, ambiguous locator gap, determinism, side-effect summary. |

## Acceptance

- [x] Review bundles are secret-free.
- [x] Reports are deterministic.
- [x] Safety gaps are explicit and actionable.
