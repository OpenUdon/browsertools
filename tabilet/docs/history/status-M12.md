# Retired milestone M12 - Migrate Browsertools-Owned Examples From UWS

**Milestone.** M12
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M12.md
**Source specification.** tabilet/memory-bank/milestone.md#m12-migrate-browsertools-owned-examples-from-uws
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] Browser-profile workflow examples live under Browsertools `examples/`. - [x] UWS no longer carries browser scraping/lookup example bundles. - [x] Browsertools docs state that browser-boundary examples belong here. - [x] Ownership scan confirms no browser scraping/lookup example bundles remain in UWS.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M12 Migrate Browsertools-Owned Examples From UWS

**Goal.** Move examples and fixtures that exercise browser evidence, scraping,
crawling, reviewed UI profiles, or browser-backed wrapper sidecars out of
`../uws` and into Browsertools.

Acceptance:

- Browser-profile workflow examples live under Browsertools `examples/`.
- Adapter and evidence fixtures live under Browsertools `testdata/` or
  Browsertools `examples/`, not under UWS.
- UWS keeps only schema/spec fixtures required to validate the UWS wire
  contract and `browser.1.5` sub-spec.
- Cross-repo docs point authors to Browsertools for browser-profile examples.

````

## Status record

````markdown
# Status M12: Migrate Browsertools-Owned Examples From UWS

## Goal

Move examples and fixtures that exercise browser evidence, scraping, crawling,
reviewed UI profiles, or browser-backed wrapper sidecars out of `../uws` and
into Browsertools.

## State

Completed.

## Planned Tasks

| Item | State | Notes |
|---|---|---|
| Move browser-profile workflow examples | `[+]` | Moved scraper and lookup examples from `../uws/examples/` to `examples/`. |
| Add examples ownership docs | `[+]` | `examples/README.md` records the UWS/Browsertools boundary. |
| Update public project docs | `[+]` | Browsertools owns browser-profile, scraping, crawling, and wrapper-service examples. |
| Run ownership scan | `[+]` | Confirmed UWS no longer has browser example bundles. |

## Acceptance

- [x] Browser-profile workflow examples live under Browsertools `examples/`.
- [x] UWS no longer carries browser scraping/lookup example bundles.
- [x] Browsertools docs state that browser-boundary examples belong here.
- [x] Ownership scan confirms no browser scraping/lookup example bundles remain in UWS.
````
