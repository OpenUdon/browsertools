# Status M20 - Playwright-Go Acquisition Foundation

## Goal

Establish a pinned and isolated Playwright-Go authoring boundary before live
browser acquisition is introduced.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, review, and verify the acquisition foundation | `[+]` | Browsertools `6e66cf5` pins Playwright-Go v0.6201.0/Playwright 1.62.1, isolates a fakeable capture lifecycle and capability policy, adds an offline installed-driver/browser doctor, corrects executable-path and nil-session fail-closed findings, keeps default tests browser/network-free, and promotes E03/P03/A02/E04 with explicit safety boundaries. |

## Acceptance

- [x] Playwright remains isolated from portable contracts and production
  execution.
- [x] The doctor proves driver and on-disk browser readiness without installing
  or launching a browser.
- [x] Default and cross-repository verification passes after milestone review.
