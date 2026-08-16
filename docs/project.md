# Browsertools Project Description

`browsertools` is a browser-profile authoring and review toolkit for OpenUdon.
It helps produce portable UWS `browser-profile` source documents from real
website UI evidence while keeping live browser execution, credentials, and
runtime policy outside the artifact.

The main idea is simple:

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```

The browser profile is the contract. Playwright, llm-scraper, Crawl4AI, and
Firecrawl are tools that can help observe, draft, validate, or revalidate that
contract.

## Problem

Many useful workflows are exposed only through web UIs:

- internal admin consoles
- SaaS pages without complete public APIs
- reports and dashboards with no export endpoint
- legacy apps with forms but no machine-readable contract
- product surfaces where the API exists but does not cover the needed task

An LLM or browser agent can often perform the task once, but repeating that
open-ended browsing process is hard to review and risky for side-effectful
actions. A browser profile turns a learned UI capability into a bounded,
reviewed artifact.

## Product Boundary

Browsertools owns the tooling around browser profiles:

- explicitly selected, bounded authoring-time browser acquisition
- collecting or importing UI evidence
- normalizing evidence into secret-free records
- proposing draft browser profiles
- validating profiles against the UWS browser-profile schema
- producing review bundles
- maintaining browser-profile, scraping, crawling, and wrapper-service examples
- recording confidence, expiry, fallback reasons, side effects, and
  confirmation policy
- defining revalidation contracts

Browsertools does not own:

- the UWS wire schema
- OpenAPI/API-source discovery
- provider catalogs
- live credentials
- retained or production browser sessions
- stored cookies
- captcha or MFA handling
- production browser execution
- runtime retries, throttling, or account selection

## Relationship To UWS

UWS owns workflow structure and operation binding. UWS 1.5 supports
`sourceDescriptions[].type: browser-profile`. A UWS operation binds to a
browser profile action using the same generic source selector pattern used by
other source families:

```yaml
sourceDescriptions:
  - name: ui
    type: browser-profile
    url: ./profiles/example.yaml

operations:
  - operationId: read_status
    sourceDescription: ui
    sourceOperationId: read_status
```

The browser profile itself owns UI action metadata:

- action name
- parameters
- accessibility-first locators
- declarative macro sequence
- outputs
- side effects
- confirmation policy
- origin allowlist
- evidence
- confidence
- expiry
- verification metadata

Browsertools should validate against the UWS browser-profile schema rather than
forking the contract.

## Relationship To OpenAPI

OpenAPI describes a stable HTTP service. It should not pretend that a website
UI is an API.

When a browser-backed wrapper service exists, the boundary is:

```text
real website UI
  -> browser runtime / scraper implementation
  -> stable wrapper HTTP service
  -> OpenAPI describes wrapper
  -> UWS binds to OpenAPI operation
```

When UWS binds directly to the UI capability, the boundary is:

```text
real website UI
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
  -> browser runtime executes with bound session/credentials
