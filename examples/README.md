# Browsertools Examples

These examples are Browsertools-owned browser-profile workflow examples. They
pair UWS workflow documents with reviewed `browser-profile` artifacts to show
how UI-backed capabilities are authored, reviewed, and bound from UWS.

The UWS repository owns the normative UWS schema, validator, execution model,
and `browser.1.5` profile sub-spec. Browsertools owns examples that depend on
browser evidence, scraping/crawling adapters, reviewed UI profiles, wrapper
OpenAPI sidecars, or browser-profile review bundles.

Keep examples here when they demonstrate:

- browser-profile source documents
- scraper/crawler evidence import
- Playwright, llm-scraper, Crawl4AI, or Firecrawl adapter fixtures
- reviewed UI capability profiles
- browser-backed wrapper OpenAPI sidecars
- normalized evidence and digest-bound review bundles that can be verified
  without contacting the original site

UWS should keep only minimal schema/spec fixtures needed to validate the wire
contract.

`wrapper-service/` also demonstrates the full offline CLI inputs: normalized
`evidence.json`, explicit `draft-spec.yaml`, the resulting profile, and a
digest-bound overlay/review bundle.
