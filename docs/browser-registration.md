# Browser Registration Profiles

Browsertools supports the additive `uws.browser-registration.1.0` contract as
offline producer tooling. Registration is not authentication: it creates a
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

The registration producer instead uses the separate
`browsertools.registration-author-session.v1` protocol and emits a private
`browsertools.registration-authoring.v1` result. The version split is a safety
boundary:

- the registration session admits exact-origin GET/HEAD observation only;
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

M26 publishes the browser-independent Go contracts only. It does not add a
live registration command or change `authorworker`; the existing
`browsertools author-session chromium` command still speaks authenticated
author-session v2. A guarded Chromium implementation must satisfy these
contracts independently before a live registration producer is available.

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

The server emits only `hello`, `state`, `observation`, and fixed-code
`diagnostic` messages. An observation contains an exact origin, a
disclosure-checked path, a monotonically increasing generation, and reduced
accessibility candidates. The contract has no backend node-ID field; raw
labels remain only in the browser process long enough to be reduced.
The only backend session methods are `Observe`, `Navigate`, and `Close`;
`Navigate` accepts the closed `GET`/`HEAD` enum. There is no API or message for
typing, focus, click, submit, POST approval, origin expansion, script, DOM or
page content, capture, cookie/storage access, or session export.

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

`finish` first closes the backend and validates that its request count is the
sum of bounded GET and HEAD counts. It writes no result and discloses no
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

The supported typed entry points are:

```go
completion, err := registrationauthorsession.Serve(ctx, in, out, browser,
    registrationauthorsession.ServeOptions{Clock: clock})
createdAt := clock().UTC().Truncate(time.Second)
result, err := registrationauthorresult.Build(
    registrationauthorresult.BuildRequest{Completion: completion, CreatedAt: createdAt})
resultDigest, err := registrationauthorresult.Digest(result)
written, err := registrationauthorresult.WritePrivateExclusive(privateRoot, result)
```

`Browser` in this example is an implementation of the deliberately narrow
registration interface, not the authenticated author-session browser. The
caller retains `written.Path` privately; a cross-process consumer exchanges a
separately protected result and its digest, never the path.

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
