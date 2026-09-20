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
