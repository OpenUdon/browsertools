# Retired milestone M27 - Query-Safe Registration Authoring V2 Contracts

**Milestone.** M27
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M27.md
**Source specification.** tabilet/memory-bank/milestone.md#m27-query-safe-registration-authoring-v2-contracts
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 2
**Verification.** Review iteration 2 passed with no remaining P1/P2. Recorded checks: No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M27 Query-Safe Registration Authoring V2 Contracts

**Goal.** Add reviewed structural-query support for BRP authoring without
changing v1 or allowing query material into observation and diagnostic
surfaces.

Dependencies: published Browsertools A08 and published UWS registration 1.0.

Acceptance:

- Session/result v1 fixtures and existing BAP authoring bytes remain exact.
- V2 accepts only canonical bounded literal structural queries and rejects
  duplicate/empty/sensitive keys, secret-shaped values, templates, fragments,
  userinfo, malformed encoding, and origin escape.
- One rule covers initial navigation, later GET/HEAD navigation, and every BRP
  `navigate`; observations expose only origin and disclosure-safe path.
- Focused, full, compatibility, security, and bounded review gates pass.

Status: complete at Browsertools
`062506694d2e71edbe3fff646e3f567cdb063e35`; see
[status-M27.md](status-M27.md). Its successor A09 implementation is published,
and the coordinated v16 result is published at Tofu `95752f6` after E11 passed.

````

## Status record

````markdown
# M27 Query-Safe Registration Authoring V2 Contracts

Item | State | Notes
--- | --- | ---
M27.1 Baseline reconciliation and evolution start | `[+]` | Reconciled clean published Browsertools A08 `39e32c1d6f601561cc5c13ec85201815ce85ab9b`, published UWS `895aa4546067e25f9dd525b1356abf1945d223b4`, Browserdriver `f4d76f8`, Udon `33e0889`, OpenUdon `42767a1`, Tofu `9ebe82a`, and W8M `2300edb`. Indexed M27/A09, created evolution prompt v16 with no result, and generated/materialized the exact eight-status task-commit goal. No push, target access, browser launch, runtime execution, or external mutation occurred.
M27.2 Additive session/result v2 contracts | `[+]` | Browsertools `61566d437163811dc462fe28dc07cba0b084da02` adds explicit session/result v2 identities while retaining `Protocol`/`Schema` as v1 defaults. One query validator admits canonical `action=startnew`-shaped literals only within finite URL/query/item/key/value bounds and rejects duplicate/empty/credential-shaped keys, secret/PII-shaped or templated values, malformed/noncanonical encoding, userinfo, fragments, and v1 queries with fixed errors. V2 session output carries its selected protocol but exposes no query; focused and full standalone tests/vet/diff pass with existing v1 tests unchanged.
M27.3 Profile-wide retained-query validation | `[+]` | Browsertools `a6eaa9bca0680e5479bc75ea25b2eb5b16d52775` applies the same validator to the v2 start URL, every later GET/HEAD command, every flow's retained BRP `navigate`, candidate construction, session review, result construction, and result reconstruction. Candidate building defaults to v1 and selects v2 explicitly. Exact-origin membership is rechecked from the profile inventory; errors name only flow/index or fixed stages, and observation facts remain origin plus disclosure-safe path. Focused tests cover safe retained literals, unsafe keys/templates/secrets/repeats/fragments/relative/origin escape, review-time rejection, result-time rejection, and unchanged v1 construction.
M27.4 Compatibility, documentation, and qualification | `[+]` | Browsertools `40cec1df35c8ac48f7608080859887592842d8f1` pins the exact legacy BAP/BRP compatibility fixture hash, adds a separate v2 fixture, and documents additive semantics, query disclosure limits, and deliberate selection. Full standalone tests/vet, race, pinned vulnerability scan, UWS tests/vet, OpenUdon consumer tests/vet, and diff gates pass. Review iteration 1 found three P2 gaps: encoded path templates were not rejected, non-ASCII query keys could weaken credential-shape review, and multiple invalid profile flows could produce nondeterministic first diagnostics. Browsertools `062506694d2e71edbe3fff646e3f567cdb063e35` rejects decoded path templates, requires portable ASCII query keys, strengthens fixed secret literals, and validates sorted flow names. Iteration 2 reviewed the full M27 delta and found no remaining P1/P2 or higher-severity issue. M27 is complete locally and unpushed; A09 is reconciled to the exact head.

Dependencies: completed and published Browsertools A08
`39e32c1d6f601561cc5c13ec85201815ce85ab9b`; published UWS
`895aa4546067e25f9dd525b1356abf1945d223b4`. OpenUdon E10 published through
`42767a160dc88ac18ac5d624ad9e17151ba50d77` and must be rechecked before
consumer qualification.

M27 adds no submit, POST, credential, account, session-export, target-contact,
or runtime-execution authority. Its reviewed successor implementation is
published with A09 at Browsertools
`d26f2982db352619d7a7f6563add802b56e10824`. OpenUdon E11 passed and evolution
result v16 is published in Tofu commit `95752f6`.
````
