# Retired milestone M02 - Module Scaffold And Browser-Profile Validation

**Milestone.** M02
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M02.md
**Source specification.** tabilet/memory-bank/milestone.md#m02-module-scaffold-and-browser-profile-validation
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] `go.mod` exists. - [x] Profile validation supports JSON and YAML. - [x] Tests cover valid and invalid browser-profile cases. - [x] UWS schema compatibility is checked.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M02 Module Scaffold And Browser-Profile Validation

**Goal.** Add the Go module and local validation for UWS browser-profile
documents.

Acceptance:

- `go.mod` declares `github.com/OpenUdon/browsertools`.
- `profile` package validates JSON/YAML browser-profile documents against
  `../uws/versions/browser.1.5.json` or an embedded synchronized copy with a
  parity check.
- Tests cover valid minimal profile, invalid action, invalid output presence
  type, origin shape, side-effect/confirmation shape, and schema load errors.
- Commands `go test ./...`, `go vet ./...`, and
  `(cd ../uws && go test ./versions)` pass.

````

## Status record

````markdown
# Status M02: Module Scaffold And Browser-Profile Validation

## Goal

Add the Go module and local validation for UWS browser-profile documents.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Create Go module | `[+]` | `go.mod` declares `github.com/OpenUdon/browsertools`, Go 1.25.4. Added to `../go.work`. |
| Add profile validation package | `[+]` | `profile/` package validates JSON/YAML against embedded `browser.1.5.json` schema via `santhosh-tekuri/jsonschema/v6`. |
| Add schema parity check | `[+]` | `profile/schema_parity_test.go` canonical-JSON compares embedded copy vs `../uws/versions/browser.1.5.json`; override via `UWS_SCHEMA_DIR`. |
| Add validation fixtures | `[+]` | `profile/testdata/valid_minimal.{yaml,json}` + 4 focused invalid cases: missing sequence, presence type mismatch, bad origin shape, sideEffect/confirmation inconsistency. |
| Add baseline checks | `[+]` | `go test ./...`, `go vet ./...`, `git diff --check`, `(cd ../uws && go test ./versions)` all pass. |

## Acceptance

- [x] `go.mod` exists.
- [x] Profile validation supports JSON and YAML.
- [x] Tests cover valid and invalid browser-profile cases.
- [x] UWS schema compatibility is checked.
````
