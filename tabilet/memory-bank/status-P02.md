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
