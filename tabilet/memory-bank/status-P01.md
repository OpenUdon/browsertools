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
