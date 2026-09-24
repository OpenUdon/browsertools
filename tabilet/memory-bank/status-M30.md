# M30 UWS 1.11 Browser Profile Adoption

## State

Complete locally. This milestone owns only Browsertools source adoption. Publication and downstream pin adoption remain separate review units.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| M30.1 | `[+]` | Pins published UWS `e9b6181`; exports Browser 1.8/1.9 discriminators and schema bytes; typed parsing and origin checks accept versioned templates and brace escapes. Offline `versionedTemplates` opt-in selects 1.8 or 1.9 and preserves legacy Browser 1.5 output. Synthetic positive, negative, round-trip, compatibility, and parity checks pass. No live browser action. |

## Review Gate

- Iteration 1 found a P2 compatibility regression: automatic template detection upgraded an existing Browser 1.5 draft. Replaced automatic migration with explicit `versionedTemplates` opt-in and added a legacy placeholder regression test.
- Iteration 2 reviewed the full implementation, diff, safety boundary, published pin, tests, and current documentation. No P1/P2 finding remains. Gate passes.

## Verification

- `GOWORK=off go test ./...`, `GOWORK=off go vet ./...`, `go test ./...`, `go vet ./...`, `git diff --check`.
- UWS source contract: `(cd ../uws && go test ./...)`.
- Bounded review: up to ten iterations, preserving each iteration and findings in this record. No P1/P2 may remain.

All listed commands passed on 2026-09-24. Profile schema parity, 1.8/1.9 typed round trips, unsupported-version rejection, template placement rejection, text-sink rejection, and legacy draft output checks passed. Publication and OpenUdon/Udon/Browserdriver pin adoption are downstream work.
