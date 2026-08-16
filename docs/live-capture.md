# Safe Live Chromium Capture

Browsertools can acquire minimal accessibility-first evidence from an
explicitly selected site. This is authoring-time observation, not production
workflow execution: it performs no clicks, typing, submission, upload,
credential lookup, MFA handling, or session export.

## Install And Verify

Browsertools pins Playwright-Go `v0.6201.0` and Playwright `1.62.1`. Installation
is always a separate operator action:

```bash
go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6201.0 install chromium
go run ./cmd/browsertools playwright doctor --engine chromium
```

The doctor starts only the installed driver and verifies the Chromium
executable. It performs no installation, browser launch, or site access.

## Capture

Create the ignored private cache, then name the initial URL and every exact
origin needed by the page:

```bash
go run ./cmd/browsertools capture chromium \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --allow-origin https://assets.example.test \
  --cache-root ./.browsertools-cache \
  --action-hint read_dashboard \
  --retain-for 24h
```

HTTPS is required except for literal loopback IPs and `localhost`, which keeps
synthetic local capture possible. URL userinfo, credential-shaped URL values,
and sensitive query/fragment keys fail closed. The initial URL's origin must be
listed explicitly; redirects and subresources are checked against the same
list.

The command returns only a cache manifest containing a `sha256:` entry ID.
Page content is stored as `private_raw` with `0700` directories, `0600` files,
finite retention, and `publication_eligible: false`. Raw content is never sent
to stdout.

## Safety Envelope

Each capture uses the bundled Chromium executable in a new non-persistent,
headless context with the Chromium sandbox enabled. The Chromium child receives
only a small non-credential environment allowlist, and Browsertools supplies no
headers, cookies, HTTP credentials, permissions, storage state, proxy,
extensions, or arbitrary JavaScript.

The acquisition boundary:

- permits only GET and HEAD requests to exact allowed origins;
- blocks service workers and WebSockets;
- rejects child-frame navigation, popups, downloads, dialogs, and file
  choosers;
- drops images, media, fonts, manifests, text tracks, and other non-essential
  resources;
- bounds navigation and total time, routed request count, declared and
  observed response bytes, ARIA depth, JSON-LD document count, serialized
  evidence size, and private retention;
- captures a Playwright default-mode ARIA snapshot without boxes or element
  references, plus valid `application/ld+json` documents;
- destroys the context before returning the cache manifest.

The response-byte gate checks declared `Content-Length` before accepting the
result and verifies observed transfer sizes after requests finish. The total
deadline and isolated browser process remain the outer bound for servers that
stream without a declared length.

## Review And Normalize

Exporting raw material requires an explicit file path, creates the file with
mode `0600`, and never permits overwrite:

```bash
mkdir -m 700 -p ./captures
go run ./cmd/browsertools cache get \
  --root ./.browsertools-cache \
  --id sha256:REPLACE_WITH_CAPTURE_ID \
  --at 2026-08-16T12:00:00Z \
  --out ./captures/member.capture.json
```

Inspect the file locally. Remove or replace any private values before importing
it. Then declare the review result explicitly:

```bash
go run ./cmd/browsertools evidence import \
  --adapter playwright \
  --input ./captures/member.capture.json \
  --origin https://example.test \
  --action-hint read_dashboard \
  --redaction-status redacted \
  --redacted-field structuredData[0].memberEmail \
  --out ./evidence.json
```

Use `--redaction-status not_required` only after inspecting the exact fixture
and confirming it contains no sensitive values. The adapter parses the bounded
ARIA snapshot into accessibility locator candidates and proposes only
non-credential-shaped JSON-LD property names/types; it never copies JSON-LD
values into normalized evidence. Ambiguous locators and conflicting JSON-LD
types remain review diagnostics.

Files below `captures/` and `.browsertools-cache/` are ignored by this
repository. Do not move raw captures into tracked examples.

Reviewed normalized evidence can next enter the deterministic terminal guide.
Validated profiles can use the same acquisition envelope for value-free,
current-page locator/wait/output checks. Neither path executes a profile macro;
see [Guided capability authoring and live checks](guided-authoring.md).

Screenshots, traces, HAR, exact-ID deletion, and Firefox/WebKit comparison are
separate E04 opt-ins. They retain this read-only exact-origin boundary and do
not change the normalized capture path described here; see
[Private rich evidence and cross-engine portability](advanced-evidence.md).

## Tests

Default tests use fakes and synthetic loopback URLs and never install or launch
a browser. With the exact driver and Chromium installed, run the explicit
loopback-only integration test:

```bash
BROWSERTOOLS_LIVE_TEST=1 go test ./capture -run PlaywrightLiveCaptureLoopback
```
