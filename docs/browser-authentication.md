# Browser Authentication Profiles

Browsertools validates and reviews the additive
`uws.browser-authentication.1.0` profile. These profiles describe sign-in UI
evidence; they are not browser drivers and do not contain credentials or
sessions.

Authentication profiles remain package-local in this release. Local discovery
returns them as `browser_authentication_profile` candidates, but capability
bundle construction and static-registry publication reject them. This prevents
login topology from becoming public catalog content before a separate
distribution policy is reviewed.

## Offline authoring

Start with an explicit draft specification matching the public profile fields
except for the fixed `profile` discriminator:

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

Drafting is deliberately non-inferential: Browsertools inserts the fixed
profile name and validates the author's exact origins, locators, credential
slots, challenge choice, effects, success condition, and lifecycle metadata.
It never chooses an MFA alternative or invents a login step.

## Safety gates

- One profile is at most 1 MiB and one YAML/JSON document.
- Exact HTTPS origins are required, except loopback HTTP fixtures.
- Navigation and success origins must be declared.
- Referenced credential slots must exist; a TOTP challenge requires a
  `totp_seed` slot.
- Challenge flows must declare `sends_mfa_challenge`.
- Secret-shaped values, email-address values, phone-number values, redaction
  markers, and inline secret fields fail validation.
- Expiry is inclusive: a profile is stale when assessment time equals its
  expiry instant.

Credential values, OTP responses, cookies, storage state, browser processes,
and session persistence remain runtime-owned. A package carries only the
reviewed recipe and its digest-bound review record.

## Headed assisted observation

`auth-assist chromium` validates an explicit profile against a live sign-in UI
without taking possession of the credentials or session. It is an
authoring-time observation command, not a runtime login command.

First author and inspect the profile offline. Step indexes in the profile are
zero-based. Then select every MFA alternative that should remain in the output,
repeat every declared application and authentication origin as an explicit
approval, and predeclare a maximum POST count only for the interactive steps
that are expected to submit authentication material:

```bash
go run ./cmd/browsertools auth-assist chromium \
  --profile ./browser-authentication/member.yaml \
  --flow member_login_push \
  --approve-origin https://members.example.test \
  --approve-origin https://login.example.test \
  --post-budget member_login_push:3=2 \
  --out ./browser-authentication/member.assisted.json
```

`--approve-origin` must exactly equal the profile's declared origin set: an
omission or extra origin fails before Chromium starts. A `--post-budget` uses
`FLOW:ZERO_BASED_STEP=COUNT`, applies only while that authored
`type_credential`, `click`, or `challenge` step is active, and is capped at 32.
With no budget for a step, every POST during that step is blocked. `PUT`,
`PATCH`, `DELETE`, and other mutating methods are always blocked. GET, HEAD,
and CORS OPTIONS remain subject to the exact origins and overall request/byte
bounds.

For each selected flow, Browsertools:

1. Opens a new visible, non-persistent Chromium context with no supplied
   headers, proxy, permissions, credentials, cookies, or storage state.
2. Executes only the profile's declared absolute navigation steps.
3. Checks each declared role/name/text locator by visibility and match count;
   value-based locators are rejected because input values are never inspected.
4. Arms the declared POST ceiling and asks the operator to complete the
   `type_credential`, `click`, or `challenge` step directly in the browser.
   The terminal accepts only an empty completion line. Never paste a username,
   password, OTP, or credential environment-variable name into the terminal.
5. Proves the declared success origin and locator exactly once, then destroys
   the context. Every alternative starts again in a fresh context.

No profile or review is constructed until all selected contexts have been
destroyed. Cancellation, an ambiguous/missing locator, an origin escape, a
request/resource limit, an unarmed or excessive POST, popup, child-frame
navigation, download, file chooser, JavaScript dialog, WebSocket/event stream,
service worker, page crash, or unexpected page close fails the run and writes
no artifact. This intentionally excludes popup SSO, embedded sign-in frames,
CAPTCHA, enrollment, recovery, consent, logout, and other non-sign-in flows.

The output path must be a new `.json` file and is created with mode `0600`;
stdin is reserved for empty-line signals and stdout is never an artifact sink.
The observation timestamp comes from the process clock when the live run
starts; the command does not accept a caller-supplied evidence time.
The local `browsertools.assisted-authentication.v1` envelope contains:

- the selected, freshly verified `uws.browser-authentication.1.0` profile;
- an exact digest and `browsertools.authentication-review.v1` review;
- exact origins, declared profile paths, locator match counts, and approved vs.
  observed POST counts for each flow.

It contains no page URL, request URL or body, response body, DOM, accessibility
snapshot, screenshot, input value, credential, OTP, OAuth state, cookie,
storage, browser profile, or live session handle. The evidence source is fixed
to `browsertools_assisted_auth_value_free`; selected but unused credential slots
and unselected flows are omitted, and the verification count records exactly
this one completed assisted run. The artifact remains package-local and is not
eligible for capability-bundle or static-registry publication.

Default tests use fake sessions and never launch a browser or contact a site.
The command requires the same separately installed pinned Chromium used by the
other explicit acquisition commands. A desktop-only, loopback-only headed
smoke test is explicit:

```bash
BROWSERTOOLS_AUTH_LIVE_TEST=1 go test ./capture -run PlaywrightAuthHeadedLoopback
```
