# Retired milestone M21 - Authenticated Goal-Directed Browser Authoring Design

**Milestone.** M21
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M21.md
**Source specification.** tabilet/memory-bank/milestone.md#m21-authenticated-goal-directed-browser-authoring-design
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] The A02/A03 boundary and authenticated-dashboard gap are explicit. - [x] Human, model-disclosure, action, origin, POST, and completion authority is unambiguous and cannot be bypassed by `--yes`. - [x] No credential, cookie, storage state, page handle, or raw page/network material crosses the process boundary. - [x] Popup/frame portability and old-version compatibility are specified. - [x] Downstream implementation and qualification order is explicit.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M21 Authenticated Goal-Directed Browser Authoring Design

**Goal.** Specify an end-to-end, human-authenticated authoring flow that can
learn a reviewed post-login capability without exporting browser session state
or allowing model proposals to bypass human and Browsertools policy gates.

Acceptance:

- `docs/authenticated-goal-authoring.md` records the A02/A03 gap, complete
  human/iCoT/Browsertools/UWS/Udon/Browserdriver responsibility matrix, strict
  local protocol, observation/result contracts, failure behavior, exclusions,
  operator example, delivery order, and synthetic/opt-in fixture plan.
- Credential and MFA values stay human-entered in Chromium. Browsertools owns
  one non-persistent context; cookies, storage, handles, raw page/network
  material, and resumable sessions never cross a process boundary.
- iCoT must disclose the exact provider/model before reduced page semantics
  reach an LLM and must preserve a human-only fallback. Clicks, new origins,
  POST authority, and final completion retain exact human approvals that
  `--yes` cannot bypass.
- The public-contract proposal is additive UWS 1.8/browser 1.6/authentication
  1.1/call 1.1 with bounded popup/frame contexts and old main-page syntax;
  existing versioned documents remain immutable and accepted.
- Browsertools implementation is sequenced behind UWS, then trusted Udon and
  Browserdriver replay, explicit OpenUdon orchestration, and cross-repository
  qualification. Playwright-Go authoring stays distinct from runtime replay.

````

## Status record

````markdown
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
````
