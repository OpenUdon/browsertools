# A10 Registration Third-Party Resource Denial Remediation

Item | State | Notes
---|---|---
A10.1 Separate safe denied resources from fatal policy violations | `[+]` | `capture/playwright_registration.go` now classifies method and persistent-channel safety before origin disposition, preserves fatal navigation/invalid-authority behavior, and counts canonical unapproved non-navigation GET/HEAD resources while returning a nonfatal denial. Focused decision-matrix tests pass, including unapproved mutation, persistent-resource, navigation, malformed URL, continued approved traffic, and exact accounting cases.
A10.2 Prove zero-contact headed behavior and update owned documentation | `[+]` | The installed Playwright 1.62.1 doctor passed on human-visible `DISPLAY=:0`. The trusted sandbox-helper headed test used separate approved and blocked loopback origins; the approved page remained observable, the blocked server received zero requests, denied-attempt accounting was exact, and the test/Node/Chromium descendants stopped. README, browser-registration, architecture, and technical documentation now describe the generic distinction without a target-specific origin.
A10.3 Complete qualification, bounded review, and publication handoff | `[+]` | Pinned Go 1.26.6 full tests, vet, focused race, exact `govulncheck@v1.6.0`, read-only Playwright doctor, headed two-origin loopback, formatting, diff, and privacy checks pass. Review iteration 5 found no P1/P2-or-higher issue, so the bounded review gate passes. Browsertools is published at `3107470313d447e29c5ac5912c3cc9d221d46967` (`v0.0.0-20260829181035-3107470313d4`), and OpenUdon adopted and qualified that exact module at `dd7437c0149903ee7af987cd2e02380735ccbc40`. Downstream target use remains separately authorized.

## Review provenance

- Review: **W02.3 registration-authoring diagnostic**, finding `F01`.
- Source priority: not supplied. Local severity: **P2**.
- Review baseline: not supplied. Revalidated at Browsertools
  `ee565625619bb6148bdb3cc589cfb6466d8837c1` with a clean worktree; no
  uncommitted change was part of the evidence.
- Baseline evidence: `capture/playwright_registration.go` installed a route
  that aborted a denied request before contact, while
  `registrationNetworkGuard.allowBrowser` converted every failed
  resource-origin check into terminal `origin_escape`.
- Current implementation evidence: `capture/playwright_registration.go`
  distinguishes well-formed unapproved non-navigation GET/HEAD resources from
  fatal violations, and `capture/playwright_registration_test.go` proves the
  decision matrix, continued observation, exact accounting, and headed
  two-origin zero-contact behavior.
- Lineage: the defect affects the completed A09 guarded Chromium registration
  producer and its A08 network-policy foundation. It does not reopen either
  completed milestone.

## Review disposition

Finding | Source priority | Local severity | Disposition | Owner
---|---|---|---|---
F01 — safely blocked third-party GET/HEAD resource poisons registration observation | not supplied | P2 | Implemented, qualified, published, and adopted by OpenUdon without widening origins | A10.1–A10.3

## Verification

- Focused registration guard tests and race test.
- Explicit installed headed two-origin loopback with zero blocked-origin
  contact; no public target access.
- `go test ./...`, `go vet ./...`, pinned vulnerability gate when locally
  available, formatting, and `git diff --check`.
- Exact published-module OpenUdon tests, vet, `make check`, sibling/API
  boundary checks, and documentation-memory validation.

## Bounded review-fix gate

Ordinary review intake did not start the gate counter. The maximum is 10
iterations.

- Iteration 1 — one P2: the new headed loopback closed the session only on its
  success path, so an assertion or observation failure after `Open` could leave
  the test Chromium tree alive. One lower-severity documentation finding: the
  milestone current-state paragraph still described the pre-implementation
  guard. Both fixes passed focused and headed verification.
- Iteration 2 — one P2: the iteration-1 `t.Cleanup` ran after the loopback
  servers' function defers, so an early failure could wait in server teardown
  before closing the browser. Replace it with a later-registered function defer
  so session teardown always runs first. The fix passed the headed zero-contact
  loopback and focused race suite.
- Iteration 3 — reviewed the full A10 implementation, both preceding fixes,
  decision ordering, zero-contact evidence, accounting, documentation, and
  status changes. No code P1/P2 remained, but a later inventory found that this
  status still presented baseline behavior and disposition as current.
- Iteration 4 — one P2: review provenance still called the pre-fix
  `origin_escape` behavior “current evidence,” and the disposition still said
  only “remediate.” Correct the status to separate baseline from current
  implementation and publication state. The correction passed structural and
  diff checks.
- Iteration 5 — reviewed the complete implementation, tests, owned docs,
  milestone/status truth, provenance, and blocker handoff. No P1, P2, or
  higher-severity finding remains; the bounded review-fix gate passes at
  iteration 5.

## Implementation notes

- 2026-08-29 — A10.1 focused verification passed with pinned Go 1.26.6:
  `go test -count=1 ./capture -run 'TestRegistrationNetworkGuard|TestRegistrationV2URLFacts|TestValidateRegistrationBrowserRequest|TestPlaywrightRegistrationHasNoInput'`.
- 2026-08-29 — A10.2 read-only Playwright doctor passed for Playwright 1.62.1
  and installed Chromium. The explicit headed two-origin loopback passed on
  human-visible X11 with the trusted sandbox helper; the blocked origin saw
  zero contact and no test worker/Node/Chromium descendant remained.
- 2026-08-29 — Final local verification passed full `go test -count=1 ./...`,
  `go vet ./...`, `go test -count=1 -race ./capture`, formatting, diff, and
  target-detail exclusion under pinned Go 1.26.6. The user-authorized exact
  `govulncheck@v1.6.0` fetch and scan then passed with no called vulnerability;
  the tool is cached without changing the module graph or repository files.
- 2026-08-29 — A disposable Go workspace bound clean OpenUdon
  `1255a449083165fbff3a1e1c02e3b44c87d7213e` to the local Browsertools A10
  tree. OpenUdon's full tests passed; vet and focused `browsercandidate`,
  `browsertransactioneval`, `icot`, and `cmd/icot` tests passed offline after
  the locked graph was cached. The workspace was removed and OpenUdon stayed
  clean.
- 2026-08-29 — Browsertools A10 was published at
  `3107470313d447e29c5ac5912c3cc9d221d46967`
  (`v0.0.0-20260829181035-3107470313d4`). OpenUdon pinned that exact module,
  updated its content-trust dependency lock, passed its complete offline
  consumer gates, and published the adoption at
  `dd7437c0149903ee7af987cd2e02380735ccbc40`.
