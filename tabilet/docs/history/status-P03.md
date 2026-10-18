# Retired milestone P03 - Guided Capability Authoring And Live Checks

**Milestone.** P03
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-P03.md
**Source specification.** tabilet/memory-bank/milestone.md#p03-guided-capability-authoring-and-live-checks
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### P03 Guided Capability Authoring And Live Checks

**Goal.** Guide an operator from bounded evidence to an explicit draft and
review bundle while keeping intent and safety decisions human-authored.

Acceptance:

- A terminal wizard presents deterministic accessibility candidates and asks
  for action IDs, parameters, outputs, macro intent, side effects,
  confirmation, expiry, and ambiguity decisions; it never infers a mutation.
- Read-only live checks can validate declared locators, waits, and output
  shapes against the same origin-bounded ephemeral acquisition contract.
- Generated specs, profiles, evidence, decisions, and reviews pass the
  existing strict draft/revalidation/promotion gates and are deterministic for
  the same accepted answers and evidence.
- The interface does not parse Playwright codegen output, accept arbitrary
  JavaScript, or expose a general browser command DSL.

````

## Status record

````markdown
# Status P03 - Guided Capability Authoring And Live Checks

## Goal

Guide explicit capability intent from evidence through strict review and
read-only live checks.

## State

Completed.

## Dependencies

- Browsertools E03 safe Chromium live capture.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, document, review, and verify guided capability authoring | `[+]` | Browsertools `de6b53c` adds the deterministic strict guided-authoring envelope and value-free Chromium live checks. Review fixes made candidate IDs order-independent, enforced evidence-kind/template-origin/credential-output checks, bounded matching work, restricted CSS to portable syntax, declined value inspection, and prevented live checks from serializing raw fixtures. |
````
