# Retired milestone M24 - Review Remediation And Consistency Hardening

**Milestone.** M24
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-M24.md
**Source specification.** tabilet/memory-bank/milestone.md#m24-review-remediation-and-consistency-hardening
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** legacy
**Review iterations.** not recorded
**Legacy closure.** The milestone predates the persisted review gate; its original closure evidence is the literal status record, and this retirement does not re-review the original change.
**Verification.** Recorded checks from the original closure (not re-run at this evidence commit): No separate verification or acceptance section was recorded; the literal status record is the closure evidence.
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### M24 Review Remediation And Consistency Hardening

**Goal.** Close confirmed correctness, evidence-boundary, resource, network,
artifact, CLI, and documentation review findings while preserving existing
command names and portable artifact versions.

Acceptance:

- Profile templates cannot bypass origin checks; nested step/wait decoders are
  strict; browser 1.5-1.7 dispatch through pinned UWS schema APIs.
- Adapter output provenance is honest, emitted origins use the accepted
  canonical spelling, unbound hints cannot auto-promote, and guided
  declarations remain explicit operator authority.
- Bundle size/text bounds precede one secret scan; cache references retain
  expiry; cache leases recover through nonce-owned standard-library leases
  whose in-lease commit sources fence expired writers.
- Authoring and registry byte/address policies fail closed, including reserved
  IPv4 and unsafe 6to4/Teredo embeddings; loopback is the only CLI host opt-in.
- Overlay bindings, production encodings, equality helpers, and public
  cache/bundle/registry errors retain typed integrity and policy outcomes.
- Structured command registration provides top/group help and rejects invalid
  or non-exact formats/enums before I/O, network, cache mutation, or browser
  startup.
- Project, product, architecture, tech-stack, milestone, historical evolution,
  and permanent status records are synchronized.

Status: complete; see [status-M24.md](status-M24.md).

````

## Status record

````markdown
# Status M24 - Review Remediation And Consistency Hardening

## Goal

Close the cross-cutting correctness, evidence-boundary, resource, network,
artifact, CLI, and documentation findings without changing existing command
names or portable artifact versions.

## State

Complete. All repository, security, and cross-repository acceptance gates pass.

| Item | State | Notes |
|---|---|---|
| Fail-closed profile templates and strict nested decoding | `[+]` | Templates are substituted only in path/query positions under provable origins; malformed/dynamic authorities and nested unknown JSON/YAML fields fail closed. |
| Typed profile marshal and version consistency | `[+]` | Empty validation schemas and empty navigate payloads retain presence; browser 1.5-1.7 use pinned UWS schema APIs and future discriminators fail explicitly. |
| Honest normalized output evidence | `[+]` | Output invariants, deep locator copies, unbound extraction hints, explicit guided declarations, and canonical origin comparisons/emission are enforced. |
| Shared strict adapter boundary | `[+]` | Four adapters share bounded unknown-field/trailing/EOF decoding and conformance coverage while retaining fixture-specific cardinality limits. |
| Cache expiry and crash-recoverable leases | `[+]` | Nonzero assessment clocks, exact expiry, nonce-owned lease directories, stale recovery, in-lease commit fencing, and typed public errors are covered. |
| Bounded capability-bundle validation | `[+]` | Size checks precede one logical secret scan, text fields are bounded, binary companions are decoded once for scanning, and cache-reference expiry is bound. |
| Network and registry policy hardening | `[+]` | A03 declared/completed byte limits, loopback-only opt-in, expanded reserved ranges, 6to4/Teredo checks, typed errors, and propagated comparisons are covered. |
| Overlay and production integrity hardening | `[+]` | Sidecar envelope/mappings/review binding are verified and discarded marshal/parse/comparison failures are propagated. |
| Structured CLI and early validation | `[+]` | Registry-backed top/group/command help and exact-token pre-I/O enum/format validation retain exit-code and command-name compatibility. |
| Post-implementation review regressions | `[+]` | Canonical-equivalent adapter origins emit canonically, whitespace-padded formats fail before dispatch, and stale writers cannot commit after lease recovery. |
| Documentation and historical consistency | `[+]` | Project/memory-bank contracts, M16 sections, v11 prompt, wrapper key, and author-session v1 supersession notes are synchronized. |
| Full acceptance matrix | `[+]` | `GOWORK=off go test ./...`, race, vet, pinned `govulncheck`, `git diff --check`, and the UWS, OpenUdon, and Apitools test suites pass. |
````
