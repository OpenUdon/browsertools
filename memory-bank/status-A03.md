# Status A03 - Persistent Authenticated Browser Authoring

## Goal

Keep one headed Chromium context alive across human login/MFA and post-login
exploration without exporting session or credential material.

## State

Complete.

## Dependencies

- Browsertools M21 authenticated goal-authoring design.
- UWS 1.8 browser-context contracts.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, review, and verify the persistent author session | `[+]` | Browsertools `50bbfdd` adds the strict NDJSON state machine, explicit CLI, headed non-persistent Chromium owner, human credential/OTP/push checkpoints, exact same-origin GET and separately approved origin/click/POST authority, popup/frame registry, finite bounds, sanitized environment, fixed failure diagnostics, teardown-before-result, fake-session coverage, and an opt-in installed-Chromium redirect-login fixture. No value, cookie, storage state, raw semantic tree, selector, browser handle, or resumable context crosses the process boundary. |

## Verification

- [x] Workspace and standalone `go test ./...` pass.
- [x] `go vet ./...` and `git diff --check` pass.
- [x] Default tests are browser-free and network-free.
- [x] `BROWSERTOOLS_AUTHOR_LIVE_TEST=1` is required for the headed loopback
  fixture; it was not enabled because the pinned browser is not installed here.
