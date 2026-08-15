# Publishable Browser Capability Bundles

A capability bundle is Browsertools' inert publication unit for one exact,
reviewed browser capability release. It is suitable for a static catalog,
OpenUdon materialization, and runtime verification; it is not executable by
itself and never contains a browser session.

The `browsertools.capability-bundle.v1` wire shape has four boundaries:

1. `payload` contains the stable ID and semantic release, normalized profile,
   exact review artifact, normalized evidence, optional UWS companions, and
   license/provenance.
2. `descriptor` identifies the canonical payload bytes with SHA-256.
3. `assessment` uses `github.com/OpenUdon/evidence/artifact` to bind the
   payload to active/stale/revoked/superseded lifecycle evidence and every
   supporting artifact descriptor.
4. The complete bundle digest identifies the canonical JSON envelope for a
   registry blob.

Bundle construction and verification are offline. Verification re-runs the
UWS browser-profile validator and Browsertools' review/revalidation gates at a
caller-supplied time. It also proves the profile, review, evidence, companion,
cache, payload, and lifecycle digests; requires confirmation for every
mutation; and rejects secret-like values and session material.

## Build and verify

First produce a promotable `review.Bundle`. Then construct the publication
unit from the same profile and normalized evidence:

```go
value, err := bundle.Build(bundle.BuildOptions{
    ID:          "example/status",
    Release:     "1.0.0",
    Source:      "reviewed_synthetic_fixture",
    License:     "CC0-1.0",
    Authors:     []string{"Example Maintainer"},
    Profile:     prof,
    Review:      reviewed,
    Evidence:    records,
    PublishedAt: assessedAt,
})
wire, err := bundle.CanonicalJSON(value, assessedAt)
identity, err := bundle.Digest(value, assessedAt)
```

The equivalent file-only CLI accepts repeatable UWS companions as explicit
`TARGET=PATH` mappings:

```bash
browsertools bundle build \
  --id example/status --release 1.0.0 \
  --profile ./profile.yaml --review ./review.json --evidence ./evidence.json \
  --source reviewed_fixture --license CC0-1.0 \
  --published-at 2026-08-16T00:00:00Z \
  --uws workflow.uws.yaml=./workflow.uws.yaml \
  --out ./capability-bundle.json

browsertools bundle verify \
  --input ./capability-bundle.json --at 2026-08-16T00:00:00Z
```

`bundle build` accepts at most one stdin input and refuses to overwrite without
`--force`. `bundle verify` uses the supplied `--at` time; it never consults the
wall clock.

## Cache promotion boundary

The Go API accepts a `CachedArtifact` only when the caller supplies both the
cache manifest and exact bytes. It rehashes those bytes and calls
`cache.ValidateForPublication`. Consequently, `private_raw`, ineligible,
expired, invented, or tampered cache records cannot become bundle provenance.
Raw captures remain in the explicit private cache.

## Companion boundary

Companions are optional `.uws.json`, `.uws.yaml`, or `.uws.yml` documents with
safe relative target paths. Browsertools validates their UWS semantics, scans
them for inline secrets/session material, preserves their exact bytes, and
binds each descriptor into the assessment. Browser binaries, drivers,
credentials, cookies, local/session storage, screenshots, and raw pages are not
companion types.

Canonical synthetic fixtures live in
`testdata/capability-bundles/{read-only,confirmed-side-effect}.json`. They are
built from UWS' canonical `testdata/browser-profile` profiles and workflows and
are the shared compatibility inputs for Browsertools, Udon, and OpenUdon.
