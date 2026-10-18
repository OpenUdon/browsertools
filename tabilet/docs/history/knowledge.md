# Superseded knowledge journal

## 2026-10-18 — Active milestone narrative retired to history

**Original source.** `tabilet/memory-bank/milestone.md`, sections
`## Current State` and `## Delivery Strategy`.

**Replacement reason and evidence.** An explicit cleanup request retired every
completed milestone (M01-M33, P01-P04, E01-E06 and A01-A13) into frozen history
records on 2026-10-18. The narrative described each delivered milestone and the
numbered delivery sequence; keeping it active would retain exactly the
accumulated history the retirement removes. Current accepted behavior remains in
[product.md](../../memory-bank/product.md),
[architecture.md](../../memory-bank/architecture.md), and
[tech-stack.md](../../memory-bank/tech-stack.md). Milestone evidence is in the
[history index](index.md) and its linked records. No accepted behavior, wire,
contract or authority changed.

**Superseded wording from milestone.md.**

````markdown
## Current State

A13 implementation review is complete at iteration 1. Its candidate uses UWS
1.2, author-session/result v4 and review v3; complete OpenUdon/W8M qualification
and exact runtime adoption remain downstream. See evolution v19.

The repository exists as a public Git checkout with symlink-facing harness
paths:

```text
AGENTS.md -> ../tofu/browsertools/AGENTS.md
memory-bank -> ../tofu/browsertools/memory-bank
evolution -> ../tofu/browsertools/evolution
```

M01 through M18, P01, and E01 are implemented: the Go module, schema
validation, normalized evidence model, draft builder, review bundle, adapters,
revalidation contracts, OpenUdon handoff fixtures, migrated examples,
wrapper-service guidance, and public API docs are in place. M17 added the
complete typed profile contract and canonical origin/duration safety
primitives. E01 made fixture revalidation
deterministic and decision-aware. P01 made drafting explicit and unified review
with revalidation. M18 added the offline CLI and digest-verified handoff
examples. E02 added the private cache boundary, P02 added canonical capability
bundles, and M19 added service-free static distribution. A01 adds package-local
browser authentication profile tooling; downstream execution and workflow
authoring adoption landed in Udon M27 and OpenUdon A02. M20 pins and isolates
Playwright-Go, records the supported capability surface, and adds an offline
doctor for already installed authoring browser components. E03 adds explicit
headless Chromium acquisition into the finite-retention private raw cache with
exact-origin, read-only routing, activity, and resource bounds.
P03 adds deterministic terminal-guided capability intent plus value-free
current-page live checks without macro execution. A02 adds headed observation
of explicitly authored authentication alternatives: credential/click/MFA work
stays human-operated, exact origins and per-step POST ceilings are preapproved,
contexts are per-flow and ephemeral, and only value-free local review material
survives.

E04 now evaluates private rich evidence and cross-browser portability without
borrowing A02's sign-in contexts or POST authority. Screenshot/trace/HAR sets
are explicit finite private bundles; Firefox/WebKit run the same value-free
profile checks against Chromium; unsupported surface pressure is recorded
without changing browser.1.5. Candidate directions remain unnumbered until
their promotion triggers are met.

M21 fixed the authenticated goal-directed authoring direction and UWS 1.8 now
supplies the additive context contracts. A03 implements the strict Browsertools
child protocol and one headed Chromium context across human login/MFA and
post-login exploration. Browser/session material never leaves the process;
E05 and P04 own the reduced-evidence and promotion details already staged by
that implementation. E05 now verifies disclosure-safe observation reduction,
stable candidate identity, bounded typed actions, typed goal proof, and human
completion. P04 now produces deterministic digest-bound authentication and
capability candidates under the private root after browser teardown.

M22 closed the authenticated-authoring integration review. It
narrows candidate authority to one observation generation, makes contexts and
phase transitions explicit, preserves the complete exploration trace in the
capability candidate, and adds exact producer/consumer/runtime seam gates.

