# Retired milestone A03 - Persistent Authenticated Browser Authoring

**Milestone.** A03
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A03.md
**Source specification.** tabilet/memory-bank/milestone.md#a03-persistent-authenticated-browser-authoring
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Workspace and standalone `go test ./...` pass. - [x] `go vet ./...` and `git diff --check` pass. - [x] Default tests are browser-free and network-free. - [x] `BROWSERTOOLS_AUTHOR_LIVE_TEST=1` is required for the headed loopback fixture; it was not enabled because the pinned browser is not installed here.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A03 Persistent Authenticated Browser Authoring

**Goal.** Keep one headed, non-persistent Chromium context alive across human
authentication and bounded post-login exploration behind a strict local
protocol.

Acceptance:

- `browsertools author-session chromium --private-root ...` reserves stdout for
  closed, bounded `browsertools.author-session.v1` NDJSON and never persists its
  transcript.
  This historical A03 wire was superseded by A04's strict
  `browsertools.author-session.v2`; the ownership and non-persistence boundary
  remains unchanged.
- Browsertools-issued candidate IDs are the only interaction targets. Human
  credential/OTP values remain in Chromium; phone-push checkpoints carry no
  invented locator.
- Clicks, new origins, and authentication POST ceilings have exact, non-reusable
  approvals; new-origin link clicks require origin approval and then a separate
  action approval.
- Unexpected navigation, origin escape, malformed input, denial, ambiguity,
  CAPTCHA, browser failure, timeout, unsupported surfaces, or teardown failure
  closes the context and emits no result.
- Popup/frame contexts are bounded, exact-origin, uniquely identified, and no
  live handle, cookie, storage state, input value, or raw page material crosses
  the process boundary.
- Default tests use fake sessions and policy units; the installed headed
  redirect-login loopback fixture remains an explicit desktop opt-in.

````

## Status record

````markdown
# Status A03 - Persistent Authenticated Browser Authoring

## Goal

Keep one headed Chromium context alive across human login/MFA and post-login
exploration without exporting session or credential material.

## State

Complete.

## Dependencies

- Browsertools M21 authenticated goal-authoring design.
- UWS 1.8 browser-context contracts.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, review, and verify the persistent author session | `[+]` | Browsertools `50bbfdd` adds the strict NDJSON state machine, explicit CLI, headed non-persistent Chromium owner, human credential/OTP/push checkpoints, exact same-origin GET and separately approved origin/click/POST authority, popup/frame registry, finite bounds, sanitized environment, fixed failure diagnostics, teardown-before-result, fake-session coverage, and an opt-in installed-Chromium redirect-login fixture. No value, cookie, storage state, raw semantic tree, selector, browser handle, or resumable context crosses the process boundary. |

## Verification

- [x] Workspace and standalone `go test ./...` pass.
- [x] `go vet ./...` and `git diff --check` pass.
- [x] Default tests are browser-free and network-free.
- [x] `BROWSERTOOLS_AUTHOR_LIVE_TEST=1` is required for the headed loopback
  fixture; it was not enabled because the pinned browser is not installed here.
````
