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
