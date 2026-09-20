# Query-Safe Browser Registration Authoring Result

Browsertools preserves `browsertools.registration-author-session.v1` and
`browsertools.registration-authoring.v1` byte-for-byte, including query
rejection and the worker's v1 default. Explicit protocol selection adds the
separate v2 session and result wires for reviewed literal structural queries.

One bounded canonical URL validator now covers initial navigation, later
GET/HEAD commands, retained UWS registration `navigate` steps, and Chromium
requests and redirects. It requires absolute fragment-free exact-origin URLs,
unique nonempty non-credential keys, and non-secret values; it rejects
userinfo, templates, malformed or noncanonical encoding, repeated keys,
origin escape, and sensitive material. Observations and fixed diagnostics
retain only origin and disclosure-safe path. Query text is absent from logs,
filenames, result paths, and qualification evidence.

The guarded Chromium producer remains credential-free and no-submit: it has no
input, click, POST, account, session, cookie/storage, page-content, or export
authority. V1 fixture bytes, default behavior, BAP authoring, v2 query matrices,
redirect enforcement, cancellation, teardown, race, vet, module integrity, and
headed loopback gates pass at `d26f2982db352619d7a7f6563add802b56e10824`.

OpenUdon E11 independently qualifies the full downstream path with a value-free
18/18 report and 20 digest-linked artifacts. Authoring performs only GET/HEAD;
the sole approved loopback POST occurs later in Browserdriver v4, and no named
registration session survives. The Browsertools implementation commit is
published as `v0.0.0-20260826163208-d26f2982db35`; this result was subsequently
published with Tofu `95752f6`. Neither publication grants W8M contact or live
target authority.
