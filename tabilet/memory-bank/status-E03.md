# Status E03 - Safe Chromium Live Capture

## Goal

Acquire bounded accessibility-first evidence in an ephemeral Chromium context.

## State

Completed.

## Dependencies

- Browsertools M20 Playwright-Go acquisition foundation.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, review, and verify safe Chromium capture | `[+]` | Browsertools `184a2fb` adds headless/non-interactive Chromium capture with exact HTTPS/loopback origins, GET/HEAD-only routing, sandboxed ephemeral contexts, closed popup/frame/download/dialog/file-chooser/protocol behavior, finite time/count/byte/depth/retention bounds, private-cache-only output, strict reviewed-fixture import, synthetic/fake default coverage, and an opt-in installed-browser loopback test. Review removed headed mode from E03, prevented credential-shaped JSON-LD outputs and raw stdout/overwrite paths, and fixed ARIA mapping text being misread as locator roles. |

## Acceptance

- [x] Live acquisition is explicit, Chromium-only, exact-origin, bounded, and
  non-interactive.
- [x] Raw ARIA/JSON-LD is finite-retention `private_raw` content and normalized
  evidence still requires explicit review/redaction.
- [x] Default tests remain browser/network-free; installed-browser verification
  is an explicit loopback-only gate.
