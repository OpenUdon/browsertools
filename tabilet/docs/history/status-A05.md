# Retired milestone A05 - Author-Session Navigation Lifecycle Hardening

**Milestone.** A05
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A05.md
**Source specification.** tabilet/memory-bank/milestone.md#a05-author-session-navigation-lifecycle-hardening
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - Full workspace and `GOWORK=off` tests/vet plus `git diff --check` passed. - The installed headed redirect-login loopback passed. - The published pseudo-version resolved normally, and the clean OpenUdon scenario matrix passed with no skipped or quarantined case.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A05 Author-Session Navigation Lifecycle Hardening

**Goal.** Make real headed author-session transitions deterministic when an
approved click navigates the current page or opens a reviewed popup.

Dependencies: Browsertools A04, OpenUdon E04, Browserdriver M06, and Udon M33.

Acceptance:

- The headed subprocess inherits only the display/window-session variables
  required by installed Playwright; credential, token, and provider variables
  remain excluded.
- Approved current-page navigation waits for load settlement before the next
  observation, and `target`/`formtarget` popup clicks await the new page before
  portable-context reconciliation.
- Redirect-login and popup loopbacks pass through the installed production
  Browsertools binary without changing the author-session/result v2 wire or
  profile semantics.
- Workspace/standalone tests and the coordinated 21-case OpenUdon loopback
  matrix pass at the exact published revisions.

Status: complete in Browsertools commit
`4d940eaaae16390dd79b65f1829d66e198095d7d`; see
[status-A05.md](status-A05.md).

````

## Status record

````markdown
# Status A05 - Author-Session Navigation Lifecycle Hardening

## Goal

Settle approved headed navigation and popup creation before Browsertools emits
the next reduced author-session observation.

## State

Complete.

## Dependencies

- Browsertools A04.
- OpenUdon E04 commit `0ea9f3eff281cffbcd699b2b62905090b51d5e28`.
- Browserdriver M06 commit `056969a8a94e0b3d61d6ea0f3d35e27f89e8acd4`.
- Udon M33 commit `2d2f4979d9a66f5bcb723ab4944e17f31dd1a8b8`.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Publish and qualify headed lifecycle settlement | `[+]` | Browsertools `4d940eaaae16390dd79b65f1829d66e198095d7d` (`v0.0.0-20260817224213-4d940eaaae16`) forwards only headed display/session-bus variables, waits for approved ordinary-page navigation, and captures target/formtarget popups before context reconciliation. Full workspace/standalone tests and the coordinated OpenUdon 21/21 loopback plus 4/4 public canaries passed. |

## Verification

- Full workspace and `GOWORK=off` tests/vet plus `git diff --check` passed.
- The installed headed redirect-login loopback passed.
- The published pseudo-version resolved normally, and the clean OpenUdon
  scenario matrix passed with no skipped or quarantined case.
````
