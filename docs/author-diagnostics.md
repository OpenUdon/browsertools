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
