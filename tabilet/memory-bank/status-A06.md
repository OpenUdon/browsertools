# A06 Reusable Isolated Author-Session Worker

| Item | State | Notes |
| --- | --- | --- |
| A06.1 Importable process entry | `[+]` | `authorworker.Run` accepts context, private root, optional driver directory, stdin, and stdout and invokes the existing Chromium author-session implementation without exposing Playwright objects. |
| A06.2 Standalone CLI reuse | `[+]` | `browsertools author-session chromium` delegates production execution to `authorworker` while retaining its injectable test seam, exact arguments, v2 protocol, exit behavior, and missing-private-root usage failure. |
| A06.3 Boundary propagation | `[+]` | The worker owns an interruptible `io.ReadCloser`; cancellation closes it without a copying pipe or leaked reader goroutine, emits `canceled` rather than `protocol_limit`, closes the live browser session, and waits for teardown before return. |
| A06.4 One-binary consumer boundary | `[+]` | OpenUdon links the entry only for a privately stabilized, separately re-executed hidden `icot` worker; no authoring engine or HTTP handler receives a page, context, browser session, or private result. |
| A06.5 Regression coverage | `[+]` | Focused worker/CLI tests cover required inputs, hello/close negotiation, malformed input, cancellation while blocked at a human protocol checkpoint, browser-session closure, driver propagation, and compatibility behavior; full Browsertools tests and vet pass in the coordinated workspace. |
| A06.6 Lifecycle and approval bounds | `[+]` | Standalone SIGINT/SIGTERM uses worker cancellation, close/teardown failures return nonzero, and approval identifiers fail closed at 9,999 before the fixed-width form can overflow. |
| A06.7 Read-only driver preflight | `[+]` | Doctor and author-session startup verify installed Node/CLI files and exact Playwright 1.62.1 metadata without cache creation, driver execution, installation, or network access. |
| A06.8 Safe disclosure surfaces | `[+]` | The full doctor report remains CLI-only; the separate UI-safe shape omits browser executables and path-bearing errors, and the exported operator disclosure states that useful-label reduction is heuristic rather than DLP. |
| A06.9 Publication handoff | `[+]` | Commit `7ea7e832d7f85060cf57a7a08bf9af6bb7eca896` is published; OpenUdon pins `v0.0.0-20260821154836-7ea7e832d7f8`, and its early `GOWORK=off` iCoT build resolves without a local replacement. |
| A06.10 Disclosure and producer limits | `[+]` | The shared `disclosurepath` package rejects unsafe escaped observation, goal, popup, and frame paths before protocol output or result construction; author-session also enforces 64 contexts, 256 cumulative unique diagnostics, exact checkpoint input kinds, unique dashboard proof, and a valid `main` context on terminal closed state. |
| A06.11 Coordinated UWS revision | `[+]` | Browsertools now tests against the UWS pseudo-version used by OpenUdon's compatibility lock (`v0.0.0-20260819172405-e81d0dee410a`); full standalone Browsertools tests and vet pass at that revision. |
| A06.12 Hardened publication handoff | `[+]` | Historical source publication confirmed: exact `e392fd080fc47789ac64edd5fe7953a8cf104ee0` is an ancestor of independently fetched origin/main `b45a66ba43114409af62dcf851de14d45648a5b1`. Its timestamp matches `v0.0.0-20260822182551-e392fd080fc4`. Prior local verification remains recorded; no new module download, consumer-pin qualification or runtime adoption is claimed. |

The original A06 publication remains complete; the hardened follow-up source
publication is historical and independently confirmed. The stale marker is
corrected without replaying its consumer-pin qualification. This milestone does not change
author-session/result v2, install Chromium, expose
credentials, or move browser execution into OpenUdon's engine or HTTP process.

## Historical publication pointer correction — 2026-10-10

The independently fetched exact approved Browsertools origin/main contains
e392fd080fc47789ac64edd5fe7953a8cf104ee0. No new source or dependency publication
is needed for this old handoff. Original A06 acceptance stays frozen; this is
a factual correction of its previously nonterminal follow-up, not a new review
iteration or consumer adoption. Existing later published/qualified consumer
records retain their own proofs and pins.

Superseded current wording is preserved:

``````markdown
A06.12 Hardened publication handoff | `[~]` | Revision `e392fd080fc47789ac64edd5fe7953a8cf104ee0` is locally verified and recorded downstream as `v0.0.0-20260822182551-e392fd080fc4`, but is not yet published as a remotely resolvable pseudo-version; OpenUdon standalone pin qualification remains downstream of publication.

The original A06 publication remains complete; the hardened follow-up is
implemented and awaiting coordinated publication.
``````
