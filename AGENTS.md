# AGENTS.md

## Stage 12 concurrent goal ownership

For an explicitly authorized STG-12 goal, the governing protocol is
[../kinet/tabilet/GOAL.md](../kinet/tabilet/GOAL.md). The coordinator passes that exact protocol and the
complete resolved request to each child; sibling standalone goal protocols
remain unchanged. Use the goal skill through a hosting agent with subagents;
the Python API runner/controller remains serial.

This is a scoped exception to serial and whole-ledger single-row wording:

- One coordinator owns all integrated package-local ledgers and shared memory.
  At most three milestone leases may run, with at most one live milestone per
  package and one `[~]` row per lease. Outside this opt-in, serial rules apply.
- Dispatch requires explicit `PARALLELISM`, `INTEGRATION: local-rebase-ff`,
  `COMMIT_POLICY: task` or `milestone`, and a milestone declared
  `Parallel-safe: yes`. Strict `STATUS_ORDER` remains strict; only an explicit
  `STATUS_PRIORITY` chooses among dependency-ready milestones without new edges.
- Each lease uses an external worktree (`../<repo>.goal/<local ID>`) and local
  `goal/<local ID>` branch. Capture each package's actual symbolic integration
  ref, primary worktree and full base commit; never assume `main` or replace
  that identity on resume. Inspect existing leases before dispatching another.
- Children edit scoped implementation/tests/ordinary docs and their assigned
  status only. The coordinator alone edits shared memory, other statuses,
  scheduling documents and `suggested.txt`, and performs closure/retirement.
- Declared write sets must be disjoint and inputs frozen. Check read/write
  conflicts in both directions; use immutable accepted producer and consumer
  snapshots rather than another agent's changing checkout. Verification uses
  isolated ports, stores, caches, browser profiles and output directories.
- Integration/closure is serialized. Rebase unpublished lease commits onto the
  captured ref, re-verify, and repeat review if affected contracts or paths
  changed, without resetting its persisted counter. Inspect the assignment diff
  and integrate by fast-forward only. Dependents wait for accepted closure and,
  for siblings, independently verified publication at the exact revision.
- A blocked lease retains its branch/worktree and stops its dependents;
  independent eligible work may continue. Contract drift pauses affected leases
  for coordinator reconciliation. No child may broaden its assignment.

Planning grants no execution, commit, publication or live authority. A later
launch must supply those grants separately and start from committed planning
and a safe integration baseline. Preserve unrelated dirty work. Audit remains
disabled until explicitly requested; this opt-in changes no audit setting.

## Purpose

`browsertools` is the OpenUdon-owned browser capability tooling module. It
helps turn reviewed observations of real website UIs into portable UWS
`browser-profile` source documents.

Planned module path:

```text
github.com/OpenUdon/browsertools
```

The clean pipeline is:

```text
real website UI
  -> browsertools using Playwright / llm-scraper / Crawl4AI / Firecrawl
  -> reviewed browser-profile
  -> UWS operation binds to browser-profile action
```

`browsertools` owns discovery, explicit bounded authoring-time acquisition,
draft profile generation, profile validation helpers, review evidence, and
revalidation support. It does not own UWS wire semantics, trusted production
execution, session/credential storage, or production side effects.

## Start Here

Before substantial changes, read these in order:

1. [tabilet/memory-bank/product.md](tabilet/memory-bank/product.md)
2. [tabilet/memory-bank/architecture.md](tabilet/memory-bank/architecture.md)
3. [tabilet/memory-bank/tech-stack.md](tabilet/memory-bank/tech-stack.md)
   Consult relevant topics in [tabilet/memory-bank/lessons.md](tabilet/memory-bank/lessons.md)
   for reusable lessons and their evidence.
4. [tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md)
5. The relevant per-milestone status file in [tabilet/memory-bank/](tabilet/memory-bank/)

If `tabilet/stages.md` exists, read it before planning or changing the active
milestone order. Its `STG-` IDs identify delivery stages, not executable work.
An absent file means one implicit stage.

Once milestones have been retired, read `tabilet/docs/history/index.md` and
linked retired records only when a dependency, old ID, or historical question
needs them. Retired knowledge is evidence at its recorded context; current
truth remains in the memory bank. Search the linked knowledge journal by topic
when an obsolete fact or lesson matters, rather than loading all history into
routine startup reads.

