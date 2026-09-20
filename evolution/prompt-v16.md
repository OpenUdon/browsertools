# Query-Safe Browser Registration Authoring

Preserve `browsertools.registration-author-session.v1` and
`browsertools.registration-authoring.v1` byte-for-byte, including their query
rejection and the registration worker's v1 default. Add explicit v2 contracts
that may retain a reviewed literal structural query only when the complete URL
is absolute, fragment-free, canonically encoded, bounded, exact-origin, free
of templates/userinfo, and has unique nonempty non-credential keys with
non-secret values.

Apply one validator to initial navigation, later GET/HEAD navigation, every
retained UWS browser-registration `navigate` step, and the guarded Chromium
request/redirect boundary. Queries may exist only in the reviewed canonical
source and navigation command; observations, diagnostics, logs, filenames,
private result paths, and retained qualification evidence expose only exact
origin plus disclosure-safe path.

Keep Browsertools no-submit and credential-free. Add no registration runtime,
POST, submit, input, account, session, cookie/storage, or target authority.
Defer result v16 until the cross-package OpenUdon E11 qualification passes.