E06 closes the bounded live-label trust-boundary findings. Browsertools now
owns one exported canonical reducer, replaces suspicious labels with fixed
markers before candidate identity is derived, and exposes only a closed reason
for independent consumer checks. OpenUdon A05 pins the published E06 commit and
enforces that canonical output plus the negotiated candidate ceiling.

A04 replaces live MFA/output inference with the strict author-session v2
contract. The human selects one checkpoint-compatible MFA kind and may review
up to 16 current-generation accessibility outputs. Action-time exact-name or
unique-role proof remains Browsertools-owned, result selections are value-free,
and browser 1.7 is emitted only when typed accessibility conversion requires
UWS 1.9 and Browserdriver v3.

A05 closes the real headed navigation lifecycle findings exposed by the
OpenUdon scenario corpus. The sanitized child environment now forwards only
display/session-bus variables, and approved ordinary-page or popup-producing
clicks settle before Browsertools emits the next reduced observation. The v2
protocol and portable profile contracts are unchanged.

M23 closes the network-policy and dependency-safety review. Direct guard
coverage is complete, one private core owns bounded accounting without
widening any browser surface's authority, registry clients reject CGNAT
targets, and a pinned vulnerability gate runs after the required Go and module
upgrades.

M24 closes the repository-wide review findings without changing command names
or artifact versions. Profile validation uses pinned UWS browser 1.5-1.7
schemas; adapter provenance, cache/bundle expiry, resource scanning, registry
addressing, overlay binding, typed errors, and CLI dispatch now share explicit
bounded contracts.

M25 closes the author-session containment findings without changing the v2
wire. Frame names now share the candidate reduction boundary, narrowed MFA
compatibility is preserved exactly, and human waits do not consume the active
browser-operation budget.

A06 makes the Chromium author-session runner importable without making it an
in-process browser service. The standalone CLI and iCoT's hidden re-executed
worker share one context/private-root/driver/stdin/stdout entry point;
protocol, process, Playwright, context, and private-result ownership remain
unchanged. Implementation and coordinated consumer verification are complete;
commit `7ea7e832d7f85060cf57a7a08bf9af6bb7eca896` is published and OpenUdon pins
its ordinary pseudo-version without a local replacement.

A07 completes the additive registration producer boundary. It consumes the separate
UWS browser-registration 1.0 profile/call contracts and adds only offline,
file-first validation, deterministic drafting, and digest-bound review.
Browsertools does not register an account, resolve a credential, perform a
registration submit, or claim runtime support.

M26 is complete at Browsertools `c52af3c058eb0f2cd021487260a9a692c67fdeb4`
after ten bounded review iterations. Its first task
freezes the existing author-session contracts and records the separate
versioned no-submit registration-authoring decision; its second implements the
strict browser-independent registration session wire whose complete backend
surface is reduced observation, approved-origin GET/HEAD navigation, and
validated teardown. M26.3 adds the deterministic private BRP result, exact
source/review/result digests, reviewed inert submit and call-safety metadata,
closed diagnostics, and owner-only persistence. M26.4 publishes the typed Go
surface, exact wire/result documentation, and authenticated-version
compatibility fixture without adding a live command. Final standalone,
vulnerability, UWS, and OpenUdon consumer gates pass. A08 subsequently
implemented and published the guarded GET/HEAD-only Chromium BRP producer on
that exact contract; OpenUdon A19/A20/E10 consume its exact published revision.

A08.4 integration later supplied one factual M26 lifecycle correction at
Browsertools `1dd5797ce19b52d23b633703259abeb231db9cd2`: real sub-second clocks
are truncated to the contract's canonical whole-second `observedAt`. No wire or
authority changed.

