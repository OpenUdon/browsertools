# Retired milestone M14 - Public API Stabilization And Docs

**Milestone.** M14
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M14.md
**Source specification.** tabilet/memory-bank/milestone.md#m14-public-api-stabilization-and-docs
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] `profile.Profile.Schema` (was `.Profile`) reads unambiguously. - [x] All exported packages have doc comments. - [x] README shows a working quick-start pipeline snippet. - [x] End-to-end integration test covers adapter→draft→review. - [x] All example profiles validate against `browser.1.5`. - [x] All baseline checks pass.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M14 Public API Stabilization And Docs

**Goal.** Stabilize pre-1.0 public package names, docs, examples, and
compatibility checks.

Acceptance:

- Public package docs describe supported adapter evidence and non-goals.
- README contains quick start commands using synthetic fixtures.
- Compatibility tests pin profile validation behavior against UWS.
- All baseline checks pass.

````

## Status record

````markdown
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
````
