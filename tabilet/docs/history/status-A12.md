# Retired milestone A12 - Generic registration 1.1 producer

**Milestone.** A12
**Outcome.** completed
**Retired.** 2026-10-18
**Source status.** tabilet/memory-bank/status-A12.md
**Source specification.** tabilet/memory-bank/milestone.md#a12-generic-registration-11-producer
**Evidence.** 9ba44d5896e5ff265b6eb926306e1591c375f7d5
**Worktree.** includes uncommitted changes
**Review.** passed
**Review iterations.** 6
**Verification.** Review iteration 6 passed with no remaining P1/P2. Recorded checks: The full Browsertools browser-free suite passes after v3 producer integration. The real installed, sandbox-required Chromium v3 matrix passes both the conditional two-step wizard and the rejected preview-triggered POST; the fixture server receives zero mutations. The actual producer result passes its independent verifier. This is synthetic evidence only. Review iteration 1 identifies three P2 gaps in the new boundary: new control metadata needs deep-copy protection in candidate/result accessors, the final serialized protocol frame needs its own byte bound, and v3 credential macros must bind the observed credential control kind. These are being corrected before publication; the milestone remains open. The iteration-1 corrections pass the full offline suite, vet and the focused session/candidate/result/capture race suite. Published UWS source checks pass. Review iteration 2 identifies a ...
**Consolidated into.** No current-truth change; accepted behavior is recorded in [product.md](../../memory-bank/product.md), [architecture.md](../../memory-bank/architecture.md), [tech-stack.md](../../memory-bank/tech-stack.md), [lessons.md](../../memory-bank/lessons.md), and the embedded record.

## Milestone specification

````markdown
### A12 Generic Registration 1.1 Producer

**Goal.** Extend the existing no-submit registration producer with the
published UWS registration 1.1 typed-input contract, richer reduced
observations, an explicit bounded public-option preview, and additive
author-session/result v3 contracts, without a crawler, target-specific logic,
live target authority, or private input resolution.

Dependencies: published UWS `9ff877ebce55`, and completed A09 through A11.

Acceptance:

- Registration 1.1 profile, draft, and review helpers ship with unchanged 1.0
  defaults, so existing v1 and v2 producers keep their exact semantics.
- v3 observations and the bounded preview carry only reduced public metadata.
  Filled values, credentials, and raw pages stay excluded, and preview cannot
  perform consent, verification, or submission.
- The private producer result v3 is independently verifiable, and worker and
  CLI integration select v3 deliberately rather than by inference.
- The full offline suite, vet, focused race checks, published UWS source
  checks, and the native positive and adverse Chromium matrix pass.
- OpenUdon consumer qualification completes against the published pin.

Downstream impact: A12 feeds Udon M39, Browserdriver M11, OpenUdon A29, and
W8M W15 qualification.

Status: complete and published at `ec0b9e9d6ca1`; A12.4 passes the complete
three-unit W8M W15 consumer gate and exact tested-runtime adoption. Final
review iteration 6 has no P1/P2 findings. See [status-A12.md](status-A12.md).


## Completed generic registration delivery

[A12](status-A12.md) completes the owner-approved registration 1.1 producer:
existing UWS typed-input contracts, richer reduced observations, explicit
bounded public-option preview, and additive author-session/result v3. A12
is qualified through Udon M39, Browserdriver M11, OpenUdon A29 and W8M W15.
The complete three-unit consumer gate and exact tested-runtime adoption pass.
Existing v1/v2 producers retain their semantics. Synthetic qualification and
focused publication are included in the approved implementation plan; no live
target authority or private input resolution is granted to Browsertools.

This file owns Browsertools milestone sequencing, acceptance criteria, current
state, and the status-file index.

````

## Status record

````markdown
# A12 — Generic registration 1.1 producer

