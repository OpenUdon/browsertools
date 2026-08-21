# Authenticated Goal-Directed Browser Authoring

This document specifies the reviewed path from a human-authenticated browser
session to portable UWS authentication and capability profiles. It is the
design contract for Browsertools M21; implementation is staged across UWS,
Browsertools, Browserdriver, Udon, and OpenUdon.

The operator entry point is an explicit OpenUdon command:

```bash
icot browser-author live \
  --example ./examples/member-dashboard \
  --url https://members.example.com/login \
  --dashboard-url https://members.example.com/dashboard \
  --goal "reach the member dashboard and learn how to read account status" \
  --origin https://members.example.com \
  --origin https://login.example-idp.com \
  --private-root /private/operator/member-authoring
```

The iCoT UI is the primary human authoring surface and launches this capture
through the bundled isolated worker. `browser-author live` remains a separately
selected expert terminal fallback; it uses the same worker by default, while
`--browsertools /absolute/path/browsertools` is only a compatibility override.
`--yes` never approves model disclosure, authentication, a new origin, an
action, or goal completion.

## Current boundary and gap

Browsertools A02 can observe an already authored
`uws.browser-authentication.1.0` recipe. It creates a fresh headed context for
each flow, lets the human operate the page, and retains only value-free checks.
It deliberately closes that context at the end of authentication and rejects
popups and child frames.

That is correct for recipe verification, but it cannot author a capability
that is visible only after sign-in. The authenticated dashboard exists in the
same browser context as the login result; closing the context loses the only
safe way to inspect it without exporting cookies or storage. Browsertools E03,
P03, and E04 cannot fill the gap because their live paths are headless,
read-only, and unauthenticated. A03 therefore adds a new author-session package
instead of widening `authassist.Session`.

## Target flow and responsibility matrix

```text
human operator
  -> OpenUdon/iCoT interview, disclosure, action, and completion gates
  -> Browsertools author-session process
  -> one headed Playwright-Go Chromium context
       human types credentials and MFA directly in Chromium
       Browsertools returns reduced semantic candidates only
  -> private authenticated-authoring envelope
  -> OpenUdon independently validates and stages canonical reviewed profiles
  -> Udon approval and challenge broker
  -> trusted Browserdriver replay in a separate browser session
```

| Component | Owns | Must not own |
|---|---|---|
| Human operator | Credential/MFA entry, origin/action decisions, final goal confirmation | Portable selectors or session export |
| iCoT/OpenUdon | Goal interview, typed planning, LLM disclosure, human fallback, candidate review, atomic package staging | Cookies, input values, browser handles, trusted replay |
| Browsertools | Live context, origin/network enforcement, reduced observations, candidate IDs, portable trace synthesis | Credential values, arbitrary model actions, production sessions |
| UWS | Immutable portable context/profile schemas and semantic validation | Authoring sessions or runtime secrets |
| Udon | Runtime approval, lowering, credential-slot and challenge brokering, executor lifecycle | Page discovery or profile invention |
| Browserdriver | Trusted execution of reviewed closed vocabularies | Authoring, LLM calls, raw selector/script input |

No cookie, storage state, Playwright page, browser handle, or resumption token
crosses a process boundary. Browsertools owns one non-persistent context for
the lifetime of one child process. The context cannot be exported, resumed, or
recovered after exit.

## Human checkpoints and model disclosure

The iCoT interview converts prose such as “follow the successful redirect”
into one reviewed typed choice:

- `continue_current_page`
- `navigate_absolute`
- `ask_after_authentication`

Before any page-derived semantics reach an LLM, iCoT names the provider and
model and asks once per run for disclosure approval. Denial switches the run
to human-guided selection; it does not close an otherwise healthy author
session. iCoT sends only the reduced observation defined below, never a raw
Browsertools transcript or browser material.

Observation is automatic within an approved context. A bounded same-origin
GET navigation may be proposed and executed automatically. The human must
approve every click, every new exact origin, and every non-GET request. Login
submit steps use an explicit finite POST budget scoped to the approved
candidate and active authentication phase. Browsertools still validates each
action and network request independently; a model proposal is not authority.

Goal completion has two gates. A typed predicate must be satisfied by reduced
evidence, and the human must confirm completion. The example predicate can
require the approved dashboard origin and exact path plus one uniquely
matched accessible target. A successful login redirect belongs to the
authentication trace. A capability whose purpose is to land on the dashboard
ends with its own portable wait or presence step so replay proves the
capability result rather than relying on the authentication success check.

## Local author-session protocol

