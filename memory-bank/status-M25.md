# Status M25 - Author-Session Containment Remediation

## Goal

Close the reviewed frame-disclosure, MFA-compatibility, and interactive
lifecycle gaps without changing the strict author-session v2 wire.

## State

Complete. The authoring boundary preserves narrow MFA compatibility, reduces
frame identities consistently, and excludes human waits from active browser
time.

| Item | State | Notes |
|---|---|---|
| Frame-name disclosure boundary | `[+]` | Playwright capture and the protocol server independently require exact canonical `ReduceAccessibilityLabel` output for non-empty frame names. |
| Exact MFA compatibility subsets | `[+]` | Candidate facts can carry a unique same-family subset; checkpoints reject cross-family, duplicate, or non-MFA inventories. |
| Active-operation timeout | `[+]` | Open, observe, focus, execute, and completion re-observation share the finite active budget; operator waits are excluded. |
| Regression coverage | `[+]` | Unsafe frame names, narrowed OTP/WebAuthn families, invalid challenge inventories, and human-idle behavior are covered. |
| Acceptance gates | `[+]` | Focused tests, standalone tests, vet, formatting, and diff checks pass. |

Implementation commit: Browsertools `6c3deeb`.
