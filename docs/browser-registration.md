# Browser Registration Profiles

## Registration 1.2 verification candidate

`registration-author-session chromium --protocol v4` observes public metadata
for one standard Turnstile, reCAPTCHA v2 or hCaptcha widget bound to the selected
submit control's POST form. It proposes provider, activation and destination;
the operator grants traffic with an explicit `approve_verification` command
carrying the candidate ID and reviewed `humanVerification` descriptor. Detection
alone grants no provider authority. Missing or ambiguous bindings stop.

V4 preserves typed public field review and preview, and emits registration 1.2,
registration review v3 and authoring result v4. Consumers select these versions
explicitly. Results bind the reviewed dependency authority to the observed
submit and canonical profile. Provider request/POST/body counts are separate
from application GET/HEAD counts. Application mutations, registration inputs and
submission remain prohibited; the whole v4 session is no longer GET/HEAD-only.

Adapter-owned HTTPS policies cover Turnstile's challenge origin, documented
Google/recaptcha.net paths and hCaptcha's DNS domain family. Limits are at most
256 provider requests, 32 MiB of bodies delivered to the browser and a
nonrenewable 120-second phase, tightened by consumer/session authority. Provider
redirects, top-level navigation, application frames, popups, downloads and
persistent channels stop. Bodies are buffered before budget checking; the budget
does not bound the transport's process heap. No provider key, response field or
challenge content enters observations, profiles or results. Permission does not
automatically reload a page or activate Submit.

Ordinary tests are offline. An authorized local Chromium smoke uses
`BROWSERTOOLS_VERIFICATION_LIVE_TEST=1 go test ./capture -run
TestPlaywrightRegistrationVerificationLoopbackOptIn -count=1`, with synthetic
loopback fixtures only. Provider integration and real authoring remain separate.

Browsertools supports the additive `uws.browser-registration.1.0` and `1.1`
contracts as offline producer tooling, emitting the oldest sufficient version.
Filled private input values and their resolution remain owned by UWS and the
runtime. Registration is not authentication: it creates a
remote account, while `uws.browser-authentication` signs in an existing
identity and establishes a named execution-local session.

## Owned boundary

Browsertools can:

- validate a registration profile through UWS' public embedded schema and
  semantic validator;
- reject inline secret-, credential-, and PII-shaped material before schema
  diagnostics can disclose it;
- build the exact profile from a complete explicit specification without
  inferring actions;
- calculate deterministic profile digests and expiry; and
- produce and verify a package-local digest-bound review.

All three CLI commands are file-only and offline:

```bash
browsertools registration-profile validate --input PROFILE --at RFC3339
browsertools registration-draft build --spec SPEC --out PROFILE
browsertools registration-review bundle --profile PROFILE --at RFC3339 --out REVIEW
```

Browsertools does not launch a browser for these commands. It does not resolve
credentials, inspect an account value, execute a submit, approve a mutation,
handle a human checkpoint, retry an outcome, or perform cleanup. A review
bundle proves only the exact inert profile digest and lifecycle; it is not
evidence that registration was attempted or succeeded.

## Separate no-submit authoring wire

Registration observation does not extend the authenticated author-session
protocol. The existing `browsertools.author-session.v2` wire includes bounded
human credential/MFA checkpoints, approved clicks and POST windows, a named
authentication session, and a paired authentication/capability result. Those
semantics remain immutable for existing consumers.

The registration producer uses separate v1 and v2 contracts. V1 remains
`browsertools.registration-author-session.v1` with private result
`browsertools.registration-authoring.v1`; it still rejects every query. V2 is
`browsertools.registration-author-session.v2` with private result
`browsertools.registration-authoring.v2`; it adds only reviewed literal
structural-query retention. The version split is a safety boundary:

- the registration session admits exact-origin GET/HEAD observation only;
- canonical unapproved non-navigation GET/HEAD subresources are accounted and
  aborted before contact without expanding the origin allowlist or failing an
  otherwise clean observation;
- no message can focus or type a credential, click or submit a control,
  approve POST, establish/export a session, or invoke a runtime;
- the result contains exactly one `uws.browser-registration.1.0` candidate and
  binds its existing `browsertools.registration-review.v1` review;
- the result records the reviewed submit description as inert profile
  material and separately proves that the producer never executed it; and
- neither wire claims an account attempt, outcome, duplicate check, cleanup,
  or registration-runtime support.

Adding registration message variants to author-session v2 would let version-
compatible consumers mistake observation for its intentionally broader login
authority. Returning a registration candidate from authenticated-authoring v2
would also violate that result's fixed BAP+BCP and named-session composition.
Separate v1 discriminators make unsupported cross-use fail during negotiation
or strict decoding.

