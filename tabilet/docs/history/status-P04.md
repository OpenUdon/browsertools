# Retired milestone P04 - Authenticated Profile Synthesis

**Milestone.** P04
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-P04.md
**Source specification.** tabilet/memory-bank/milestone.md#p04-authenticated-profile-synthesis
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Generated candidates validate against the sibling UWS 1.8 schemas in the coordinated workspace. - [x] Standalone tests preserve the published older UWS pin and skip only the not-yet-published authentication 1.1 schema assertion. - [x] Deterministic comparison and digest-tampering tests pass. - [x] Result construction happens only after successful context teardown.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### P04 Authenticated Profile Synthesis

**Goal.** Produce deterministic, digest-bound portable profile candidates only
after the reviewed live context is closed successfully.

Acceptance:

- The `browsertools.authenticated-authoring.v1` envelope contains canonical
  candidate profiles, independent digest-bound reviews, value-free executed
  trace, typed goal proof, human confirmation, origin/context inventory,
  finite bounds, and fixed diagnostics.
- Authentication redirect success remains in the authentication candidate;
  the capability candidate ends in its own portable wait and boolean presence
  output for the reviewed goal.
- Browsertools emits the oldest sufficient versions: main-only capability
  output remains browser 1.5, while context-qualified capability output uses
  1.6 and exact-path/context authentication uses 1.1.
- Context graphs, origins, trace references, popup bindings, paths, goal proof,
  bounds, and profile digests are validated before serialization.
- The envelope is exclusive mode `0600` under an existing non-symlink private
  root, is never packaged or written to stdout, and is absent after any failure.

````

## Status record

````markdown
# Status P04 - Authenticated Profile Synthesis

## Goal

Synthesize deterministic reviewed authentication and capability candidates
without promoting the live author session or its transcript.

## State

Complete.

## Dependencies

- Browsertools A03 persistent author session.
- Browsertools E05 reduced authenticated exploration.
- UWS browser 1.6 and browser-authentication 1.1 context contracts.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, review, and verify authenticated candidate synthesis | `[+]` | Browsertools `50bbfdd` builds deterministic `browsertools.authenticated-authoring.v1` output with separately digest-bound authentication/capability reviews, selected portable trace, typed goal proof, human confirmation, canonical origins/contexts/bounds, and fixed diagnostics. Authentication success carries the exact reviewed path; capability output has an independent final wait/presence action. Contexts select browser 1.6/authentication 1.1 only when required, while main capability output remains browser 1.5. Structural and workspace-UWS validation, tamper, determinism, graph, origin, popup-binding, safe-path, teardown, private-mode, and no-overwrite checks fail closed. |
| Preserve browser 1.6 context fields through typed downstream handoff | `[+]` | Browsertools `b73e1d4` extends the shared typed profile model and JSON/YAML unions for context maps, object navigation, context-qualified steps/waits/outputs, and `opensContext`. Workspace schema validation and typed round trips prove Udon can lower 1.6 without silently erasing context authority; browser 1.5 parsing and standalone tests remain compatible. |

## Verification

- [x] Generated candidates validate against the sibling UWS 1.8 schemas in the
  coordinated workspace.
- [x] Standalone tests preserve the published older UWS pin and skip only the
  not-yet-published authentication 1.1 schema assertion.
- [x] Deterministic comparison and digest-tampering tests pass.
- [x] Result construction happens only after successful context teardown.
````
