# Retired milestone A07 - Secret-Safe Browser Registration Profile Tooling

**Milestone.** A07
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A07.md
**Source specification.** tabilet/memory-bank/milestone.md#a07-secret-safe-browser-registration-profile-tooling
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A07 Secret-Safe Browser Registration Profile Tooling

**Goal.** Add the offline producer-side tooling needed to create, validate,
and review the separate UWS browser-registration 1.0 contract without moving
account creation, credentials, human verification, or runtime execution into
Browsertools.

Dependencies: UWS browser-registration/profile-call 1.0 at commit `a539de1`
and the existing Browsertools authentication profile lifecycle primitives.

Acceptance:

- Typed registration profile loading validates through UWS, rejects multiple
  documents, inline secret/PII-shaped material, unsafe final symlinks, expired
  recipes, and future discriminators; canonical digest, expiry, sorted flow,
  and exact-origin helpers remain deterministic.
- An explicit draft spec inserts only the fixed registration 1.0 discriminator
  and infers no locator, credential slot, effect, checkpoint, submission, or
  success condition.
- A digest-bound review bundle copies the exact profile, records a fixed
  assessment time and expiry, and fails promotion after expiry without
  refreshing authoring evidence or claiming that an account was created.
- Offline `registration-profile validate`, `registration-draft build`, and
  `registration-review bundle` commands preserve current file/stdin/output,
  strict decoding, exit-code, deterministic serialization, and help behavior.
- Synthetic provider-free tests cover valid JSON/YAML, symbolic credentials,
  exact origins, required mutation confirmation, exactly one submit, fixed
  human checkpoints, duplicate/ambiguity/cleanup call validation through UWS,
  secret/PII rejection, lifecycle, digest tamper, CLI routing, and absence of
  browser/network activity.
- Product, architecture, tech-stack, README, and status documentation keep
  live registration, credential resolution, CAPTCHA/MFA/email handling,
  account selection, runtime approvals, account cleanup, registry publication,
  and production execution outside Browsertools.
- Coordinated workspace tests pass against UWS commit `a539de1`; a public
  module pin advances only after that UWS commit is published, and no local
  replace enters the committed module.

Status: complete in Browsertools commit
`c37744a1a546b3a1a078ae777f3ce6e6cc611b56`; see
[status-A07.md](status-A07.md).

````

## Status record

````markdown
# A07 Secret-Safe Browser Registration Profile Tooling

Item | State | Notes
--- | --- | ---
A07.1 Offline registration profile lifecycle | `[+]` | Commit `c37744a1a546b3a1a078ae777f3ce6e6cc611b56` adds the UWS-backed typed profile, deterministic explicit draft, metadata- and digest-bound review, offline CLI commands, synthetic tests, shared pre-schema secret/PII decoding, synchronized public/memory documentation, and the published UWS pseudo-version pin without browser or network execution.

This milestone does not authorize or perform account registration, credential
resolution, human verification, runtime approval, cleanup, or target access.

Workspace and `GOWORK=off` full tests/vet, full race tests, pinned
`govulncheck v1.6.0`, formatting, and diff checks pass. The module resolves UWS
`v0.0.0-20260825173657-a539de17eea3`; no local replacement is committed. The
review bundle rejects profile, assessment-time, expiry, promotability, and gap
tampering and never claims an account attempt or result.
````