V2 accepts an absolute, fragment-free URL with a query only when the URL and
query are already canonical and bounded. Query keys are unique and nonempty;
credential-shaped keys, secret/PII-shaped values, templates, userinfo,
malformed encoding, empty query items, repeated keys, and origin escape are
rejected before browser work. The same validator covers `start`, later
GET/HEAD `navigate`, and every retained BRP `navigate` step. The query remains
only in the reviewed canonical URL/profile source. Observations, diagnostics,
logs, filenames, result paths, and safe lifecycle evidence expose only exact
origin and disclosure-safe path.

The existing `browsertools author-session chromium` command continues to speak
authenticated author-session v2. The separate guarded Chromium producer is
available only as `browsertools registration-author-session chromium`; its
name, package, wire, browser interface, and result remain independent.

### Registration author-session v1

`registrationauthorsession.Serve` takes ownership of a closeable input and
exchanges newline-delimited JSON. Context cancellation closes that input so a
blocked read cannot keep the browser session alive. Every
message carries `protocol: "browsertools.registration-author-session.v1"`.
Input is limited to 256 KiB per line and 32 JSON nesting levels; duplicate
names, unknown or message-inappropriate fields, invalid UTF-8, and trailing
JSON fail closed.

| Client type | Additional fields | Valid phase |
|---|---|---|
| `start` | `profileId`, canonical query-free `url`, sorted exact `origins`, optional finite `bounds` | `awaiting_start` |
| `navigate` | `method` (`GET` or `HEAD`), canonical query-free `url` on an already approved origin | `observing` |
| `observe` | none | `observing` |
| `review` | complete `profile`, sorted current `candidateIds`, selected `flow`, explicit `cleanupDisposition` | `observing` |
| `finish` | none | `reviewed` |
| `close` | none | any open phase |

Registration author-session v2 has the same message union, phases, authority,
and bounds. Its only semantic difference is the strict retained-query rule
above. Callers select the v2 protocol explicitly; an omitted selection remains
v1.

The server emits only `hello`, `state`, `observation`, and fixed-code
`diagnostic` messages. An observation contains an exact origin, a
disclosure-checked path, a monotonically increasing generation, and reduced
accessibility candidates. The contract has no backend node-ID field; raw
labels remain only in the browser process long enough to be reduced.
The only backend session methods are `Observe`, `Navigate`, and `Close`;
all accept bounded contexts, and `Close` receives a fresh cleanup context even
when the parent session is canceled. `Navigate` accepts the closed `GET`/`HEAD`
enum. There is no API or message for
typing, focus, click, submit, POST approval, origin expansion, script, DOM or
page content, capture, cookie/storage access, or session export.

`capture.NewPlaywrightRegistrationBrowser` is the headed Chromium
implementation of that narrow interface. It launches one sandbox-required,
non-persistent context with the established sanitized browser environment,
blocks service workers, popups, downloads, dialogs, file choosers, WebSockets,
event streams, spontaneous or cross-origin navigations, and every method other
than GET or HEAD before continuation. It also intercepts canonical unapproved
non-navigation GET/HEAD subresources before contact, counts the denied attempt,
and continues without admitting that origin. Malformed URLs and every unsafe
method, channel, navigation, or bound remain terminal policy violations. It
accounts completed response bytes, obtains candidates only from bounded ARIA
snapshots, omits child-frame content with fixed diagnostics, and never calls a
Playwright input, click, page-value, capture, cookie, or storage API. Explicit
HEAD uses the context's request client with redirects and retries disabled and
is admitted/accounted by the same exact-origin guard before the request starts.

`registrationauthorworker.Run` is the reusable isolated process entry. It
performs read-only exact-version driver preflight, serves the closed protocol,
waits for clean browser teardown, and then calls `FinalizePrivate`. It returns
only success or failure: the digest-named result is discovered through the
separately protected owner-only root and its path never crosses NDJSON or
stderr. Cancellation closes the owned input and synchronously tears down the
session. A re-executing parent is responsible for stable executable hashing,
minimal environment construction, and process-group cleanup; the worker
exposes no Playwright type or in-process browser handle.

On Linux hosts where AppArmor disables Chromium's unprivileged user-namespace
sandbox, Browsertools may forward `CHROME_DEVEL_SANDBOX` to Chromium only when
it identifies a root-owned, single-link, mode-4755 regular helper on a setuid
filesystem whose resolved path is unchanged and whose complete ancestor chain
is root-owned and non-writable (apart from root-owned sticky directories).
User-controlled, linked, writable, relative, or unsupported-platform helper
paths are omitted. Chromium sandbox enforcement remains mandatory;
Browsertools never supplies a sandbox-disable flag.

The standalone entry is:

```bash
browsertools registration-author-session chromium \
  --private-root PRIVATE_ROOT [--driver-dir INSTALLED_DRIVER_DIRECTORY] \
  [--protocol v1|v2]
```

