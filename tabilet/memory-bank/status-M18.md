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
