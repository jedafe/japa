# JapaConnect Browser Extension

New-tab replacement showing your migration feed — destination visa intel,
safety alerts, and community activity. Thin client over the public Agent
Data API (`/api/v1/public/*`); see `docs/superpowers/specs/2026-09-24-browser-extension-design.md`.

## Build

```bash
bun install
bun run build          # outputs .output/chrome-mv3/
bun run check          # svelte-check
```

## Local development against a dev API

Point the extension at a local API with `WXT_JC_SITE_URL` (drives
`host_permissions`, the API base, and deep links):

```bash
WXT_JC_SITE_URL=http://localhost:8081 bun run build
```

Then load `.output/chrome-mv3` as an unpacked extension.

**Chromium required for sideloading:** Google Chrome ≥139 silently ignores
`--load-extension` for branded builds. Use Chromium, Edge/Chrome dev
channels, or `chrome://extensions` → Developer mode → "Load unpacked" on a
build that still permits it. Store installs are unaffected.

Anonymous tabs show the popular feed. To unlock the personalized digest,
click **Sign in with JapaConnect** — the extension runs the §120 OAuth
flow (`launchWebAuthFlow` → `/oauth/authorize` + PKCE → `/oauth/token`,
identity via `/oauth/userinfo`) and stores the token pair locally. As a
fallback you can paste a `jc_live_`/`jc_test_` key (from Settings → API
keys on the site). Revoke a connected browser under Settings → Connected
apps on the web.
