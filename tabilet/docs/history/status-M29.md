# Retired milestone M29 - Latest Browser Profile Set Adoption

**Milestone.** M29
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M29.md
**Source specification.** tabilet/memory-bank/milestone.md#m29-latest-browser-profile-set-adoption
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - `go test ./...`, `go vet ./...`, `git diff --check`, `GOWORK=off go test ./...`. - `go mod tidy` leaves `go.mod` and `go.sum` unchanged. - `(cd ../uws && go test ./...)` for rows touching browser-profile compatibility and `(cd ../openudon && go test ./...)` for rows touching exports or discovery behavior. - `GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...` at M29.6.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M29 Latest Browser Profile Set Adoption

**Goal.** Make every profile version in the pinned UWS revision - browser 1.5
through 1.7, authentication 1.0 and 1.1, and registration 1.0 and 1.1 -
first-class across Browsertools discovery, drafting, validation helpers, tests,
and documentation, while keeping every existing 1.0 and 1.5 output
byte-identical and every exported API addition additive.

Dependencies: published UWS `9ff877ebce55`, which is already pinned; completed
A12.1 through A12.3; and unchanged author-session and result wires.

Acceptance:

- The `profile` package exports the accepted browser discriminators and
  per-version pinned schema bytes, parity tests cover 1.5 through 1.7, and no
  production code repeats a browser discriminator literal.
- Local discovery accepts context-capable authentication recipes and still
  rejects unknown authentication versions explicitly.
- Authentication drafting selects 1.1 only when a 1.1-only feature is present
  and produces digest-identical 1.0 output for existing specifications.
- Registration call 1.1 validation and discovery-free registration exports are
  test-covered without adding a submit, runtime, or private-input path.
- AGENTS, README, docs, Go doc comments, and the memory bank state the accepted
  set and the oldest-sufficient-version rule.
- Standalone, `GOWORK=off`, vet, pinned vulnerability, diff, UWS, and OpenUdon
  gates pass with no wire, fixture digest, or authority change.

Downstream impact: OpenUdon stages an authentication 1.1 profile that local
discovery previously dropped, so its staged-capture test counted a source set
that this milestone widens. OpenUdon `d042e82` now counts capability profiles
specifically and passes against both the published pin and this successor, so
pin adoption needs no further OpenUdon change.

Status: complete and published at Browsertools `5136589`; source publication
does not change the A12/W15 qualified runtime. See [status-M29.md](status-M29.md).

````

## Status record

````markdown
# M29 Latest Browser Profile Set Adoption

State markers and commit rules are defined in [milestone.md](milestone.md).

## State

Complete and published at Browsertools `5136589a29c52f0e5a32d0d415a488942c417887`,
confirmed against Browsertools `origin/main` on 2026-09-13. The A12/W15 qualified
runtime remains pinned to `ec0b9e9d6ca1`; adopting M29 into that runtime requires
separate qualification and adoption. The pinned UWS revision already published
every profile in scope, so this milestone
adopted that set across discovery, drafting, validation helpers, tests, and
documentation. No wire, fixture digest, dependency, CLI, or authority
changed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M29.1 Export profile version constants and per-version schema access | `[+]` | Browsertools `5ac0712` exports `SchemaV15`, `SchemaV16`, `SchemaV17`, `SupportedSchemas`, `SupportsSchema`, and `SchemaBytesFor`, so callers read any accepted version from the pinned UWS module instead of repeating a discriminator literal. `Validate` dispatches through `SupportsSchema` with its existing error message. The embedded 1.5 copy and `SchemaBytes` remain parity-only. Parity tests cover every accepted version, an unsupported version fails explicitly, and a browser 1.7 typed accessibility-output fixture joins the valid set. |
| M29.2 Replace discriminator literals with constants | `[+]` | Browsertools `c6779a3` points the draft builder, authenticated-authoring result, and registration author result at `profile.SchemaV15` through `SchemaV17`, `browserauthentication.ProfileName` and `ContextProfileName`, and `browserregistration.ProfileName`. No behavior or emitted byte change; existing literal assertions in the result and draft tests stay as the wire pin. |
| M29.3 Discover authentication 1.1 recipes | `[+]` | Browsertools `c7badb9` accepts `browserauthentication.ContextProfileName` beside `ProfileName` in local discovery and keeps the explicit rejection of unknown authentication versions. Adds a popup and frame 1.1 fixture, authprofile parse coverage for declared contexts and an exact success path, and discovery coverage for both acceptance and unknown-version rejection. No new source kind or candidate field. |
| M29.4 Draft authentication 1.1 by oldest-sufficient selection | `[+]` | Browsertools `f431066` adds an optional `Spec.Contexts` and selects `ContextProfileName` only for declared contexts, context-qualified steps, `opensContext`, the navigate object form, or a success path, matching the existing authenticated-authoring and registration-draft selection. A main-only specification stays digest-identical to the published 1.0 fixture. Tests pin each 1.1-only feature separately, prove an undeclared context still fails, and round-trip a 1.1 recipe through the draft and review commands. |
| M29.5 Cover registration-call 1.1 and discovery-free exports | `[+]` | Browsertools `c9bfe61` pins the explicit call-version contract: the private input binding is required by call 1.1 and refused by call 1.0, and the unversioned validator keeps its 1.0 meaning. Portable registration exports are asserted to omit discovery inventory in both the draft builder and the v3 reviewed candidate source. Test-only; no exported helper, since OpenUdon selects the UWS validator directly. |
| M29.6 Sweep documentation and complete the acceptance matrix | `[+]` | Browsertools `5136589` and Tofu `AGENTS.md` state the accepted set (browser 1.5-1.7, authentication 1.0-1.1, registration 1.0-1.1) and the oldest-sufficient-version rule across README, docs, and package comments, leaving historical milestone records as written. Full matrix passes: standalone and `GOWORK=off` tests and vet, pinned `govulncheck` with no vulnerability affecting this code, diff checks, and the complete UWS suite. |

## Compatibility And Non-Goals

- Every existing browser 1.5, authentication 1.0, and registration 1.0 output
  stays byte-identical; drafting continues to emit the oldest sufficient
  version.
- Public API changes remain additive. No `LocalSourceKind`, candidate field,
  wire schema, digest-pinned fixture, or CLI command changes.
- Registration source discovery, the private registration-input helpers,
  `browserregistration.Discovery` population, and auth-assist context support
  stay out of scope.
- No new browser authority, target, submit path, or runtime behavior.

## Downstream Impact

OpenUdon stages an authentication 1.1 profile that local discovery previously
dropped, so its workspace build sees both the authentication and the capability
candidate. `TestEngineStagesPreparedBrowserCaptureWithoutOverwriting` in
`internal/icot/browser_author_v2_test.go` counted every discovered source and
so depended on that omission. OpenUdon `d042e82` now counts capability profiles
specifically, which states what staging guarantees and holds against both the
published pin `ec0b9e9d6ca1` and this successor. Both its workspace and
`GOWORK=off` suites, vet, and repository boundary check pass, so no further
OpenUdon change is required at pin adoption.

## Verification

- `go test ./...`, `go vet ./...`, `git diff --check`, `GOWORK=off go test ./...`.
- `go mod tidy` leaves `go.mod` and `go.sum` unchanged.
- `(cd ../uws && go test ./...)` for rows touching browser-profile
  compatibility and `(cd ../openudon && go test ./...)` for rows touching
  exports or discovery behavior.
- `GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...` at M29.6.
````
