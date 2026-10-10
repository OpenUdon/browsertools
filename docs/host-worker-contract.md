# Managed capture worker contract

Kinet and other hosts can use `authorworker.Launch` and
`registrationauthorworker.Launch` with an explicit `workerhost.Host` capability.
The embedding host selects an already qualified, stable executable and its
arguments, exact worker/driver/Node/Playwright/Chromium closure, original
admitted deadline, launch nonce, sorted exact origins and independently issued
capture binding. Host-only installation/display/result-root handles cannot come
from profile bytes, model output or a request DTO. Runtime credential values
and retained sessions are outside these capture APIs. Human capture credential
and TOTP entry remains directly in the existing headed browser protocol.

```go
type Host interface {
    Launch(context.Context, workerhost.LaunchSpec) (workerhost.Launch, error)
    Join(context.Context, workerhost.JoinRequest) (workerhost.JoinResult, error)
}
```

This capture interface is additive Browsertools API, not a replacement for
M51's frozen native Udon execution ABI. `workerhost.Closure` retains that
artifact's identity field spelling so a Kinet adapter can bind the same exact
installation facts. `LaunchSpec.BindingSHA256` is a host-issued capture binding;
it neither substitutes for native runtime plan/claim/session structures nor
authorizes an operation. There is no OpenUdon or Udon import.

`Launch` must establish host-owned default-closed all-traffic containment
**before starting any selected process or browser capability**. It binds
the exact closure, nonce, origins, original deadline and stable process and
containment leases. The host verifies actual artifact hashes and current human
policy independently; strings and true flags are trusted-host attestations,
never proof supplied by a model, worker result or DTO. Missing capability,
unsupported platform/channel, or failed containment refuses. There is no direct
network or legacy-launch fallback.

Containment covers fetch/XHR, every subresource and redirect intermediate,
frames/popups, WebSockets, DNS, connect tunnels, service workers/background
traffic, WebRTC/UDP, debugging and downloads. Existing navigation/origin guards
remain defense in depth; `visitedUrls` is not proof. A host either closes every
independent route or refuses launch. Kinet M56 owns the actual implementation
and bypass/teardown qualification. These APIs and fake unit witnesses do not
qualify it.

Only a matching successful `Launch` exposes the channel to the exchange
callback. Managed code checks exact worker and driver closure, executable and
arguments, nonce, capture identity, full native protocol, origin inventory,
sandbox and original admitted deadline. Host mutation cannot redefine the
guard's independent expected origin or argument slices. Every send/receive has
a 1 MiB maximum and the original runtime deadline, even when a callback uses a
background context. Send/receive/Close must honor cancellation. Callback code
must join its own work before returning; the host retains stable channel and
reader ownership.

**The host channel must recheck current exact authority and acquire every
required durable dispatch claim immediately before an effect-capable send.**
This includes approved capture interactions and continuations under their native
policy. Profile/schema validity, a launched process or an issued question does
not substitute for that claim. Browsertools implements no second claim/policy
engine. After human waits, the host rechecks current origin/identity/approval
state under the original deadline, without holding a credential/app writer
lock or transferring an old grant. Runtime private credential aggregation,
registration input, automatic TOTP, session save and report-v5 semantics remain
their native and Kinet owners' work.

After exchange completion, failure or cancellation, managed code fences the
channel against further sends, cancels runtime work, closes transport and calls
`Join` under a separate positive timeout of at most30seconds. Both partial
launch errors and invalid returned bindings still receive this bounded join
attempt. The expected original spec accompanies returned partial custody;
an error with no returned channel is not evidence that preparatory processes
are absent. Join must settle the worker, Node/Chromium descendants, preparatory
processes, host callbacks and readers by stable identities before containment
release. Closing a channel is not a joined process tree.

Unproved join, wrong joined nonce/leases, an expired close budget or a join error
returns the fixed `ErrUnjoined`; the host retains custody/quarantine and refuses
release, retry, session save/reuse or another execution until reconciliation.
Acknowledgements returned after the independent join deadline are refused even
when they claim success. Late received data is fenced against runtime
cancellation and channel closure before it reaches the callback.
An abandoned close remains host-owned. A joined launch failure or typed
exchange error still does not prove no dispatch or no business effect. There
is no automatic continuation, fresh-context fallback, successor approval or
runtime report interpretation in this API.

Authenticated capture requires the exact existing
`browsertools.author-session.v2` protocol. Registration capture explicitly
selects the complete existing registration-author-session v1, v2, v3 or v4
discriminator; omitted protocols and shorthand are refused by managed entry.
Registration remains no-submit authoring, with no account creation, credential
resolver or registration-to-authentication context transfer. Private result
consumption, independent validation and host publication occur only after
joined teardown and the existing exact host gates.

`authorworker.Run`, `registrationauthorworker.Run` and their CLI commands retain
their accepted explicit legacy profiles and default wires. A managed launch
never invokes them as a fallback. Its host-selected child image may call the
existing process entry **inside the already established containment**. Imports
alone never install or start a browser. All author/result/review and
registration v1–v4 bytes stay unchanged.

Default tests use fake hosts/channels and disposable roots. They verify pre-input
refusal, exact identity comparisons, full legacy protocol selection, channel
bounds, cancellation, partial launch joining, redacted uncertainty and expired
teardown. They perform no real browser/helper launch, fixture networking,
credential access, deployment or actual consumer adoption. The unchanged M51
manifest and host fixtures remain independent frozen contract inputs.