A08 is complete and published at Browsertools
`39e32c1d6f601561cc5c13ec85201815ce85ab9b`
(`v0.0.0-20260825225202-39e32c1d6f60`). It adds the guarded headed Chromium
producer, explicit deterministic BRP construction, independently reconstructed
private result lifecycle, and separate reusable worker/CLI. Three review
iterations pass, including a fully protected administrator-supplied Linux
sandbox-helper path and a real sandboxed loopback with only GET/HEAD. OpenUdon
A19, P05, A20, and E10 are complete; private W8M W01 passes offline; and
evolution result v15 records the completed cross-package sequence. No live
target, account, workflow, or deployment authority was added.

M27 is complete at Browsertools
`062506694d2e71edbe3fff646e3f567cdb063e35`. It preserves the complete registration session/result v1
contract and adds query-capable v2 wires with one canonical retained-query
validator across session navigation and every BRP `navigate` step. A09 is
complete and published at Browsertools
`d26f2982db352619d7a7f6563add802b56e10824`, with deliberate worker protocol
selection and guarded Chromium enforcement.
OpenUdon E11 passed and Browsertools evolution result v16 is recorded locally
in Tofu commit `95752f6`, which is published on Tofu `main`. Neither status
grants submit, runtime, account, or public-target authority.

M28 is complete and published at Browsertools
`75fd5c3ab81f904243f8c2650c61ba1cd8c00540`
(`v0.0.0-20260826234723-75fd5c3ab81f`). It pins published UWS 1.9.1 and adds
an explicit browser-profile resolver for advisory content-trust analysis.
Browser-derived outputs default to untrusted while their value capability is
classified separately; Browsertools validation, execution, Browser 1.7, and
runtime authority are unchanged. Udon M37, OpenUdon A22/P06/E12, and Ramen M73
provide downstream compatibility evidence against the published contract.

A10 is complete and published for one confirmed P2 in the completed A09
guarded registration producer. The implementation keeps a
canonical unapproved non-navigation GET/HEAD resource blocked before contact
without recording a terminal `origin_escape`. Cross-origin navigation,
mutation methods, persistent channels, malformed URLs, bounds, and all other
existing policy violations remain fatal. Local qualification and bounded
review, including the pinned vulnerability gate, pass at iteration 5.
Browsertools `3107470313d447e29c5ac5912c3cc9d221d46967` is published, and
OpenUdon adopted its exact pseudo-version at
`dd7437c0149903ee7af987cd2e02380735ccbc40`.

A11 is complete and published for one W03/W04 deep-review containment correction. A failed
Playwright abort on an already-denied registration route must latch the fixed
`route_abort` policy failure rather than being discarded. The local code and
direct regression test pass full local verification and bounded review;
Browsertools A11 is published at `ce06b13bfef8d1776c3aa019322619c90dacbbd2`;
downstream pin adoption remains separately gated.

