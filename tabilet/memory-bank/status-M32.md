# Status M32 — Browser 1.10 match-count profile support

**State:** Active. M32.1 and M32.2 are complete; M32.3 is in progress.

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
| M32.3 Verify, review and publish | `[~]` | Standalone/workspace tests and vet, the pinned UWS suite, and diff checks pass. Bounded review iteration 1 finds no P1/P2. Publish and verify the exact Browsertools module for Browserdriver M15. |

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
- Published Browsertools module resolution and remote-head verification remain
  the final M32.3 steps.
