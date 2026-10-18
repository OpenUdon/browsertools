# Retired milestone M08 - Crawl4AI Adapter Spike

**Milestone.** M08
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M08.md
**Source specification.** tabilet/memory-bank/milestone.md#m08-crawl4ai-adapter-spike
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Crawl4AI evidence can feed draft output declarations. - [x] CSS/XPath remains constrained to output fallback evidence. - [x] No live crawling in default tests.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M08 Crawl4AI Adapter Spike

**Goal.** Import Crawl4AI crawl/scrape/extraction output as evidence.

Acceptance:

- Adapter consumes saved markdown and JSON extraction fixtures.
- CSS/XPath extraction schemas are recorded as output fallback evidence only,
  not as action locators.
- Tests cover schema-based extraction, LLM extraction notes, and fallback
  reason propagation.

````

## Status record

````markdown
# Status M08: Crawl4AI Adapter Spike

## Goal

Import Crawl4AI crawl/scrape/extraction output as evidence.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define fixture format | `[+]` | JSON: url, observedAt, actionHint, markdown, extracted[] (key/type/selector/value). `url` and `observedAt` are required; fixture origin must match `opts.Origin`. |
| Implement adapter | `[+]` | `adapter/crawl4ai/`: extracted items become CandidateOutputs with source=css; warn diagnostic added. No live crawling. |
| Map CSS/XPath evidence | `[+]` | CSS selectors go to CandidateOutput.Selector only; CandidateLocators is always empty. |
| Add tests | `[+]` | Fixture round trip, CSS-not-locators assertion, warn-diagnostic check, missing-origin/observedAt/redaction-status errors, and origin safety. |

## Acceptance

- [x] Crawl4AI evidence can feed draft output declarations.
- [x] CSS/XPath remains constrained to output fallback evidence.
- [x] No live crawling in default tests.
````
