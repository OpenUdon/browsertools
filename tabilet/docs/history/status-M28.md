# Retired milestone M28 - UWS 1.9.1 Browser Content-Trust Resolver

**Milestone.** M28
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M28.md
**Source specification.** tabilet/memory-bank/milestone.md#m28-uws-191-browser-content-trust-resolver
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - `GOWORK=off go test ./...` and `GOWORK=off go vet ./...`. - `GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...`. - Hosted Browsertools CI at the published commit. - Exact downstream Udon M37, OpenUdon E12, and Ramen M73 compatibility gates. - `git diff --check`.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M28 UWS 1.9.1 Browser Content-Trust Resolver

**Goal.** Supply Browsertools-owned operation contracts to UWS 1.9.1's
explicit advisory analyzer without changing browser-profile schemas,
validation, or execution.

Dependencies: published UWS 1.9.1 at `9e676eaa469e` and unchanged Browser 1.7.

Acceptance:

- One public resolver owns only explicitly supplied browser-profile source
  descriptions and returns deterministic RFC 6901-relative data,
  instruction, authority, output-provenance, and value-capability contracts.
- Browser-derived outputs default to untrusted; strings, narrowed scalars,
  composites, presence outputs, and inline enums retain distinct capability
  classifications without laundering provenance.
- Missing profiles/actions/outputs, cancellation, caller mutation, and future
  unsupported forms fail or remain unknown through stable advisory behavior.
- Complete standalone tests, vet, vulnerability, diff, and exact downstream
  Udon/OpenUdon/Ramen compatibility gates pass.
- Browser 1.7 bytes, ordinary validation, browser execution, and runtime
  authority remain unchanged.

Status: complete and published at Browsertools
`75fd5c3ab81f904243f8c2650c61ba1cd8c00540`; see
[status-M28.md](status-M28.md).

````

## Status record

````markdown
# M28 UWS 1.9.1 Browser Content-Trust Resolver

State markers and commit rules are defined in [milestone.md](milestone.md).

## State

Complete and published at Browsertools
`75fd5c3ab81f904243f8c2650c61ba1cd8c00540`
(`v0.0.0-20260826234723-75fd5c3ab81f`).

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M28.1 Pin UWS 1.9.1 and add the browser-profile resolver | `[+]` | Browsertools `e5aaa6413e9e9d6cbba914cbf4d6d02699771efc` pins UWS `9e676eaa469e` and adds `contenttrust.NewResolver`. The resolver owns only caller-supplied browser-profile source descriptions, models request and typed text placeholders as data, navigation and explicit option/confirmation inputs as authority or instruction channels, and marks browser-derived outputs untrusted without inheriting input provenance. Strings remain free text unless constrained by an inline string enum; integer, number, boolean, null, and `presence: true` outputs are constrained scalars; arrays and objects are composite; unsupported future forms remain unknown. Deterministic channel ordering, RFC 6901 token handling, cancellation, missing profile/action/output failures, and defensive profile cloning are covered by tests. |
| M28.2 Document, verify, and publish the advisory boundary | `[+]` | Browsertools `75fd5c3ab81f904243f8c2650c61ba1cd8c00540` documents the explicit resolver entry point, untrusted browser-output defaults, provenance-versus-capability distinction, and non-enforcement boundary. Hosted CI passed module download, the complete standalone test suite, vet, pinned vulnerability scanning, and diff checks. No browser profile schema, Browser 1.7 artifact, validation behavior, execution behavior, or browser authority changed. |
| M28.3 Complete downstream compatibility evidence | `[+]` | Udon M37 (`207e7f1`) consumes M28 through its explicit profile analyzer, OpenUdon A22/P06/E12 (`cc378be`, with hosted-CI hermeticity follow-up `2c99fde`) qualifies warning-only review evidence and legacy packages, and Ramen M73 (`e9f99ca`) preserves declarations and surfaces findings through validation. Their content-trust gates passed against the exact published Browsertools and UWS revisions. |

## Compatibility And Non-Goals

- The resolver is an explicit adapter for `contenttrust.Analyze`; it is not
  called by Browsertools profile validation or browser execution paths.
- Browser-derived values remain untrusted even when a scalar type or inline
  enum removes free-text capability.
- M28 does not revise immutable Browser 1.7 artifacts, add runtime enforcement,
  expose browser content in reports, or grant browser/session/target authority.

## Verification

- `GOWORK=off go test ./...` and `GOWORK=off go vet ./...`.
- `GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...`.
- Hosted Browsertools CI at the published commit.
- Exact downstream Udon M37, OpenUdon E12, and Ramen M73 compatibility gates.
- `git diff --check`.
````
