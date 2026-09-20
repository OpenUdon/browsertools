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
