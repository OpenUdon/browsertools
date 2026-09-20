# Result V6 - Private, Headless, Exact-Origin Capture

Browsertools now acquires depth-limited ARIA snapshots and valid JSON-LD through
an explicitly selected headless Chromium command. Every capture uses a sandboxed
non-persistent context, exact HTTPS or loopback origins, GET/HEAD-only routing,
blocked service workers/WebSockets/frames/popups/downloads/dialogs/file
choosers, and finite time, request, response, evidence, and retention bounds.

Captured page material goes directly into the non-publishable private cache;
stdout carries only the manifest. Export requires a new `0600` file and cannot
overwrite or stream raw content. Normalization remains a separate reviewed
adapter step, excludes credential-shaped JSON-LD properties, and copies only
property names/types rather than values. Headed interaction remains deferred to
A02, where authentication-specific controls apply.
