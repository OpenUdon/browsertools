# Status E06 - Author-Session Label Contract Hardening

## Goal

Make Browsertools the canonical owner of accessibility-label reduction and
keep suspicious live observations usable through fixed safe markers.

## State

Complete.

## Dependencies

- Browsertools E05/M22 authenticated observation and freshness contracts.
- Evidence's unchanged generic secret detector.
- Coordinated OpenUdon A05 consumer validation.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, publish, and verify the canonical live-label contract | `[+]` | Browsertools commit `55f40e56e03e2ce878521a50bc38a3674e18defc` exports the closed reducer, applies it before candidate IDs, and covers every reason, marker idempotency, security copy, both phrase lists, PII, existing and provider credentials, dotted identifiers, safe hostnames, normalization, and raw-value non-disclosure. It is published without a tag as `v0.0.0-20260817000231-55f40e56e03e`; workspace and standalone tests/vet and diff checks passed. OpenUdon A05 consumes that exact version. |

## Evolution Review

No new evolution version: E06 hardens the existing authenticated-authoring
boundary without changing product direction, wire shape, or ownership.
