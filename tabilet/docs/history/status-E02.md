# Retired milestone E02 - Private Browser Experience Cache

**Milestone.** E02
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-E02.md
**Source specification.** tabilet/memory-bank/milestone.md#e02-private-browser-experience-cache
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Raw browser experiences remain private and never enter tracked examples or registry bundles. - [x] Cache behavior is deterministic, bounded, atomic, and offline. - [x] Existing adapter, draft, review, and revalidation flows remain compatible.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### E02 Private Browser Experience Cache

**Goal.** Cache caller-supplied raw browser experiences and derived safe
artifacts locally without making raw captures publishable or changing the
offline-by-default acquisition boundary.

Acceptance:

- `cache.Store` uses an explicit non-symlink root, content-addressed entries,
  `0700` directories, `0600` files, atomic writes, and versioned manifests.
- Put/get/list/prune operations enforce a 20 MiB item limit, caller-supplied
  capture kind, creation/expiry times, and deterministic metadata; duplicate
  bytes reuse an entry.
- Entries distinguish `private_raw`, `normalized_evidence`, `profile`, and
  `review_bundle`; only the latter three may be considered for publication and
  only after existing redaction/promotion verification succeeds.
- Traversal, symlinks, special files, expired entries, digest mismatches,
  cancellation, partial writes, and permission failures fail closed.
- CLI cache commands remain local/offline and never launch a browser, inspect
  cookies, or upload data.

````

## Status record

````markdown
# Status E02 - Private Browser Experience Cache

## Goal

Add an explicit-root, content-addressed local cache for browser experience
inputs and safe derived artifacts while keeping raw captures private.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define cache records and store API | `[+]` | Added versioned immutable entries, four kinds, explicit times/provenance, SHA-256 identity, size/media metadata, and Put/Get/List/Prune APIs. |
| Implement safe filesystem storage | `[+]` | Explicit non-symlink root, restrictive modes, atomic directories, deduplication/conflict detection, pre-commit item-cap enforcement, 20 MiB bound, verification, cancellation, and deterministic listing. |
| Enforce privacy and promotion boundaries | `[+]` | `private_raw` can never be publication eligible; every other kind requires an explicit eligibility marker and later independent P02 verification. |
| Add offline cache CLI | `[+]` | Added put/get/list/prune with explicit root/times, stdin/stdout, overwrite protection, annotations, and JSON/text reports; no network or browser launch. |
| Add tests and docs | `[+]` | Covered expiry/prune, collisions, tamper, symlinks, modes, cancellation, no-progress/size bounds, unknown manifests, raw-publication rejection, CLI lifecycle, and ignored cache roots. |

## Acceptance

- [x] Raw browser experiences remain private and never enter tracked examples or registry bundles.
- [x] Cache behavior is deterministic, bounded, atomic, and offline.
- [x] Existing adapter, draft, review, and revalidation flows remain compatible.
````
