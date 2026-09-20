# A08 No-Submit Browser Registration Profile Producer

Item | State | Notes
--- | --- | ---
A08.1 Guarded Chromium registration observation | `[+]` | Browsertools `3da34ff308af60dc2e0e2b4f6f703a4d53d11d17` adds one headed, sandbox-required, ephemeral Playwright implementation of the completed M26 interface. Its separate pre-request guard admits only exact-approved-origin GET/HEAD, explicitly accounts guarded context HEAD with redirects/retries disabled, blocks mutation methods, spontaneous navigation, persistent resources, popups, downloads, dialogs, file choosers, service workers, and response/request overflow, and returns only bounded main-document ARIA role/name/match observations with fixed child-frame diagnostics. Source gates prove no input/click/page-value/capture/cookie/storage APIs. Focused race/vet/format/diff tests pass. The installed pinned driver/browser preflight passes; a headed Xvfb loopback attempt failed closed before launch because this host's AppArmor policy provides no usable Chromium sandbox, and no disable override was used. The opt-in test exists for the required sandbox-capable A08.5/E10 environment.
A08.2 Deterministic BRP construction | `[+]` | Browsertools `3b4c3e15f80f8c72489e023686fb45bc54c9996f` adds browser-independent `registrationauthor.Build`. It requires a complete explicit draft, whole-second assessment, exact origins/flow, sorted reviewed IDs, exact submit ID, and explicit approval-symbol/duplicate/ambiguity/cleanup controls; it reconstructs current-generation candidate identities and binds the profile's sole accessibility-name submit without inferring any locator, slot, step, checkpoint, success, or safety choice. Its immutable-by-API candidate returns defensive copies and the exact M26 review transition. Focused standalone race/vet/format/diff tests pass, including deterministic canonical bytes, UWS validation, missing-decision, drift, ambiguity, leak, mutation-isolation, and complete fake-session round trips.
A08.3 Private result and review lifecycle | `[+]` | Browsertools `25d1acd37c68e373037638a62f460db69ac83b88` adds `registrationauthorresult.FinalizePrivate` as the supported post-teardown lifecycle. It builds and independently strict-decodes before persistence, creates once through one anchored owner-only root, reopens the mode-0600 regular file through that same root, repeats exact byte/digest/freshness reconstruction, revalidates root identity, and removes only its exact new member after a post-create failure. The returned private path remains in process and never enters the envelope. Ten standalone race repetitions plus vet/format/diff pass across success, root/member mode and symlink, overwrite, tamper, expiry/future time, invalid teardown accounting, secret/PII, mutation, and failed-finalization cleanup cases.
A08.4 CLI and isolated worker integration | `[+]` | Browsertools `984873a426d305f2be8eb56d3ac2710624fe858b` adds `registrationauthorworker.Run` and the distinct `registration-author-session chromium` entry. Production performs read-only exact-driver preflight, owns closeable v1 NDJSON input, composes only the narrow guarded browser, waits for clean teardown, and finalizes under a required private root without returning or printing a path/digest. Explicit close creates nothing; cancellation interrupts blocked input and synchronously closes the session; fixed CLI errors hide backend/private detail; real sub-second clock, strict result decoding/mode, help/dispatch, missing inputs, driver propagation/preflight, output failure, and no Playwright/environment surface pass focused race/vet tests. The re-executing OpenUdon parent retains the already qualified stable executable, minimal environment, and process-group containment mechanisms; A19 integrates this entry through that boundary.
A08.5 Security qualification, review, and publication | `[+]` | Complete and published. Default standalone test/vet/race, pinned `govulncheck` v1.6.0, UWS, current OpenUdon consumer, Windows/Darwin cross-compile, formatting/diff, and sandboxed installed headed-loopback gates pass. Browsertools `a638c368a00abd463319b0b86de0bf48358b0040` adds validated official setuid-helper support and iteration-1 fix `39e32c1d6f601561cc5c13ec85201815ce85ab9b` hardens its full path. Iteration 3 closes review with no unresolved P1/P2. Exactly `39e32c1d6f601561cc5c13ec85201815ce85ab9b` was pushed to `origin main`, resolves independently as `v0.0.0-20260825225202-39e32c1d6f60`, and is reconciled into A19, A20, and E10. No tag, release, PR, deployment, target access, account action, or other push occurred.

Dependencies: completed Browsertools M26 at
`c52af3c058eb0f2cd021487260a9a692c67fdeb4`. The exact completed A08 head is
published at `39e32c1d6f601561cc5c13ec85201815ce85ab9b`; there is no tag,
release, PR, deployment, target access, registration attempt, sign-in, or
other push.

Browsertools evolution result v15 records the completed sequence after
OpenUdon E10 and offline private W8M W01 passed.

Bounded review log:

- Iteration 1 found one P2 sandbox-path integrity issue. The new Linux helper
  check protected only the helper and its immediate parent, so the documented
  host-admin-controlled guarantee did not cover a symlinked or writable
  ancestor; it also forwarded non-executable setuid files that Chromium would
  reject later. Browsertools `39e32c1d6f601561cc5c13ec85201815ce85ab9b`
  now requires an unchanged resolved path, exact mode 4755, a setuid
  filesystem, and a fully root-owned ancestor chain with only root-owned sticky
  writable directories allowed. Focused race/vet regressions and the installed
  sandboxed headed loopback pass after the fix.
- Iteration 2 found one P2 provenance defect: the status ledger expanded the
  abbreviated A08.4, sandbox-portability, and iteration-1 commit IDs to
  nonexistent hashes instead of recording the repository's exact commits.
  Tofu `767100c00dbb48b233b1a05728c82e94aa4754b1` corrects all three identities;
  each now resolves to the exact Browsertools commit.
- Iteration 3 reviewed the full A08 implementation, both fixes, tests,
  documentation, failure semantics, disclosure, compatibility, and operational
  boundary and found no P1/P2 or higher-severity issue. The bounded gate passes.
  Final `GOWORK=off` Browsertools default/vet/race and pinned
  `govulncheck@v1.6.0`, workspace and standalone UWS test/vet, workspace and
  standalone OpenUdon test/vet, Windows/Darwin capture cross-compilation,
  formatting/diff, and the sandboxed installed headed loopback all pass at
  Browsertools `39e32c1d6f601561cc5c13ec85201815ce85ab9b`.

Operational qualification note: one preliminary direct-Chromium sandbox probe
logged a background public component-registration request attempt before it
was terminated. It accessed no product target, account, credential, or user
data. The disposable profile was removed, direct probes were discontinued, and
the passing installed gates use Playwright's background-network suppression
plus the exact loopback route guard.

M77 reconciliation: the private producer result must be sufficient for an
independent consumer to construct the `registration` candidate state of
`openudon.browser-profile-transaction.v1`. Candidate order is the single BRP;
the `browsertools.registration-authoring.v1` result/review must prove the exact
source and review digests, canonical origins and freshness, symbolic bindings,
no session, and the no-submit fact. It must not emit OpenUdon
preparation/promotion state or any private path.

M26 completion reconciliation: A08 implements rather than revises the separate
`browsertools.registration-author-session.v1` and
`browsertools.registration-authoring.v1` contracts. The producer must use the
exported reducer disclosure, preserve latest-generation exact submit binding,
pass a fresh bounded context to teardown, bind `observedAt` to the accepted
observation, return the private result only in process, and use the anchored
owner-only no-replace writer. The complete reviewed producer head is now the
published dependency for OpenUdon A19/A20/E10.
