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
