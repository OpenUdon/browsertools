# Browser Registration Profiles

Browsertools supports the additive `uws.browser-registration.1.0` contract as
offline producer tooling. Registration is not authentication: it creates a
remote account, while `uws.browser-authentication` signs in an existing
identity and establishes a named execution-local session.

## Owned boundary

Browsertools can:

- validate a registration profile through UWS' public embedded schema and
  semantic validator;
- reject inline secret-, credential-, and PII-shaped material before schema
  diagnostics can disclose it;
- build the exact profile from a complete explicit specification without
  inferring actions;
- calculate deterministic profile digests and expiry; and
- produce and verify a package-local digest-bound review.

All three CLI commands are file-only and offline:

```bash
browsertools registration-profile validate --input PROFILE --at RFC3339
browsertools registration-draft build --spec SPEC --out PROFILE
browsertools registration-review bundle --profile PROFILE --at RFC3339 --out REVIEW
```

Browsertools does not launch a browser for these commands. It does not resolve
credentials, inspect an account value, execute a submit, approve a mutation,
handle a human checkpoint, retry an outcome, or perform cleanup. A review
bundle proves only the exact inert profile digest and lifecycle; it is not
evidence that registration was attempted or succeeded.

## Portable safety contract

Credential slots are symbolic `identifier` or `password` names. Their values
must be entered by a human in the private browser or injected by a trusted
runtime; they never enter the profile, specification, review, CLI arguments,
stdout, logs, or Browsertools environment bindings.

Every flow declares exact HTTPS application/registration origins, exactly one
`submit`, `creates_account`, and `confirmationPolicy.required: true`. A
`human_checkpoint` records only its fixed kind and optional reviewed locator;
it never carries a CAPTCHA response, verification token, MFA response, or
consent value.

The corresponding `uws.browser-registration-call.1.0` envelope fixes:

```yaml
duplicatePrevention: operator_attestation
onDuplicate: fail
ambiguousOutcome: stop_without_retry
cleanupDisposition: delete_separately # or retain_dedicated_test_identity
```

The symbolic approval field is not proof of approval. A trusted downstream
runtime must bind actual human approval to the exact operation, profile bytes,
symbolic credential mapping, origin inventory, duplicate attestation, and
cleanup disposition immediately before the profile's one submit.

## Distribution

Registration profiles and reviews are package-local workflow inputs. They are
not browser capability bundles and cannot be published through Browsertools'
static registry. Raw authoring evidence, page content, captures, credentials,
account identifiers, verification material, and browser state do not belong in
the portable artifacts.

UWS owns the profile and call schemas. OpenUdon owns user-facing selection,
package review, and trusted handoff. Udon and Browserdriver own optional runtime
execution, credential resolution, human-checkpoint interaction, network and
redirect containment, approval enforcement, and value-free execution evidence.
