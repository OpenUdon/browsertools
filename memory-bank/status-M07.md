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
