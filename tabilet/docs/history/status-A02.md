# Retired milestone A02 - Assisted Authentication Profile Capture

**Milestone.** A02
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A02.md
**Source specification.** tabilet/memory-bank/milestone.md#a02-assisted-authentication-profile-capture
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A02 Assisted Authentication Profile Capture

**Goal.** Let an operator manually complete sign-in and MFA in a headed,
ephemeral browser while Browsertools records only secret-free recipe evidence.

Acceptance:

- Browsertools never accepts credential environment-variable names or values,
  types credentials, answers MFA, reads cookies/storage, or exports storage
  state; the operator interacts directly with the headed browser.
- Exact login, challenge, and success origins are approved before navigation;
  unexpected origins, popups, downloads, permissions, and side-effectful
  non-authentication actions fail closed.
- The operator explicitly selects credential-slot locators, MFA alternatives,
  submit steps, and success evidence; secret scanning/redaction runs before
  any authentication draft or review artifact is written.
- Current-page locator/success checks reuse P03's value-free reporting shape;
  input values are never inspected, and authentication observation cannot
  accept caller-supplied Playwright selectors or probes.
- Closing or failing the session destroys the context. Output is a local
  `uws.browser-authentication.1.0` draft/review, never a session artifact or
  registry entry.
- Every selected alternative uses a separate context; only declared
  accessibility match counts and exact origins cross the browser boundary.
  POST is blocked except under an explicit finite ceiling for the active
  authored authentication step, and all other mutating methods stay blocked.
- The process clock supplies evidence time. The final exact-verifiable bundle
  is atomically created as a new `0600` file only after every context closes;
  stdin carries empty-line control signals and stdout carries no artifact.

````

## Status record

````markdown
# Status A02 - Assisted Authentication Profile Capture

## Goal

Observe operator-completed headed sign-in without receiving credentials or
retaining sessions.

## State

Complete.

## Dependencies

- Browsertools E03 safe Chromium live capture.
- Browsertools P03 guided capability authoring and value-free live checks.
- Browsertools A01 authentication profile tooling.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, review, and verify assisted authentication capture | `[+]` | Browsertools `f06f83c` adds profile-driven headed/manual flow observation, independent ephemeral contexts, exact origin equality, value-free locator/success checks, step-scoped bounded POST authority, fail-closed browser surfaces, process-clock evidence, exact bundle verification, atomic private output, browser-free tests, and an opt-in loopback headed smoke test. Review fixes closed caller-time, partial-output, weak-verifier, partial-session, mutable-locator, adapter-origin/count/POST, identifier-PII, canonical-loopback, and final teardown-race gaps. Browsertools still cannot type/click, receive credential/MFA values, inspect page/session data, or publish authentication artifacts. |
````