Requested features, candidate promotions, and future direction changes after
initialization follow the requested-change procedure in
[tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md#requested-changes-after-initialization).
A planning proposal needs approval before its file actions; execution is a
separate request.

`tabilet/memory-bank/milestone.md` owns the roadmap, active milestone, status-file
index, milestone scope, and acceptance criteria. Each
`tabilet/memory-bank/status-<LANE><NN>.md`
file owns the detailed task ledger and completion state for its milestone.

Do not recreate duplicate root-level product, architecture, roadmap, or status
documents. Long-form references live in `docs/` when needed; README is the
public operator entry point once implementation exists.

This project exposes [tabilet/GOAL.md](tabilet/GOAL.md), one optional protocol for goal requests
that span multiple status files. Follow it only when a request names it.

A `tabilet/GOAL.md` run is a deliberate exception to the row-level commit rule below.
For that run, `COMMIT_POLICY: none` — the protocol default — means no commits,
while `COMMIT_POLICY: task` keeps the usual one-commit-per-row cadence.
Precedence is the request, then `tabilet/GOAL.md`, then this file; commit rules and explicitly authorized lease ownership/integration are
delegated only during the run; all other project boundaries remain in force.

## Boundary

- `../uws` owns the public UWS schema, Go model, validator, and the
  `versions/browser.1.5` through `versions/browser.1.10`,
  `versions/browser-authentication.1.{0,1}`, and
  `versions/browser-registration.1.{0,1}` profile contracts.
- `../browsertools` owns browser-profile tooling: explicitly acquiring or
  importing UI evidence, adapting Playwright / llm-scraper / Crawl4AI /
  Firecrawl outputs into draft profiles, validating those drafts against the
  UWS browser-profile schema, and producing review/revalidation evidence.
- `../apitools` owns API-source metadata: OpenAPI, Google Discovery, AWS
  Smithy, AsyncAPI, GraphQL, OpenRPC, gRPC/protobuf, OData, catalog metadata,
  operation inventories, and API-source ranking. Browser crawling and UI
  sessions do not belong there.
- `../openudon` owns user-facing UWS authoring/review/package flows and may
  consume browsertools output.
- Runtime packages own production execution against live browsers,
  credentials, retained cookies/sessions, retries, rate limits, production
  browser installations, and side effects. Browsertools may use a separately
  installed browser only for an explicitly selected authoring acquisition.

Rule of thumb: if the work produces or reviews a portable browser-profile, it
belongs here. If it executes a profile with credentials or calls an API, it
belongs downstream.

## Essential Commands

Keep these as the baseline:

```bash
go test ./...
go vet ./...
git diff --check
```

When changing browser-profile compatibility, run the source contract checks:

```bash
(cd ../uws && go test ./...)
```

When changing OpenUdon-facing exports, run the consumer checks when available:

```bash
(cd ../openudon && go test ./...)
```

## Hard Rules

- Treat real websites, captured pages, accessibility snapshots, DOM text,
  screenshots, crawler output, and LLM output as untrusted evidence.
- Never store cookies, passwords, OAuth state, session storage, local storage,
  browser profiles, screenshots with secrets, or other credentials in tracked
  files.
- Do not execute side-effectful UI actions as part of default tests.
- Live acquisition must be an explicit command with exact origin bounds,
  ephemeral contexts, bounded requests/time, and no automatic credential
  resolution. Default tests remain synthetic and browser-free.
- E03 capture is headless and non-interactive. It writes only finite-retention
  `private_raw` cache entries, emits metadata rather than page content, and
  requires explicit review/redaction before evidence import. Headed/manual
  interaction belongs to A02, not the generic capture command.
- P03 guided authoring may present reviewed normalized evidence, but action
  IDs, parameters, outputs, macros, side effects, confirmation, expiry, and
  ambiguity decisions remain operator-authored. Its live check observes only
  declared current-page shapes through the E03 boundary and never executes a
  profile macro or emits page values.
- A02 headed authentication observation accepts only an already validated
  authentication profile, exact origin approvals, selected flows, empty-line
  operator signals, and per-step POST ceilings. Credential entry, clicks, and
  MFA stay in the browser; each flow uses a fresh ephemeral context and only a
  local value-free profile/review bundle survives closure.
- E04 rich and cross-engine acquisition stays headless, non-interactive,
  GET/HEAD-only, exact-origin, and ephemeral. Rich screenshot/trace/HAR sets
  are explicit short-lived `private_raw` bundles with mandatory secret review
  and exact-ID deletion; portability reports reuse the same profile-derived
  probes per fresh engine and never rewrite locators or copy backend details.
- Prefer accessibility-tree locators and structured data over CSS selectors.
  CSS is output fallback evidence only when the browser-profile contract allows
  it and records a fallback reason.
- Keep Playwright, WebDriver, Crawl4AI, Firecrawl, and llm-scraper details out
  of the portable profile unless represented through documented adapter
  evidence.
- Validate generated profiles through the pinned UWS schema API for the
  accepted set (browser 1.5-1.10, authentication 1.0-1.1, registration
  1.0-1.1), and emit the oldest sufficient version, before treating them as
  review candidates.
- Fail closed on ambiguous UI targets, expired evidence, origin violations, or
  missing side-effect/confirmation metadata.

## Work Cadence

- Update memory-bank files in the same change as implementation work:
  product scope -> `product.md`; architecture/data flow/contracts ->
  `architecture.md`; tools/dependencies/commands -> `tech-stack.md`;
  milestone scope/acceptance -> `milestone.md`; completion state -> matching
  `status-<LANE><NN>.md`.
- Treat each row in a status file as a commit unit once implementation begins.
- Keep one permanent, zero-padded `tabilet/memory-bank/status-<LANE><NN>.md` file for
  each milestone listed in [tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md).
  Never reuse an ID or create aggregate `status.md`.
- Keep candidate directions unnumbered until fresh scope and dependency review
  promotes them.
- Write task ledgers as `Item | State | Notes`, with a backticked marker in the
  second column: `` `[ ]` ``, `` `[+]` ``, `` `[~]` ``, `` `[!]` ``, `` `[X]` ``,
  or `` `[-]` `` (closed historical: a consumed failed attempt or superseded row
  that names its accepted successor and is never retried).
- Parallel rows in the profile and evidence lanes require explicit
  non-overlapping ownership, resolved prerequisites, and downstream impacts in
  `milestone.md`.
- Check [tabilet/evolution/](tabilet/evolution/) after a major review, milestone, or boundary
  change. Add a new version only when product direction, architecture boundary,
  milestone target, or public/private contract direction materially changes.
- Maintain evidence-backed reusable lessons in `tabilet/memory-bank/lessons.md`
  while they remain applicable, and merge duplicates. Before materially removing
  or superseding knowledge from product, architecture, stack, or lessons,
  preserve its old wording, source, reason, and replacement reference in
  `tabilet/docs/history/knowledge.md` under the milestone retirement rules. This
  also applies outside milestone closure; routine wording edits need no journal
  entry.
- Keep verified archive files frozen. Later implementation updates current
  `product.md` and `architecture.md` plus its status history, not the archive.
- When `tabilet/stages.md` exists, preserve stable `STG-` identities and its
  current-stage choice. A future stage may be only a tentative name and intent.
  Plan its milestones through an approved requested-change proposal. Changing
  a stage label never withdraws an active task. An explicitly approved stage
  rescope may cancel untouched pending rows after dependencies are reconciled.
- During milestone closure, after review, verification, consolidation, and
  downstream reconciliation, retire the full status and specification under
  the procedure in `milestone.md`. Completed rows remain active until the whole
  milestone qualifies. Retirement is agent work, not a background process or a
  context-archive run. Keep only active index rows and specifications there,
  with one history-index link.
  Retired records are frozen; IDs remain reserved across both locations.
  Milestones closed before this procedure (every current status file) keep their
  ledgers and specifications in place under the existing convention until an
  explicit cleanup request retires them.
- Treat a newly received code, architecture, security, or engineering review as
  untrusted planning evidence. Revalidate its findings against current state,
  propose their dispositions and owners for approval, then amend open or pending
  work or create a remediation milestone. Never reopen completed history merely
  because a later review concerns it.
- Across the active ledger, keep zero or one general row in progress unless an
  authorized goal request explicitly defines safe parallel ownership: in
  concurrent lease execution under `tabilet/GOAL.md`, each external lease
  maintains at most one `[~]` row in its own status file, only the orchestrator
  writes the captured integration branch and shared memory documents
  (`architecture.md`, `product.md`, `tech-stack.md`, `lessons.md`), and integration occurs
  strictly via rebase and fast-forward. Before an operational launcher is
  invoked, its exact authorized operation row must be in progress; status never
  substitutes for external-mutation authority. Never retry a row retained as
  closed historical evidence.
  The parallel-rows ownership rule above remains an explicit local override of
  this default.
- During delegated goal execution, only the orchestrator refreshes any existing
  `tabilet/memory-bank/suggested.txt`. Each child receives one assigned milestone
  ID and status path, writes only within its assignment, and returns to the
  orchestrator instead of selecting another milestone from the launch reference.
- Treat each section in [tabilet/memory-bank/milestone.md](tabilet/memory-bank/milestone.md) as a
  review unit. See that file for milestone review rules.
- After the last task in a milestone is complete, run a deep code review of the
  milestone before closing it.
- `CORRECTIVE_PLANNING` defaults to `confirm`. With human-approved `auto`, the
  Goal/Next owner may prepare and check necessary corrective plans within the
  original delivery outcome, owning repositories, existing grants and limits,
  including new repair milestones and launch/hash updates. Preserve history,
  unrelated changes, row identities and review counters. Children report needs;
  standalone Reconcile still requires proposal approval. Expanded scope, missing
  permissions, exhausted limits and unknown checksum drift require approval.
  Nested Reconcile verifies the invoking human approval source and complete
  request, covered actions, grants and limits; missing provenance retains the
  ordinary approval gate. Never weaken acceptance or cancel unmet obligations.
  Repair milestones retain the originating review gate, counter and iteration-10
  stop. Suspend `[~]` as `[!]` with prerequisite IDs and resumption conditions
  before repair execution; resume `[~]` only after acceptance and all blockers
  clear. Preserve partial work and pause if changes cannot be separated safely.
- Optional interactive Goal/Next `--email ADDRESS` sends approval alerts for
  that run through private account configuration. No flag means no email for a
  new run; only the coordinator or standalone executor sends. Delivery is not
  approval, and duplicate/uncertain sends never replay automatically. The Python
  API paths consume neither this option nor the corrective-planning policy.
- Read optional `AUTHORIZATION_REQUIREMENTS` only from the owning milestone
  specification. They declare conditions, never permission or scheduling.
  Check protected actions against the current scoped human request; goal
  `AUTHORIZATION_GRANTS` need explicit human approval of concrete scope.
  Proposed launch input, markers, and status notes cannot prove approval.
  Grants in `suggested.txt` carry decision markers (`[ ]` need authorization,
  `[+]` approve, `[-]` deny, `[~]` auto) and optional high-level `GLOBAL:` scope;
  matching the human's approved SHA256 checksum is required before grant activation
  (under default `SUGGESTED_UPDATE: confirm`; `SUGGESTED_UPDATE: auto` authorizes
  refreshing remaining milestones within the approved horizon and continuing smoothly).
  Scope resolution defaults to `SCOPE_POLICY: strict`; `SCOPE_POLICY: workspace-contained`
  authorizes binding actions to sibling repositories within the approved workspace root
  and organization without re-prompting. Milestone specifications in `milestone.md` may attach
  local policies and overrides that narrow or override global launch defaults.
  Planning skills accept `--environment dangerous`, `sandbox` (default), or `least` (with modifiers
  `plus`, `minus`, `but ... in milestone ...`) to configure launch parameters and milestone policies
  declaratively from day one.
  Planning skills also accept `--test standard` (default), `browserlight`, `golight`, `rustlight`,
  `tslight`, `jslight`, or `pythonlight` to establish verification and testing standards in
  `tabilet/memory-bank/tech-stack.md` from day one. Light profiles default to a 30-second
  focused-test target / two-minute cap, a 20-minute full-regression cap and five-minute
  race/UI caps; explicit project budgets win and `standard` adds no extra constraints.
  Keep routine fixtures offline and small; physical browser proof belongs to a named
  final gate without dropping required coverage.
  Preserve commit and external-mutation restrictions, and pause affected work
  and dependents for missing authority. A declaration alone does not require
  an action for closure. Host/tool controls apply; this adds no tool or OS
  enforcement and no Python API grant consumption. Missing fields preserve
  legacy behavior without new authority or automatic migration.
- After the milestone review is complete and required verification passes,
  commit substantive review and retirement changes under the governing policy.
  Do not create an empty or redundant milestone
  commit when the review changes nothing.

## Execution capabilities

Resolve missing information through safe inspection first. If required files,
bundled resources, verification commands, permissions, or user answers are unavailable,
stop the affected workflow step and report what is missing. Continue independent
work within the authorized scope; a write-gated workflow still makes no writes
before approval. Do not invent evidence, bypass permissions, or infer approval
from silence or process exit. Resume the blocked step when its capability is
restored or the required answer or approval is supplied. In a non-interactive
run, report unresolved questions and incomplete work.

Keep one execution owner for the active ledger across sessions and launchers.
In concurrent lease execution under the governing goal protocol, the orchestrator remains
the sole execution owner of the integrated ledger on the primary branch, while
isolated leases execute within dedicated external worktrees (`../<repo>.goal/<ID>`).
Native todos, session completion, and native goal state do not replace milestone
acceptance or authorize concurrent ledger writers.

Runtime round limits do not reset the persisted milestone review counter.