Browsertools exposes:

```bash
browsertools author-session chromium --private-root /private/operator/member-authoring
```

The command uses newline-delimited JSON on stdin/stdout. Stdout is protocol
only. Every object has `protocol: "browsertools.author-session.v2"`, a closed
`type`, and only the fields defined for that message. Sizes, collection counts,
timeouts, identifiers, strings, origins, paths, and action budgets are bounded.
Unknown message types or fields fail closed. Protocol v1 input is rejected;
there is no compatibility or fallback mode.

Client input types are:

- `start`: one absolute initial URL, approved exact origins, clean required
  dashboard URL, typed goal predicate, and finite session limits.
- `observe`: request a reduced observation for a Browsertools context.
- `focus_human_input`: focus one Browsertools-issued credential/OTP candidate;
  Browsertools never types or reads its value.
- `human_input_complete`: leave the distinct human-input state for the exact
  checkpoint candidate. Identifier/password checkpoints accept no challenge
  kind. OTP candidates require one exact reviewed `totp`, `sms_otp`,
  `email_otp`, or `voice_otp` choice; non-input MFA candidates require one
  exact reviewed `push`, `push_number_match`, `passkey`, or `security_key`
  choice.
- `execute`: propose a closed action against a Browsertools candidate ID.
- `approve` and `deny`: answer the exact pending origin/action/POST request.
- `human_complete`: record the operator's completion decision after the typed
  predicate is satisfied and carry the complete reviewed `outputs` array.
  Each output names one current candidate ID, safe key, scalar/presence type,
  and `exact_name` or `unique_role` locator mode.
- `finish`: close the live context and request deterministic candidate output.
- `close`: destroy the context without producing a promotable result.

Server output types are:

- `hello`: fixed protocol/capability negotiation.
- `state`: a closed author-session phase and current context ID.
- `observation`: reduced semantic candidates only.
- `approval_required`: one exact origin, action, or POST decision.
- `human_checkpoint`: credential, OTP/MFA, CAPTCHA, or completion attention.
  MFA checkpoints expose only the `challengeKinds` compatible with the
  observed input/non-input checkpoint.
- `diagnostic`: one fixed public code and closed metadata.
- `result`: the completed private envelope after teardown.

When the browser can identify a narrower compatible set (for example only
`totp` or only `passkey`), the checkpoint carries that exact subset. A backend
that cannot distinguish within a family uses the bounded family fallback.
Mixed-family and duplicate inventories are rejected.

The advertised total timeout is a cumulative active-browser budget. Waiting
for the operator to type credentials, retrieve MFA, approve a push, or review
outputs does not consume it. Parent cancellation still closes the browser.

Messages are legal only in their documented phase. The closed phase table is
`awaiting_start` -> `authentication` -> `human_input` -> `authentication` ->
`exploration` -> `completed`;
authentication success is captured at the first reviewed dashboard
observation, and only exploration can request completion. Any navigation or
action invalidates prior goal evidence and human confirmation, and no action is
legal after completion. At most one approval is pending. `approve` and `deny`
carry the matching approval ID and cannot grant future authority. Executor and
model actions reference Browsertools-issued candidate IDs; CSS, XPath,
coordinates, JavaScript, raw Playwright objects, DOM paths, and caller-supplied
locators are rejected.

`bounds.maxOutputs` defaults to 16, is requested as 16 by OpenUdon, and is
absolutely capped at 32 on the wire. This authoring release accepts no more
than 16 reviewed selections. Zero outputs is valid because `goal_present` is
always retained. Output keys must match
`^[A-Za-z][A-Za-z0-9_-]{0,63}$`; `goal_present`, credential-shaped keys,
duplicates, stale/ambiguous candidates, form controls, and marker labels fail
closed. At completion Browsertools re-enumerates the final context without
reading values. `exact_name` requires one current match for the canonical
role/name tuple. `unique_role` additionally requires exactly one current
match for the role alone and omits the name from the portable locator.

Cancellation, EOF, timeout, malformed input, an unknown message, browser/page
crash, unexpected navigation, unapproved origin, excess POST, CAPTCHA,
ambiguous target, denied required approval, extra popup/frame, or teardown
failure closes the entire context and returns no promotable artifact. A
CAPTCHA may be reported as a human checkpoint diagnostic, but solving or
recording it is outside this contract and the session terminates.

## Reduced observations

One observation contains only:

- the exact approved origin and clean path without query or fragment;
- a Browsertools page/frame context ID;
- the complete reviewed context inventory known at that observation;
- observation-generation-scoped candidate IDs;
- each candidate's accessibility role and redacted accessible label;
- bounded match counts and closed diagnostic codes.

