# Status M33 — Browser shape producer and host-neutral worker contract

**State:** Pending. No row has started. Bounded review 0/10.

**Stage.** Kinet STG-12, Phase A. The [Stage 12 contract](../../../kinet/docs/stage12.md)
coordinates the work; this milestone and status own acceptance. Planning was
approved on 2026-10-09 (Kinet R65). It authorizes these planning files only.

**Source baseline.** `2cdd788e2f9ed38536fd48d743200e89eff7cf32` (clean at planning).
The current published UWS pin is `v0.0.0-20260925154821-80ee9bfb24a6`.

**Goal.** Produce verifiable UWS browser-profile shape tables from exact profile
bytes. Let a non-OpenUdon host, Kinet, run the author and registration workers
under a host-neutral contract.

**Dependencies.**

- [Kinet:M51](../../../kinet/tabilet/docs/history/status-M51.md) consumer contract.
- Accepted and published [UWS:C10](../../../uws/tabilet/memory-bank/status-C10.md).

**Downstream.**

- [OpenUdon:P10](../../../openudon/tabilet/memory-bank/status-P10.md)
- [Udon:M53](../../../udon/tabilet/memory-bank/status-M53.md)
- [Kinet:M56](../../../kinet/tabilet/memory-bank/status-M56.md)
- [Kinet:W20](../../../kinet/tabilet/memory-bank/status-W20.md)

**Compatibility.**

- Browser 1.5–1.10 validation and all profile bytes stay unchanged.
- These wires stay byte-compatible:
  - `browsertools.author-session.v2`;
  - `browsertools.authenticated-authoring.v2`;
  - registration author-session and authoring v1–v4;
  - profile review records.
- No profile schema change; UWS owns profile schemas.
- No live browser action, and no OpenUdon or Udon import.

**Planning reconciliation.** Stage 12 planning review, 2026-10-09; source priority not supplied. Review baseline and current revalidation: `2cdd788e2f9ed38536fd48d743200e89eff7cf32`; includes the uncommitted Stage 12 planning files. F01 (confirmed P1; containment owner Kinet:M56) amends existing M33.3 host launch requirements. Evidence: `../browserdriver/src/driver.ts` navigation-only guard and `docs/openudon-integration.md`; Kinet:M51 freezes the exact required capabilities before producer implementation. This is approved review intake, not a closing-review iteration; all rows and review counters remain pending/0.

**Parallel planning provenance.** Stage 12 parallel execution review,
2026-10-09; source priority and separate review baseline not supplied. Current
revalidation `2cdd788e2f9ed38536fd48d743200e89eff7cf32`, including uncommitted planning files. F01 (confirmed local P2) adopts scoped workflow ownership; F02 (confirmed Lower) replaces strict ordering with readiness; F04 (confirmed local P2) requires frozen consumer checks.
Evidence: the old Kinet goal/launch rules, owning agent rules, existing pending
dependencies and sibling consumer checks; Udon `go.mod`/`pkg/execute` import no
OpenUdon code. This approved intake changes no row state or review counter.

## Dispatch and lease boundaries

**Depends on.** Kinet:M51, UWS:C10. All required prerequisites must have
accepted closure at exact revisions; sibling producer adoption also needs
independently verified publication. A priority position never supplies authority.

**Downstream impacts.** OpenUdon:P10, Udon:M53, Kinet:M56, Kinet:W20.

**Write set.** The owning `browsertools/` package's implementation, tests, ordinary
documentation, manifests and qualification outputs only as required by this
milestone's existing scope, plus `tabilet/memory-bank/status-M33.md` in its
assigned worktree. Excludes `AGENTS.md`, `tabilet/GOAL.md`, shared memory-bank
files, other statuses, evolution, stages, history/knowledge, the package audit
database/sidecars, coordination docs and launch input. The coordinator alone applies shared-memory and closure
changes serially; no child writes a sibling repository or user ledger.

**Contracts read.** Immutable exact prerequisite artifacts listed above, the
M51 native-owner-reviewed contract/fixtures when applicable, the assigned
package baseline and frozen shared-memory/consumer snapshots captured at
dispatch. Cross-package checks use read-only exact snapshots or approved
published module inputs, never changing sibling checkouts. Record full source,
artifact and fixture hashes in the later execution brief; contract drift pauses
affected leases for coordinator reconciliation. Existing no-workspace/no-directory
substitution requirements for ordinary published adoption remain in force.

