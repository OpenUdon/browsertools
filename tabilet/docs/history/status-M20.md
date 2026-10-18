# Retired milestone M20 - Playwright-Go Acquisition Foundation

**Milestone.** M20
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M20.md
**Source specification.** tabilet/memory-bank/milestone.md#m20-playwright-go-acquisition-foundation
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Playwright remains isolated from portable contracts and production execution. - [x] The doctor proves driver and on-disk browser readiness without installing or launching a browser. - [x] Default and cross-repository verification passes after milestone review.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M20 Playwright-Go Acquisition Foundation

**Goal.** Establish one pinned, reviewable Playwright-Go boundary before any
live acquisition command is exposed.

Acceptance:

- Browsertools directly pins Playwright-Go `v0.6201.0`, which embeds
  Playwright `1.62.1`, matching Browserdriver's browser protocol generation.
- `capture` defines a fakeable runtime lifecycle and an explicit adopted,
  private, deferred, or excluded disposition for the upstream capability
  surface; Playwright types do not enter portable Browsertools contracts.
- `playwright doctor` starts/stops only an already installed driver, verifies
  the selected executable exists as a regular file, and never installs a
  browser, launches one, or contacts a site.
- Default tests are browser- and network-free and cover invalid engines,
  missing driver/browser/session state, cancellation, cleanup, and capability
  policy.
- Product, architecture, dependency, CLI, safety, and downstream milestones
  reflect the authoring-acquisition boundary.

````

## Status record

````markdown
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
````
