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
commands remain file-first and do not acquire live evidence. Live Chromium
capture is introduced separately with exact-origin and ephemeral-context
controls.

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

Authentication recipes are intentionally excluded from static registry
publication in this release. They contain only symbolic credential bindings;
actual credentials, MFA responses, cookies, and sessions stay in the private
runtime.

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
