# Result V9 - One Private Bundle, Fresh Engine Comparisons

Browsertools can now capture an explicitly selected screenshot/trace/HAR set in
one headless Chromium context under the same exact-origin GET/HEAD guard used by
minimal live acquisition. The context closes before bytes return. A canonical
manifest and exact members become one deterministic, at-most-20-MiB
`browsertools.private-rich-evidence.v1` ZIP in the restrictive private cache.
It uses the process clock, defaults to one-hour retention, expires within 24
hours, requires local secret review before export, has no promotion path, and
is removable only through a repeated exact cache digest.

Portability checking validates one browser.1.5 profile and action set, then
opens independent Chromium, Firefox, and/or WebKit lifecycles. Each engine runs
the same closed profile-derived locator, wait, and output-shape probes. The
value-free `browsertools.portability-check.v1` report records every requested
engine and fixed unavailable/acquisition/check/baseline/shape diagnostics; it
does not retain page values, browser errors, or rewritten locators.

The maintained contract-pressure report keeps screenshot/trace/HAR private,
defers download/upload/permission behavior, and records concrete popup, frame,
and bounded visual-locator candidates for a future reviewed UWS browser.1.6.
No browser.1.5 schema or profile key changes in this milestone, and neither E04
path can borrow A02's headed operator window or POST authority.
