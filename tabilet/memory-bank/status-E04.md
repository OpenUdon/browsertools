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
