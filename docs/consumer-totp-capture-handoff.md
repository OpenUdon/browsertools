# Consumer TOTP capture and input lessons

Received 2026-10-01 from a private consumer's completed W27 integration. This
record transfers reusable observations and code ownership; it contains no
target origin, account identifier, private profile, capture or credential.
The receiving Browsertools baseline is `2cdd788`; the consumer qualified
Browsertools `3abe70efc03d9ccb97b8b30e5e86328f60a70c64`. Later documentation
does not select a new runtime.

## Authored challenge and runtime credential are different inputs

During capture, the person enters a current authenticator code into the real
challenge field. A value-free authentication profile records
`challenge.kind: totp`, its reviewed locator and a `totp_seed` credential slot.
The trusted runtime later receives the setup seed and computes the code.
It must not treat a recovery code or six-digit current code as that seed.
Recovery lists and setup keys are separate concepts; an issuer-specific shape
check belongs to the consumer, not to the portable profile schema.

Existing implementation: [authorresult](../authorresult/result.go) emits the
challenge slot and the `sends_mfa_challenge` effect. Existing regression
`TestBuildAuthorsEveryReviewedMFAKindExactly` checks each supported challenge
kind, the locator and exact TOTP slot/kind. A synthetic reduced trace with
`Kind=focus_human_input`, `InputKind=otp`, `ChallengeKind=totp` reaches this
producer without real credentials or a browser. This is the focused
reproduction and regression for the received contract.

**Disposition:** supported and exercised through the qualified consumer;
no Browsertools implementation or schema delta is required. Credential
resolution/code generation remains a runtime responsibility. OpenUdon owns
the [integration handoff](../../openudon/docs/consumer-totp-qualification-handoff.md).

## Isolate desktop input as well as browser state

An ephemeral browser context does not isolate the desktop mouse or focus.
A separate X server can preserve headed coverage without stealing the host
pointer, but its clipboard is independent. A visible field and a completed
human checkpoint do not establish that Paste delivered text. Confirm visible
entry while preserving value-free evidence; never infer authentication success
from focus/checkpoint completion.

A synthetic reproduction uses two disposable X servers and a custom selection
with a harmless marker. Setting the selection in one server must not create
it in the other. This does not require touching the owner's clipboard or
capturing a screen. The consumer verified this separation and later verified
Paste through host-desktop credential dialogs while keeping the browser on
a separate display. Direct browser capture instead used visible typing.

**Disposition:** documented operating guidance; no automatic clipboard bridge
or Browsertools credential collector is added. A future reviewed authoring
launcher may select a bridge only with an explicit privacy/ownership design.
That optional UX direction belongs to Browsertools author-session tooling and
OpenUdon's supervised UI; it is unpromoted and grants no live authority.

## Preserve failure and teardown as separate evidence

A later successful capture does not establish an earlier failed attempt's
cause. Retain consumed claims and join the primary journey error with cleanup
errors; both must remain fatal. A worker exit, controller result or outer
process cleanup alone does not prove the authored authentication passed.
Use reviewed challenge metadata to identify the native human checkpoint and
require the protected success-page proof before promoting a profile.

**Disposition:** existing profile/trace validation and consumer cleanup checks
apply. No generic capture behavior change is needed for this delivery. Keep
uncertain observations as uncertain; no historical registration acceptance is
inferred from this TOTP sign-in proof.

## Verification and boundary

The focused browser-free command is:

```sh
go test ./authorresult -run '^TestBuildAuthorsEveryReviewedMFAKindExactly$'
```

Default tests and vet remain browser-free. The consumer separately completed
fresh native qualification, fresh synthetic TOTP/count journeys, independent
verification and one authorized live TOTP count with clean teardown. This
received record creates no acquisition, publication or runtime-adoption
authority. No source or dependency pin changes are selected here.

Receipt checks on 2026-10-01: the focused producer test passed in 0.086 s;
default browser-free tests passed in 60.464 s, vet in 6.785 s, and diff checks
passed. No browser, acquisition or source-code change was involved.
