# Status M17 - Typed Profile Contract And Safety Primitives

## Goal

Replace the minimal profile view with the complete engine-neutral browser.1.5
contract and deterministic origin/duration safety helpers.

## State

Completed.

## Task Ledger

| Item | State | Notes |
|---|---|---|
| Complete typed profile model | `[+]` | Added strict action, seven-macro, wait, locator, output, origin, side-effect, confirmation, and inline-schema types. |
| Add validated JSON/YAML parsing | `[+]` | `ParseJSON`, `ParseYAML`, and `LoadFile` validate before returning typed profiles; YAML decoding requires EOF after the first document. |
| Add canonical safety primitives | `[+]` | Canonical HTTP(S) origins, literal navigation checks including fail-closed relative targets with multiple origins, and reference-aware ISO durations replace loose/approximate behavior. |
| Add compatibility and union tests | `[+]` | All macros, output sources, origin forms, calendar boundaries, examples, and UWS schema parity are covered. |

## Acceptance

- [x] Browser-profile consumers no longer need raw action maps.
- [x] Closed unions reject invalid macro/wait shapes.
- [x] Literal off-origin navigation fails before runtime handoff.
- [x] No runtime or Playwright dependency was added.
