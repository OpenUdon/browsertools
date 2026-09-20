# Status M09: Firecrawl Adapter Spike

## Goal

Import Firecrawl search/scrape/crawl/map/extract/interact output as evidence.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define fixture format | `[+]` | JSON: url, observedAt, actionHint, scrapeId (excluded), markdown, extract map, links. `url` and `observedAt` are required; fixture origin must match `opts.Origin`. |
| Implement adapter | `[+]` | `adapter/firecrawl/`: extract fields become CandidateOutputs; scrapeId/jobId parsed but excluded from records. |
| Strip service-specific runtime details | `[+]` | ScrapeID and JobID in Fixture struct are never written to evidence.Record. |
| Add tests | `[+]` | Fixture round trip, scrapeId exclusion, ID diagnostic, missing-origin/observedAt/redaction-status errors, origin safety, and type inference (string/boolean/array/number). |

## Acceptance

- [x] Firecrawl output can inform profile drafts.
- [x] Hosted-service API details do not become portable profile fields.
- [x] No Firecrawl API calls in default tests.
