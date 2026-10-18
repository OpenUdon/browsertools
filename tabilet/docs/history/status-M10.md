# Retired milestone M10 - Revalidation Contracts

**Milestone.** M10
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M10.md
**Source specification.** tabilet/memory-bank/milestone.md#m10-revalidation-contracts
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Dry-run revalidation is side-effect safe. - [x] Live browser revalidation remains explicit and gated behind `LiveRevalidator`. - [x] Results are deterministic and list path-tagged check failures. - [x] Output extraction shape and ambiguous locator evidence block promotion.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M10 Revalidation Contracts

**Goal.** Define dry-run revalidation contracts for reviewed profiles.

Acceptance:

- Revalidation checks origin allowlist, locator presence/ambiguity, output
  extraction shape, expiry, and side-effect-safe waits.
- Default implementation works on saved synthetic evidence.
- Live browser revalidation remains explicit and disabled in default tests.

````

## Status record

````markdown
# Status M10: Revalidation Contracts

## Goal

Define dry-run revalidation contracts for reviewed profiles.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define `revalidate.Result` type | `[+]` | `Result{OK bool, Failures []Failure}`. `Failure` has Kind, Field, Message. CheckKind values cover origin, missing/ambiguous locators, expiry, output shape, CSS fallback, confirmation, and safe waits. |
| Implement fixture-based `Revalidator` | `[+]` | `Check(prof, records)` runs all checks: origin allowlist, locator presence/ambiguity, expiry via `internal/duration.Parse`, output shape, CSS fallback, side-effect confirmation, and side-effect-safe waits. |
| Define `Revalidator` interface | `[+]` | `FixtureRevalidator` wraps `Check`; `LiveRevalidator` stub always returns `ErrLiveNotSupported`. |
| Add synthetic tests | `[+]` | Tests cover clean checks, origin mismatch, missing/ambiguous locators, expiry, invalid output shapes, CSS fallback, side-effect confirmation, safe waits, sorted failures, live stub behavior, and the fixture revalidator interface. |
| Refactor duration parsing | `[+]` | Extracted `parseISO8601Duration` from `review/` into `internal/duration.Parse`; both `review` and `revalidate` packages use the shared implementation. |
| Share profile semantic checks | `[+]` | Output shape, CSS fallback, and side-effect rules live in `internal/profilechecks` and are consumed by both review and revalidate. |

## Acceptance

- [x] Dry-run revalidation is side-effect safe.
- [x] Live browser revalidation remains explicit and gated behind `LiveRevalidator`.
- [x] Results are deterministic and list path-tagged check failures.
- [x] Output extraction shape and ambiguous locator evidence block promotion.
````
