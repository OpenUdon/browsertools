# Retired milestone E03 - Safe Chromium Live Capture

**Milestone.** E03
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-E03.md
**Source specification.** tabilet/memory-bank/milestone.md#e03-safe-chromium-live-capture
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Live acquisition is explicit, Chromium-only, exact-origin, bounded, and non-interactive. - [x] Raw ARIA/JSON-LD is finite-retention `private_raw` content and normalized evidence still requires explicit review/redaction. - [x] Default tests remain browser/network-free; installed-browser verification is an explicit loopback-only gate.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### E03 Safe Chromium Live Capture

**Goal.** Acquire accessibility-first evidence from an explicitly selected
site in a fresh Chromium context without turning acquisition into runtime
execution.

Acceptance:

- A capture request requires an exact initial HTTPS or loopback HTTP URL,
  explicit allowed origins, finite navigation/total timeouts, and bounded
  response and evidence sizes.
- Chromium launches headless in a new ephemeral context; routing rejects
  unapproved document, subresource, redirect, popup, download, dialog, file
  chooser, and persistent-protocol activity before data is accepted, and no
  permissions are granted.
- The command captures a reviewed minimal ARIA snapshot plus structured data
  candidates into private raw output, then uses existing adapters/redaction to
  create normalized evidence. It does not click, submit, upload, or resolve
  credentials.
- Default tests use a synthetic loopback site and fakes; any real-browser test
  is explicit and separately gated.

````

## Status record

````markdown
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
````