**Parallel-safe.** yes. Eligible only under the explicit Stage 12 lease opt-in, with no dependency path or bidirectional read/write conflict against any running lease.
At most one live milestone per package. All tests use private lease ports,
disposable stores/caches/browser profiles and unique output directories.

## Tasks

| Item | State | Notes |
| --- | --- | --- |
| M33.1 Adopt the published UWS C10 contract | `[ ]` | Move from UWS `80ee9bf` to the exact accepted and published C10 revision. Preserve Browser 1.5–1.10 schema dispatch, typed profiles, round trips and every listed wire. |
| M33.2 Browser shape-table producer and verifier | `[ ]` | Public `BuildBrowserShapeTable` / `VerifyBrowserShapeTable` from exact browser, authentication and registration profile bytes. Deterministic, bounded and offline. Projects action keys, typed parameters and outputs, side-effect class, confirmation policy, origins, credential-slot names and existing named-session metadata. Malformed or unsupported input refuses. |
| M33.3 — Host-neutral worker contract | `[ ]` | Document and test `authorworker` and `registrationauthorworker` as host-neutral, naming Kinet as a host. Include M51-required launch capabilities so M56 can contain the browser process tree and bind exact origins/identity/lifetime before any capture dispatch; no unconstrained fallback launch. Host owns network policy/claims, workers preserve conservative post-dispatch outcomes. Replace OpenUdon-only host text in `docs/openudon-integration.md` while preserving its historical meaning. No wire change. |
| M33.4 Verify, review and publish | `[ ]` | Standalone and workspace tests and vet, the pinned UWS suite, consumer builds (OpenUdon, Udon, Kinet workers), bounded review, and an exact publication handoff under named authority. |

## Review Gate

Not started. Persist each iteration before reviewing.

## Verification

- `go test ./...` and `go vet ./...` (standalone `GOWORK=off`).
- `(cd ../uws && go test ./...)` against the pinned contract.
- Consumer builds and `git diff --check`.
- Disposable fixtures only; no live browser.

## Frozen M51 producer input checkpoint — 2026-10-09

Kinet:M51.3 completed at local commit `589b844628b358167fe0a0137eded11c1028875c`. The [native-owner-reviewed contract](../../../kinet/docs/stage12-browser-contract.md), [host ABI](../../../kinet/docs/stage12-browser-host.go.txt), [public declarations](../../../kinet/docs/stage12-browser-public.go.txt) and [fixture manifest](../../../kinet/fixtures/stage12-browser-v1/manifest.json) SHA256 `0c445a5c90d2c09be561e713c364747f4ab9a3698ea8ab46e7b7b774ccf16bad` are exact producer inputs. Both native/public contract reviews pass with zero P1/P2/P3. This is input review, not this producer's implementation, publication or closing review. Kinet:M51 is now accepted locally after all five rows, review1, clean source-built checks and consumed fixture qualification; only that complete retired dependency satisfies its dispatch gate. Rows and persisted review counters remain unchanged.

Use the exact source/extraction/identity maps, per-leaf old driver protocols, complete-plan credential lease, private registration inputs, automatic TOTP versus claimed push continuations, original admitted deadline and separate bounded teardown. The supported consumer profile has one durable session binding per execution and permits other fresh named contexts. M16's immutable host-private save plan preserves v2–v11; candidate creation precedes Join and encrypted host acceptance follows Join/current-generation checks. Preserve report-v5 uncertainty and independently reproduce canonical/golden digests and positive/negative host witnesses. No current artifact claims real conformance or adoption.

**Accepted local M51 prerequisite.** [Retired M51](../../../kinet/tabilet/docs/history/status-M51.md) at observed task/review evidence `33980aafdc614424f61c32a2d79171689c7952bc` (reviewed implementation874c89e, declared coordinator closure) qualifies the unchanged frozen consumer manifest0c445a5c…. Browser qualification1+3 is consumed. This producer remains pending with its original row count/review counter; exact accepted publication and its own browser authority, where required, remain separate.
