# Private authenticated worker diagnostics

`authorworker.Options.DiagnosticPath` optionally reserves one private mode-0600
file beneath an existing mode-0700 directory. It emits
`browsertools.author-diagnostic.v1` with a closed class after the worker returns.
The native parent, not a public author-session client, opts into this channel.
A file that is missing, empty or malformed is not evidence of success.

`authordiagnostic` reduces typed backend errors, cancellation and deadlines;
it never parses or retains raw error strings. Stage/reason pairs distinguish
driver, headed launch, context/page/guard setup, navigation and policy rejection.
Observation-time origin rejection retains its policy class. That does not prove
that the request was prevented. Unknown errors stay unknown. Reader validation
requires the exact canonical schema and rejects additional or duplicate fields.

Public author-session v2 and legacy diagnostic meanings are unchanged. No
network allowlist, identity, browser option, request allowance or credential
behavior is widened. Opting in does not authorize a live capture or retry.

The W13.1c local redirect control exposed an existing admission defect: a
redirect to an unapproved loopback origin reached that endpoint before observation
rejected it. The diagnostic candidate only records this rejection. A14.3 owns
containment repair before source publication/qualification can support another
live operation. Development tests use only test-owned loopback fixtures.


## Redirect admission candidate

Authenticated authoring installs Chromium browser-target Fetch admission before
creating a page. Every initial request and redirect hop passes the same exact
origin, navigation, request and POST budget. The browser-wide boundary includes
popup, child-frame and worker requests without per-target attachment races.
Chromium retains its HTTP transport, response headers, cookies, compression and
redirect methods. Existing declared-size and completed-transfer accounting
remains; it is not a hard streaming-byte or host-wide isolation guarantee.

Interception stays installed until browser destruction. Unsupported setup, lost
control, malformed events, command timeout or more than 128 pending request
commands fails closed. No request/response body, header or credential is read or
exported by the guard. Public messages and registration behavior are unchanged.

The explicit local regression selection is
`BROWSERTOOLS_AUTHOR_LIVE_TEST=1 go test ./capture -run TestAuthorRedirectContainmentLoopbackOptIn -count=1`.
Use the pinned installed driver, sandbox and a supervised isolated headed display.
Fixtures are test-owned loopback endpoints with independent arrival counters.
This opt-in test grants no real-site authority. Earlier per-target candidates
and their failures remain in the W13.1c private repair kit; they are not selected.


## Optional original startup snapshot

`authorworker.Options.StartupDiagnosticSink` and
`capture.NewPlaywrightAuthorBrowserWithDiagnostics(driverDirectory,
blockedScriptOrigin, sink)` accept a caller-owned
`chan<- authordiagnostic.StartupFailure`. Nil is the default: driver stdout and
stderr remain discarded and no original-error formatting or snapshot output
occurs. This channel does not change `DiagnosticPath`, its strict v1/v2 records,
native NDJSON, public errors, policy, browser options, or registration workers.

This option is **only for explicitly authorized private synthetic diagnostics**.
Original backend errors and driver stderr may contain paths, URLs, page text or
secrets. The trusted caller owns the decision to enable it, any serialization,
private storage, lease binding, redaction and finite retention. It grants no
browser, filesystem, network, retry or cleanup authority. The producer neither
opens a diagnostic path nor writes a new stderr/stdout envelope for this option.

Create a channel with capacity at least one for each single worker lifetime and
inspect/drain it within that lifetime. One failed `Open` offers one snapshot;
preflight failure in `Run` offers one instead before `Open` is reached. A
successful startup emits none. Constructor/worker-boundary refusals and abrupt
process loss may occur without a snapshot. An empty channel never proves
success. An unbuffered, full or closed channel drops the snapshot immediately;
the sender never closes it, waits for a receiver, calls arbitrary sink code or
starts a delivery goroutine. A contained closed-channel send panic cannot
replace the original operation failure. The caller must not depend on delivery
as a prerequisite to settlement.

`StartupFailure` keeps these distinct:

- `Operation`: closed startup operation (`validate`, `preflight`, `driver`,
  `launch`, `context`, `permissions`, `guard`, `page`, `timeout`, `navigation`,
  `navigation_response`).
- `Class` and `Rejection`: the existing validated closed classifications,
  calculated from the original typed cause before backend privacy flattening
  and before deferred cleanup. Guard policy rejection is captured before the
  outer policy-conversion defer. Unknown stays unknown.
- `Types`: positive `Timeout`, `TargetClosed`, `Canceled`, and `Deadline` facts
  from wrapped sentinel identity only. Error text never mints a typed fact.
- `RawError` and `DriverStderr`: separate `PrivateCapture` values with `Text`,
  `Truncated`, `Missing` and `CaptureError`. Original text retains its first
  16 KiB; a per-Open concurrent ring retains the latest 64 KiB of driver stderr
  and continues accepting all writes after overflow. Caps count original bytes
  before downstream encoding. No raw error object or `Unwrap` is retained.

Snapshots precede failed-Open cleanup; cleanup errors and later stderr cannot
replace or append to the original snapshot. Failed formatting marks only
`RawError.CaptureError`/`Missing`; trusted backend `Error` methods must return
promptly. No arbitrary formatter timeout is claimed. Ring writes and snapshots
invoke no caller code; an empty ring is `Missing`, including preflight, where no
driver exists. Truncation is independent of missing/capture failure, and none
of these states proves complete cause, effects, process Join or safe replay.
`StartupFailure.Valid` checks closed operations/class/rejection and memory caps;
the downstream carrier must additionally validate its own framing and binding.

Offline controls use injected driver failures and the real `authorsession.Serve`
path, fake browsers for native success/refusal, and test-owned absent driver
paths for preflight. They launch no browser, driver or external endpoint.
