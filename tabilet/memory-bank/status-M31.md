# M31 Browser 1.8 Integer Fidelity

## State

Complete locally. This is a Browsertools correction discovered during Udon integration. Publication and downstream pin adoption remain separate.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M31.1 | `[+]` | Retains exact signed 64-bit Browser 1.8 defaults through direct/validated typed JSON and YAML parsing, `Value`, cloning, offline drafts, and review/publication bundle decode. Browser 1.9 rejects unsafe defaults. Existing fixtures remain stable and all required checks pass. |

## Review Gate

- Iteration 1 found a P2 strictness regression: a custom `Profile.UnmarshalJSON` decoder would have bypassed a parent bundle decoder's unknown-field rejection. The profile decoder now rejects unknown fields itself, with a nested-field regression test.
- Iteration 2 reviewed number fidelity, schema/version boundaries, YAML scalar output, bundle decode, compatibility, tests, and current documentation. No P1/P2 finding remains. Gate passes.

## Verification

- `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`.
- `(cd ../uws && go test ./...)` for the Browser 1.8/1.9 source contract.
- Review each iteration up to ten; no P1/P2 finding may remain.

All listed commands passed on 2026-09-24. Focused tests cover signed-int64 max defaults in Browser 1.8 JSON, YAML, direct typed decode, clones, generic values, drafts, and bundles, plus Browser 1.9's safe-integer rejection and bundle unknown-field rejection. Udon runtime integration remains its own milestone.
