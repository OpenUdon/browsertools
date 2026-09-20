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
