# Retired milestone M32 - Browser 1.10 match-count profile support

**Milestone.** M32
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M32.md
**Source specification.** tabilet/memory-bank/milestone.md#m32-browser-110-match-count-profile-support
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 1
**Verification.** Review iteration 1 passed with no remaining P1/P2. Recorded checks: - `GOWORK=off go test -count=1 ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, and `git diff --check` pass. - `(cd ../uws && go test ./...)` passes at the published UWS 1.10 source. - Focused draft/profile tests and vet pass; schema parity covers every accepted discriminator and unsupported 1.11 rejection. - GitHub `main` resolves to `3abe70efc03d9ccb97b8b30e5e86328f60a70c64`. Go resolves `v0.0.0-20260925161530-3abe70efc03d` to the same origin hash, with module sum `h1:MaR46QbewrQNNN453TIws0A3dZeen9QJwiUVBGehZiA=`. - Focused tests and vet pass in the downloaded published module using the installed Go 1.26.6 binary with the module cache offline. The first offline invocation selected a toolchain that required checksum lookup while SumDB was disabled; the installed-toolchain rerun passed without changing sources.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M32 Browser 1.10 Match-Count Profile Support

Add the separately published UWS Browser 1.10 selector match-count profile to
Browsertools typed validation and offline draft construction. Preserve Browser
1.9 output and prior profile bytes. Tests must cover zero, one, multiple, invalid
and missing count forms, with no page text or attributes in outputs. The exact
UWS M05 commit is a prerequisite; runtime execution remains downstream. See
[status-M32.md](status-M32.md).


### M32 Browser 1.10 Match-Count Profile Support

**Goal.** Add Browser 1.10 to typed profile validation, round-trip handling, and
offline draft authoring using the published UWS M05 profile definition. Preserve
Browser 1.9 and older bytes and oldest-sufficient output behavior.

**Acceptance.** Validate and round-trip count action/output shapes, preserve
zero and multiple-match results as typed nonnegative integers, and reject invalid
or unsupported values without retaining page text or attributes. Add fixtures and
focused/full tests and vet; review the full milestone diff. No live browser
action or publication beyond the reviewed Browsertools source is in scope.

Status: complete and published at Browsertools
`3abe70efc03d9ccb97b8b30e5e86328f60a70c64`, with no open P1/P2 in bounded
review iteration 1. The exact UWS dependency is
`v0.0.0-20260925154821-80ee9bfb24a6`. Browserdriver M15 owns downstream runtime
support and pin adoption.

````

## Status record

````markdown
# Status M32 — Browser 1.10 match-count profile support

**State:** Complete and published at Browsertools `3abe70efc03d9ccb97b8b30e5e86328f60a70c64`.

**Goal.** Add the published UWS Browser 1.10 profile to typed validation,
round-trip handling and offline draft authoring.

**Dependency.** UWS M05 is published at `80ee9bfb24a688b5e875dadf9ecacdc65398f1ff` (`v0.0.0-20260925154821-80ee9bfb24a6`); Browsertools pins this exact source.

**Compatibility.** Preserve Browser 1.9 and older profile bytes and existing
oldest-sufficient behavior. No live browser action is included.

## Tasks

| Item | State | Notes |
| --- | --- | --- |
| M32.1 Add Browser 1.10 typed profile validation | `[+]` | Pins UWS `v0.0.0-20260925154821-80ee9bfb24a6`; adds Browser 1.10 schema dispatch and typed `matchCount`, `within`, and `visibility` fields. Focused profile tests, profile vet, pinned UWS tests, and `git diff --check` pass. |
| M32.2 Add round-trip and offline authoring coverage | `[+]` | Explicit match-count outputs select Browser 1.10; valid profiles round-trip through JSON and YAML with typed nonnegative integer bounds. Invalid declarations fail draft validation, evidence candidates do not override explicit output intent, and legacy template/profile selection remains unchanged. Focused draft/profile tests, vet, and `git diff --check` pass. |
| M32.3 Verify, review and publish | `[+]` | Standalone/workspace tests and vet, the pinned UWS suite, and diff checks pass. Bounded review iteration 1 finds no P1/P2. Published and verified as `v0.0.0-20260925161530-3abe70efc03d`; Go module origin hash matches this commit. |

## Review Gate

- Iteration 1 reviewed the full M32 diff, exact UWS pin, schema dispatch,
  typed JSON/YAML round trips, explicit count-driven version selection,
  legacy template behavior, invalid declarations, and the browser/runtime
  boundary. No P1/P2 finding remains. `tabilet/evolution/` was checked; this
  adopts the established UWS profile boundary without changing Browsertools'
  product direction or ownership.

## Verification

- `GOWORK=off go test -count=1 ./...`, `GOWORK=off go vet ./...`,
  `go test ./...`, `go vet ./...`, and `git diff --check` pass.
- `(cd ../uws && go test ./...)` passes at the published UWS 1.10 source.
- Focused draft/profile tests and vet pass; schema parity covers every accepted
  discriminator and unsupported 1.11 rejection.
- GitHub `main` resolves to `3abe70efc03d9ccb97b8b30e5e86328f60a70c64`.
  Go resolves `v0.0.0-20260925161530-3abe70efc03d` to the same origin hash,
  with module sum `h1:MaR46QbewrQNNN453TIws0A3dZeen9QJwiUVBGehZiA=`.
- Focused tests and vet pass in the downloaded published module using the
  installed Go 1.26.6 binary with the module cache offline. The first offline
  invocation selected a toolchain that required checksum lookup while SumDB
  was disabled; the installed-toolchain rerun passed without changing sources.
````
