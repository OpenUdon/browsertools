# OpenUdon Integration

This is the canonical relationship reference for OpenUdon/iCoT and
Browsertools. iCoT is the primary end-user authoring entry point across API,
browser, and runtime-handoff sources. Browsertools is its specialized
browser-source engine and shared validation library, not a parallel authoring
product or the production runtime.

## Relationship and ownership

iCoT retains goal interviewing, LLM/human interaction, source selection, and
package staging. When a reviewed workflow needs UI-only acquisition, iCoT
either consumes an existing Browsertools artifact offline or delegates live
acquisition to a separate Browsertools worker process. The normal single-binary
distribution re-executes a privately stabilized `icot` copy through the
importable `authorworker` entry point; the standalone Browsertools CLI remains
an expert compatibility and maintainer surface. Browsertools owns
Playwright-based acquisition, browser safety policy, profile synthesis, and
the shared validation library for browser capability and authentication
profiles.

| Component | Ownership |
|---|---|
| iCoT / OpenUdon | Primary user-facing authoring flow, API-first source selection, goal and safety interview, LLM/human interaction, independent validation, and atomic package staging |
| Browsertools | Playwright-based acquisition, live browser safety enforcement, reduced observations, portable profile synthesis, review artifacts, and shared profile validation |
| UWS | Portable browser profile, authentication, context, and workflow schemas plus semantic validation |
| Udon | Separate runtime approval, lowering, credential-slot and MFA challenge brokering, and executor lifecycle |
| Browserdriver | Trusted replay of reviewed browser operations in a new private runtime session |

The Browsertools CLI primarily exposes a machine-facing author-session protocol
and maintainer/offline acquisition, review, and registry tools. Production
runtime replay remains with Udon and Browserdriver.

## Integration paths

### Offline reviewed-artifact consumption

Normal iCoT authoring does not launch Browsertools or a browser. It accepts
explicit reviewed capability profiles, capability bundles, guided-authoring
results, local authentication profiles, and optional value-free verification
reports. OpenUdon applies Browsertools' shared validators and its own package
policy, keeps API sources preferred when they cover the capability, and stages
only approved canonical profiles plus safe digest and lifecycle metadata. Raw
captures, private evidence, credentials, cookies, and browser sessions never
enter the package.

