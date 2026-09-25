# Status M32 — Browser 1.10 match-count profile support

**State:** Active. M32.1 is complete; M32.2 is pending.

**Goal.** Add the published UWS Browser 1.10 profile to typed validation,
round-trip handling and offline draft authoring.

**Dependency.** UWS M05 is published at `80ee9bfb24a688b5e875dadf9ecacdc65398f1ff` (`v0.0.0-20260925154821-80ee9bfb24a6`); Browsertools pins this exact source.

**Compatibility.** Preserve Browser 1.9 and older profile bytes and existing
oldest-sufficient behavior. No live browser action is included.

## Tasks

| Item | State | Notes |
| --- | --- | --- |
| M32.1 Add Browser 1.10 typed profile validation | `[+]` | Pins UWS `v0.0.0-20260925154821-80ee9bfb24a6`; adds Browser 1.10 schema dispatch and typed `matchCount`, `within`, and `visibility` fields. Focused profile tests, profile vet, pinned UWS tests, and `git diff --check` pass. |
| M32.2 Add round-trip and offline authoring coverage | `[ ]` | Preserve prior profile bytes and reject unsupported or invalid forms without retaining page text or attributes. |
| M32.3 Verify, review and publish | `[ ]` | Run focused/full tests and vet; complete bounded review and publish for Browserdriver M15. |
