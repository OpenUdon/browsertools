# Retired milestone P02 - Publishable Browser Capability Bundle

**Milestone.** P02
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-P02.md
**Source specification.** tabilet/memory-bank/milestone.md#p02-publishable-browser-capability-bundle
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] A bundle is inert, portable, deterministic, and content-addressed. - [x] Verification proves exact profile/review/evidence identity and current lifecycle status. - [x] Browsertools still owns no workflow execution, credentials, or registry service.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### P02 Publishable Browser Capability Bundle

**Goal.** Define the canonical inert unit that can be reviewed, copied into a
static registry, and consumed by OpenUdon by digest.

Acceptance:

- `artifact.Bundle` is versioned and binds registry-neutral identity/version
  metadata, the typed browser profile, review bundle, normalized evidence,
  optional companion UWS documents, provenance/license metadata, and shared
  Evidence descriptors/lifecycle assessment.
- Bundle construction accepts only a currently promotable review bundle,
  excludes private/raw cache entries, validates every declared artifact,
  verifies all digests, and produces deterministic canonical JSON.
- Bundle verification rejects expired, stale, revoked, superseded-without-
  successor, schema-invalid, origin-conflicting, secret-bearing, or tampered
  content.
- Optional UWS companions remain inert attachments and are validated by UWS or
  OpenUdon consumers; they do not make Browsertools an authoring runtime.

````

## Status record

````markdown
# Status P02 - Publishable Browser Capability Bundle

## Goal

Create the canonical digest-bound publication unit for reviewed browser
profiles and their safe evidence.

## State

Completed.

## Dependencies

- UWS-B01 canonical browser-profile fixtures and validation helper.
- Evidence A01 content descriptors and lifecycle assessments.
- Browsertools E02 cache classifications and export boundary.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Define bundle wire contract | `[+]` | Added `browsertools.capability-bundle.v1`: canonical identity/release payload, profile, review, evidence, UWS companions, provenance/license, cache references, Evidence descriptors/assessment, and full canonical digest. |
| Implement deterministic build and verify | `[+]` | Build/Verify/Parse/CanonicalJSON/Digest re-run UWS and Browsertools gates at explicit times, derive expiry, normalize ordering, prove every payload/supporting binding, and bound the complete serialized bundle rather than only its payload. |
| Enforce safe publication | `[+]` | Exact cache bytes are rehashed; raw/ineligible/expired entries, secret-like values, session material, invalid companions, stale reviews, unsafe paths, and unconfirmed mutations fail closed. |
| Add migration and fixtures | `[+]` | Added bundle documentation, offline build/verify CLI, and canonical read-only/confirmed-side-effect bundle fixtures derived from UWS-B01 inputs for downstream consumers. |
| Run compatibility gates | `[+]` | Added standalone CI and passed standalone/workspace test and vet, bundle/cache/CLI race tests, diff checks, and focused/full UWS, Evidence, OpenUdon, and Udon suites. |

## Acceptance

- [x] A bundle is inert, portable, deterministic, and content-addressed.
- [x] Verification proves exact profile/review/evidence identity and current lifecycle status.
- [x] Browsertools still owns no workflow execution, credentials, or registry service.
````
