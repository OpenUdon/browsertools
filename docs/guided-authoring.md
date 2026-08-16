# Guided Capability Authoring And Live Checks

Browsertools can guide an operator from reviewed normalized evidence to a
strict browser-profile review candidate. The guide records explicit intent; it
does not turn a captured page or a Playwright trace into an executable script.

## Prerequisite

Start with normalized evidence produced after the raw capture was inspected and
redacted. `private_raw` cache content is not accepted by the guide.

```bash
go run ./cmd/browsertools evidence import \
  --adapter playwright \
  --input ./captures/member.capture.json \
  --origin https://example.test \
  --redaction-status not_required \
  --out ./evidence.json
```

Use `not_required` only after reviewing the exact raw fixture. See
[Safe live capture](live-capture.md) for the private-cache handoff.

## Terminal Guide

Run the wizard with an explicit assessment time. Standard input is reserved for
answers, prompts go to standard error, and the JSON artifact goes only to
`--out` or standard output.

```bash
go run ./cmd/browsertools guide author \
  --evidence ./evidence.json \
  --at 2026-08-16T12:00:00Z \
  --out ./guided-authoring.json
```

The wizard first presents deterministic IDs for the reviewed records, observed
origins, accessibility locators, and output candidates. It then asks for:

- profile title, provider, login-state requirement, origin allowlist,
  observation kind, confidence, and expiry;
- explicit action IDs and the evidence records supporting each action;
- scalar parameters and whether each is required;
- output candidates, including an explicit `none` choice;
- each closed `browser.1.5` macro and its locator, parameter reference, and
  optional wait;
- side effects and confirmation policy;
- a rationale for every selected ambiguous locator.

The guide permits only `navigate`, `click`, `type_text`, `check_radio`,
`uncheck`, `select_option`, and `wait_for`. Typed/select values are symbolic
parameter references, not terminal-entered values. It accepts no JavaScript,
Playwright codegen output, XPath action locator, event callback, or general
browser command.

The artifact uses `browsertools.guided-authoring.v1` and contains the exact
accepted `spec`, generated `profile`, action-bound normalized `evidence`,
ambiguity `decisions`, and digest-bound `review`. Browsertools returns it only
after the existing draft, UWS schema, fixture revalidation, expiry, review, and
digest-verification gates all pass. Repeating the command with the same evidence,
answers, and assessment time produces the same JSON bytes.

An omitted output map in a hand-authored `draft.Spec` retains the historical
candidate-import behavior. The guide always writes an explicit output map; an
operator's `none` answer therefore remains empty and cannot silently import
candidate outputs.

## Read-Only Live Check

After reviewing a profile, check one or more actions against an explicitly
chosen current page:

```bash
go run ./cmd/browsertools live-check chromium \
  --profile ./profile.yaml \
  --url https://example.test/member \
  --allow-origin https://example.test \
  --action read_dashboard \
  --at 2026-08-16T12:00:00Z \
  --out ./live-check.json
```

The command reuses the same headless Chromium, exact-origin, ephemeral context,
GET/HEAD routing, closed browser-surface policy, and time/request/byte/depth
bounds as `capture chromium`. Every allowed origin must also appear in the
profile. The result is `browsertools.live-check.v1` and is bound to the exact
profile digest.

The check does not execute any profile macro, including `navigate`. It visits
only the operator-supplied `--url`, then observes declared accessibility
locators, locator/navigation waits, and output sources/types for the selected
actions. A locator passes only when it resolves exactly once. JSON-LD checks
compare property presence and JSON type. Accessibility, microdata, and plain
CSS checks return only match counts and shape types. Page values, ARIA content,
JSON-LD values, selectors, cookies, and storage are not written to the report or
serialized as a raw capture.

Portable CSS outputs are forced through plain CSS. Playwright selector engines
and Playwright-only pseudo-selectors fail closed. Value-based accessibility
locators are also unsupported because inspecting a live input value could read
a credential.

A current-page check cannot prove a state that appears only after a click or
mutation. Such a wait or output fails closed until the operator can present it
without asking Browsertools to perform the action. Assisted manual observation
belongs to the separately bounded authentication milestone; production action
execution remains runtime-owned.

## Test Boundary

Default guide and live-check tests use reviewed synthetic evidence and fake
acquirers. They do not install a browser, contact a site, or execute an action.
The existing opt-in loopback test covers both capture and the value-free live
check when the pinned driver and Chromium are already installed:

```bash
BROWSERTOOLS_LIVE_TEST=1 go test ./capture -run PlaywrightLiveCaptureLoopback
```
