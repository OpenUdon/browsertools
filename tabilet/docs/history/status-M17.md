# Retired milestone M17 - Typed Profile Contract And Safety Primitives

**Milestone.** M17
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M17.md
**Source specification.** tabilet/memory-bank/milestone.md#m17-typed-profile-contract-and-safety-primitives
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Browser-profile consumers no longer need raw action maps. - [x] Closed unions reject invalid macro/wait shapes. - [x] Literal off-origin navigation fails before runtime handoff. - [x] No runtime or Playwright dependency was added.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M17 Typed Profile Contract And Safety Primitives

**Goal.** Replace the minimal map-oriented consumer view with a complete,
engine-neutral browser.1.5 model and deterministic safety primitives.

Acceptance:

- `profile.Profile` types every action, macro union, wait union, locator,
  output, origin, side effect, confirmation policy, and inline schema.
- JSON/YAML parsing validates before returning typed documents.
- Literal navigation targets, canonical origins, and reference-aware
  ISO-8601 durations fail closed with path-tagged diagnostics.
- UWS schema parity and all seven macro round trips pass.

````

## Status record

````markdown
# Status M17 - Typed Profile Contract And Safety Primitives

## Goal

Replace the minimal profile view with the complete engine-neutral browser.1.5
contract and deterministic origin/duration safety helpers.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Complete typed profile model | `[+]` | Added strict action, seven-macro, wait, locator, output, origin, side-effect, confirmation, and inline-schema types. |
| Add validated JSON/YAML parsing | `[+]` | `ParseJSON`, `ParseYAML`, and `LoadFile` validate before returning typed profiles; YAML decoding requires EOF after the first document. |
| Add canonical safety primitives | `[+]` | Canonical HTTP(S) origins, literal navigation checks including fail-closed relative targets with multiple origins, and reference-aware ISO durations replace loose/approximate behavior. |
| Add compatibility and union tests | `[+]` | All macros, output sources, origin forms, calendar boundaries, examples, and UWS schema parity are covered. |

## Acceptance

- [x] Browser-profile consumers no longer need raw action maps.
- [x] Closed unions reject invalid macro/wait shapes.
- [x] Literal off-origin navigation fails before runtime handoff.
- [x] No runtime or Playwright dependency was added.
````
