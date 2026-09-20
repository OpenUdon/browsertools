# Status M14: Public API Stabilization And Docs

## Goal

Stabilize pre-1.0 public package names, docs, examples, and compatibility checks.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Fix `profile.Profile.Profile` field name clash | `[+]` | Renamed to `Schema string` (json/yaml tags kept as `"profile"`). Updated callers in validate_test.go. |
| Align evidence/profile timestamp field naming | `[+]` | Added doc comment to `profile.Evidence.LearnedAt` distinguishing it from `evidence.Record.ObservedAt`. |
| Audit and complete package-level doc comments | `[+]` | All 9 packages verified — all had doc comments already in place. |
| Add README quick-start | `[+]` | README updated with module path, go get, full pipeline snippet (adapter→draft→review), and evidence import fix. |
| Add end-to-end integration test | `[+]` | `integration_test.go` (package browsertools_test): synthetic Playwright fixture → adapter → draft.Build → review.Build; asserts Validation.Valid and len(Gaps)==0. |
| Add compatibility pin test | `[+]` | `TestExampleProfilesValidate` in integration_test.go: profile.ValidateFile on all examples/*/browser-profiles/*.yaml and examples/wrapper-service/browser-profile.yaml. |
| Final baseline gate | `[+]` | `go test ./...`, `go vet ./...`, `git diff --check`, `(cd ../uws && go test ./versions)` all pass. |

## Acceptance

- [x] `profile.Profile.Schema` (was `.Profile`) reads unambiguously.
- [x] All exported packages have doc comments.
- [x] README shows a working quick-start pipeline snippet.
- [x] End-to-end integration test covers adapter→draft→review.
- [x] All example profiles validate against `browser.1.5`.
- [x] All baseline checks pass.
