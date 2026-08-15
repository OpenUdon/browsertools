# Static Browser Capability Registry

Browsertools' registry is a directory of ordinary static files. It has no
accounts, database, upload API, authentication tokens, custom server, or
membership service.

```text
index.json
blobs/
  sha256/
    <64 lowercase hexadecimal characters>
```

`index.json` is the canonical, versioned search index. Each blob is the exact
canonical JSON of one `browsertools.capability-bundle.v1`; its path is derived
only from the complete bundle SHA-256 digest. Blob paths are immutable. An
index entry records ID/release, title, origins, action names/count,
license/provenance, the bundle descriptor, and an Evidence lifecycle
assessment.

## Publish locally

Build and verify a capability bundle before publication, then update an
explicit local catalog checkout:

```bash
browsertools registry publish \
  --root ./public-registry \
  --bundle ./capability-bundle.json \
  --at 2026-08-16T00:00:00Z

browsertools registry verify \
  --location ./public-registry \
  --at 2026-08-16T00:00:00Z
```

Publication takes a local transaction lock, verifies the full bundle, reuses
identical blobs, installs new blobs with exclusive content-addressed paths, and
atomically replaces the index. A conflicting ID/release is rejected.

A new release can supersede an existing release of the same capability:

```bash
browsertools registry publish \
  --root ./public-registry \
  --bundle ./capability-bundle-2.0.0.json \
  --supersedes example/status@1.0.0 \
  --at 2026-08-20T00:00:00Z
```

The old blob remains immutable and addressable; only its index lifecycle
becomes `superseded` with the new digest as successor. The Go
`registry.UpdateLifecycleLocal` API supports reviewed `stale` and `revoked`
transitions. Revoked and superseded states are terminal.

## Contribution and review

A public catalog can use an ordinary repository pull-request workflow:

1. A contributor builds a reviewed capability bundle without raw captures.
2. They run `bundle verify` and `registry publish` against a catalog checkout.
3. They commit only the changed `index.json` and immutable digest blob.
4. CI runs `registry verify`, fixture tests, secret scanning, and repository
   policy checks without network access.
5. Maintainers review profile origins, actions, evidence, license, mutation
   confirmation, expiry, and lifecycle intent before merging.

Repository hosting supplies contributor identity, review, moderation, and
audit history. Browsertools does not reproduce those features.

## Serve static files

Upload or deploy the directory with existing repository automation to any
static HTTPS host, such as GitHub Pages or object storage/CDN hosting. Configure
`index.json` for short cache revalidation and `blobs/sha256/*` as immutable;
the digest path makes long-lived blob caching safe. Deployment credentials stay
in the chosen CI/hosting system, never in Browsertools or a bundle.

Browsertools has no remote publish command. It only writes an explicit local
root. Copying that root to a host is an external, reviewed deployment step.

## Search and pull

Local reads never require network permission:

```bash
browsertools registry search \
  --location ./public-registry --query status \
  --at 2026-08-16T00:00:00Z

browsertools registry pull \
  --location ./public-registry \
  --id example/status --release 1.0.0 \
  --at 2026-08-16T00:00:00Z --out ./bundle.json
```

Remote reads require HTTPS and explicit approval:

```bash
browsertools registry search \
  --location https://catalog.example.org/browser/ \
  --query status --at 2026-08-16T00:00:00Z \
  --network allow
```

The default network policy is `never`; `ask` returns an approval-required
error for a caller such as OpenUdon to surface. Network operations have one
eight-second total deadline, a 20 MiB per-response ceiling, a three-result
default, at most five HTTPS redirects, and DNS/dial checks that reject
localhost, private, link-local, unspecified, and multicast addresses. The
unsafe-host switch is for local TLS test servers only.

Search reads only `index.json`. Pull and verify fetch exact digest paths and
then validate the blob descriptor, complete bundle digest, embedded payload,
review/evidence bindings, metadata, lifecycle, and expiry. Inactive entries are
hidden and cannot be pulled by default; historical retrieval must be explicit.

## Local discovery

`browsertools.DiscoverLocalSources` scans caller-selected file/directory roots
for browser profiles and full capability bundles. It is offline, rejects
symlinks and non-regular inputs, deduplicates exact bytes, and reports validated
candidates separately from ambiguity, rejection, and truncation. Defaults are
10,000 visited entries, 100 accepted candidates, and 20 MiB per file. Reaching
a bound is a visible blocker with guidance to narrow the root.

Directory names such as `browser-profiles` and `capability-bundles` add a small
ranking hint only after content validation. Names are never proof of source
kind.
