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
