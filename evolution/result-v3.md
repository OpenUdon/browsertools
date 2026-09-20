# Result V3 - Typed, Explicit, Digest-Bound Browser Profiles

Browsertools now exposes the complete engine-neutral browser.1.5 model. Drafts
combine normalized evidence with explicit action intent instead of inventing
macros or side-effect safety. Fixture revalidation matches every declared
locator, persists ambiguity decisions, and evaluates origins/expiry/completion
waits at a caller-supplied time.

Review bundles use that same gate and bind the exact profile and evidence with
canonical SHA-256 digests. The offline CLI covers validation, adapter import,
drafting, review, and revalidation. OpenUdon and wrapper examples verify their
saved evidence and digests. Live browser execution, sessions, credentials, and
side effects remain downstream runtime responsibilities.