`PRIVATE_ROOT` must already be a non-symlink mode-0700 directory. The
`--protocol` selector defaults to `v1`; callers must choose `v2` deliberately,
and stdin/stdout must use its exact matching discriminator.
SIGINT/SIGTERM follows the same cancellation path. No path, digest, backend
error detail, credential value, account value, or raw observation is printed.
There is no registration submit or runtime command.

`registrationauthor.Build` is the deterministic bridge from that reduced
protocol observation to an explicit profile review. Its request contains the
complete `registrationdraft.Spec`, whole-second assessment time, canonical
approved origins, exact flow, sorted reviewed candidate IDs, exact submit
candidate ID, and explicit fixed call controls. The builder reconstructs every
candidate ID from generation/role/name/order, rejects stale or fabricated
generations, ambiguity, noncanonical labels, origin/time drift, and a submit
locator that is not the exact selected accessibility role/name. It does not
fill any missing locator, credential slot, step, checkpoint, success condition,
duplicate/ambiguity behavior, or cleanup choice. The returned object exposes
defensive copies and produces the exact M26 `review` message; it is not a
result, approval, runtime request, or private artifact.

Reduction is heuristic, not data loss prevention: ordinary names,
identifiers, and order numbers can remain in accessibility labels. Every UI or
terminal that displays or retains registration candidates must show
`registrationauthorsession.AccessibilityLabelDisclosure` and obtain human
review before result creation.

`review` accepts the whole schema-valid, current BRP rather than fragments. It
also selects one existing profile flow and one of the UWS cleanup dispositions.
Credential bindings and an approval claim are deliberately not review-message
fields. Candidate IDs must belong to the latest observation generation and
must resolve to unique, non-redacted accessibility names.

`finish` first closes the backend within the declared navigation timeout and
validates that its request count is the sum of bounded GET and HEAD counts. It
writes no result and discloses no
private path on the protocol. Only then does `Serve` return an in-process
`Completion` to the caller. EOF, cancellation, invalid network accounting, or
teardown failure returns no completion.

### Registration-authoring result v1

`registrationauthorresult.Build` converts that clean completion into one
private `browsertools.registration-authoring.v1` envelope. It contains:

- exact `browsertools`/result/session provenance and canonical
  `createdAt`/`observedAt`/`expiresAt` lifecycle times;
- the canonical `uws.browser-registration.1.0` source, its SHA-256 digest, and
  the existing `browsertools.registration-review.v1` bundle and digest;
- sorted exact origins, symbolic credential-slot inventory, and the selected
  flow's sorted effects, checkpoints, and success condition;
- exactly one reviewed current-generation, accessibility-name submit
  description with `executed: false`;
- fixed symbolic `approvalSymbol: registration_approval`,
  `operator_attestation`, `fail`, and
  `stop_without_retry` call controls plus the explicitly reviewed cleanup
  disposition; and
- finite bounds, observation and GET/HEAD accounting, closed diagnostics, zero
  mutation requests, and false submit/account/session/runtime claims.

Source and review digests cover compact JSON without a trailing newline. The
result digest returned by `registrationauthorresult.Digest` covers the exact
deterministic result bytes including their final newline. OpenUdon must verify
all three independently and use the result digest in transaction provenance.

`registrationauthorresult.Decode` rejects oversized, deeply nested,
duplicate-name, unknown-field, trailing, non-UTF-8, stale, noncanonical, and
digest-inconsistent results. `WritePrivateExclusive` accepts only an existing
owner-only, non-symlink directory and creates a mode-0600 digest-named file
without replacement. Its returned path is process-private and must never be
copied to protocol output, a prompt, a package, a report, or goal state.

`registrationauthorresult.FinalizePrivate` is the complete supported
post-teardown path. It builds from the clean in-process completion, marshals and
strict-decodes an independent reconstruction at the requested assessment time,
creates the digest-named file through one anchored owner-only root, reopens it
through that same root, and repeats byte/digest/lifecycle validation before
returning. The final file must remain a mode-0600 regular file and the root path
must still identify the anchored mode-0700 directory. Any post-create failure
removes only the exact newly created member. The returned path remains
process-private; only the reconstructed envelope and exact digest are eligible
for a separately protected consumer channel.

The supported typed entry points are:

```go
completion, err := registrationauthorsession.Serve(ctx, in, out, browser,
    registrationauthorsession.ServeOptions{
        Clock: clock, Protocol: registrationauthorsession.ProtocolV2,
    })
createdAt := clock().UTC().Truncate(time.Second)
finalized, err := registrationauthorresult.FinalizePrivate(
    registrationauthorresult.FinalizeRequest{
        Completion: completion, CreatedAt: createdAt,
        AssessmentAt: createdAt, PrivateRoot: privateRoot,
    })
```

