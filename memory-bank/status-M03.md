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
