# Status M33 — Browser shape producer and host-neutral worker contract

**State:** M33.1–M33.2 complete; M33.3–M33.4 pending. Bounded review 0/10.

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
- Accepted and published [UWS:C10](../../../uws/tabilet/docs/history/status-C10.md).

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
| M33.1 Adopt the published UWS C10 contract | `[X]` | Adopted `v0.0.0-20261009220914-f01a2542410c` from independently qualified public archive, no replacement. Full offline standalone tests and vet pass, including schema/round-trip/wire compatibility.  Move from UWS `80ee9bf` to the exact accepted and published C10 revision. Preserve Browser 1.5–1.10 schema dispatch, typed profiles, round trips and every listed wire. |
| M33.2 Browser shape-table producer and verifier | `[X]` | Verified byte-bound/lossless public APIs in `browser_shapes.go`; eleven exact C10 native vectors and unchanged M51 golden/fixture hashes pass, with adversarial Unicode/duplicate/depth/YAML/budget/source/effect/unsafe-integer checks.  Public `BuildBrowserShapeTable` / `VerifyBrowserShapeTable` from exact browser, authentication and registration profile bytes. Deterministic, bounded and offline. Projects action keys, typed parameters and outputs, side-effect class, confirmation policy, origins, credential-slot names and existing named-session metadata. Malformed or unsupported input refuses. |
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

## Accepted UWS:C10 prerequisite — 2026-10-09

Exact public implementation f01a2542410c583d0ea909dadd8f17527cc0d27d resolves as v0.0.0-20261009220914-f01a2542410c, archive h1:TTjAn0++TdGHY0vahbXw3XsICQaavBytoqTSHu0K6o0= and GoMod h1:DlqFOnO9lbmYWLLIh5WicNX6NTWIuytU6mIHmxj9BVw=. Whole review1 and independently fetched closing publication 60e1baa0470cd7dca5d3166d344896bc0689abf7 qualify this input. See [public proof](../../../uws/docs/c10-publication.md). Own adoption, other prerequisites, custody and qualification remain pending; no task/review/pin change. Scoped source-publication authority is separate conversation approval; this record grants none. Audit stays disabled.

## Authorized lease checkpoint — 2026-10-10

Assigned `Browsertools:M33` only, `goal/M33` in `/home/peter/Workspace/browsertools.goal/M33`. Captured integration is `refs/heads/main` in `/home/peter/Workspace/browsertools`; original goal base `c146fa8db0675ab3cba01748aafaecd1c5ae9f83` remains unchanged. Dispatch/review baseline is `c209612df309228c26d8645d0fadb653b6334911`. Task commits and offline verification are authorized under the resolved Stage12 goal. Protected child grants are empty; the coordinator alone integrates, closes and publishes. Shared memory and historical records are excluded. Audit remains disabled.

Frozen protocol SHA256 `cad15b1c175094505d380c581f577bde7ced4109ffda40b4b39d5ffe6ae88113`; resolved request `71c9cd3cb4126d6c40b89f491495dc849805aaee281815d729946d6d85519f8c`; context `54bd2fe09d3f67a01c31f911976349cff15e572e8e3460a09811f5916caeaa9b`. The existing human approval context was passed by the coordinator; these hashes and status notes are evidence, never approval. Exact M51 manifest `0c445a5c90d2c09be561e713c364747f4ab9a3698ea8ab46e7b7b774ccf16bad` and frozen consumer sources are used without editing siblings. No live/browser/download operation is authorized.

M33.1 verification: sanitized `GOWORK=off GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` Go1.26.6, private module/build/temp caches, compiler parallelism2. `go test ./...` and `go vet ./...` pass. First setup attempt refused missing cached dependencies offline; copying exact already-cached dependency archives resolved it without retrieval. C10 public archive SHA256 `5f0478a4a2a87fa54e921a685d70364ade125d01cb5dacc0bcd09ec11d7a2c47` is preserved; module/GoMod sums match the accepted prerequisite. No schema, wire, default or browser action changed.

M33.2 verification: focused new producer tests, full offline `go test ./...`, `go vet ./...` and `git diff --check` pass. The complete action/flow digest never uses typed projection; parameter/output/registration schemas retain native numbers, extraction annotations and conditions. Sources and selectors use native deterministic UWS table ordering. Named session names remain existing operation-level call metadata; no invented profile field or private session values. These are shape witnesses, not actual runtime/browser qualification. Review remains0.
