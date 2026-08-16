# browsertools

`browsertools` is OpenUdon's tooling project for turning real website UI
evidence into reviewed UWS browser capability profiles.

It exists for the gap where a task is exposed through a web UI and no suitable
API source document is available. The output is not a browser command trace and
not an OpenAPI substitute. The output is a portable, reviewed
`browser-profile` document that a UWS workflow can bind to.

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```

## Quick Start

```
go get github.com/OpenUdon/browsertools
```

The full pipeline from evidence to a reviewable bundle:

```go
import (
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
)

// 1. Import saved Playwright snapshot as normalized evidence.
a := &playwright.Adapter{}
records, err := a.Import(snapshotJSON, adapter.Options{
	Origin:          "https://example.test",
	ActionHint:      "read_status",
	RedactionStatus: evidence.RedactionNotRequired,
})

// 2. Add explicit action intent. Evidence never invents a click or assumes
// that an action is read-only.
result, err := draft.Build(records, draft.Spec{
	Info:            profile.Info{Title: "Example", Origin: profile.Origins{"https://example.test"}},
	ObservationKind: profile.ObservationAccessibilitySnapshot,
	Confidence:      profile.ConfidenceMedium,
	ExpiresAfter:    "P30D",
	Actions: map[string]draft.ActionSpec{
		"read_status": {
			Sequence: []profile.Step{
				{Kind: profile.StepNavigate, Navigate: "/status"},
				{Kind: profile.StepWaitFor, WaitFor: &profile.WaitForCondition{
					Locator: &profile.Locator{Role: profile.RoleStatus, Name: "OK"},
				}},
			},
			SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
			ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
		},
	},
})

// 3. Build a digest-bound bundle at an explicit assessment time.
assessedAt := time.Now().UTC()
reviewed, err := review.Build(result.Profile, records, result.Decisions, assessedAt)
if err != nil || !reviewed.Promotable() {
	// fix gaps before promoting
}

// 4. Wrap the exact reviewed inputs in an inert publication bundle.
published, err := bundle.Build(bundle.BuildOptions{
	ID: "example/status", Release: "1.0.0", Source: "reviewed_fixture",
	License: "CC0-1.0", Profile: result.Profile, Review: reviewed,
	Evidence: records, PublishedAt: assessedAt,
})
```

The CLI exposes the same offline pipeline:

```bash
go run ./cmd/browsertools profile validate --input ./profiles/example.yaml
go run ./cmd/browsertools evidence import \
  --adapter playwright --input ./capture.json \
  --origin https://example.test --redaction-status not_required \
  --out ./evidence.json
go run ./cmd/browsertools draft build \
  --evidence ./evidence.json --spec ./draft-spec.yaml \
  --out ./profile.yaml
go run ./cmd/browsertools review bundle \
  --profile ./profile.yaml --evidence ./evidence.json \
  --at 2026-08-14T00:00:00Z --out ./review-bundle.json
go run ./cmd/browsertools revalidate check \
  --profile ./profile.yaml --evidence ./evidence.json \
  --at 2026-08-14T00:00:00Z --out ./revalidation.json
go run ./cmd/browsertools bundle build \
  --id example/status --release 1.0.0 \
  --profile ./profile.yaml --review ./review-bundle.json \
  --evidence ./evidence.json --source reviewed_fixture --license CC0-1.0 \
  --published-at 2026-08-14T00:00:00Z --out ./capability-bundle.json
go run ./cmd/browsertools bundle verify \
  --input ./capability-bundle.json --at 2026-08-14T00:00:00Z
go run ./cmd/browsertools registry publish \
  --root ./public-registry --bundle ./capability-bundle.json \
  --at 2026-08-14T00:00:00Z
go run ./cmd/browsertools registry search \
  --location ./public-registry --query status \
  --at 2026-08-14T00:00:00Z
```

Browser acquisition is an explicit, separately installed authoring feature.
Browsertools pins Playwright-Go `v0.6201.0` (Playwright `1.62.1`). Install the
matching driver and Chromium deliberately, then verify the local installation:

```bash
go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.0 install chromium
go run ./cmd/browsertools playwright doctor --engine chromium
```

`playwright doctor` starts and stops only the installed Playwright driver. It
does not install software, contact a site, or launch a browser. The other CLI
commands remain file-first unless they are explicitly named live acquisition,
check, or assisted-authentication commands.

Safe live capture is Chromium-only, headless, non-interactive, and private. It
requires every exact origin and writes page material only to a finite-retention
`private_raw` cache entry:

```bash
go run ./cmd/browsertools capture chromium \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --cache-root ./.browsertools-cache \
  --action-hint read_dashboard \
  --retain-for 24h
