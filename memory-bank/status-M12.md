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
