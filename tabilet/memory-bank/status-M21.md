# Status M21 - Authenticated Goal-Directed Browser Authoring Design

## Goal

Fix the reviewed cross-repository contracts for human-authenticated,
same-context, goal-directed browser authoring before implementation begins.

## State

Complete.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Document, review, and promote authenticated goal authoring | `[+]` | Browsertools `63b1d81` adds the M21 design, explicit operator command, responsibility matrix, strict local protocol, disclosure/action/completion gates, reduced observation and private result contracts, additive UWS context strategy, runtime separation, failure/exclusion policy, milestone sequence, and synthetic/opt-in fixture plan. A03/E05/P04 wait for the UWS contracts. |

## Acceptance

- [x] The A02/A03 boundary and authenticated-dashboard gap are explicit.
- [x] Human, model-disclosure, action, origin, POST, and completion authority is
  unambiguous and cannot be bypassed by `--yes`.
- [x] No credential, cookie, storage state, page handle, or raw page/network
  material crosses the process boundary.
- [x] Popup/frame portability and old-version compatibility are specified.
- [x] Downstream implementation and qualification order is explicit.
