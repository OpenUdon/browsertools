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
