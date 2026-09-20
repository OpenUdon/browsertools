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
