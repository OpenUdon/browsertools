# A11 Registration Route-Abort Containment Correction

Item | State | Notes
---|---|---
A11.1 Retain denied-route abort failures | `[+]` | `capture/playwright_registration.go` now latches fixed `route_abort` when Playwright cannot abort an already-denied request; allowed-route continuation failures remain fixed `route_continue`.
A11.2 Add direct regression coverage and complete local review | `[+]` | Direct error-branch coverage, full Browsertools tests, vet, focused capture race, formatting, diff, privacy, and bounded re-review pass. No P1/P2 or higher-severity finding remains in the local A11 diff.
A11.3 Publish and hand off exact adoption | `[+]` | Published by normal fast-forward at exact Browsertools tip `ce06b13bfef8d1776c3aa019322619c90dacbbd2`; independent remote-tip reread matched. OpenUdon and W8M adoption remain separate.

## Review provenance

- Source: deep W03/W04 implementation review; source priority and baseline were
  not supplied.
- Local severity: P1.
- Revalidated against published A10
  `3107470313d447e29c5ac5912c3cc9d221d46967` before the local correction.
- Local unpublished implementation commits are
  `b426899debc6ca4dc71a7452ed22b5f9cbd5b3ab` and
  `ce06b13bfef8d1776c3aa019322619c90dacbbd2`.
- No browser, target request, dependency change, commit, or publication was
  part of implementation.

## Verification

- Pinned Go 1.26.6 focused registration policy tests.
- Full tests, vet, focused capture race, formatting, diff, and privacy checks.
- Bounded review of the complete A11 diff and downstream contract surface.

## Bounded review-fix gate

Maximum 10 iterations. No P1/P2 or higher-severity finding may remain before
A11.2 completes.

Review iteration 1 found no additional P1/P2 after the direct abort/continue
error regression, full suite, vet, focused race, and downstream API review.

## Publication

- Browsertools `origin/main` advanced by normal fast-forward from published A10
  `3107470313d447e29c5ac5912c3cc9d221d46967` to A11
  `ce06b13bfef8d1776c3aa019322619c90dacbbd2`.
- The remote tip was independently reread after push. No force push, browser,
  target request, or runtime action occurred.
