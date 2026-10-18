# Retired milestone E06 - Author-Session Label Contract Hardening

**Milestone.** E06
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-E06.md
**Source specification.** tabilet/memory-bank/milestone.md#e06-author-session-label-contract-hardening
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### E06 Author-Session Label Contract Hardening

**Goal.** Make Browsertools the canonical owner of accessibility-label
reduction so suspicious live page copy becomes a safe marker without aborting
a genuine authoring session, while substituted or noncanonical producer output
still fails closed downstream.

Dependencies: Browsertools E05/M22, Evidence's existing generic secret
detector, and coordinated OpenUdon A05 consumption of the published E06
commit.

Acceptance:

- `authorsession.ReduceAccessibilityLabel` returns one canonical value and one
  closed `unchanged`, `normalized`, `too_long`, `sensitive`, or
  `prompt_injection` reason without retaining the rejected raw label.
- Unicode whitespace/control normalization, the 256-byte bound, both existing
  phrase lists, email/phone/Browsertools secret patterns, and every change made
  by `evidence/redact.String` map deterministically to the documented markers.
- Marker values are idempotent; ordinary headings, empty canonical labels, and
  safe hostnames remain unchanged; provider/GitHub/AWS credentials and
  JWT-like dotted identifiers are redacted.
- Author-session observations apply the reducer before candidate IDs and never
  emit raw rejected values. Suspicious labels continue as fixed markers under
  the unchanged `browsertools.author-session.v1` JSON contract.
  This historical E06 wire was superseded by A04's strict
  `browsertools.author-session.v2`, which retains the same reducer.
- Workspace and standalone tests/vet plus diff checks pass. The implementation
  commit is published without a tag and resolves as an ordinary Go
  pseudo-version for OpenUdon A05.

Status: complete; see [status-E06.md](status-E06.md).

````

## Status record

````markdown
# Status E06 - Author-Session Label Contract Hardening

## Goal

Make Browsertools the canonical owner of accessibility-label reduction and
keep suspicious live observations usable through fixed safe markers.

## State

Complete.

## Dependencies

- Browsertools E05/M22 authenticated observation and freshness contracts.
- Evidence's unchanged generic secret detector.
- Coordinated OpenUdon A05 consumer validation.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, publish, and verify the canonical live-label contract | `[+]` | Browsertools commit `55f40e56e03e2ce878521a50bc38a3674e18defc` exports the closed reducer, applies it before candidate IDs, and covers every reason, marker idempotency, security copy, both phrase lists, PII, existing and provider credentials, dotted identifiers, safe hostnames, normalization, and raw-value non-disclosure. It is published without a tag as `v0.0.0-20260817000231-55f40e56e03e`; workspace and standalone tests/vet and diff checks passed. OpenUdon A05 consumes that exact version. |

## Evolution Review

No new evolution version: E06 hardens the existing authenticated-authoring
boundary without changing product direction, wire shape, or ownership.
````