```

The command blocks non-GET/HEAD requests, unapproved origins, child frames,
service workers, WebSockets, popups, downloads, dialogs, file choosers, and
non-essential resources. It uses finite navigation, total, request, response,
ARIA-depth, evidence-size, and retention limits. Only cache metadata is printed;
raw ARIA/JSON-LD content is never written to stdout. See
[Safe live capture](docs/live-capture.md) for the review/redaction handoff.

Explicit private screenshots, traces, and minimal-content HAR can be captured
as one short-lived, non-publishable ZIP:

```bash
go run ./cmd/browsertools rich-capture chromium \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --cache-root ./.browsertools-cache \
  --artifact screenshot --artifact trace --artifact har \
  --retain-for 1h
```

The command prints only cache metadata. Export requires a new `0600` file;
review for secrets is mandatory, and `cache delete` requires the exact digest
twice. Rich artifacts have no publication or normalized-evidence path.

After normalization, the terminal guide makes every capability and safety
decision explicit and emits one deterministic envelope containing the accepted
spec, generated profile, action-bound evidence, ambiguity decisions, and
promotable review:

```bash
go run ./cmd/browsertools guide author \
  --evidence ./evidence.json \
  --at 2026-08-16T12:00:00Z \
  --out ./guided-authoring.json
```

A separately selected live check can compare declared locators, waits, and
output shapes with the current page without executing any profile macro or
emitting page values:

```bash
go run ./cmd/browsertools live-check chromium \
  --profile ./profile.yaml \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --action read_dashboard \
  --at 2026-08-16T12:00:00Z \
  --out ./live-check.json
```

Both paths keep sequence intent, side effects, confirmation, expiry, and
ambiguity decisions human-authored. The live check reuses the exact-origin
ephemeral capture policy, accepts plain CSS outputs but no Playwright selector
language, and writes only profile-bound match/type facts. See
[Guided capability authoring and live checks](docs/guided-authoring.md).

The same profile-derived read-only checks can be compared without locator
rewrites across explicitly installed engines:

```bash
go run ./cmd/browsertools portability check \
  --profile ./profile.yaml \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --action read_dashboard \
  --engine chromium --engine firefox --engine webkit \
  --out ./portability.json
```

Chromium is the required baseline. Missing engines and shape differences are
fixed value-free diagnostics, not silent fallbacks. See
[Private rich evidence and cross-engine portability](docs/advanced-evidence.md).

Portable sign-in recipes use a separate additive contract and remain local to
the workflow package:

```bash
go run ./cmd/browsertools auth-draft build \
  --spec ./authentication-spec.yaml \
  --out ./browser-authentication/member.yaml
go run ./cmd/browsertools auth-profile validate \
  --input ./browser-authentication/member.yaml \
  --at 2026-08-16T00:00:00Z
go run ./cmd/browsertools auth-review bundle \
  --profile ./browser-authentication/member.yaml \
  --at 2026-08-16T00:00:00Z \
  --out ./browser-authentication/member.review.json
```

An explicit headed authoring command can then observe the exact recipe while
the operator enters credentials and completes MFA directly in the browser:

```bash
go run ./cmd/browsertools auth-assist chromium \
  --profile ./browser-authentication/member.yaml \
  --flow member_login_push \
  --approve-origin https://members.example.test \
  --approve-origin https://login.example.test \
  --post-budget member_login_push:3=2 \
  --out ./browser-authentication/member.assisted.json
```

The profile supplies every locator, challenge alternative, submit step, and
success condition; Browsertools does not infer them. Each selected flow runs in
a separate visible ephemeral context. Browsertools navigates declared URLs and
counts declared accessibility locators, but the operator performs every
credential, click, and challenge step and signals completion with an empty
terminal line. A POST is blocked unless its exact zero-based flow step has an
explicit bounded `--post-budget`; all other mutating methods are blocked.

The `0600` output is created only after every context has closed and contains a
selected `uws.browser-authentication.1.0` profile, its digest-bound review, and
value-free origin/count/request evidence. It cannot use stdin for a profile,
stdout for the artifact, or overwrite a path. Authentication recipes are also
excluded from static registry publication. Actual credentials, MFA responses,
OAuth state, cookies, storage, and sessions stay in the browser or downstream
private runtime. See [Browser authentication profiles](docs/browser-authentication.md).

Caller-supplied raw captures and derived artifacts can be kept in an explicit
private local cache. Raw entries can never be publication eligible:

```bash
go run ./cmd/browsertools cache put \
  --root ./.browsertools-cache --input ./capture.json \
  --kind private_raw --media-type application/json \
  --created-at 2026-08-14T00:00:00Z
go run ./cmd/browsertools cache list \
  --root ./.browsertools-cache --at 2026-08-14T12:00:00Z
go run ./cmd/browsertools cache prune \
  --root ./.browsertools-cache --at 2026-09-14T00:00:00Z
go run ./cmd/browsertools cache delete \
  --root ./.browsertools-cache --id sha256:EXACT_ID --confirm-id sha256:EXACT_ID
