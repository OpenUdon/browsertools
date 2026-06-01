# Wrapper-Service and OpenAPI Guidance

This document explains the architectural split when a browser-backed wrapper
service exposes a stable HTTP API, and how `overlay.Sidecar` links the two
artifacts as advisory review evidence.

## The split

```
website UI
  → browser-profile (browsertools)
      → browser runtime executes profile
          → wrapper service HTTP API
              → OpenAPI describes wrapper service
                  → UWS workflow binds to wrapper API
```

| Artifact | Owned by | Purpose |
|---|---|---|
| `browser-profile` | browsertools | Describes the UI binding behind the wrapper |
| `wrapper.openapi.yaml` | the wrapper service | Authoritative contract for the HTTP API |
| `overlay.Sidecar` | browsertools | Advisory evidence linking the two |
| API inventory / ranking | `github.com/OpenUdon/apitools` | Provider catalog — not here |

## What each artifact does

### browser-profile

The browser-profile describes *how the wrapper service interacts with the web UI*:
which actions to perform, which a11y locators to use, what to extract, and what
side-effect/confirmation policy applies. It is the result of a human-reviewed
evidence collection process and is validated against the UWS `browser.1.5` schema.

### wrapper OpenAPI

The wrapper service's OpenAPI document describes the *stable HTTP API surface*
that callers use. It does not describe browser interactions. OpenAPI is the right
tool for a stable, versioned HTTP service; browser-profile is the right tool for
the UI binding behind it.

### overlay.Sidecar

The `overlay.Sidecar` is **advisory review evidence only**. It:
- Links each OpenAPI `operationId` to the corresponding browser-profile action.
- Embeds the `review.Bundle` so reviewers can check the promotion gate.
- Records the lifecycle state (`draft`, `reviewed`, `exported`, `stale`).

It does **not** replace the wrapper service's OpenAPI or act as a runtime contract.
A sidecar with `Lifecycle = "stale"` means the profile or API changed and the
mappings need reverification.

### Promotion gate

Before using a sidecar in production, verify:

```
sidecar.ReviewBundle.Validation.Valid == true
len(sidecar.ReviewBundle.Gaps) == 0
sidecar.Lifecycle == "reviewed" || sidecar.Lifecycle == "exported"
```

## Boundary with apitools

`github.com/OpenUdon/apitools` owns:
- Discovery of available API sources for a given capability.
- Provider catalog metadata (name, stability, rate-limits).
- Ranking and selection of API sources.

browsertools does **not** move any of that here. The browser-profile and overlay
sidecar are evidence artifacts for a *specific, already-chosen* web UI target.

## See also

- `examples/wrapper-service/` — synthetic example with all three artifacts.
- `overlay/overlay.go` — Go struct definition for `Sidecar` and `OperationMapping`.
- `docs/openudon-integration.md` — how OpenUdon consumes review bundles.
