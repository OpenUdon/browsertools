# OpenUdon Integration

This document describes how `github.com/OpenUdon/openudon` consumes browsertools
review bundles and how UWS workflows bind to reviewed browser-profile actions.

## The artifact

A `review.Bundle` is a JSON-serializable struct produced by `review.Build`. It
carries everything OpenUdon needs to decide whether a browser-profile is ready
to use. JSON field names are listed below; Go struct field names are PascalCase
(e.g. `Bundle.Validation`, `Bundle.SideEffects`).

| JSON field | Go type | Purpose |
|---|---|---|
| `profile` | `map[string]any` | The raw reviewed profile document |
| `validation` | `ValidationReport` | Schema check result — **must be `valid: true` to promote** |
| `evidence` | `EvidenceSummary` | Secret-free summary of how the profile was learned |
| `gaps` | `[]Gap` | Issues blocking promotion — **must be empty (`[]`) to promote** |
| `confidenceRationale` | `string` | Human-readable explanation of the confidence value |
| `expiryNote` | `string` | Revalidation schedule note |
| `origins` | `OriginSummary` | Origin allowlist |
| `sideEffects` | `SideEffectSummary` | Write actions and confirmation requirements |

### Promotion gate

A bundle is promotable when both conditions hold:

```
bundle.Validation.Valid == true
len(bundle.Gaps) == 0
```

Fail either check → the profile must not be used in production.

## UWS workflow binding

A UWS workflow binds to a reviewed browser-profile action using
`sourceDescriptions` and `operations`. The step itself uses `operationRef`
to point at the operation, which declares `sourceDescription` (the profile
name) and `sourceOperationId` (the action key inside the profile).

```yaml
sourceDescriptions:
  - name: my-profile
    type: browser-profile          # tells UWS this is a browser-profile source
    url: ./profiles/example.yaml   # path or URL to the reviewed profile file

operations:
  - operationId: go_to_article
    sourceDescription: my-profile       # matches sourceDescriptions[].name
    sourceOperationId: navigate_to_article  # must match an action key in the profile
    request:
      body:
        title: "$inputs.title"
    outputs:
      is_disambiguation: "$response.body.is_disambiguation"

workflows:
  - workflowId: lookup
    steps:
      - stepId: navigate
        operationRef: go_to_article     # references the operation above
        inputs:
          title: "$inputs.title"
```

See `examples/openudon-binding/binding.uws.yaml` for a full worked example and
`examples/openudon-binding/review-bundle.json` for the corresponding bundle shape.

## Cross-repo verification gate

Once `github.com/OpenUdon/openudon` imports a review bundle, run its tests to
confirm compatibility:

```bash
(cd ../openudon && go test ./...)
```

This gate should be added to the browsertools CI when the openudon import lands.

## What OpenUdon does NOT get from browsertools

- Browser session management (credentials, cookies, retries) — those live in the
  browser runtime, not in the review bundle.
- UWS schema ownership — that lives in `github.com/OpenUdon/uws`.
- API-source or OpenAPI metadata — that lives in `github.com/OpenUdon/apitools`.
