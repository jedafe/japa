# Security Policy — JapaConnect

## Scope

JapaConnect is a multi-surface platform: Go API (`internal/`, `cmd/`), SvelteKit
web apps (`apps/main`, `apps/admin`, `apps/control`, `apps/academy`, `apps/tools`,
`apps/docs`), Flutter mobile (`apps/mobile` + `packages/japaconnect_core`),
browser extension (`apps/extension`), and Tauri desktop (`apps/studio-desktop`).

This policy covers authentication, authorization, data isolation, input
validation, abuse prevention, payments, secrets handling, and frontend/mobile
security across all of those surfaces.

## Supported versions

Only the current `main` branch is supported. Security fixes land on `main`
first; backports are done case-by-case.

## Reporting a vulnerability

Email the maintainers with: affected surface, steps to reproduce, and impact.
Do not open public issues for unpatched vulnerabilities. Do not run destructive
exploits against production or third-party systems — use local fixtures and
dry runs.

## Authentication & sessions

- Passwords: bcrypt cost 12, configurable policy, HIBP breach check, history.
- JWT: HS256 with `kid` rotation (`jwt_signing_keys`, AES-GCM via
  `JC_ENCRYPTION_KEY`); 15-minute access TTL; per-token JTI revocation
  (`jc:revoked_jti:{jti}`).
- Refresh: one-time-use rotation with reuse detection; theft revokes the whole
  session family. Web uses `HttpOnly SameSite=Strict jc_refresh`; mobile uses
  JSON body + `X-Device-Id`.
- TOTP required for admin panel when `auth.totp.required_for_admin` is set.
- OAuth account merge requires provider-verified email; unverified emails use a
  pending-link flow, never silent merge.
- Desktop `exchange-key` requires TOTP for admin scopes, binds the session to
  the key id, and audit-logs issuance.
- Password-reset token validity is never disclosed via a page oracle.

## Authorization & isolation

- Dynamic RBAC: User Groups → Roles → Permissions; `*` wildcard is
  super-admin-only and every grant is audit-logged.
- Permission cache (L1 60s / L2 10m) is invalidated on grant, revoke, ban, and
  moderator change; admin capabilities resolve through short-TTL paths.
- Tenant/space isolation resolves per space (owner / moderator / static node).
- Impersonation is enforced: downstream code resolves the effective user,
  per-request audit carries both admin and impersonated ids, admin routes keep
  the real identity, and admin targets can never be impersonated.
- Control-plane routes (`/control/*`) require per-route permissions, not just
  `control.panel.access`.
- CSRF: double-submit `csrf_token` + `X-CSRF-Token` + Origin allowlist for
  cookie-authenticated browser flows. `Authorization: Bearer` / `X-API-Key`
  requests are exempt by design (custom headers are not forgeable cross-site).

## Input validation & abuse

- All SQL is parameterized (pgx/sqlc `$1`); search filters compile through an
  AST with Meili escaping.
- Spam pipeline + censor middleware + `tight` rate limits protect
  content, blog, comments, scam-alerts, forum, and reviews create/update paths;
  comment edits are re-scanned.
- File uploads: two-step initiate→confirm with MIME sniffing (never the client
  header), extension cross-check, per-context allowlists, AV scan, traversal
  guards; SVG is raster-only; bulk deletes are capped.
- Link preview: SSRF guard (scheme, 15-CIDR private-IP block, DNS all-records
  check, per-hop redirect validation, dial-time IP block against rebinding),
  1MB cap, 24h cache on normalized URLs, generic error mapping (no DNS oracle).
- Data import: allowlisted columns only — no `password_hash` / ledger-balance
  injection; row-count caps; dedicated `data_imports.manage` permission; audit
  logged. Exports serve as `attachment` with `nosniff`.

## Payments, webhooks, secrets

- Provider keys never share values (no `API_KEY=IPN_SECRET` fallback);
  unsigned-webhook bypass is config-gated, default off, and audited.
- Refunds call the provider API; discounts enforce `max_uses` atomically;
  money uses int64 minor units.
- Outbound webhooks: HMAC-SHA256, encrypted secrets at rest, challenge
  activation, rotatable secrets, SSRF-safe delivery with retries.
- SMTP headers/addresses are CRLF-sanitized; TLS required in production.
- Crypto: AES-256-GCM `v1:<keyid>:<nonce>:<ct>`, rotation via
  `JC_ENCRYPTION_KEY_PREVIOUS`. No vault/KMS yet — see residual risk.
- Production requires `JC_DATABASE_URL`, `JC_COOKIE_DOMAIN`, `JC_JWT_SECRET`
  (≥32), `JC_ENCRYPTION_KEY`, plus payment/SMTP/VAPID/S3 settings. Never ship
  the dev `.env` (weak JWT/seed password, live-pattern keys) or compose
  defaults (`trust`, default secrets, wide ports) to prod.

## Frontend / mobile

- Web apps keep tokens in `HttpOnly` cookies (main/academy/tools) or
  in-memory only (admin/control); never `localStorage`.
- All `{@html}` sinks pass through `sanitizeContentHtml` / DOMPurify or the
  `control/utils.ts` parser allowlist; never render `{@html t(...)}` or raw
  LLM HTML. `customCss`/`customJs` settings sit behind CSP nonces + staff audit.
- Admin destructive actions use explicit `hardDeletePermission` (no fallback);
  all row actions are server-enforced (client gating is cosmetic).
- Desktop secrets live in the OS keychain with an encrypted `state.db`;
  legacy `localStorage` keys are wiped after migration.
- Mobile: refresh token in `FlutterSecureStorage`, HTTPS/WSS enforced in
  release, WS reconnect capped with backoff, ticket query encoded.
- WebSocket tickets are single-use with <60s TTL; event payloads are
  schema-validated and re-authorized server-side.

## Rate limits

Tiers: `tight` 10/min, `moderate` 60/min, `loose` 300/min, `webhook` 3000/min,
`oauth_token` 120/min, `form_submission` 10/hour. Abuse-sensitive routes
(registration, login, password reset, consent, account delete, media proxy,
feeds, imports) carry explicit tiers — never bare `loose`.

## Residual risk (accepted, tracked)

- No vault/KMS for secrets; rotation covers `JC_ENCRYPTION_KEY` only.
- Valkey outage degrades revocation/rate-limit/brute-force to logged
  fail-open with alerts; Valkey is required in production and monitored.
- DOCX ZIP-bomb / XML entity / PDF page-bomb limits are caps, not full
  sandboxing; `ffmpeg` must be present in the runtime image for video.
- WS tickets travel in the query string (access-log exposure) — mitigated by
  single-use + short TTL.
