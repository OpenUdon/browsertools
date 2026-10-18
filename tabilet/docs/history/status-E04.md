# Retired milestone E04 - Advanced Evidence And Cross-Browser Portability

**Milestone.** E04
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-E04.md
**Source specification.** tabilet/memory-bank/milestone.md#e04-advanced-evidence-and-cross-browser-portability
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Default rich, portability, cache lifecycle, capability-policy, and CLI tests use fakes/synthetic data and require no browser or network. - [x] Rich member/per-bundle size limits, one-hour default and 24-hour maximum retention, restrictive cache modes, mandatory secret-review annotation, non-publication, and exact-ID deletion are covered. - [x] Chromium is mandatory; engine selection is explicit and deterministic; unavailable/acquisition/check/baseline/shape outcomes are fixed diagnostics with no backend or page values. - [x] Rich and portability backends share E03's exact-origin GET/HEAD network and blocked-surface policy and expose no A02 interaction or POST interface. - [x] Installed rich-Chromium and three-engine tests are separate loopback-only opt-ins (`BROWSERTOOLS_RICH_LIVE_TEST=1` and `BROWSERTOOLS_PORTABILITY_LIVE_TEST=1`). The offline doctor confirmed that the pinned ...
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### E04 Advanced Evidence And Cross-Browser Portability

**Goal.** Evaluate richer Playwright evidence and additional engines without
weakening the minimal portable profile or private-data boundary.

Acceptance:

- Screenshots, traces, and HAR are explicit private opt-ins with restrictive
  storage, size/time retention, secret review, and deletion controls; none is
  publication eligible by default.
- Firefox and WebKit runs compare the same reviewed read-only capability
  against Chromium and produce diagnostics rather than silently rewriting
  locators.
- Popup, iframe, download, upload, permission, and visual-interaction pressure
  is recorded as supported-private, deferred, or a concrete browser.1.6
  proposal; Browsertools does not widen browser.1.5 ad hoc.
- Rich/cross-browser acquisition remains read-only and cannot reuse A02's
  authentication contexts, operator interaction windows, or POST ceilings.
- Default verification remains synthetic/offline, and rich or cross-browser
  integration suites require explicit installed-engine gates.

````

## Status record

````markdown
# Status E04 - Advanced Evidence And Cross-Browser Portability

## Goal

Evaluate private rich evidence and cross-engine portability without widening
the portable browser contract implicitly.

## State

Complete.

## Dependencies

- Browsertools E03 safe Chromium live capture.
- Browsertools P03 guided capability authoring and live checks.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, review, and verify advanced evidence and portability | `[+]` | Browsertools `d2e9db7` makes screenshot/trace/HAR one process-timed, finite-retention, non-publishable private ZIP with secret-review metadata and confirmed exact-ID deletion. Fresh Chromium/Firefox/WebKit contexts run one unchanged profile-derived probe plan and emit fixed value-free diagnostics. The capability-pressure inventory records blocked/deferred surfaces and concrete popup/frame/visual browser.1.6 candidates without changing browser.1.5. |

## Verification

- [x] Default rich, portability, cache lifecycle, capability-policy, and CLI
  tests use fakes/synthetic data and require no browser or network.
- [x] Rich member/per-bundle size limits, one-hour default and 24-hour maximum
  retention, restrictive cache modes, mandatory secret-review annotation,
  non-publication, and exact-ID deletion are covered.
- [x] Chromium is mandatory; engine selection is explicit and deterministic;
  unavailable/acquisition/check/baseline/shape outcomes are fixed diagnostics
  with no backend or page values.
- [x] Rich and portability backends share E03's exact-origin GET/HEAD network
  and blocked-surface policy and expose no A02 interaction or POST interface.
- [x] Installed rich-Chromium and three-engine tests are separate loopback-only
  opt-ins (`BROWSERTOOLS_RICH_LIVE_TEST=1` and
  `BROWSERTOOLS_PORTABILITY_LIVE_TEST=1`).

The offline doctor confirmed that the pinned driver is absent in this
environment, so the opt-in installed-engine tests were not run and no software
was installed implicitly.

## Review Findings Resolved

- [x] Replaced independent member writes with one deterministic private bundle
  so a cache failure cannot leave a misleading partial artifact set.
- [x] Registered trace/HAR cleanup before either recorder starts, including the
  trace-started/HAR-start-failed path.
- [x] Added an aggregate raw-member cap and final bundle cap in addition to the
  per-member ceiling.
- [x] Made the destructive cache open non-creating and non-chmodding, so a
  mistyped deletion root is a no-op.
````
