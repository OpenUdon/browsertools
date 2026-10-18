# Retired milestone M31 - Browser 1.8 Integer Fidelity

**Milestone.** M31
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M31.md
**Source specification.** tabilet/memory-bank/milestone.md#m31-browser-18-integer-fidelity
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 2
**Verification.** Review iteration 2 passed with no remaining P1/P2. Recorded checks: - `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`. - `(cd ../uws && go test ./...)` for the Browser 1.8/1.9 source contract. - Review each iteration up to ten; no P1/P2 finding may remain. All listed commands passed on 2026-09-24. Focused tests cover signed-int64 max defaults in Browser 1.8 JSON, YAML, direct typed decode, clones, generic values, drafts, and bundles, plus Browser 1.9's safe-integer rejection and bundle unknown-field rejection. Udon runtime integration remains its own milestone.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M31 Browser 1.8 Integer Fidelity

**Goal.** Preserve signed 64-bit Browser 1.8 integer defaults through typed parsing, value conversion, cloning, and offline draft construction, so downstream runtime parameter preparation receives the reviewed exact value.

Dependencies: M30 local source adoption. Udon's Browser 1.8 execution integration is the downstream consumer.

Acceptance: JSON and YAML parse, typed clone, generic value, review and publication bundle decode, and draft clone preserve a default above JavaScript's safe-integer range exactly; Browser 1.9 continues to reject unsafe integer defaults through the pinned UWS validator. Existing profile output and APIs remain compatible. Full pinned/workspace tests, vet, diff check, and bounded review pass. No live browser action or publication.

Status: complete locally, bounded review iteration 2 passed with no open P1/P2;
downstream runtime integration remains separate. See [status-M31.md](status-M31.md).

````

## Status record

````markdown
# M31 Browser 1.8 Integer Fidelity

## State

Complete locally. This is a Browsertools correction discovered during Udon integration. Publication and downstream pin adoption remain separate.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M31.1 | `[+]` | Retains exact signed 64-bit Browser 1.8 defaults through direct/validated typed JSON and YAML parsing, `Value`, cloning, offline drafts, and review/publication bundle decode. Browser 1.9 rejects unsafe defaults. Existing fixtures remain stable and all required checks pass. |

## Review Gate

- Iteration 1 found a P2 strictness regression: a custom `Profile.UnmarshalJSON` decoder would have bypassed a parent bundle decoder's unknown-field rejection. The profile decoder now rejects unknown fields itself, with a nested-field regression test.
- Iteration 2 reviewed number fidelity, schema/version boundaries, YAML scalar output, bundle decode, compatibility, tests, and current documentation. No P1/P2 finding remains. Gate passes.

## Verification

- `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`.
- `(cd ../uws && go test ./...)` for the Browser 1.8/1.9 source contract.
- Review each iteration up to ten; no P1/P2 finding may remain.

All listed commands passed on 2026-09-24. Focused tests cover signed-int64 max defaults in Browser 1.8 JSON, YAML, direct typed decode, clones, generic values, drafts, and bundles, plus Browser 1.9's safe-integer rejection and bundle unknown-field rejection. Udon runtime integration remains its own milestone.
````
