# Browser shape tables

`browsertools.BuildBrowserShapeTable` and `VerifyBrowserShapeTable` operate
offline on caller-supplied exact JSON/YAML profile bytes. The producer adopts
published UWS C10 `v0.0.0-20261009220914-f01a2542410c` and returns its
`binding.ShapeTable` contract. It accepts Browser 1.5–1.10, authentication
1.0–1.1 and registration 1.0–1.2 through the existing native validators.

```go
options := browsertools.BrowserShapeOptions{
    Sources: []browsertools.BrowserShapeSourceInput{
        {ID: "reviewed-browser", Content: exactProfileBytes},
    },
}
table, err := browsertools.BuildBrowserShapeTable(ctx, options)
// Independently load the expected exact source set before accepting a claim.
err = browsertools.VerifyBrowserShapeTable(ctx, options, table)
```

An ID is the caller's source binding. `URL` is optional inert provenance, never
a fetch instruction. This API has no file loader, browser, credential resolver,
private runtime or ambient configuration. Errors are fixed and contain no
source values. A failure returns no partial table. Cancellation is cooperative;
a consumer requiring hard CPU, RSS or deadline enforcement uses an isolated
worker.

Every source binds SHA256 of its exact original bytes. Every native action or
flow has an exact `id` selector with its original key. Selected SHA256 covers
the **complete** action/flow, including descriptions, empty and false values,
sequence, confirmation, extensions and all native fields. JSON decoding rejects
duplicate names, invalid UTF8 and unpaired UTF16 escapes; canonical encoding
retains numeric tokens, recursively sorts keys, preserves arrays, uses Go's
`Encoder.SetEscapeHTML(false)` and removes its single trailing newline.
The unchanged M51 canonical golden is checked independently.

YAML is one document with string keys and no aliases, anchors, merge keys or
custom tags. JSON-shaped numeric tokens retain their lexemes, including signed
64-bit integers, decimal trailing zeros and negative zero. Other finite YAML
integers normalize to exact decimal; unsupported numeric forms refuse. A YAML
and JSON spelling may bind different source and selected digests; callers use
the exact artifact they approved.

The output retains the full parameters schema under input `body`, each full
native output declaration including extraction metadata, exact effects and
confirmation policy, sorted canonical origins, symbolic credential slot names
and kinds, and registration slot schemas and conditions. All declared
credential slots remain explicit required bindings. Registration input slots
retain false requiredness and conditional declarations. An authenticated action
retains `authentication_required`; its generic security remains unknown rather
than claiming anonymous use. Named sessions are operation-level native UWS call
metadata (`x-uws-browser-authentication` / `x-uws-browser-session`), supplied and
verified by the caller; profile bytes contain no session name to invent.
Authentication's `establishes_session` and symbolic declarations remain inert.

Projection completeness means the native declaration was preserved; it does
not claim supported runtime protocols, successful extraction, authentication,
current approval, traffic containment, retained session state or legal policy.
UWS owns binding/flow/portability analysis, including partial or unknown schema
proof. An independently reproduced table grants no capture or execution.

Limits are fixed ceilings: 32 sources, 1 MiB per source, 32 MiB total input,
10,000 operations and 64 document levels, plus UWS's 8 MiB table and 256 KiB
per schema bounds. Options can only lower per-source and operation limits.
Claim verification checks inventories and aggregate raw metadata before
serialization, then compares the entire reproduced table including identities,
provenance, completeness and all projected native metadata.

`testdata/browser-shapes` preserves the eleven C10 native source vectors and
six unchanged M51 fixture files, with their original hash manifest. They are
synthetic inputs and provide no real browser qualification or consumer adoption.