M29 is complete and published at Browsertools `5136589`. The A12/W15 qualified
runtime remains pinned to `ec0b9e9d6ca1`; M29 source publication does not repin
that closure. The pinned UWS revision `9ff877ebce55` already publishes browser 1.5
through 1.7, authentication 1.0 and 1.1, and registration 1.0 and 1.1, so M29
made that whole set first-class in discovery, drafting, validation helpers,
tests, and documentation. Existing 1.0 and 1.5 outputs stay byte-identical,
public API additions are additive, and no wire, fixture digest, or authority
changed. Authentication 1.1 recipes are now discoverable, which OpenUdon
absorbed in one staged-capture assertion at `d042e82`.

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```


## Delivery Strategy

Build browsertools in reviewable slices:

1. `M01`: harness, boundary, roadmap, and memory-bank setup.
2. `M02`: Go module scaffold, schema loading, and profile validation.
3. `M03`: normalized evidence model and synthetic fixture policy.
4. `M04`: deterministic draft profile builder.
5. `M05`: review bundle and safety/gap reports.
6. `M06`: Playwright evidence adapter spike.
7. `M07`: llm-scraper adapter spike.
8. `M08`: Crawl4AI adapter spike.
9. `M09`: Firecrawl adapter spike.
10. `M10`: revalidation contracts and dry-run checks.
11. `M11`: OpenUdon integration handoff.
12. `M12`: migrate Browsertools-owned examples from UWS.
13. `M13`: wrapper-service/OpenAPI guidance and examples.
14. `M14`: public API stabilization and documentation.
15. `M15`: revalidation evidence coverage hardening.
16. `M16`: parallel-lane harness migration.
17. `M17`: typed browser-profile contract and safety primitives.
18. `E01`: deterministic evidence matching and fixture revalidation.
19. `P01`: strict draft specifications and unified promotion bundles.
20. `M18`: offline CLI and digest-bound downstream handoff.
21. `E02`: private browser-experience cache with explicit retention and
    publication boundaries.
22. `P02`: canonical publishable browser capability bundles using shared
    Evidence descriptors and assessments.
23. `M19`: static registry layout, local publisher, bounded reader, discovery
    API, and CLI integration.
24. `A01`: local-only browser authentication profile validation, drafting,
    review, and discovery.
25. `M20`: pinned Playwright-Go acquisition foundation and installation
    doctor.
26. `E03`: exact-origin, ephemeral, Chromium-first live evidence capture.
27. `P03`: terminal-guided capability authoring and live read-only checks.
28. `A02`: headed manual assisted-authentication profile observation.
29. `E04`: opt-in rich evidence and cross-browser portability evaluation.
30. `M21`: authenticated goal-authoring design, protocol, responsibility
    matrix, compatibility strategy, and qualification plan.
31. `A03`: persistent authenticated headed author session with human
    credential/MFA checkpoints and exact action/origin authority.
32. `E05`: disclosure-safe reduced observations, typed goal evidence, and
    bounded portable exploration actions.
33. `P04`: deterministic authenticated-authoring envelope and oldest-sufficient
    portable profile synthesis.
34. `M22`: authenticated-authoring freshness, replayability, and exact
    cross-repository qualification hardening.
35. `E06`: canonical author-session label reduction and downstream
    canonicality contract.
36. `A04`: human-reviewed MFA kind and bounded typed accessibility outputs
    through strict author-session/result v2 contracts.
37. `A05`: headed environment and approved navigation lifecycle hardening.
38. `M23`: network-policy guard consolidation, CGNAT rejection, and pinned
    vulnerability checking.
39. `M24`: review remediation and cross-cutting consistency hardening.
40. `M25`: author-session frame, MFA, and interactive-time containment.
41. `A06`: reusable isolated author-session worker for one-binary iCoT distribution.
42. `A07`: secret-safe offline browser-registration profile tooling.
43. `M26`: immutable existing-protocol decision and separate no-submit registration-authoring contracts.
44. `A08`: guarded Chromium BRP construction, private review lifecycle, worker integration, and publication.
45. `M27`: additive query-safe registration author-session/result v2 contracts with immutable v1 compatibility.
46. `A09`: deliberate v2 worker selection and guarded Chromium retained-query producer qualification.
47. `M28`: UWS 1.9.1 browser-profile resolver and downstream advisory content-trust qualification.
48. `A10`: nonfatal denial of unapproved non-navigation GET/HEAD resources with zero-contact qualification.
49. `A11`: fail-closed registration route-abort error retention.
50. `A12`: generic registration 1.1 producer with v3 observations, bounded
    preview, and additive author-session/result contracts.
51. `M29`: first-class adoption of the pinned UWS browser, authentication, and
    registration profile set.
52. `M30`: adopt UWS 1.11 and Browser 1.8/1.9 in browser-profile validation and offline authoring.
53. `M31`: preserve exact Browser 1.8 integer defaults through typed profiles and offline drafts.
54. `M32`: add typed Browser 1.10 selector match-count support to validation and offline authoring.
````
