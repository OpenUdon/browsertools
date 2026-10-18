# Retired milestone A01 - Browser Authentication Profile Tooling

**Milestone.** A01
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A01.md
**Source specification.** tabilet/memory-bank/milestone.md#a01-browser-authentication-profile-tooling
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Valid secret-free sign-in recipes can be drafted, validated, reviewed, and discovered offline. - [x] Expired, ambiguous, unsafe-origin, undeclared-slot, secret/PII-shaped, multi-document, and registry-publication cases fail closed. - [x] Credentials, MFA responses, browser state, and execution remain private runtime responsibilities.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A01 Browser Authentication Profile Tooling

**Goal.** Support the additive UWS sign-in recipe as a local, reviewed package
artifact without publishing login topology or executing authentication.

Acceptance:

- Typed parsing, deterministic drafting, digest-bound review, inclusive expiry,
  and local discovery cover `uws.browser-authentication.1.0`.
- Secret/PII-shaped values, unsafe origins, invalid slot/challenge links, and
  trailing documents fail closed.
- The static registry rejects authentication recipes; credentials, MFA
  responses, sessions, and live browser behavior remain runtime-owned.

````

## Status record

````markdown
# Status A01 - Browser Authentication Profile Tooling

## Goal

Add local-only authoring, review, and discovery support for the additive UWS
browser authentication profile without moving credentials, sessions, or live
authentication into Browsertools.

## State

Completed.

## Dependencies

- UWS `uws.browser-authentication.1.0` and
  `uws.browser-authentication-call.1.0` supplements.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement package-local authentication tooling | `[+]` | Added typed parsing, exact-origin/slot/challenge validation, secret/PII rejection, inclusive expiry, deterministic draft/review APIs and CLI commands, digest-bound review, local discovery inventory, synthetic tests, docs, and an explicit static-registry publication rejection. Browsertools still performs no live authentication. |

## Acceptance

- [x] Valid secret-free sign-in recipes can be drafted, validated, reviewed,
  and discovered offline.
- [x] Expired, ambiguous, unsafe-origin, undeclared-slot, secret/PII-shaped,
  multi-document, and registry-publication cases fail closed.
- [x] Credentials, MFA responses, browser state, and execution remain private
  runtime responsibilities.
````
