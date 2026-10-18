# Retired milestone M15 - Revalidation Evidence Coverage Hardening

**Milestone.** M15
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M15.md
**Source specification.** tabilet/memory-bank/milestone.md#m15-revalidation-evidence-coverage-hardening
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): ```bash go test ./... go vet ./... git diff --check (cd ../uws && go test ./versions) ```
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M15 Revalidation Evidence Coverage Hardening

**Goal.** Tighten fixture-based revalidation so reviewed browser profiles
cannot pass without saved evidence for every declared action.

Acceptance:

- Revalidation emits `missing_evidence` when an action has no matching evidence
  record by `ActionHint`.
- The check stays fixture-only and does not launch a browser or execute UI
  actions.
- Tests cover the missing-evidence failure path and baseline checks pass.

````

## Status record

````markdown
# Status M15 - Revalidation Evidence Coverage Hardening

## Goal

Tighten fixture-based revalidation so reviewed browser profiles cannot pass
without saved evidence for every declared action.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Add missing-evidence check | `[+]` | `revalidate.Check` now emits `missing_evidence` when a profile action has no matching evidence record by `ActionHint`. |
| Preserve side-effect safety boundary | `[+]` | The new check is fixture-only and does not add live browser execution, credentials, cookies, sessions, or side effects. |
| Add regression coverage | `[+]` | `TestCheckMissingActionEvidence` proves action evidence gaps fail closed before promotion. |

## Acceptance

- [x] Every profile action must have matching saved evidence during fixture revalidation.
- [x] Missing action evidence is reported with a deterministic, path-tagged failure.
- [x] Default tests remain side-effect free and do not launch a browser.

## Verification

```bash
go test ./...
go vet ./...
git diff --check
(cd ../uws && go test ./versions)
```
````
