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
