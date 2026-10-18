# Retired milestone M07 - llm-scraper Adapter Spike

**Milestone.** M07
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M07.md
**Source specification.** tabilet/memory-bank/milestone.md#m07-llm-scraper-adapter-spike
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] LLM output remains untrusted evidence. - [x] Output schemas can inform profile `outputs`. - [x] No llm-scraper runtime dependency leaks into core profile contracts.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M07 llm-scraper Adapter Spike

**Goal.** Import llm-scraper structured extraction output as candidate output
schema evidence.

Acceptance:

- Adapter consumes saved llm-scraper result fixtures and JSON Schema/Zod-shaped
  schema exports when available.
- LLM-produced fields remain candidate evidence until validated.
- Tests prove output schema candidates can become browser-profile `outputs`
  without turning LLM prompts into portable profile fields.

````

## Status record

````markdown
# Status M07: llm-scraper Adapter Spike

## Goal

Import llm-scraper structured extraction output as candidate output schema evidence.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define fixture format | `[+]` | JSON: url, observedAt, actionHint, schema.properties map, extracted values. `url` and `observedAt` are required; fixture origin must match `opts.Origin`. |
| Implement adapter | `[+]` | `adapter/llmscraper/`: properties become CandidateOutputs with source=microdata (best-effort); diagnostic warns reviewer. |
| Map output schemas | `[+]` | JSON Schema property types mapped to CandidateOutput.Type. |
| Add tests | `[+]` | Fixture round trip, LLM-fields-candidate-only diagnostic, no-schema graceful handling, missing-origin/observedAt/redaction-status errors, and origin safety. |

## Acceptance

- [x] LLM output remains untrusted evidence.
- [x] Output schemas can inform profile `outputs`.
- [x] No llm-scraper runtime dependency leaks into core profile contracts.
````
