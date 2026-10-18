# Retired milestone M01 - Harness And Boundary Setup

**Milestone.** M01
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M01.md
**Source specification.** tabilet/memory-bank/milestone.md#m01-harness-and-boundary-setup
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] `AGENTS.md` exists through the tracked symlink target. - [x] `memory-bank/product.md` exists. - [x] `memory-bank/architecture.md` exists. - [x] `memory-bank/tech-stack.md` exists. - [x] `memory-bank/milestone.md` exists. - [x] `memory-bank/status-M01.md` exists. - [x] `evolution/prompt-v1.md` exists. - [x] `evolution/result-v1.md` exists. - [x] Documentation states the clean pipeline and package boundaries. - [x] No implementation dependency is introduced.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M01 Harness And Boundary Setup

**Goal.** Establish Browsertools as the dedicated downstream package for
browser-profile tooling, separate from UWS schema ownership, API-source
metadata tooling, OpenUdon authoring, and runtime execution.

Acceptance:

- `AGENTS.md`, `tabilet/memory-bank/product.md`, `tabilet/memory-bank/architecture.md`,
  `tabilet/memory-bank/tech-stack.md`, `tabilet/memory-bank/milestone.md`, and
  `tabilet/memory-bank/status-M01.md` exist through the tracked symlink target.
- `tabilet/evolution/prompt-v1.md` and `tabilet/evolution/result-v1.md` record the initial
  direction.
- Documentation states the clean pipeline and the package boundaries.
- No implementation dependency is introduced during harness setup.

````

## Status record

````markdown
# Status M01: Harness And Boundary Setup

## Goal

Establish Browsertools as the dedicated downstream package for browser-profile
tooling and record the initial direction before implementation begins.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Create symlink-target harness files | `[+]` | `AGENTS.md`, `memory-bank/`, and `evolution/` are populated under `../tofu/browsertools`. |
| Record product scope | `[+]` | Product scope centers on real website UI -> browsertools -> reviewed browser-profile -> UWS binding. |
| Record architecture boundary | `[+]` | UWS owns schema; apitools owns API metadata; browsertools owns profile tooling; runtimes own execution. |
| Record tech stack defaults | `[+]` | Go-first baseline with optional adapter boundaries and fixture-first tests. |
| Build milestone roadmap | `[+]` | M01-M14 sequence created. |
| Add evolution snapshot | `[+]` | Initial prompt/result files describe the package split and milestone direction. |
| Scaffold Go module | `[+]` | `go.mod` added in M02; browsertools added to workspace `go.work`. |

## Boundary Checks

- No browser runtime execution belongs in M01.
- No UWS schema changes belong in M01.
- No API-source catalog behavior belongs in M01.
- No credentials, sessions, cookies, screenshots, or live-site captures are
  tracked in M01.

## Acceptance

- [x] `AGENTS.md` exists through the tracked symlink target.
- [x] `memory-bank/product.md` exists.
- [x] `memory-bank/architecture.md` exists.
- [x] `memory-bank/tech-stack.md` exists.
- [x] `memory-bank/milestone.md` exists.
- [x] `memory-bank/status-M01.md` exists.
- [x] `evolution/prompt-v1.md` exists.
- [x] `evolution/result-v1.md` exists.
- [x] Documentation states the clean pipeline and package boundaries.
- [x] No implementation dependency is introduced.
````
