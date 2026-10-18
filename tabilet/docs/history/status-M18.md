# Retired milestone M18 - Offline CLI And Digest-Bound Handoff

**Milestone.** M18
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M18.md
**Source specification.** tabilet/memory-bank/milestone.md#m18-offline-cli-and-digest-bound-handoff
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): - [x] The full supported pipeline is callable without a network or browser. - [x] Public handoff examples verify against exact artifacts. - [x] All local and cross-repository gates pass.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M18 Offline CLI And Digest-Bound Handoff

**Goal.** Expose the complete file-first pipeline and migrate public handoff
examples to the redesigned contracts.

Acceptance:

- CLI commands cover profile validation, all adapter imports, strict drafting,
  review bundles, and fixture revalidation.
- CLI supports one stdin input, stdout output, explicit redaction, deterministic
  assessment times, overwrite protection, and documented exit codes.
- OpenUdon and wrapper examples verify against their exact profile/evidence
  digests.
- Public docs, migration guidance, memory bank, and evolution record the break.

````

## Status record

````markdown
# Status M18 - Offline CLI And Digest-Bound Handoff

## Goal

Ship the complete file-only pipeline and migrate public integration examples.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Add offline CLI | `[+]` | Commands cover validation, four adapter imports, drafting, bundle construction, and fixture revalidation. |
| Harden CLI I/O | `[+]` | One stdin input, stdout, explicit redaction, exclusive overwrite protection, deterministic `--at`, and exit codes 0/1/2. |
| Migrate examples | `[+]` | OpenUdon and wrapper bundles now carry exact digests and saved normalized evidence. |
| Update public docs | `[+]` | README, project/integration/wrapper docs, examples, and breaking migration guide use the new contracts. |
| Run final compatibility gates | `[+]` | Package, race, vet, build, diff, UWS, and OpenUdon checks all pass. |

## Acceptance

- [x] The full supported pipeline is callable without a network or browser.
- [x] Public handoff examples verify against exact artifacts.
- [x] All local and cross-repository gates pass.
````
