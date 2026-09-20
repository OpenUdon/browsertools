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
