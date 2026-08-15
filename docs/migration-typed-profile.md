# Typed Profile and Promotion Migration

Browsertools is pre-1.0. The typed-profile safety redesign intentionally
replaces the earlier map-oriented authoring API rather than retaining unsafe
compatibility shims.

## API Changes

| Previous API | Replacement |
|---|---|
| `profile.LoadProfileFile` | `profile.LoadFile`, returning the complete typed `profile.Profile` |
| Minimal `profile.Action` | Typed parameters, sequence union, outputs, side effects, and confirmation policy |
| `draft.Options` / `draft.ProfileInfo` | `draft.Spec` with one explicit `draft.ActionSpec` per action |
| `draft.Result.Draft map[string]any` | `draft.Result.Profile *profile.Profile` |
| Automatic `navigate: "/"`, click, and `read_only` defaults | Explicit sequence and side-effect intent; missing intent blocks drafting |
| `review.Build(profileMap, records)` | `review.Build(profile, records, decisions, assessedAt)` |
| Manual `Validation.Valid && len(Gaps)==0` checks | `Bundle.Promotable()` followed by `review.Verify(...)` at handoff time |
| `revalidate.Check(profileMap, records)` | `revalidate.CheckAt(profile, records, decisions, assessedAt)` |
| `LiveRevalidator` stub | Removed; Browsertools revalidation is fixture-only |

`profile.Validate` remains available for JSON-compatible values, but now adds
engine-neutral semantic checks such as literal navigation origin validation.

## Draft Specifications

Evidence can justify a locator or output, but cannot establish whether the
intended action is a click, text entry, state change, or read. A `draft.Spec`
therefore declares each action's parameters, macro sequence, side effects, and
confirmation policy explicitly. Ambiguous locator evidence additionally needs
an `evidence.LocatorDecision` with a rationale.

## Bundle Handoff

Review bundles now carry canonical profile and evidence SHA-256 digests,
fixture revalidation results, assessment time, and locator decisions. A bundle
that was promotable when created can later fail `review.Verify` because the
profile/evidence changed or the profile expired. Deployment and package
handoff code must call `review.Verify` with the exact profile and evidence.

## Runtime Boundary

The redesign does not add live browser execution. Playwright engines, browser
contexts, credentials, cookies, retries, and production side effects remain
downstream runtime responsibilities.
