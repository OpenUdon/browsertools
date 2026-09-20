# Status M22 - Authenticated Authoring Qualification Hardening

## Goal

Close the cross-repository review findings that could make a reviewed live
authoring result stale, incomplete, or impossible to replay, and replace
component-local confidence with exact producer/consumer/runtime seam tests.

## State

Completed.

## Dependencies

- Browsertools A03, E05, and P04.
- UWS 1.8 browser-context contracts.
- Browserdriver M03, Udon M29, and OpenUdon A04/E02.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Harden live author-session authority and evidence freshness | `[+]` | Candidate authority is global to one observation generation, Playwright re-resolves the reviewed semantic tuple before focus/click, observations publish complete contexts, phases/proofs fail closed, frames/popups are reconciled, and completed-request body sizes enforce the byte budget. |
| Synthesize a replayable authentication/capability split | `[+]` | Authentication success is fixed at the first reviewed dashboard observation; the capability candidate preserves every approved exploration navigation/click and ends in its own wait/presence assertion. |
| Qualify the exact downstream seams and reconcile records | `[+]` | OpenUdon consumes and atomically stages a real Browsertools envelope, Udon replays the producer's authentication 1.1/browser 1.5 pair through driver v3, feature skips are removed, and the named matrix passes. |

## Acceptance

- [x] Candidate authority is scoped to one observation generation and is
  revalidated against current accessible semantics immediately before focus or
  click.
- [x] Context inventory and post-popup active context are explicit protocol
  facts; missing, detached, changed, duplicate, or ambiguous contexts fail
  closed.
- [x] Completion evidence is current, phase transitions are closed, and no
  authoring action can run after completion.
- [x] Actual response sizes, including chunked responses without
  `Content-Length`, count against the finite byte budget.
- [x] Authentication success proves the dashboard boundary, while the
  capability candidate replays the complete reviewed exploration trace and
  terminates in the typed goal assertion.
- [x] A real Browsertools-produced envelope crosses OpenUdon validation, and
  its exact authentication/capability versions cross Udon into Browserdriver.
- [x] Workspace and standalone gates use dependency revisions that know the
  released UWS 1.8 contracts; no compatibility assertion is skipped.

Verification completed with full Browsertools tests/vet, standalone
`GOWORK=off` tests/vet against UWS `c9665a61adfb`, Browsertools qualification
commit `53a1502b75b9`, the OpenUdon 11-gate browser integration matrix, and
cross-repository release/diff checks. Both revisions are published and exactly
pinned. Fresh-cache resolution from their ordinary GitHub module origins passed
with global/system Git configuration disabled; no URL mapping or local
replacement is required.

Post-publication CI was also reconciled in Browsertools `af61106`: local-source
discovery tests use the repository's checked-in byte-identical profile fixture
instead of assuming a sibling UWS checkout, and the workflow uses Node 24-based
action generations. Full tests/vet pass from a standalone clone with no sibling
repository, and hosted run `31977203294` passed every required step.
