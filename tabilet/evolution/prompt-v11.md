# Prompt V11 - One Process Owns the Authenticated Author Session

Implement Browsertools A03 as one strict headed Chromium authoring process that
owns its non-persistent context across human login/MFA and post-login
exploration. Accept only bounded Browsertools-issued semantic candidates and
explicit origin, action, navigation, and POST approvals. Keep credentials,
cookies, storage, raw page/network material, browser handles, and resumable
session state inside the process, destroy the context on every failure, and
emit only the finite private authoring result after teardown.
