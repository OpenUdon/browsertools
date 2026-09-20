# Result V5 - Pinned Acquisition Boundary And Safe Expansion Horizon

Browsertools now pins Playwright-Go v0.6201.0 (Playwright 1.62.1) behind a
fakeable `capture` lifecycle, records which upstream capabilities are adopted,
private, deferred, or excluded, and exposes an offline doctor for installed
driver/browser readiness. The doctor never installs or launches a browser and
verifies the selected executable exists on disk.

The approved horizon expands authoring in bounded stages: safe Chromium
evidence capture, guided capability authoring/read-only checks, headed manual
authentication observation without credential ingress, then private rich
evidence and cross-browser evaluation. Portable profiles remain closed and
engine-neutral; production browser execution, credentials, retained sessions,
and side effects remain downstream.