`authorsession.ReduceAccessibilityLabel` owns the canonical label policy. It
collapses Unicode whitespace and controls, replaces values over 256 bytes and
secret/PII-shaped values with `[redacted]`, and replaces prompt-injection
phrases with `[untrusted-label]`. The two markers, empty labels, ordinary
headings, and safe hostnames remain stable under repeated reduction. Its closed
reason vocabulary is `unchanged`, `normalized`, `too_long`, `sensitive`, and
`prompt_injection`; rejected raw values never belong in diagnostics.

Browsertools applies that reducer before candidate IDs are derived. An
independent consumer verifies that reducing each incoming label again produces
the exact incoming value before it displays or discloses an observation. This
canonicality check detects substituted or noncanonical producer output without
duplicating the browser-specific phrase or credential policy. Candidate IDs
are derived deterministically within one observation generation from the
context, role, canonical label, and ordinal, but reveal none of the original
page content. A subsequent observation expires every earlier candidate in that
context. Immediately before focus or click, the Playwright adapter
re-enumerates current accessible semantics and requires the approved role, raw
accessible label, input kind, exact target origin, and unique match count to
remain identical; it never caches a locator as authority. Explicitly opened
popup IDs and their parent topology appear in the next state and observation,
so the client never has to guess a portable context name.

The protocol never exposes DOM, ARIA snapshots, page text, screenshots, input
values, cookies, local/session storage, request or response bodies, headers,
query strings, fragments, browser exception prose, or environment values.

## Result and promotion contracts

After successful completion and browser teardown, Browsertools emits a
deterministic `browsertools.authenticated-authoring.v2` envelope under the
operator's private root. It contains:

- candidate `uws.browser-authentication` and `uws.browser` profiles using the
  oldest sufficient supported versions;
- digest-bound Browsertools authentication and capability reviews;
- the executed portable action trace using only candidate/context IDs and
  closed actions;
- the typed goal predicate and evidence plus human confirmation;
- the exact approved origin/context inventory and finite session bounds;
- sorted, value-free `outputSelections` containing each reviewed candidate,
  key/type/locator mode, context, resolved role/name, observation generation,
  and exact/role match proof;
- fixed diagnostics and no raw protocol transcript.

The authentication profile proves the first reviewed dashboard boundary. The
capability profile independently retains every approved exploration navigation
and click, in order, before a final portable wait/presence assertion for the
typed goal. It always retains `goal_present`, then adds reviewed outputs in
sorted-key order. Accessibility text may be declared as `string`, `integer`,
`number`, or `boolean`; `presence` remains a Boolean match without a text read.
This prevents a successful login redirect from being mistaken for proof that
the post-login capability was learned.

The envelope is `0600`, atomically created, excluded from capability bundles,
registries, example packages, and normal iCoT transcripts, and never written
to stdout. OpenUdon consumes it atomically only after independent schema,
semantic, digest, redaction, freshness, origin, context, trace, goal, and review
checks. OpenUdon replays its existing validation and review construction rather
than trusting a Browsertools “valid” bit. Only canonical secret-free profiles
and existing safe review metadata may enter the example. A failed or rejected
consumption leaves the example unchanged.

## UWS popup and frame extension

UWS 1.8 publishes additive immutable documents:

- `uws.browser.1.6`
- `uws.browser-authentication.1.1`
- `uws.browser-authentication-call.1.1`

The existing UWS 1.7, browser 1.5, authentication 1.0, and call 1.0 documents
remain byte-immutable and accepted. Both new profile documents add an optional
`contexts` map; omitted `context` always means the implicit `main` page.

```yaml
contexts:
  idp_popup:
    kind: popup
    parent: main
    origin: https://login.example.com

  login_frame:
    kind: frame
    parent: main
    origin: https://login.example.com
    path: /embedded/login
    name: Login
```

Context IDs are bounded identifiers. The parent graph is acyclic and has a
maximum depth of four. Every origin appears in the profile allowlist. Frames
declare an exact clean path or name and must resolve uniquely. A popup is
created only by one explicit approved click whose `opensContext` names the
declared child; automatic, missing, duplicate, or multiple popups fail closed.

