# Retired milestone M30 - UWS 1.11 Browser Profile Adoption

**Milestone.** M30
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M30.md
**Source specification.** tabilet/memory-bank/milestone.md#m30-uws-111-browser-profile-adoption
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 2
**Verification.** Review iteration 2 passed with no remaining P1/P2. Recorded checks: - `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`. - UWS source contract: `(cd ../uws && go test ./...)`. - Bounded review: up to ten iterations, preserving each iteration and findings in this record. No P1/P2 may remain. All listed commands passed on 2026-09-24. Profile schema parity, 1.8/1.9 typed round trips, unsupported-version rejection, template placement rejection, text-sink rejection, and legacy draft output checks passed. Publication and OpenUdon/Udon/Browserdriver pin adoption are downstream work.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M30 UWS 1.11 Browser Profile Adoption

**Goal.** Adopt the remotely published UWS 1.11 source revision and Browser 1.8/1.9 profiles in Browsertools' typed validation and offline draft authoring, while retaining oldest-sufficient output and the existing runtime authority boundary.

Dependencies: published UWS `e9b6181be0abb7f683fdb624d4dba282a59991d1`; M29 complete. Browserdriver and OpenUdon adoption are separate downstream owners.

Acceptance: the standalone pinned module accepts and round-trips 1.8/1.9, rejects unknown versions and invalid templates, emits 1.8/1.9 only when authored features require them, and preserves existing output digests. Full tests, vet, schema parity, documentation, and a bounded review pass. No live browser action or publication.

Status: complete locally, bounded review iteration 2 passed with no open P1/P2;
publication and downstream pin adoption remain separate. See [status-M30.md](status-M30.md).

````

## Status record

````markdown
# M30 UWS 1.11 Browser Profile Adoption

## State

Complete locally. This milestone owns only Browsertools source adoption. Publication and downstream pin adoption remain separate review units.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M30.1 | `[+]` | Pins published UWS `e9b6181`; exports Browser 1.8/1.9 discriminators and schema bytes; typed parsing and origin checks accept versioned templates and brace escapes. Offline `versionedTemplates` opt-in selects 1.8 or 1.9 and preserves legacy Browser 1.5 output. Synthetic positive, negative, round-trip, compatibility, and parity checks pass. No live browser action. |

## Review Gate

- Iteration 1 found a P2 compatibility regression: automatic template detection upgraded an existing Browser 1.5 draft. Replaced automatic migration with explicit `versionedTemplates` opt-in and added a legacy placeholder regression test.
- Iteration 2 reviewed the full implementation, diff, safety boundary, published pin, tests, and current documentation. No P1/P2 finding remains. Gate passes.

## Verification

- `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`.
- UWS source contract: `(cd ../uws && go test ./...)`.
- Bounded review: up to ten iterations, preserving each iteration and findings in this record. No P1/P2 may remain.

All listed commands passed on 2026-09-24. Profile schema parity, 1.8/1.9 typed round trips, unsupported-version rejection, template placement rejection, text-sink rejection, and legacy draft output checks passed. Publication and OpenUdon/Udon/Browserdriver pin adoption are downstream work.
````
