# Private Rich Evidence And Cross-Engine Portability

E04 exposes more of Playwright-Go without turning Playwright artifacts or
engine-specific selectors into UWS browser semantics. Rich evidence remains
private raw material. Cross-engine checks remain value-free observations of an
already reviewed `uws.browser.1.5` profile.

Neither path is a runtime. Both are headless, non-interactive, GET/HEAD-only,
exact-origin acquisitions in fresh ephemeral contexts. They cannot receive an
authentication profile, operator completion signals, POST ceilings,
credentials, headers, cookies, storage state, permissions, scripts, uploads,
or retained sessions.

## Install Engines Deliberately

Browsertools never installs browsers. Install only the pinned engines needed
for the selected command, then verify each one offline:

```bash
go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.0 install chromium firefox webkit
go run ./cmd/browsertools playwright doctor --engine chromium
go run ./cmd/browsertools playwright doctor --engine firefox
go run ./cmd/browsertools playwright doctor --engine webkit
```

## Private Screenshot, Trace, And HAR

Every rich artifact is an explicit opt-in. Chromium is the only rich-artifact
CLI engine because a screenshot or trace is evidence for local review, not a
portable cross-engine result:

```bash
go run ./cmd/browsertools rich-capture chromium \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --cache-root ./.browsertools-cache \
  --artifact screenshot \
  --artifact trace \
  --artifact har \
  --retain-for 1h
```

The command derives `capturedAt` from its process clock and returns only one
cache manifest. The payload is a deterministic
`browsertools.private-rich-evidence.v1` ZIP containing a digest-bound manifest
and exactly the selected files. The bundle is capped at 20 MiB, its raw members
are capped individually and at 19 MiB in total, default retention is one hour,
and the maximum retention is 24 hours. The HAR uses Playwright's minimal mode
with resource content omitted; the shared route rejects every method except
GET/HEAD. Traces omit source files and trace timeline screenshots; a standalone
screenshot exists only when explicitly selected. These reductions do not make
either artifact secret-free.

The cache root and entry directories are `0700`; payloads and manifests are
`0600`. The entry is always `private_raw`, carries
`publication_eligible: false` and `secret_review: pending`, and
cannot enter a capability bundle or registry publication.

Export only to a new private file, inspect all selected members locally for
credentials, personal data, URLs, headers, page content, and session material,
then either retain it only for the bounded debugging purpose or delete it. A
rich ZIP has no normalized-evidence promotion path:

```bash
go run ./cmd/browsertools cache get \
  --root ./.browsertools-cache \
  --id sha256:REPLACE_WITH_EXACT_ID \
  --at 2026-08-16T12:00:00Z \
  --out ./captures/member.rich.zip

go run ./cmd/browsertools cache delete \
  --root ./.browsertools-cache \
  --id sha256:REPLACE_WITH_EXACT_ID \
  --confirm-id sha256:REPLACE_WITH_EXACT_ID
```

`cache delete` accepts only a verified `private_raw` entry and requires the
same complete SHA-256 ID twice. A mismatched confirmation or a derived artifact
fails without deletion. `cache prune` remains the bulk finite-retention control.

## Chromium, Firefox, And WebKit Comparison

Portability checking requires Chromium as the declared baseline and at least
one explicitly selected alternate engine:

```bash
go run ./cmd/browsertools portability check \
  --profile ./profiles/example.yaml \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --action read_status \
  --engine chromium \
  --engine firefox \
  --engine webkit \
  --out ./portability.json
```

The command creates a new driver/browser/context lifecycle for each engine and
derives the report time from the process clock. Each engine receives the same
validated profile, selected action names, exact origins, resource ceilings,
and profile-derived locator/wait/output probes. Browsertools does not rewrite a
locator, substitute CSS, execute a sequence macro, or copy backend errors into
the report.

The deterministic `browsertools.portability-check.v1` report contains only the
profile digest, declared paths, match counts, type/reachability facts, and fixed
diagnostics:

- `engine_unavailable`
- `browser_observation_failed`
- `profile_check_failed`
- `chromium_baseline_unavailable`
- `check_shape_mismatch`

Every selected engine is attempted. A missing engine therefore produces an
explicit unavailable result and a rejected exit status; it never silently
shrinks the comparison.

## Browser 1.5 Pressure And 1.6 Candidates

`browsertools playwright capabilities` emits the stable E04 capability policy
and `browsertools.browser-contract-pressure.v1` inventory. It records pressure
without changing `browser.1.5`:

| Surface | E04 disposition | Contract direction |
|---|---|---|
| Screenshot, trace, HAR | Supported private | Review-only raw evidence; no portable field. |
| Popup | 1.6 proposal candidate | Add an explicit page-context reference tied to an opener step and exact origin; never infer “latest page.” |
| Iframe | 1.6 proposal candidate | Add an exact-origin frame-context chain using reviewed accessibility-first frame references; never accept arbitrary frame scripts. |
| Visual interaction | 1.6 proposal candidate | Add a bounded reviewed visual-locator evidence type with coordinate-space, viewport, scale, confidence, expiry, and an accessibility fallback or explicit absence rationale. |
| Download | Deferred | Define runtime-owned artifact lifecycle, size, media, confirmation, and destination semantics first. |
| Upload | Deferred | Define runtime-owned private-input binding and confirmation semantics first. |
| Permission | Deferred | Define exact-origin, least-privilege runtime grants and expiry first. |

Any future portable change belongs in a reviewed UWS `browser.1.6` schema and
version note. Browsertools does not add ad hoc keys to `browser.1.5` profiles.

## Verification

Default unit and CLI tests use synthetic observations and fake engines. They do
not install or launch a browser. Installed-engine tests are loopback-only and
separately gated:

```bash
BROWSERTOOLS_RICH_LIVE_TEST=1 go test ./capture -run PlaywrightRichCaptureLoopback
BROWSERTOOLS_PORTABILITY_LIVE_TEST=1 go test ./capture -run PlaywrightPortabilityLoopback
```
