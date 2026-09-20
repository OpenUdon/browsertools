# UWS 1.9.1 Browser Content-Trust Resolver

Adopt UWS 1.9.1 additively by providing a public Browsertools resolver for its
explicit advisory content-trust analyzer. The resolver must own only supplied
browser-profile sources, describe RFC 6901-relative data, instruction, and
authority channels, default browser-derived outputs to untrusted, and keep
value capability distinct from provenance.

Strings are free text unless an inline enum constrains them; numeric, boolean,
null, presence, and inline-enum outputs are constrained scalars; structured
outputs are composite; unsupported future forms remain unknown. Validation,
browser execution, runtime authority, and immutable Browser 1.7 artifacts do
not change. Require deterministic tests and exact Udon, OpenUdon, and Ramen
compatibility evidence before recording completion.
