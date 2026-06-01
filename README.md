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
    "github.com/OpenUdon/browsertools/adapter"
    "github.com/OpenUdon/browsertools/adapter/playwright"
    "github.com/OpenUdon/browsertools/draft"
    "github.com/OpenUdon/browsertools/evidence"
    "github.com/OpenUdon/browsertools/review"
)

// 1. Import saved Playwright snapshot as normalized evidence.
a := &playwright.Adapter{}
records, err := a.Import(snapshotJSON, adapter.Options{
    Origin:          "https://example.test",
    ActionHint:      "read_status",
    RedactionStatus: evidence.RedactionNotRequired,
})

// 2. Build a deterministic draft profile from the evidence.
result, err := draft.Build(records, draft.Options{
    Info:         draft.ProfileInfo{Title: "Example", Origin: "https://example.test"},
    Confidence:   "medium",
    ExpiresAfter: "P30D",
})

// 3. Build a review bundle — check Bundle.Gaps before promoting.
bundle := review.Build(result.Draft, records)
if !bundle.Validation.Valid || len(bundle.Gaps) > 0 {
    // fix gaps before promoting
}
```

Validate an existing profile file:

```bash
go test ./profile/ -run TestValidateExampleProfiles
```

## What It Owns

- Browser-profile validation helpers for UWS `browser-profile` documents.
- Secret-free evidence records from browser and scraper tooling.
- Draft profile generation from reviewed evidence.
- Review bundles with validation, confidence, expiry, side-effect, and
  revalidation notes.
- Revalidation contracts for dry-run profile health checks.
- Browser-profile, scraper/crawler, and browser-backed wrapper examples.
- Optional adapters for Playwright, llm-scraper, Crawl4AI, and Firecrawl
  outputs.

## What It Does Not Own

- UWS schema and workflow semantics. Those live in
  [`github.com/OpenUdon/uws`](https://github.com/OpenUdon/uws).
- OpenAPI/API-source discovery and provider catalog metadata. Those belong in
  `github.com/OpenUdon/apitools`.
- Live browser execution, credentials, cookies, sessions, retries, account
  selection, or production side effects.
- A general Playwright, WebDriver, Puppeteer, or scraping DSL.

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
- [Examples](examples/README.md)

## Development

```bash
go test ./...
go vet ./...
git diff --check
(cd ../uws && go test ./versions)
```
