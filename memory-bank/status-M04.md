# Status M04: Draft Profile Builder

## Goal

Build deterministic browser-profile drafts from normalized evidence.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Add draft builder API | `[+]` | `draft.Build(records, opts)` groups by ActionHint, resolves locators, builds profile map. |
| Add action skeleton generation | `[+]` | Description, parameters scaffold, sequence (navigate + optional click), outputs (microdata/a11y/css), sideEffects, confirmationPolicy. |
| Add ambiguity handling | `[+]` | `resolveLocator` refuses >1 distinct role/name without a ReviewDecision and also refuses same-role/name candidates when adapter evidence carries `AmbiguityNote`. |
| Add diagnostics | `[+]` | ValidationErrors in Result; schema validated via profile.Validate after build. |

## Acceptance

- [x] Drafts are deterministic.
- [x] Drafts validate or return actionable diagnostics.
- [x] Ambiguous locators fail closed.
