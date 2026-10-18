# Retired milestone M13 - Wrapper-Service/OpenAPI Guidance

**Milestone.** M13
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M13.md
**Source specification.** tabilet/memory-bank/milestone.md#m13-wrapper-serviceopenapi-guidance
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] `overlay.Sidecar` struct compiles and has JSON tags. - [x] Wrapper-service boundary is documented. - [x] Advisory overlay sidecar contract is documented as review evidence only. - [x] Synthetic example is reviewable without real website dependency. - [x] Apitools/browsertools split is explicit.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M13 Wrapper-Service/OpenAPI Guidance

**Goal.** Document and fixture the split where a browser-backed wrapper service
gets an OpenAPI contract.

Acceptance:

- Documentation explains that OpenAPI describes the stable wrapper service,
  while browser-profile describes the UI binding behind the wrapper.
- Optional Browsertools OpenAPI overlay sidecars are documented as advisory
  review evidence, not as provider truth or wrapper OpenAPI replacements.
- Synthetic example includes a browser profile, a wrapper OpenAPI sketch, and a
  UWS binding option for each.
- Boundaries with `../apitools` are explicit.

````

## Status record

````markdown
# Status M13: Wrapper-Service/OpenAPI Guidance

## Goal

Document and fixture the split where a browser-backed wrapper service gets an OpenAPI contract.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement `overlay/` package | `[+]` | `overlay.Sidecar` with `OverlayVersion`, `OverlayID`, `WrapperOpenAPI`, `BrowserProfile`, `ReviewBundle review.Bundle`, `OperationMappings map[string]OperationMapping`, `Lifecycle`. Full JSON tags. |
| Write wrapper guidance doc | `[+]` | `docs/wrapper-service.md` — split diagram, artifact table, promotion gate, apitools boundary. |
| Add synthetic example | `[+]` | `examples/wrapper-service/`: `browser-profile.yaml` (validates against schema), `wrapper.openapi.yaml`, `binding.uws.yaml` (both binding options), `overlay.json`. |
| Add apitools boundary notes | `[+]` | In both `docs/wrapper-service.md` and `overlay/overlay.go` package doc. |
| Validate examples | `[+]` | `TestWrapperServiceExampleProfileValid` in `overlay/overlay_test.go` runs `profile.ValidateFile` on the example profile. |

## Acceptance

- [x] `overlay.Sidecar` struct compiles and has JSON tags.
- [x] Wrapper-service boundary is documented.
- [x] Advisory overlay sidecar contract is documented as review evidence only.
- [x] Synthetic example is reviewable without real website dependency.
- [x] Apitools/browsertools split is explicit.
````
