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
