# Retired milestone M03 - Normalized Evidence Model

**Milestone.** M03
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M03.md
**Source specification.** tabilet/memory-bank/milestone.md#m03-normalized-evidence-model
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Evidence records are prompt-safe by default. - [x] Raw evidence remains separated from normalized evidence. - [x] Tests cover deterministic serialization and redaction state.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M03 Normalized Evidence Model

**Goal.** Define secret-free evidence records for UI observations and adapter
inputs.

Acceptance:

- Evidence records capture origin, observation kind, source tool, timestamps,
  candidate accessibility locators, candidate outputs, fallback selectors,
  redaction status, diagnostics, and provenance.
- Tests cover deterministic JSON serialization, redaction markers, stable
  sorting, and synthetic fixture round trips.
- Raw evidence is separated from normalized prompt-safe evidence.

````

## Status record

````markdown
# Status M03: Normalized Evidence Model

## Goal

Define secret-free evidence records for UI observations and adapter inputs.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define evidence records | `[+]` | `evidence.Record`: origin, observationKind, observedAt, actionHint, candidateLocators, candidateOutputs, redactionStatus, redactedFields, diagnostics, provenance. |
| Add redaction markers | `[+]` | `RawRecord.Normalize()` enforces redaction status; `RawData` excluded from JSON. |
| Add deterministic serialization | `[+]` | `MarshalDeterministic` + `Sort`, `SortLocators`, `SortOutputs`, `SortDiagnostics`. |
| Add synthetic fixtures | `[+]` | All tests use synthetic in-memory records; no real-site dependency. |

## Acceptance

- [x] Evidence records are prompt-safe by default.
- [x] Raw evidence remains separated from normalized evidence.
- [x] Tests cover deterministic serialization and redaction state.
````
