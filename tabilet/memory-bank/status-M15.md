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
