# Retired milestone M09 - Firecrawl Adapter Spike

**Milestone.** M09
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M09.md
**Source specification.** tabilet/memory-bank/milestone.md#m09-firecrawl-adapter-spike
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Firecrawl output can inform profile drafts. - [x] Hosted-service API details do not become portable profile fields. - [x] No Firecrawl API calls in default tests.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M09 Firecrawl Adapter Spike

**Goal.** Import Firecrawl search/scrape/crawl/map/extract/interact output as
evidence.

Acceptance:

- Adapter consumes saved Firecrawl API response fixtures.
- API-specific job IDs, scrape IDs, and hosted-service behavior stay adapter
  evidence, not portable profile fields.
- Tests cover scrape output, map links, structured extraction, and interaction
  notes.

````

## Status record

````markdown
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
````