| Item | State | Notes |
| --- | --- | --- |
| A12.1 Adopt published registration 1.1 profile helpers | `[+]` | Published UWS 9ff877ebce55 pinned; profile/draft/review v2 implemented with unchanged 1.0 defaults. |
| A12.2 Add v3 observations and bounded preview | `[+]` | Public metadata, explicit candidate-bound preview, ordered history, evidence validation and bounded frames implemented; native mutation denial passes. |
| A12.3 Integrate private producer and suggestions | `[+]` | Independently verified result v3, deterministic unapproved suggestions and worker/CLI integration implemented. |
| A12.4 Qualify, review and publish | `[+]` | Published producer passes owner checks and complete three-unit OpenUdon/W8M consumer qualification. Exact tested runtime is adopted; final review 6 has no P1/P2 findings. |

The approved implementation extends the existing producer rather than adding
a crawler or target-specific logic. UWS schemas stay unchanged. Native output
contains only reduced public metadata; filled values, credentials and raw pages
remain excluded. Preview cannot perform consent, verification or submission.

## Implementation and review history

Review iteration 0; no completion or live evidence claimed. Maximum 10 review
iterations, with no P1/P2 finding before closure. Source baseline is
ce06b13bfef8d1776c3aa019322619c90dacbbd2; canonical harness lives in Tofu.

## Implementation verification and review

The full Browsertools browser-free suite passes after v3 producer integration.
The real installed, sandbox-required Chromium v3 matrix passes both the
conditional two-step wizard and the rejected preview-triggered POST; the
fixture server receives zero mutations. The actual producer result passes its
independent verifier. This is synthetic evidence only.

Review iteration 1 identifies three P2 gaps in the new boundary: new control
metadata needs deep-copy protection in candidate/result accessors, the final
serialized protocol frame needs its own byte bound, and v3 credential macros
must bind the observed credential control kind. These are being corrected
before publication; the milestone remains open.

The iteration-1 corrections pass the full offline suite, vet and the focused
session/candidate/result/capture race suite. Published UWS source checks pass.
Review iteration 2 identifies a P2 guidance/validation gap: known consent or
verification labels must not become ordinary private-input suggestions or
`fill_input` evidence. They must stay human checkpoints. Correcting this before
consumer adoption; legacy protocols and historical evidence remain unchanged.

Review iteration 2 corrections pass the full suite, vet, focused race and native
v3 positive/adverse matrix. Review iteration 3 traces metadata, preview,
history/result reconstruction and legacy version refusal with no remaining
P1/P2 finding in producer scope. Source commits follow rows A12.1–A12.3 in
order; shared synthetic fixture tests accompany A12.2. Full published-pin
OpenUdon consumer qualification is the remaining A12.4 closure gate.

Consumer review iteration 4 finds one remaining P2 alias: the v3 builder's
initial observation copy decodes into an already populated slice. Reuse the
existing deep-copy helper so a caller's later history edit cannot change the
candidate's observation. The source pin will advance before final adoption.

The iteration-4 deep-copy correction is published at ec0b9e9d6ca1. Repeated full
tests and vet pass. The consumer against published dependencies passes actual typed iCoT,
BRP/UWS, private form and v5 replay passes, including retained-artifact privacy
checks. Review iteration 5 finds no remaining P1/P2 in producer scope. Final
three-unit consumer qualification remains tracked by W8M W15.

Final consumer evidence: W8M W15's fresh fourth aggregate passes three complete
units, including 117 native stages and nine synthetic workflow receipts. The
original native reports and aggregate pass owner verification. Independent
recomputation verifies twenty source bindings and the same eight actual
runtime/module hashes across all three units; W15 adopts exact tested bytes.
Aggregate SHA-256: `49dc9e251c86eb4859eb12b088965b5bfe620419c68ee72f1b58aefae75c3b47`.
Qualified source heads remain Browsertools `ec0b9e9d6ca1`, OpenUdon
`1f8b08f150f9`, Udon `ad257817e5fd`, Browserdriver `22eb8f1b5e3d` and unchanged
UWS `9ff877ebce55`. Later coordination commits do not repin that qualified
closure. Failed predecessor aggregates remain failures; no live W8M operation
or account outcome is claimed. The existing evolution direction is unchanged.

Review iteration 6 confirms the complete consumer evidence and unchanged
producer boundaries with no P1/P2 findings. A12 is complete.
````
