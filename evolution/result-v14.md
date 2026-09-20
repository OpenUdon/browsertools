# Reusable Isolated Author-Session Worker Result

Browsertools A06 adds `authorworker.Run`, the Browsertools-owned process entry
for Chromium author sessions. It accepts only context, restrictive private
root, optional driver directory, stdin, and stdout, then serves the existing
author-session v2 implementation and Playwright author browser.

The standalone `browsertools author-session chromium` command now delegates to
that package while retaining its test injection seam. OpenUdon's coordinated
A16 change links the package into `icot` but invokes it only after privately
stabilizing and separately re-executing the executable, so the engine and HTTP
server never initialize Playwright.

Focused tests cover validation, protocol close negotiation, blocked-input
cancellation, browser teardown, driver preflight and propagation, approval
bounds, and safe disclosure shapes. The wire, private result, browser
installation policy, and process ownership are unchanged. Commit
`7ea7e832d7f85060cf57a7a08bf9af6bb7eca896` is published and OpenUdon pins its
ordinary pseudo-version for standalone module verification.
