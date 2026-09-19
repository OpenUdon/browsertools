# Optional script denial during authenticated authoring

The default authoring guard ends the session on any request outside approved
origins. A reviewed local invocation can opt into denying scripts from one exact
HTTPS origin without ending the session:

```text
browsertools author-session chromium --private-root PRIVATE --blocked-script-origin https://analytics.example.test
```

`capture.NewPlaywrightAuthorBrowserWithPolicy` and `authorworker.Options` expose
the same local opt-in. The immutable `authorpolicy.Policy` accepts an exact HTTPS
origin only. It matches non-navigation uppercase GET requests whose reduced CDP
resource is script. The matching request is always failed with BlockedByClient;
it is never fetched or added to approved origins. Other methods, resources,
navigation, malformed URLs, credentials, schemes and ports stay fatal. A later
origin approval cannot admit the blocked origin.

Closing state, prior failure and request accounting take precedence; denials
consume request budget. Response accounting and POST approval remain unchanged.
Browser-wide Fetch covers redirects, popups and OOPIFs before contact and stays
installed until browser destruction. Interception command failures stay fatal.
Public author-session v2, diagnostic readers and generated profiles are unchanged;
page/protocol input cannot select this policy. Registration has no such option.

Browser-free tests cover matching, malformed input, defaults, privacy, budgets
and failure precedence. The explicit `BROWSERTOOLS_AUTHOR_LIVE_TEST=1` optional
script fixture uses only owned loopback endpoints, checks zero TCP contact to
the denied HTTPS endpoint, proves main/redirect login and OOPIF login readiness,
and preserves the existing rejection when a known frame is detached. A passing
fixture is not evidence that an arbitrary site works without its script.
