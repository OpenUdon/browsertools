# Browser Authentication Profiles

Browsertools validates and reviews the additive
`uws.browser-authentication.1.0` profile. These profiles describe sign-in UI
evidence; they are not browser drivers and do not contain credentials or
sessions.

Authentication profiles remain package-local in this release. Local discovery
returns them as `browser_authentication_profile` candidates, but capability
bundle construction and static-registry publication reject them. This prevents
login topology from becoming public catalog content before a separate
distribution policy is reviewed.

## Offline authoring

Start with an explicit draft specification matching the public profile fields
except for the fixed `profile` discriminator:

```bash
go run ./cmd/browsertools auth-draft build \
  --spec ./authentication-spec.yaml \
  --out ./browser-authentication/member.yaml

go run ./cmd/browsertools auth-profile validate \
  --input ./browser-authentication/member.yaml \
  --at 2026-08-16T00:00:00Z

go run ./cmd/browsertools auth-review bundle \
  --profile ./browser-authentication/member.yaml \
  --at 2026-08-16T00:00:00Z \
  --out ./browser-authentication/member.review.json
```

Drafting is deliberately non-inferential: Browsertools inserts the fixed
profile name and validates the author's exact origins, locators, credential
slots, challenge choice, effects, success condition, and lifecycle metadata.
It never chooses an MFA alternative or invents a login step.

## Safety gates

- One profile is at most 1 MiB and one YAML/JSON document.
- Exact HTTPS origins are required, except loopback HTTP fixtures.
- Navigation and success origins must be declared.
- Referenced credential slots must exist; a TOTP challenge requires a
  `totp_seed` slot.
- Challenge flows must declare `sends_mfa_challenge`.
- Secret-shaped values, email-address values, phone-number values, redaction
  markers, and inline secret fields fail validation.
- Expiry is inclusive: a profile is stale when assessment time equals its
  expiry instant.

Credential values, OTP responses, cookies, storage state, browser processes,
and session persistence remain runtime-owned. A package carries only the
reviewed recipe and its digest-bound review record.
