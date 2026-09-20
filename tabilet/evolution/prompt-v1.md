# Prompt V1

Create the initial engineering harness for `../browsertools`.

Context:

- `browsertools` has been created as a new Git repository.
- Root paths `AGENTS.md`, `memory-bank`, and `evolution` are symlink-facing
  paths to the tracked snapshot under `../tofu/browsertools`.
- The package should follow the same engineering harness pattern as nearby
  OpenUdon packages.
- Direction from the UWS/browser-profile design discussion:

```text
OpenAPI describes your stable service.
browser-profile describes the UI binding.
tools like llm-scraper, Crawl4AI, or Firecrawl implement or help generate the binding.
```

Clean pipeline:

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```

Need:

- `AGENTS.md`
- `memory-bank/product.md`
- `memory-bank/architecture.md`
- `memory-bank/tech-stack.md`
- `memory-bank/milestone.md`
- initial per-milestone status file
- initial evolution result

Do not scaffold implementation yet. Focus on boundaries, milestones, and
reviewable direction.
