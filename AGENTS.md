# AGENTS.md

## Purpose

`browsertools` is the OpenUdon-owned browser capability tooling module. It
helps turn reviewed observations of real website UIs into portable UWS
`browser-profile` source documents.

Planned module path:

```text
github.com/OpenUdon/browsertools
```

The clean pipeline is:

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```

`browsertools` owns discovery, explicit bounded authoring-time acquisition,
draft profile generation, profile validation helpers, review evidence, and
revalidation support. It does not own UWS wire semantics, trusted production
execution, session/credential storage, or production side effects.

## Start Here

Before substantial changes, read these in order:

1. [tabilet/memory-bank/product.md](tabilet/memory-bank/product.md)
2. [tabilet/memory-bank/architecture.md](tabilet/memory-bank/architecture.md)
3. [tabilet/memory-bank/tech-stack.md](tabilet/memory-bank/tech-stack.md)
4. [tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md)
5. The relevant per-milestone status file in [tabilet/memory-bank/](tabilet/memory-bank/)

`tabilet/memory-bank/milestone.md` owns the roadmap, active milestone, status-file
index, milestone scope, and acceptance criteria. Each
`tabilet/memory-bank/status-<LANE><NN>.md`
file owns the detailed task ledger and completion state for its milestone.

Do not recreate duplicate root-level product, architecture, roadmap, or status
documents. Long-form references live in `docs/` when needed; README is the
public operator entry point once implementation exists.

This project exposes [tabilet/GOAL.md](tabilet/GOAL.md), one optional protocol for goal requests
that span multiple status files. Follow it only when a request names it.

A `tabilet/GOAL.md` run is a deliberate exception to the row-level commit rule below.
For that run, `COMMIT_POLICY: none` — the protocol default — means no commits,
while `COMMIT_POLICY: task` keeps the usual one-commit-per-row cadence.
Precedence is the request, then `tabilet/GOAL.md`, then this file; only commits are
delegated, and only during the run.

## Boundary

- `../uws` owns the public UWS schema, Go model, validator, and the
  `versions/browser.1.{5,6,7}`, `versions/browser-authentication.1.{0,1}`, and
  `versions/browser-registration.1.{0,1}` profile contracts.
- `../browsertools` owns browser-profile tooling: explicitly acquiring or
  importing UI evidence, adapting Playwright / llm-scraper / Crawl4AI /
  Firecrawl outputs into draft profiles, validating those drafts against the
  UWS browser-profile schema, and producing review/revalidation evidence.
- `../apitools` owns API-source metadata: OpenAPI, Google Discovery, AWS
  Smithy, AsyncAPI, GraphQL, OpenRPC, gRPC/protobuf, OData, catalog metadata,
  operation inventories, and API-source ranking. Browser crawling and UI
  sessions do not belong there.
- `../openudon` owns user-facing UWS authoring/review/package flows and may
  consume browsertools output.
- Runtime packages own production execution against live browsers,
  credentials, retained cookies/sessions, retries, rate limits, production
  browser installations, and side effects. Browsertools may use a separately
  installed browser only for an explicitly selected authoring acquisition.

Rule of thumb: if the work produces or reviews a portable browser-profile, it
belongs here. If it executes a profile with credentials or calls an API, it
belongs downstream.

## Essential Commands

Keep these as the baseline:

```bash
go test ./...
go vet ./...
git diff --check
```

When changing browser-profile compatibility, run the source contract checks:

```bash
(cd ../uws && go test ./...)
```

When changing OpenUdon-facing exports, run the consumer checks when available:

```bash
(cd ../openudon && go test ./...)
```

## Hard Rules

- Treat real websites, captured pages, accessibility snapshots, DOM text,
  screenshots, crawler output, and LLM output as untrusted evidence.
- Never store cookies, passwords, OAuth state, session storage, local storage,
  browser profiles, screenshots with secrets, or other credentials in tracked
  files.
- Do not execute side-effectful UI actions as part of default tests.
- Live acquisition must be an explicit command with exact origin bounds,
  ephemeral contexts, bounded requests/time, and no automatic credential
  resolution. Default tests remain synthetic and browser-free.
- E03 capture is headless and non-interactive. It writes only finite-retention
  `private_raw` cache entries, emits metadata rather than page content, and
  requires explicit review/redaction before evidence import. Headed/manual
  interaction belongs to A02, not the generic capture command.
- P03 guided authoring may present reviewed normalized evidence, but action
  IDs, parameters, outputs, macros, side effects, confirmation, expiry, and
  ambiguity decisions remain operator-authored. Its live check observes only
  declared current-page shapes through the E03 boundary and never executes a
  profile macro or emits page values.
- A02 headed authentication observation accepts only an already validated
  authentication profile, exact origin approvals, selected flows, empty-line
  operator signals, and per-step POST ceilings. Credential entry, clicks, and
  MFA stay in the browser; each flow uses a fresh ephemeral context and only a
  local value-free profile/review bundle survives closure.
- E04 rich and cross-engine acquisition stays headless, non-interactive,
  GET/HEAD-only, exact-origin, and ephemeral. Rich screenshot/trace/HAR sets
  are explicit short-lived `private_raw` bundles with mandatory secret review
  and exact-ID deletion; portability reports reuse the same profile-derived
  probes per fresh engine and never rewrite locators or copy backend details.
- Prefer accessibility-tree locators and structured data over CSS selectors.
  CSS is output fallback evidence only when the browser-profile contract allows
  it and records a fallback reason.
- Keep Playwright, WebDriver, Crawl4AI, Firecrawl, and llm-scraper details out
  of the portable profile unless represented through documented adapter
  evidence.
- Validate generated profiles through the pinned UWS schema API for the
  accepted set (browser 1.5-1.9, authentication 1.0-1.1, registration
  1.0-1.1), and emit the oldest sufficient version, before treating them as
  review candidates.
- Fail closed on ambiguous UI targets, expired evidence, origin violations, or
  missing side-effect/confirmation metadata.

## Work Cadence

- Update memory-bank files in the same change as implementation work:
  product scope -> `product.md`; architecture/data flow/contracts ->
  `architecture.md`; tools/dependencies/commands -> `tech-stack.md`;
  milestone scope/acceptance -> `milestone.md`; completion state -> matching
  `status-<LANE><NN>.md`.
- Treat each row in a status file as a commit unit once implementation begins.
- Keep one permanent, zero-padded `tabilet/memory-bank/status-<LANE><NN>.md` file for
  each milestone listed in [tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md).
  Never reuse an ID or create aggregate `status.md`.
- Keep candidate directions unnumbered until fresh scope and dependency review
  promotes them.
- Write task ledgers as `Item | State | Notes`, with a backticked marker in the
  second column: `` `[ ]` ``, `` `[+]` ``, `` `[~]` ``, `` `[!]` ``, or
  `` `[X]` ``.
- Parallel rows in the profile and evidence lanes require explicit
  non-overlapping ownership, resolved prerequisites, and downstream impacts in
  `milestone.md`.
- Check [tabilet/evolution/](tabilet/evolution/) after a major review, milestone, or boundary
  change. Add a new version only when product direction, architecture boundary,
  milestone target, or public/private contract direction materially changes.
