# Status M23 - Network Policy And Dependency Safety Hardening

## Goal

Close the reviewed Browsertools network-policy coverage and duplication gaps,
reject CGNAT registry destinations, and add a reproducible vulnerability gate.

## State

Completed.

## Dependencies

- Existing E03/P03/E04 read-only capture policy.
- Existing A02 authentication and A03-A05 author-session policies.
- Existing M19 static-registry network policy.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Cover remaining guard boundaries directly | `[+]` | Browsertools `aacede9bc5e92573356603d15cdc5f5e08d5d99f` directly covers authenticated declared response lengths and author-session origin approval across valid, invalid, exact-boundary, and sticky fail-closed cases; both formerly uncovered functions report 100% coverage. |
| Consolidate bounded network-guard state | `[+]` | Browsertools `9ba2850744ec425f8b4536443fe4b4220204d030` adds one private lock-agnostic request/response accounting core with overflow-safe cumulative bytes and sticky first violations. Thin locked adapters preserve live GET/HEAD resources and all five summary counters, authentication OPTIONS/step POSTs, and authoring dynamic origins/navigation/POST windows. |
| Reject CGNAT static-registry destinations | `[+]` | Browsertools `7e39fb9232089df954d65cdb57151e27f66a2938` rejects all of `100.64.0.0/10` after IPv4 normalization. Tests pin both inclusive endpoints, both adjacent public addresses, IPv4-mapped CGNAT, and representative public/private controls. |
| Add a pinned passing vulnerability gate | `[+]` | Browsertools `9e0c46598634e0ffeec58d7eea30842856f87d20` requires Go 1.26.6, pins `x/net` 0.56.0 and `x/text` 0.39.0 plus their MVS updates, and runs documented `govulncheck` v1.6.0 in CI. The standalone, workspace, race, vet, vulnerability, actionlint, and diff gates pass. |

## Acceptance

- [x] The two previously uncovered guard functions have direct 100% coverage.
- [x] Shared guard accounting is overflow-safe and retains the first policy
  violation without widening origin, method, resource, POST, or navigation
  authority.
- [x] CGNAT and IPv4-mapped CGNAT addresses are rejected at exact boundaries.
- [x] Pinned vulnerability, standalone, workspace, race, vet, workflow, and
  diff gates pass.

## Evolution Review

No new evolution version is needed. M23 hardens existing private network and
dependency-safety boundaries without changing product direction, ownership,
public Go APIs, or wire contracts.