```

Both are valid. The key is to keep the contracts honest:

- OpenAPI describes the wrapper service.
- Browser profile describes the UI binding.
- Runtime implementation details stay private.

## Browser-Backed OpenAPI Overlay Artifact

When a browser-backed wrapper service is used, Browsertools may emit an
advisory OpenAPI overlay sidecar. This sidecar is review evidence for how the
wrapper operation maps to a reviewed browser capability. It is not provider
truth, not a replacement for the wrapper OpenAPI document, and not a way to
make a website UI pretend to be the provider's API.

The wrapper OpenAPI document remains the executable HTTP contract. The browser
profile remains the UI capability contract. The overlay sidecar only connects
the two for review, export, and later revalidation.

Minimum sidecar contract:

- `overlayVersion: "1"`
- `overlayId`: stable identifier for the sidecar artifact.
- `wrapperOpenAPI`: reference to the wrapper OpenAPI document and version or
  digest reviewed against this sidecar.
- `browserProfile`: reference to the reviewed browser-profile artifact and
  version or digest.
- `reviewBundle`: reference to the Browsertools review bundle that supports
  the mapping.
- `operationMappings`: object keyed by wrapper OpenAPI `operationId`.

Each `operationMappings` value should identify:

- the wrapper OpenAPI operation, by `operationId` or operation ref
- the browser profile action, by action ID or ref
- request mapping summary
- response mapping summary
- declared side effects
- confirmation requirement
- review status

Overlay lifecycle:

```text
draft -> reviewed -> exported -> stale
```

`draft` means generated or edited but not accepted. `reviewed` means a human or
policy gate accepted the mapping. `exported` means the sidecar was emitted next
to a wrapper OpenAPI/profile bundle. `stale` means the wrapper
OpenAPI, browser profile, review bundle, or observed UI has changed enough that
the mapping must be checked again.

## Relationship To Apitools

`apitools` owns API-source metadata: OpenAPI, Swagger, Google Discovery, AWS
Smithy, AsyncAPI, GraphQL, OpenRPC, gRPC/protobuf, OData, provider catalog
metadata, operation inventories, and source ranking.

Browsertools should not move browser crawling or scraping into apitools.

The two projects meet only at the wrapper boundary. If browsertools helps build
a stable wrapper service, apitools can inspect the wrapper's OpenAPI document.
The UI evidence and browser-profile generation remain browsertools concerns.

## Adapter Roles

Browsertools should support tool adapters as evidence importers, not as the
portable contract itself.

### Playwright

Playwright is the primary candidate for local browser observation. It can
collect accessibility-tree snapshots, verify roles and names, test waits, and
record safe probes. Browsertools should import saved Playwright evidence first;
live browser capture is a separate explicit authoring path and remains disabled
in default tests. Browsertools pins Playwright-Go and exposes an installation
doctor without turning Playwright scripts into the portable profile contract.

### llm-scraper

llm-scraper can infer structured extraction schemas from pages using LLMs and
Playwright. Browsertools can use its output as candidate schema evidence for
profile outputs. LLM-produced fields remain untrusted until validated and
reviewed.

### Crawl4AI

Crawl4AI can provide crawl, markdown, structured extraction, CSS/XPath
extraction, and LLM extraction evidence. Browsertools can use this evidence to
draft outputs and fallback reasons. CSS and XPath evidence must not become
portable action locators; UWS browser profiles are accessibility-first for
actions.

### Firecrawl

Firecrawl can provide hosted or self-hosted scrape, crawl, map, extract, and
interaction evidence. Browsertools can import saved Firecrawl responses as
evidence. Firecrawl job IDs, API behavior, and hosted-service details stay
adapter metadata, not browser-profile fields.

## Artifact Model

The expected artifact flow is:

1. **Raw evidence**: tool-specific output from browser sessions, scrapers, or
   extractors. Raw evidence is untrusted and may contain secrets.
2. **Normalized evidence**: secret-free records with origin, observation kind,
   candidate locators, candidate outputs, diagnostics, provenance, and
   redaction state.
3. **Draft profile**: a typed candidate generated from normalized evidence and
   an explicit `draft.Spec`. Evidence may propose locators and outputs, but it
   never invents macros or assumes that an action is read-only.
4. **Validation report**: schema validation plus browsertools semantic review
   checks.
5. **Review bundle**: typed profile, profile/evidence digests, assessment time,
   validation and fixture-revalidation results, explicit locator decisions,
   unresolved gaps, confidence, expiry, origins, and side-effect assessment.
6. **Reviewed profile**: the artifact consumed by UWS/OpenUdon/browser
   runtimes.

Browsertools also owns example bundles that combine UWS workflows with reviewed
browser profiles or browser-backed wrapper artifacts. The UWS repository should
keep only schema/spec fixtures required to validate the wire contract; examples
that exercise browser evidence, scraping, crawling, reviewed UI profiles, or
wrapper sidecars belong here.

## Safety Rules

Browsertools should fail closed when:

- a locator is ambiguous
- an action or declared locator lacks matching saved evidence
- a target origin is outside the allowlist
- required side-effect metadata is missing
- confirmation policy is missing or inconsistent
- evidence is expired
- a side-effectful action has no completion wait after its final actionable
  macro
- a bundle digest no longer matches its profile or evidence
- CSS fallback output lacks a fallback reason or validation schema
- generated output fails the UWS browser-profile schema

Browsertools must not commit:

- cookies
- passwords
- OAuth state
- browser profiles
- session storage
- local storage
- private screenshots
- raw pages with secrets
- captured credentials

## Possible Future 1.6 Work

UWS 1.5 and `browser.1.5` are the current boundary: Browsertools produces
reviewed UI capability profiles, and runtimes own live browser mechanics. If
real reviewed workflows outgrow that boundary, Browsertools should collect the
pressure as candidate `browser.1.6` requirements rather than widening the
current artifact ad hoc.

Possible 1.6 candidates include bounded file upload/download declarations,
narrow drag/drop or pointer interactions, visual assertion evidence, richer
completion checks, revalidation hints, and read-only crawler policy summaries.
They should remain reviewed profile features, not Playwright command traces,
arbitrary JavaScript, raw coordinate automation, MFA/login scripting,
credential/session storage, or runtime retry/session policy.

## Implemented Milestones

1. Harness and boundary setup.
2. Go module scaffold and browser-profile validation.
3. Normalized evidence model.
4. Deterministic draft profile builder.
5. Review bundle and safety reports.
6. Playwright evidence adapter.
7. llm-scraper adapter.
8. Crawl4AI adapter.
9. Firecrawl adapter.
10. Revalidation contracts.
11. OpenUdon integration handoff.
12. Migrate Browsertools-owned examples from UWS.
13. Wrapper-service/OpenAPI guidance and examples.
14. Public API stabilization and documentation.
15. Revalidation evidence coverage hardening.
16. Parallel-lane harness migration.
17. Complete typed profile model and canonical safety primitives.
18. Deterministic evidence matching and revalidation (`E01`).
19. Strict drafting and unified promotion (`P01`).
20. Offline file-first CLI and digest-bound handoff (`M18`).
21. Private browser-experience cache (`E02`).
22. Publishable browser capability bundle (`P02`).
23. Static browser capability registry (`M19`).
24. Local-only browser authentication profile tooling (`A01`).
25. Playwright-Go acquisition foundation and installation doctor (`M20`).
26. Safe headless Chromium live capture into private raw cache (`E03`).

## CLI

The artifact CLI commands are offline and file-first. They accept `-` for one
input or for output and require explicit redaction state during adapter import.
The separately selected `playwright doctor` command starts only an installed
driver long enough to verify its pinned browser executable; it does not launch
a browser, install software, or contact a site.

```bash
browsertools profile validate --input ./profiles/example.yaml
browsertools evidence import --adapter playwright --input ./capture.json \
  --origin https://example.test --redaction-status not_required \
  --out ./evidence.json
