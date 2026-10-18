# Retired milestone M11 - OpenUdon Integration Handoff

**Milestone.** M11
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M11.md
**Source specification.** tabilet/memory-bank/milestone.md#m11-openudon-integration-handoff
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] `review.Bundle` is fully documented and JSON-tagged. - [x] Example UWS operation binding exists with `browser-profile` source type and `sourceOperationId`. - [x] Consumer contract doc explains the promotion gate (`Validation.Valid`). - [x] Browser runtime behavior remains downstream of the review bundle.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M11 OpenUdon Integration Handoff

**Goal.** Provide the artifact contracts OpenUdon needs to consume reviewed
browser profiles.

Acceptance:

- Exported review bundle shape is stable enough for OpenUdon package evidence.
- Example UWS operation binding uses `sourceDescriptions[].type:
  browser-profile` and `sourceOperationId`.
- Cross-repo OpenUdon tests or fixture checks are documented.

````

## Status record

````markdown
# Status M11: OpenUdon Integration Handoff

## Goal

Provide the artifact contracts OpenUdon needs to consume reviewed browser profiles.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Audit `review.Bundle` JSON tags and doc comments | `[+]` | All fields have `json:` tags. Fixed `Gaps` and `ActionsRequiringConfirmation` to serialize as `[]` not `null`. |
| Add explicit UWS binding fixture | `[+]` | `examples/openudon-binding/binding.uws.yaml` — uses correct `sourceDescriptions[].type: browser-profile`, `operations[].sourceOperationId`, and `steps[].operationRef` pattern from UWS 1.5.0. Paired with `review-bundle.json`. |
| Document cross-repo consumer contract | `[+]` | `docs/openudon-integration.md` — explains stable fields, promotion gate (`Validation.Valid && len(Gaps)==0`), and the correct UWS binding pattern. |
| Add cross-repo gate note | `[+]` | Documented in `docs/openudon-integration.md`: `(cd ../openudon && go test ./...)`. |

## Acceptance

- [x] `review.Bundle` is fully documented and JSON-tagged.
- [x] Example UWS operation binding exists with `browser-profile` source type and `sourceOperationId`.
- [x] Consumer contract doc explains the promotion gate (`Validation.Valid`).
- [x] Browser runtime behavior remains downstream of the review bundle.
````
