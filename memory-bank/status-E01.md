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