```

## What It Owns

- A complete typed model and validation helpers for UWS `browser-profile`
  documents.
- Typed validation, deterministic drafting, digest-bound review, freshness,
  and local discovery for package-local `uws.browser-authentication.1.0`
  recipes.
- Secret-free evidence records from browser and scraper tooling.
- Draft profile generation from reviewed evidence.
- Review bundles with validation, confidence, expiry, side-effect, and
  revalidation notes.
- Deterministic fixture-only revalidation and digest-bound promotion gates.
- A bounded, content-addressed private cache for caller-supplied experiences,
  normalized evidence, profiles, and review bundles.
- Canonical, digest-bound, lifecycle-assessed publication bundles for reviewed
  profiles, safe evidence, and optional inert UWS companions.
- A service-free static registry layout, atomic local publisher, bounded
  local/HTTPS reader, and local browser-source discovery report.
- Browser-profile, scraper/crawler, and browser-backed wrapper examples.
- Optional adapters for Playwright, llm-scraper, Crawl4AI, and Firecrawl
  outputs.
- An isolated Playwright-Go acquisition boundary, pinned capability policy,
  and offline installation doctor for authoring-only browser tooling.
- Explicit headless Chromium acquisition into the private raw cache with exact
  origins, ephemeral context destruction, and closed activity/resource bounds.
- Deterministic terminal-guided authoring that binds explicit intent, reviewed
  evidence, decisions, a valid profile, and a promotable review.
- Value-free Chromium live checks for declared locators, waits, and output
  shapes without macro execution.
- Explicit short-lived screenshot/trace/HAR bundles with mandatory secret
  review, no publication path, and exact-ID deletion.
- Fresh Chromium-baseline Firefox/WebKit comparisons of the same value-free
  profile probes, plus documented browser.1.6 contract pressure.
- Headed manual authentication observation with separate ephemeral contexts,
  exact preapproved origins, step-scoped POST ceilings, and local value-free
  draft/review bundles.

## What It Does Not Own

- UWS schema and workflow semantics. Those live in
  [`github.com/OpenUdon/uws`](https://github.com/OpenUdon/uws).
- OpenAPI/API-source discovery and provider catalog metadata. Those belong in
  `github.com/OpenUdon/apitools`.
- Production browser execution, runtime credential resolution, retained
  cookies/sessions, retries, account selection, or production side effects.
- A general Playwright, WebDriver, Puppeteer, or scraping DSL.
- Implicit browser launch or uploading cached content. Acquisition commands
  are separately selected; cache commands are local and offline, and
  publication has a separate verification boundary.
- Accounts, membership, a registry database, remote writes, or deployment
  credentials. Static catalogs are reviewed and deployed by existing
  repository/hosting workflows.
- Publication of authentication recipes through the static browser capability
  registry.

## Why Not Just OpenAPI?

OpenAPI should describe a stable HTTP service. If you build a browser-backed
wrapper service, OpenAPI should describe that wrapper. Browsertools can also
emit an advisory overlay sidecar for the wrapper, linking OpenAPI operations to
reviewed browser-profile actions and review bundles.

`browser-profile` describes the UI binding behind the wrapper or behind a UWS
browser operation:

```text
website UI
  -> browsertools-reviewed profile
  -> browser runtime executes profile
```

or:

```text
website UI
  -> browser-backed wrapper service
  -> OpenAPI describes wrapper
  -> UWS binds to wrapper API
```

## Documentation

- [Project description](docs/project.md)
- [OpenUdon integration](docs/openudon-integration.md)
- [Wrapper-service guidance](docs/wrapper-service.md)
- [Typed-profile migration](docs/migration-typed-profile.md)
- [Publishable capability bundles](docs/capability-bundles.md)
- [Static registry and contribution workflow](docs/static-registry.md)
- [Browser authentication profiles](docs/browser-authentication.md)
- [Safe live capture](docs/live-capture.md)
- [Private rich evidence and cross-engine portability](docs/advanced-evidence.md)
- [Guided capability authoring and live checks](docs/guided-authoring.md)
- [Examples](examples/README.md)

## Development

```bash
go test ./...
go vet ./...
git diff --check
(cd ../uws && go test ./...)
```

Default tests use fakes and synthetic fixtures; they do not install or launch
browsers and do not contact the network.

An installed-browser loopback integration test is opt-in:

```bash
BROWSERTOOLS_LIVE_TEST=1 go test ./capture -run PlaywrightLiveCaptureLoopback
```

Headed authentication behavior is covered by browser-free policy/state-machine
tests by default. A real site is never contacted by the test suite. An
installed-browser headed smoke test is separately opt-in and loopback-only:

```bash
BROWSERTOOLS_AUTH_LIVE_TEST=1 go test ./capture -run PlaywrightAuthHeadedLoopback
```

Rich evidence and cross-engine smoke tests are independently gated and remain
loopback-only:

```bash
BROWSERTOOLS_RICH_LIVE_TEST=1 go test ./capture -run PlaywrightRichCaptureLoopback
BROWSERTOOLS_PORTABILITY_LIVE_TEST=1 go test ./capture -run PlaywrightPortabilityLoopback
```
