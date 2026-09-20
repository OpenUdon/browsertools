# UWS 1.9.1 Browser Content-Trust Resolver Result

Browsertools now publishes `contenttrust.NewResolver` at
`75fd5c3ab81f904243f8c2650c61ba1cd8c00540`
(`v0.0.0-20260826234723-75fd5c3ab81f`) against exact UWS 1.9.1 commit
`9e676eaa469e9168225a7dcee75eb309e3499637`.

The resolver validates and defensively clones caller-supplied browser profiles,
owns only their bound UWS operations, and returns deterministic relative input
channels and output contracts. Browser-derived outputs default to untrusted;
their free-text, constrained-scalar, composite, or unknown capability is
reported separately. No runtime values or browser content enter the contract.

Standalone tests, vet, pinned vulnerability scanning, and hosted CI pass.
Udon M37, OpenUdon A22/P06/E12, and Ramen M73 qualify the exact published
resolver through explicit advisory entry points. Browser profile validation,
Browser 1.7 artifacts, planning, execution, and runtime authority remain
unchanged.
