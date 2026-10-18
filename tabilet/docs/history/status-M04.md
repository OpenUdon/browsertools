# Retired milestone M04 - Draft Profile Builder

**Milestone.** M04
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M04.md
**Source specification.** tabilet/memory-bank/milestone.md#m04-draft-profile-builder
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Drafts are deterministic. - [x] Drafts validate or return actionable diagnostics. - [x] Ambiguous locators fail closed.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M04 Draft Profile Builder

**Goal.** Build deterministic browser-profile drafts from normalized evidence.

Acceptance:

- Draft builder creates action skeletons with parameters, sequence macros,
  output declarations, side effects, confirmation policy, evidence, confidence,
  expiry, and verification metadata.
- Builder refuses ambiguous action locators unless the caller supplies an
  explicit review decision.
- Generated drafts pass schema validation or return path-tagged diagnostics.

````

## Status record

````markdown
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
````
