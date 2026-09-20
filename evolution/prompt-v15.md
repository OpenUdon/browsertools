# No-Submit Registration Authoring Producer

Preserve `browsertools.author-session.v2` and
`browsertools.authenticated-authoring.v2` as immutable authentication plus
capability contracts. Define a separate
`browsertools.registration-author-session.v1` protocol and
`browsertools.registration-authoring.v1` private result for constructing one
reviewed `uws.browser-registration.1.0` candidate without inheriting login,
session, click, submit, POST, or runtime authority.

Keep the new session exact-origin, GET/HEAD-only, value-free, finite, and
human-reviewed. It may expose only reduced current-generation accessibility
semantics and accept explicit structural decisions; it must never type or read
credentials, execute the described submit, create an account, retain a browser
session, or export private evidence. Bind the private result to the existing
registration-review v1 contract and OpenUdon's public value-free transaction
provenance.

Implement the guarded Chromium producer only after the wire contract and its
compatibility/security gates close. Publish only the reviewed A08 Browsertools
commit, keep UWS unchanged, and defer this prompt's result until OpenUdon E10
and the offline private W8M W01 reconciliation both pass.
