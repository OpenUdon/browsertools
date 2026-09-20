# Reusable Isolated Author-Session Worker

Support the unified iCoT UI and one-binary release without weakening the
Browsertools boundary. Extract the production Chromium author-session runner
behind an importable process entry accepting context, private root, optional
driver directory, stdin, and stdout. Reuse it from both `cmd/browsertools` and
iCoT's hidden separately re-executed worker.

Do not change `browsertools.author-session.v2`, result v2, Playwright/context
ownership, browser installation policy, or credential/session handling. The
entry is for a worker process, not an in-process page/context service.
