# Result V1

Initial Browsertools harness established.

## Decisions

- Browsertools is the downstream package for producing and reviewing UWS
  browser-profile artifacts from real UI evidence.
- UWS remains the source of the browser-profile schema and public workflow
  binding semantics.
- Apitools remains API-source metadata tooling and should not absorb browser
  crawling, UI sessions, or scraping adapters.
- OpenUdon consumes reviewed browser profiles and review bundles for package
  authoring.
- Runtimes own live browser execution, credentials, sessions, cookies, retries,
  rate limits, and side effects.

## Roadmap

M01-M13 now sequence the work from harness setup through validation, evidence,
draft generation, review bundles, Playwright/llm-scraper/Crawl4AI/Firecrawl
adapters, revalidation, OpenUdon handoff, wrapper-service/OpenAPI guidance, and
public API stabilization.

## Boundaries

Browsertools should be fixture-first and evidence-first. It may import saved
outputs from Playwright, llm-scraper, Crawl4AI, and Firecrawl, but those tool
details stay adapter evidence. The portable output is a reviewed UWS
browser-profile.

No implementation dependency was added in this harness step.
