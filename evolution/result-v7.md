# Result V7 - Human Intent, Strict Bundle, Read-Only Checks

Browsertools now presents deterministic evidence, origin, accessibility, and
output candidate IDs in a terminal wizard. The operator explicitly supplies
action IDs, evidence bindings, scalar parameters, outputs, closed macro steps,
waits, side effects, confirmation, expiry, and ambiguity rationales. The
resulting `browsertools.guided-authoring.v1` envelope contains the accepted
spec, canonical action-bound evidence, decisions, schema-valid profile, and
promotable digest-bound review.

`live-check chromium` derives only locator, navigation-wait, and output-shape
probes from a validated profile and selected actions. It reuses the E03 exact-
origin ephemeral Chromium policy, executes no profile macro, accepts no
caller-supplied probes or Playwright selector language, reads no input values,
and emits only profile-bound count/type/reachability facts. Raw page evidence is
discarded without serialization. Production execution and headed manual
authentication remain outside P03.
