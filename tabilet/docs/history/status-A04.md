# Retired milestone A04 - Reviewed MFA And Typed Output Authoring

**Milestone.** A04
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A04.md
**Source specification.** tabilet/memory-bank/milestone.md#a04-reviewed-mfa-and-typed-output-authoring
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - All eight MFA kinds, missing/incompatible choices, TOTP synthesis, zero/one/ 16/17 output bounds, unsafe/stale/ambiguous/control candidates, locator proof, deterministic order, profile selection, v1 rejection, teardown, and value non-disclosure are covered. - Workspace and standalone tests/vet, UWS validation, CLI tests, and `git diff --check` pass. - The coordinated OpenUdon matrix passed 11 required gates with 3 unrequested installed/headed browser opt-ins skipped.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A04 Reviewed MFA And Typed Output Authoring

**Goal.** Record the exact human-exercised MFA path and promote a bounded set
of reviewed accessibility outputs without guessing or exposing runtime values.

Dependencies: UWS 1.9/browser 1.7, Browserdriver M05, Udon M32, and OpenUdon
A06.

Acceptance:

- Author-session v2 is the only accepted live wire. Human input completion is
  a distinct state; credential checkpoints accept no challenge kind and MFA
  checkpoints require one exact compatible human choice.
- TOTP synthesizes a deterministic `totp_seed` slot. Other OTP kinds use the
  reviewed locator and human channel; push/WebAuthn kinds carry no locator or
  slot.
- Completion accepts zero through 16 current-final-observation outputs with
  safe unique keys, scalar/presence types, and exact-name or action-time
  unique-role proof. Stale, ambiguous, marker, form-control, secret-shaped,
  duplicate, or seventeenth selections fail closed.
- `goal_present` is retained, reviewed outputs are sorted, and result
  `outputSelections` contain proofs but no page values.
- Integer, number, and Boolean text selects browser 1.7/UWS 1.9; other output
  and context combinations keep the oldest sufficient browser 1.5/1.6
  profile. Authentication remains 1.1.
- Workspace/standalone gates and the clean cross-repository matrix pass with
  public dependency resolution.

Status: complete in Browsertools commit
`dd89956d02203a5c02aa4a7d13ac1e4fe040da05`; see
[status-A04.md](status-A04.md).

````

## Status record

````markdown
# Status A04 - Reviewed MFA And Typed Output Authoring

## Goal

Make the exercised MFA kind and final accessibility outputs explicit,
human-reviewed author-session v2 inputs instead of inferred authoring facts.

## State

Complete.

## Dependencies

- UWS 1.9/browser 1.7 commit `dd9eb32105131bdbc2855090ae0639b22d12de2b`
  (`v0.0.0-20260817013720-dd9eb3210513`).
- Browserdriver M05 commit `4df3c0f83f66cf9fc15f81170d31d355c9e341c2`.
- Udon M32 implementation commit `d4d47f1c59d78091717ba01135db4f9effec7fe1`
  and coordinated seam fix `f1fac6713396f4fda33547ea3fcb296166c7f63b`.
- OpenUdon A06 commit `6a08b81317a852b9b8581c502eadbbdf591508e1`.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Implement, publish, and qualify author-session v2 | `[+]` | Browsertools commit `dd89956d02203a5c02aa4a7d13ac1e4fe040da05` (`v0.0.0-20260817022912-dd89956d0220`) rejects protocol v1, makes human input completion a distinct state, records one compatible human-selected MFA kind, synthesizes deterministic TOTP slots, and accepts zero through 16 reviewed current-generation outputs with safe keys, scalar/presence types, exact-name or action-time unique-role proof, deterministic ordering, and value-free result selections. It emits browser 1.7 only for integer/number/Boolean accessibility text and otherwise preserves the oldest sufficient profile. |

## Verification

- All eight MFA kinds, missing/incompatible choices, TOTP synthesis, zero/one/
  16/17 output bounds, unsafe/stale/ambiguous/control candidates, locator proof,
  deterministic order, profile selection, v1 rejection, teardown, and value
  non-disclosure are covered.
- Workspace and standalone tests/vet, UWS validation, CLI tests, and
  `git diff --check` pass.
- The coordinated OpenUdon matrix passed 11 required gates with 3 unrequested
  installed/headed browser opt-ins skipped.
````