The review artifact and UWS binding sections below describe the core offline
handoff. The [OpenUdon iCoT operator
guide](https://github.com/OpenUdon/openudon/blob/main/docs/icot.md) documents
the accepted explicit inputs and approval flow.

### Live `author-session` orchestration

For authenticated UI-only acquisition, the primary iCoT UI or expert
`icot browser-author live` path starts a separate worker speaking the same
`browsertools author-session chromium` protocol. iCoT owns the goal,
disclosure, human-action, completion, and staging gates. Browsertools owns the one headed,
non-persistent Playwright-Go context, enforces the origin/network/action policy,
returns only reduced semantic candidates, and writes a private deterministic
result after teardown. OpenUdon independently validates that result and stages
only canonical reviewed profiles; no live context is transferred or packaged.

The live boundary is strictly `browsertools.author-session.v2` producing
`browsertools.authenticated-authoring.v2`; v1 is rejected. OpenUdon negotiates
`maxOutputs: 16`, presents only Browsertools-provided compatible MFA kinds to
the human, and sends the final reviewed output list only after confirmation.
Browsertools revalidates exact-name or role-only accessibility matching without
reading values and returns value-free selection proofs for independent
consumer validation.

The UI never accepts an arbitrary worker executable path. It performs the
typed Playwright/Chromium doctor through the same isolated re-execution with a
30-second ceiling. Only the worker process initializes Playwright; the iCoT
engine and HTTP server do not.

Browsertools exposes a separate UI-safe doctor shape for that response. It
omits `BrowserExecutable` and converts every backend/path-bearing failure to
fixed text before a UI stores, hashes, or serializes it. The standalone CLI
retains the full local doctor report for operator diagnostics. The UI also
displays `authorsession.AccessibilityLabelDisclosure`: reduction is heuristic,
not DLP, so ordinary names, identifiers, and order numbers may remain visible.

The explicit live iCoT/Browsertools design for human authentication followed
by same-context goal exploration is specified in
[Authenticated goal-directed browser authoring](authenticated-goal-authoring.md).
Its private candidate envelope is independently validated before any canonical
profile enters the artifact handoff described here.

The corresponding user-facing procedure and failure posture are in OpenUdon's
[Authenticated Browser Authoring operator
guide](https://github.com/OpenUdon/openudon/blob/main/docs/authenticated-browser-authoring.md).

### No-submit registration candidate contract

Registration does not reuse the live authenticated integration. The separate
`browsertools.registration-author-session.v1` Go contract admits only
preapproved exact-origin GET/HEAD navigation and reduced accessibility
observation. After current-generation review and clean network-accounted
teardown, `browsertools.registration-authoring.v1` binds one exact BRP source,
its existing registration-review v1 bundle, symbolic slots, one inert submit
description, checkpoints/success, and fixed duplicate/ambiguity/cleanup
posture. It establishes no session and carries no runtime authority.

The registration session never sends a private result path over NDJSON.
OpenUdon obtains the owner-readable result through a separately protected local
channel, verifies its exact result/source/review digests and freshness, and
copies only canonical source plus value-free digest/lifecycle facts into its
transaction. The current contract packages do not add a Browsertools CLI or
worker implementation; registration runtime remains unsupported and must fail
before executor invocation.

## Offline review artifact

A `review.Bundle` is a JSON-serializable Browsertools review artifact produced
by `review.Build`. It carries the digest-bound evidence needed to decide
whether its browser profile is ready for offline promotion. JSON field names
are listed below; Go struct field names are PascalCase (for example,
`Bundle.Validation` and `Bundle.SideEffects`).

| JSON field | Go type | Purpose |
|---|---|---|
| `profile` | `profile.Profile` | The complete typed reviewed profile document |
| `profileDigest` | `string` | Canonical SHA-256 binding to the exact profile |
| `evidenceDigest` | `string` | Canonical SHA-256 binding to normalized evidence |
| `assessedAt` | `string` | RFC-3339 time used for deterministic expiry checks |
| `validation` | `ValidationReport` | Schema check result — **must be `valid: true` to promote** |
| `revalidation` | `revalidate.Result` | Evidence, locator, origin, expiry, and safe-wait gate |
| `evidence` | `EvidenceSummary` | Secret-free summary of how the profile was learned |
| `decisions` | `[]evidence.LocatorDecision` | Reviewed ambiguity resolutions and rationale |
| `gaps` | `[]Gap` | Issues blocking promotion — **must be empty (`[]`) to promote** |
| `confidenceRationale` | `string` | Human-readable explanation of the confidence value |
| `expiryNote` | `string` | Revalidation schedule note |
| `origins` | `OriginSummary` | Origin allowlist |
| `sideEffects` | `SideEffectSummary` | Write actions and confirmation requirements |

### Promotion gate

A bundle is promotable at its assessment time when:

```go
bundle.Promotable()
```

Before registration or handoff, recalculate the digests and freshness gate:

```go
if err := review.Verify(bundle, profile, records, time.Now().UTC()); err != nil {
    // profile must not be registered or executed
}
```

This detects profile/evidence changes and profiles that became stale after the
original bundle was produced.

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

See `examples/openudon-binding/binding.uws.yaml`, `evidence.json`, and
`review-bundle.json` for a digest-verified worked handoff.

## Cross-repo verification gate

From sibling checkouts, run both projects' normal Go gates to confirm the
shared validation and consumer boundary:

```bash
(cd ../browsertools && go test ./... && go vet ./...)
(cd ../openudon && go test ./... && go vet ./...)
```

## What OpenUdon does NOT get from browsertools

- Goal interviewing, LLM/human interaction, source selection, or package
  staging authority — those remain with iCoT/OpenUdon.
- Production browser sessions, credential resolution, MFA brokering, retries,
  or runtime side effects — those remain with Udon and Browserdriver.
- The live Browsertools authoring context, credentials, cookies, or browser
  handles — none are transferred to OpenUdon or placed in a review bundle.
- UWS schema ownership — that lives in `github.com/OpenUdon/uws`.
- API-source or OpenAPI metadata — that lives in `github.com/OpenUdon/apitools`.