browsertools draft build --evidence ./evidence.json --spec ./draft-spec.yaml \
  --out ./profiles/example.yaml
browsertools review bundle --profile ./profiles/example.yaml \
  --evidence ./evidence.json --at 2026-08-14T00:00:00Z \
  --out ./review-bundle.json
browsertools revalidate check --profile ./profiles/example.yaml \
  --evidence ./evidence.json --at 2026-08-14T00:00:00Z \
  --out ./revalidation.json
browsertools playwright doctor --engine chromium
browsertools capture chromium --url https://example.test/member \
  --allow-origin https://example.test \
  --cache-root ./.browsertools-cache --retain-for 24h
```

Live capture is the one networked command in this milestone. It is explicit,
headless, non-interactive, exact-origin, ephemeral, read-only at the routed
request boundary, and bounded by time/count/byte/depth controls. It writes raw
ARIA and valid JSON-LD only as a non-publishable private cache entry and emits
metadata to stdout. Normalized evidence is a later, explicit import after an
operator reviews and redacts the private fixture. See
[Safe live capture](live-capture.md).

## Fixture Policy

Default tests and examples should use synthetic fixtures, local HTML, or
`https://example.test`-style placeholders. Real-site captures are brittle and
often carry policy, privacy, or session risk. They should live in ignored local
evidence directories or downstream runtime examples unless explicitly reviewed.
Local raw captures, generated evidence, generated review bundles, traces, and
HAR files are ignored by default. Tracked synthetic fixtures should live under
future `testdata/` or `examples/` paths.
