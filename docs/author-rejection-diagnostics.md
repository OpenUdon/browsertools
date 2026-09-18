# Private origin-rejection diagnostics

Newly built workers using the optional `--diagnostic-file` write
`browsertools.author-diagnostic.v2`. The public author-session protocol is
unchanged. The v1 `Write`/`Read` functions remain strict v1 APIs; v2 has separate
`WriteV2`/`ReadV2` functions. Retained v1 evidence is never rewritten.

V2 retains the closed `class` and adds exactly three strings in `rejection`:

| Field | Values |
| --- | --- |
| `boundary` | `request`, `observation`, `unknown`, `none` |
| `resource` | `document`, `stylesheet`, `image`, `media`, `font`, `script`, `xhr`, `fetch`, `event_source`, `manifest`, `text_track`, `ping`, `prefetch`, `other`, `unknown`, `none` |
| `origin_relation` | `invalid_url`, `userinfo`, `missing_host`, `local_scheme`, `unsupported_scheme`, `scheme_mismatch`, `port_mismatch`, `scheme_and_port_mismatch`, `host_mismatch`, `unknown`, `none` |

Non-origin failures require all three values to be `none`. An origin failure
with no detailed evidence uses all `unknown`. Observed-page failures require
`resource=document`. Unknown CDP resource types reduce to `other`; they are
never copied. Resource type does not identify a main frame, child frame, popup
or worker. The pinned Chromium reports the tested worker/child-frame `fetch()`
calls as `xhr`; the diagnostic preserves CDP's type, not the JavaScript API name.

Origin relationships compare only against the exact approved set, after the
existing admission check has rejected the URL. Default ports are normalized.
The reducer prefers an approved same-host/same-scheme comparison before a
same-host/same-port comparison; otherwise both scheme and effective port differ.
`local_scheme` means `about`, `data` or `blob`, without admitting them.
No origin string, URL/path/query/fragment, credential, header, body, error text
or browser state is stored. Classification never parses an error message.

The network guard preserves its first violation under its existing lock. Later
requests, observations and teardown cannot replace that rejection with a new
one. Origin/method/navigation/POST/request/response budgets are unchanged.
Diagnostic metadata supplies neither admission authority nor proof of network
containment. Both the class and detail use closed cross-field validation;
malformed, duplicate, extra, missing, null, oversized and unsafe-file inputs
are rejected by the private reader.
