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
