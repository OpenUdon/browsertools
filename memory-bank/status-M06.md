# Status M06: Playwright Adapter Spike

## Goal

Import Playwright-generated evidence into the normalized evidence model.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define saved fixture format | `[+]` | JSON: url, observedAt, actionHint, snapshot tree with role/name/children. `url` and `observedAt` are required; fixture origin must match `opts.Origin`. |
| Implement adapter | `[+]` | `adapter/playwright/`: walkSnapshot collects interactive roles; maxSnapshotDepth=64 prevents stack overflow. No live browser dependency. |
| Map locators | `[+]` | Only nodes with role in interactiveRoles enum become CandidateLocators. |
| Feed draft builder | `[+]` | Records feed draft.Build via evidence.Record.CandidateLocators. |

## Acceptance

- [x] Default tests do not launch a browser.
- [x] Candidate locators are accessibility-first.
- [x] Adapter output can generate a draft profile.