Locator-bearing steps, waits, challenges, outputs, and authentication success
may name `context`. `navigate` retains its string form and gains an object form
with `url` and optional `context`. Authentication success gains an optional
exact clean `path`. Old main-page syntax remains valid in the new schemas.
Browsertools emits browser 1.5/authentication 1.0 for main-only profiles and
uses the new versions only for a context or success-path feature. UWS schema
helpers dispatch from the document discriminator, compile each embedded schema
once, and reject unknown versions, fields, context references, cycles, unsafe
paths, and context/origin mismatches.

UWS 1.9 additionally publishes immutable `uws.browser.1.7`. Browsertools keeps
browser 1.5 for main-only string/presence output, uses browser 1.6 when a
context is the only additive need, and uses browser 1.7 when a reviewed
accessibility-text output declares integer, number, or Boolean conversion.
Browser 1.7 retains the browser 1.6 context model, requires authentication 1.1
and Browserdriver protocol v3, and fails closed on noncanonical scalar text.

## Trusted runtime replay

Playwright-Go is an authoring implementation detail. It keeps a human-visible
session alive only long enough to collect reviewed, reduced evidence. It never
becomes a trusted runtime and its browser state never becomes a UWS artifact.

Udon and Browserdriver replay the reviewed profiles later, after a separate
runtime approval, in a new private session. Browserdriver v2 remains the
authentication-1.0/main-page compatibility protocol. `udon.browser-driver.v3`
accepts authentication 1.1 followed by Browsertools' oldest-sufficient browser
1.5, context-qualified browser 1.6, or typed-output browser 1.7 capability. It
rejects missing, ambiguous, duplicate, changed, detached, or extra contexts,
revalidates cached page/frame identities before every use and at flow
completion, and continues to hide credentials, MFA values, cookies, and
session material. Udon selects v3 for authentication 1.1, browser 1.6, or
browser 1.7 and
preserves its existing credential-slot resolution, MFA challenge broker,
origin enforcement, and side-effect approval boundary.

## Failure behavior and exclusions

Every boundary fails closed and discards the live context. There is no partial
profile promotion, session recovery, transcript replay, selector fallback,
silent origin expansion, or model-only completion decision.

Initial live authoring supports headed Chromium only. CAPTCHA, enrollment,
recovery, password changes, consent, account creation, logout, downloads,
uploads, permission grants, arbitrary scripting, pixel/visual interaction,
and production-tenant qualification are excluded. Popup support is limited to
one explicitly opened exact-origin page per declared context. Frames require
portable exact metadata. Real-site proof is manual, operator-authorized, and
limited to a non-production tenant; only value-free evidence may be retained.

## Delivery sequence and qualification

1. Browsertools M21 fixes this design, protocol, responsibility matrix,
   examples, and synthetic fixture plan.
2. UWS publishes the additive context/profile contracts and compatibility
   validators.
3. Browsertools A03 implements the persistent headed author session and
   authentication continuation.
4. Browsertools E05/P04 add reduced observations, goal exploration, and
   deterministic profile/review synthesis.
5. Browserdriver M03 and Udon M29 add trusted v3 replay while retaining v2.
6. OpenUdon A04 adds explicit iCoT live orchestration and atomic candidate
   consumption.
7. OpenUdon E02 runs the cross-repository integration and release matrix.
8. Browsertools M22, Browserdriver M04, Udon M30, and OpenUdon E03 close the
   integration review with generation-scoped authority, exact response-body
   accounting, replayable exploration synthesis, and real producer-to-runtime
   seam tests.
9. UWS 1.9/browser 1.7, Browserdriver M05, Udon M32, Browsertools A04, and
   OpenUdon A06 add human-reviewed MFA kinds plus typed dashboard outputs on a
   strict v2 authoring boundary.

Default tests stay browser-free and network-free. Fake-session suites cover
every protocol phase, malformed/denied/timeout/crash path, redaction and prompt
injection, deterministic output, rollback, and sentinel credential
environments. Installed-Chromium tests are separate opt-in loopback fixtures
for redirect login, phone-push checkpoints, popup SSO, iframe login, origin
and POST approval, goal completion, and teardown.

Default qualification constructs a real Browsertools result envelope, consumes
it through OpenUdon's full validation/staging boundary, and separately decodes
the same producer's authentication/capability pair through Udon into
Browserdriver v3. Dependency pins name the exact UWS 1.9-aware commits. Before
those commits are published, standalone release testing may map the module URLs
to clean local Git clones of those exact commits; ordinary proxy/direct module
resolution is required after publication. A component-local schema fixture or
a skipped feature test is not producer/consumer/replay evidence.

Network accounting uses Playwright's completed-request size report, including
actual response-body bytes for chunked responses without `Content-Length`.
Header declarations are never treated as the response budget measurement.
