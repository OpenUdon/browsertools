# Retired milestone A09 - Guarded Chromium Registration Producer V2

**Milestone.** A09
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A09.md
**Source specification.** tabilet/memory-bank/milestone.md#a09-guarded-chromium-registration-producer-v2
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 2
**Verification.** Review iteration 2 passed with no remaining P1/P2. Recorded checks: No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A09 Guarded Chromium Registration Producer V2

**Goal.** Add explicit v1/v2 worker selection and enforce M27 through the
guarded no-submit Chromium producer.

Dependencies: M27.

Acceptance:

- The worker defaults to v1 and accepts only explicit `--protocol v1|v2`.
- Initial, later, and redirected requests apply the selected protocol's exact
  URL rule before network contact.
- Sandboxed loopbacks prove safe-query retention, query non-disclosure,
  GET/HEAD-only behavior, teardown, and unchanged v1 behavior.
- Full security and bounded review gates pass; publication waits for explicit
  authorization.

Status: complete and published at Browsertools
`d26f2982db352619d7a7f6563add802b56e10824`; the coordinated v16 result is
published at Tofu `95752f6` after E11 passed; see [status-A09.md](status-A09.md).

````

## Status record

````markdown
# A09 Guarded Chromium Registration Producer V2

Item | State | Notes
--- | --- | ---
A09.1 Explicit worker protocol selection | `[+]` | Browsertools `91f2eecc500ebf4137eacd93e1799e3b8a97cc00` adds the closed `--protocol v1|v2` CLI selector and matching reusable worker option, both defaulting to v1. Unsupported selections fail before browser construction and close owned input; v2 is carried through `ServeOptions`, BrowserRequest, completion, and private v2 result provenance. Focused worker/CLI tests prove default v1, explicit v2 with retained `action=startnew`, generic errors, query-free stdout, v2 private-result identity, close/cancellation behavior, and no new runtime command.
A09.2 Chromium retained-query enforcement | `[+]` | Browsertools `1511ef95224692e47da09e4ac5f037b46ded4fb1` carries the full selected protocol into the guarded browser request. The backend applies M27 before initial navigation, explicit GET/HEAD, current-page/frame observation, and every document redirect route; exact-origin resource requests retain ordinary page-controlled cache queries without treating them as reviewed navigation. Safe facts return only origin/path, and all rejected query material is hidden behind fixed errors. Focused tests prove v1 rejection/default, v2 `action=startnew`, unsafe key/fragment/origin rejection, safe same-origin resources, redirect-route enforcement, and no query in observation facts.
A09.3 Synthetic loopback and teardown qualification | `[+]` | Browsertools `94adf4a1bebae9058bec4fef3231fe8a9f7e99c5` adds a real headed v2 loopback that observes the exact structural `action=startnew` on GET and HEAD while Browsertools returns only origin/path, closes the ephemeral browser, and reports only GET/HEAD accounting. The existing v1 loopback remains unchanged and passes in the same sandbox-required Xvfb invocation. Worker coverage proves v2 finalization only after teardown, digest-only query-free filenames, no query on NDJSON, cancellation/input closure, no session/state export, and no submit/account/mutation claim. Focused synthetic redirect-route tests reject unsafe query, fragment, and origin escape before continuation with fixed diagnostics.
A09.4 Documentation, review, and publication readiness | `[+]` | Browsertools `d26f2982db352619d7a7f6563add802b56e10824` documents `--protocol v1|v2`, explicit Go selection, query non-disclosure, and the v1 default. Full standalone race/vet, pinned vulnerability, formatting/diff, and both sandbox-required headed v1/v2 loopbacks pass; existing UWS and OpenUdon consumer suites passed during M27 qualification and no owned interface was removed. Review iteration 1 found one P2 worker-lifecycle gap: the public `Run` entry rejected an unsupported protocol before closing its caller-owned input. The exact commit closes and tests that path. Review iteration 2 covers the full A09 delta, request/redirect/resource behavior, private lifecycle, cancellation, CLI compatibility, no-submit surface, and docs with no remaining P1/P2 or higher finding. A09 is complete locally and unpushed; publication remains unauthorized.

Dependencies: Browsertools M27. OpenUdon M78/A21/E11 consume A09 only after
its exact reviewed commit is independently resolvable under the plan's
publication gate.

A09 is complete and published at Browsertools
`d26f2982db352619d7a7f6563add802b56e10824`
(`v0.0.0-20260826163208-d26f2982db35`). OpenUdon E11 passed and evolution
result v16 is published in Tofu commit `95752f6`. No W8M target access, account
action, submit, workflow execution, or deployment occurred.
````
