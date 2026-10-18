# Retired milestone M19 - Static Browser Capability Registry

**Milestone.** M19
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M19.md
**Source specification.** tabilet/memory-bank/milestone.md#m19-static-browser-capability-registry
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] The catalog can be generated locally and served by any static HTTPS host. - [x] Ordinary consumers have read-only access; publication remains an external reviewed repository operation. - [x] No accounts, tokens, database, custom server, or remote upload path is introduced.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M19 Static Browser Capability Registry

**Goal.** Add a service-free, content-addressed catalog that maintainers can
publish locally and serve as static HTTPS files.

Acceptance:

- `registry.Index` and immutable blob layout are deterministic, versioned,
  collision-safe, and support active/stale/revoked/superseded entries without
  mutable digest targets.
- Local publish is atomic, reuses identical blobs, rejects conflicting
  identity/version entries unless explicitly superseding, and never uploads or
  reads private cache entries.
- Search/pull/verify support local directories and bounded HTTPS reads with an
  eight-second total deadline, three-result default, 20 MiB response limit,
  redirect revalidation, unsafe-host rejection, cancellation, and an explicit
  network policy.
- `DiscoverLocalSources` reports validated profiles/bundles, ambiguity,
  rejection, truncation, origin, title, action count, score, path, digest, and
  provenance for OpenUdon.
- CLI commands cover `registry publish`, `search`, `pull`, and `verify`; remote
  publication, accounts, authentication, arbitrary public writes, and a custom
  web service remain out of scope.

````

## Status record

````markdown
# Status M19 - Static Browser Capability Registry

## Goal

Publish and discover verified bundles through a static content-addressed layout
without building a membership or application service.

## State

Completed.

## Dependencies

- Browsertools P02 publishable bundle contract.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define static registry layout | `[+]` | Added canonical `browsertools.registry-index.v1` metadata and immutable `blobs/sha256/<hex>` full-bundle paths with Evidence lifecycle assessments and dangling-successor rejection. |
| Implement atomic local publishing | `[+]` | Local-only lock/transaction flow verifies exact bundles, enforces index entry/byte bounds before commit, exclusively installs/reuses blobs, atomically replaces the index, rejects coordinate collisions, and supports same-ID explicit supersession plus reviewed stale/revoked transitions. |
| Implement bounded readers | `[+]` | Local/HTTPS search, pull, and verify enforce explicit network policy, an eight-second ceiling, three-result default, 20 MiB responses, HTTPS redirect revalidation, DNS/dial private-host checks, and cancellation. |
| Add local discovery API | `[+]` | `browsertools.DiscoverLocalSources` reports validated profiles/bundles, stale state, exact digest/path/provenance, score/actions/origins/title, duplicates, ambiguity, rejection, symlinks, and visible bounds. |
| Extend CLI and documentation | `[+]` | Added local publish and read-only search/pull/verify commands plus static hosting/cache guidance and a repository pull-request contribution model with no remote upload or membership service. |
| Run security and consumer gates | `[+]` | Passed standalone/workspace test/vet/diff and race suites; local TLS tests cover policy, timeout, cancellation, size, hostile redirect, unsafe host, tamper, collision, lifecycle, locking, and symlink cases. Existing UWS/Evidence/OpenUdon/Udon suites remain compatible; OpenUdon adoption closes in A01. |

## Acceptance

- [x] The catalog can be generated locally and served by any static HTTPS host.
- [x] Ordinary consumers have read-only access; publication remains an external reviewed repository operation.
- [x] No accounts, tokens, database, custom server, or remote upload path is introduced.
````