`Browser` in this example is an implementation of the deliberately narrow
registration interface, not the authenticated author-session browser. The
caller retains `finalized.Written.Path` privately; a cross-process consumer exchanges a
separately protected result and its digest, never the path.

Registration-authoring result v2 has the same value-free shape and lifecycle
as v1, with v2 result/session provenance and independently revalidated safe
literal BRP navigation queries. Result filenames remain digest-derived and
never contain a URL or query.

## Portable safety contract

Credential slots are symbolic `identifier` or `password` names. Their values
must be entered by a human in the private browser or injected by a trusted
runtime; they never enter the profile, specification, review, CLI arguments,
stdout, logs, or Browsertools environment bindings.

Every flow declares exact HTTPS application/registration origins, exactly one
`submit`, `creates_account`, and `confirmationPolicy.required: true`. A
`human_checkpoint` records only its fixed kind and optional reviewed locator;
it never carries a CAPTCHA response, verification token, MFA response, or
consent value.

The corresponding `uws.browser-registration-call.1.0` envelope fixes:

```yaml
duplicatePrevention: operator_attestation
onDuplicate: fail
ambiguousOutcome: stop_without_retry
cleanupDisposition: delete_separately # or retain_dedicated_test_identity
```

The symbolic approval field is not proof of approval. A trusted downstream
runtime must bind actual human approval to the exact operation, profile bytes,
symbolic credential mapping, origin inventory, duplicate attestation, and
cleanup disposition immediately before the profile's one submit.

## Distribution

Registration profiles and reviews are package-local workflow inputs. They are
not browser capability bundles and cannot be published through Browsertools'
static registry. Raw authoring evidence, page content, captures, credentials,
account identifiers, verification material, and browser state do not belong in
the portable artifacts.

UWS owns the profile and call schemas. OpenUdon owns user-facing selection,
package review, and trusted handoff. Udon and Browserdriver own optional runtime
execution, credential resolution, human-checkpoint interaction, network and
redirect containment, approval enforcement, and value-free execution evidence.


## Generic registration 1.1 authoring

Select `registration-author-session chromium --protocol v3 --private-root DIR`
for `browsertools.registration-author-session.v3`. Existing v1/v2 behavior and
defaults remain unchanged. V3 produces `browsertools.registration-authoring.v3`
with registration 1.1 source and `browsertools.registration-review.v2`.
`registrationdraft.Build` chooses 1.1 only when explicit `inputSlots` exist.
Published UWS schemas and filled-input semantics remain owned by UWS.

V3 observations add optional public control definitions: supported kind,
required status, bounds and at most 32 public select options. No current input
value, checkbox state or selected option is read. Unknown widgets and unsafe
option metadata become unsupported. Reduction is heuristic, not DLP; operator
review is still required. `SuggestFields` returns deterministic unapproved
suggestions; semantic roles, conditions and ordering remain human decisions.

The closed `preview` command names a current candidate ID and generation,
`purpose: public_form_preview`, and exactly one `select` plus public `option`,
`check` plus Boolean `checked`, or non-submit `click`. It revalidates the exact
observed control before acting and emits the next observation. Consent,
verification, account creation and private input are outside this authority.
Known protected labels and submit controls are rejected. Form submission is
disabled in the authoring context, while origin, GET/HEAD, request, response,
popup, WebSocket and teardown guards remain active. A blocked mutation is a
terminal failure, never a successful preview. These are application controls,
not a claim of network-wide containment or exhaustive discovery.

Review includes `stepCandidates`, aligned with the selected flow sequence;
non-locator steps use an empty string. Every observed macro binds an exact
reviewed candidate in the bounded ordered history. Read-only clicks must bind
an exercised preview transition. Typed fills match the observed control kind
and public select choices. The separate success predicate remains reviewed
and deferred; authoring never claims a post-submit observation. V3 retains at
most the configured observation/candidate bounds and 256 KiB of observation
history, and every complete protocol frame is bounded separately.

`registrationauthor.Build` accepts the v3 history, previews and step candidates
with the explicit specification and call controls. Finalization independently
reconstructs the canonical source, all reviewed candidates, history and preview
proofs after clean teardown. New exports omit discovery inventory. The private
result remains outside packages and includes no runtime input document.

Qualification uses synthetic fixtures only:

```sh
BROWSERTOOLS_REGISTRATION_LIVE_TEST=1 go test -count=1 ./capture -run TestPlaywrightRegistrationV3LoopbackOptIn
```

The conditional two-step wizard passes through the actual NDJSON producer and
real installed sandboxed Chromium; a preview-triggered POST must fail without
reaching the fixture server. Legacy producer tests remain mandatory.
