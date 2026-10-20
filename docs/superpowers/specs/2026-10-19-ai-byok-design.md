# AI BYOK — Bring Your Own API Keys

**Status:** Design approved — pending spec review
**Date:** 2026-10-19
**Refs:** §20 (JapaBot admission/quota), §103/ADR-111 (AI Agent Executor), ADR-130 (Unified Japa AI), §21 (Control Panel AI provider management), §42 (feature flags / Control → Extensions), §103 Security (endpoint validation / SSRF)

## 1. Background

All LLM traffic today runs through platform-owned providers: super-admins
CRUD rows in `ai_providers` (encrypted keys), `LoadAndRegisterProviders`
hot-reloads them into the `Dispatcher` by category, and the ADR-130 unified
chat (`HandleUnifiedChat`) dispatches every turn through that registry.
`/ai/models?category=chat` feeds the frontend model picker with exactly the
models the platform pays for.

BYOK lets users attach their own provider API keys. A turn addressed to a
user key is dispatched to an ephemeral provider built from that key —
platform cost for the turn is zero, and the picker surfaces a "Get an API
key" affiliate link per provider, generating referral revenue.

Two constraints from the existing architecture:

- The agent loop (tools, Honcho memory, RAG pre-search, SSE streaming) lives
  entirely server-side, so user keys must be stored encrypted and used by
  the backend — browser-direct calls are a non-goal.
- The dispatcher falls back across providers on failure. BYOK turns must
  NOT fall back to platform providers — a rejected user key must never
  silently burn platform quota.

## 2. Goals

- Users can add/verify/remove their own API keys for admin-curated provider
  types.
- The `/ai` model picker shows two groups: **Platform** (models admin
  allows) and **Your keys** (models from the user's attached keys).
- Each BYOK provider row carries an affiliate `signup_url`; clicks are
  tracked.
- Admins control the BYOK allowlist (provider types, model catalogs,
  referral links) from the Control Panel.

Non-goals: browser-direct calls, reselling marked-up keys/billing,
user-supplied base URLs (v1), BYOK for embedding/moderation categories
(chat and agent turns only).

## 3. Data model

```sql
ai_byok_providers (                    -- admin-curated allowlist
  id uuid PK DEFAULT gen_random_uuid(),
  provider_type text NOT NULL UNIQUE   -- must be in SupportedProviderTypes()
    CHECK (provider_type = ANY (ARRAY['openai','anthropic','groq','perplexity'])),
  display_name text NOT NULL,
  models jsonb NOT NULL DEFAULT '[]',  -- [{id, label}] the catalog a key unlocks
  signup_url text NOT NULL DEFAULT '', -- affiliate/referral link, may be ''
  enabled boolean NOT NULL DEFAULT true,
  referral_clicks integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)

user_ai_keys (
  id uuid PK DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  byok_provider_id uuid NOT NULL REFERENCES ai_byok_providers(id) ON DELETE CASCADE,
  label text NOT NULL DEFAULT '',
  api_key_encrypted text NOT NULL,     -- crypto.Encrypt, same as ai_providers
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','invalid')),
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)
CREATE INDEX idx_user_ai_keys_user ON user_ai_keys(user_id);
```

`user_ai_keys` hard-deletes on GDPR erasure via the `users` cascade —
consistent with erasure of credential material. Plaintext keys are never
serialized; the API returns `api_key_masked` (reuse `maskAPIKey`).

## 4. Feature flag, RBAC, settings

- `feature.ai_byok` registered in `features.Registry` (feature_flags —
  never a `system_settings *.enabled` key), managed via Control →
  Extensions. Gates every new user-facing BYOK route.
- New RBAC node `ai.byok` seeded by migration, granted to the default
  member group; lets admins restrict BYOK to paid tiers later.
- `ai.chat.allowed_models` system_setting (JSON array of model ids;
  `[]`/absent = all registered models). Filters the **Platform** group in
  `/ai/models` only — dispatch is unaffected. Added to
  `system-settings-catalog` and `control/settings_catalog.go`.

## 5. API surface

User-facing (all `RequireAuth` + `feature.ai_byok` + `ai.byok`):

```
GET    /api/v1/ai/byok/providers            — enabled BYOK providers:
                                              {id, provider_type, display_name,
                                               models, signup_url, has_key}
POST   /api/v1/ai/byok/providers/{id}/referral-click
                                            — increments referral_clicks +
                                              analytics event
GET    /api/v1/me/ai-keys                   — caller's keys (masked) + provider info
POST   /api/v1/me/ai-keys                   — {byok_provider_id, api_key, label?}
PATCH  /api/v1/me/ai-keys/{id}              — {label?, api_key?} (re-encrypt on key set)
DELETE /api/v1/me/ai-keys/{id}
POST   /api/v1/me/ai-keys/{id}/verify       — cheap live check (below)
```

`POST /me/ai-keys` and `PATCH` never return plaintext. `verify` performs a
minimal provider call (short timeout) and sets `status`/`last_error`
accordingly.

Admin-facing (Control Panel, `ai.tools.manage` — same node as
`/control/ai-providers`):

```
GET    /api/v1/control/ai-byok-providers
POST   /api/v1/control/ai-byok-providers
GET    /api/v1/control/ai-byok-providers/{id}
PATCH  /api/v1/control/ai-byok-providers/{id}   — incl. models catalog, signup_url, enabled
DELETE /api/v1/control/ai-byok-providers/{id}   — cascades user keys
```

## 6. Dispatch path

`UnifiedChatRequest` gains `ByokKeyID *pgtype.UUID` (JSON `byok_key_id`).

When `ByokKeyID` is set:

1. Load `user_ai_keys` scoped `WHERE id = $1 AND user_id = $2` — ownership
   enforced in SQL, cross-user access is 404.
2. Join `ai_byok_providers`; reject if the provider row is disabled or the
   `model` isn't in its catalog (`errInvalidModel`-class 400).
3. Decrypt key → `NewProviderFromConfig(ProviderConfig{...}, plaintext)` —
   reuses every provider constructor including base-URL validation.
4. Dispatch the turn to that provider instance **directly** — bypassing
   `Dispatcher.Dispatch`/`ChatWithTools` fallback chains. Rate counters
   (`jc:ai:*`, `jc:agent:rate` when tools run) still apply; platform cost
   counters (`jc:ai:cost`, `jc:agent:cost`) are skipped — the platform
   spent nothing.
5. Provider auth/permission failures set `status='invalid'` +
   `last_error`; the response carries a `key_invalid` error code so the
   frontend can flag the key. Other failures (rate limit, 5xx) do NOT
   mark the key invalid.
6. Honcho memory, pre-search, tool catalog, SSE stream — all unchanged;
   the ephemeral provider satisfies the same `Provider` interface.

When `ByokKeyID` is nil: existing dispatcher path, unchanged.

Streaming: ephemeral providers flow through the same streaming adapters
(`openai_stream.go`, `anthropic_stream.go`) since they are the same
concrete types.

## 7. Frontend (`apps/main`)

- `GET /ai/models?category=chat` stays platform-only (filtered by
  `ai.chat.allowed_models`). The `/ai` load function additionally fetches
  `/ai/byok/providers`; the picker builds two groups:
  - **Platform** — existing model entries, sends `{model}`.
  - **Your keys** — one entry per (key × catalog model), labelled
    `{provider display_name} · {model label}`, sends `{model, byok_key_id}`.
  If `AutocompleteSelect` lacks optgroup support, use a two-section
  dropdown or label prefixes — resolved at implementation.
- **Add key flow** (dialog from the picker + settings): pick provider →
  "Get an API key →" opens `signup_url` in a new tab and fires
  `referral-click` → paste key + optional label → optional inline verify →
  key's models appear under "Your keys".
- **Settings → AI** section lists keys: provider, masked key, label,
  status badge (`invalid` highlighted), verify + delete actions.

`apps/control`: new BYOK providers screen (DataTable spec: list/create/
edit models catalog + signup_url + enabled/delete) next to the existing
AI providers page.

## 8. Affiliate mechanics

`signup_url` is a plain admin-managed outbound URL (e.g. an OpenRouter or
Groq referral link). `POST referral-click` bumps `referral_clicks` and
emits an analytics event so referral CTR is measurable per provider. No
redirect proxy in v1 — the link opens directly; the click event fires
first.

## 9. Security

- Keys encrypted at rest via `crypto.Encrypt`; never returned in any
  response; masked form only.
- No user-supplied base URLs in v1 — provider_type is validated against
  `SupportedProviderTypes()` AND the `ai_byok_providers` CHECK constraint
  (excludes `devin`/`cursor`/`xiaomi`/`ollama`/`custom` — the first four
  have admin-oriented auth/URL shapes, and `custom` requires a base URL
  which is out of scope for v1).
- Rate limits apply identically to BYOK turns (prevents using the
  platform as an amplification relay).
- Key create/delete write `audit_logs` entries.
- `ai.byok` node + `feature.ai_byok` gate every route.

## 10. Error handling

Standard error envelope. New codes: `byok_provider_not_found`,
`byok_provider_disabled`, `ai_key_not_found`, `model_not_allowed`
(BYOK model outside catalog), `key_invalid` (provider rejected the key),
`key_in_use` (credential attached to another account),
`key_already_added` (same credential already on the caller's account),
`key_limit_reached`, `too_many_rejected_keys`, `verification_failed`
(provider unreachable — transient, key not flagged).
Validation of `provider_type`, `models` shape, and `signup_url` (https
only) happens in control-panel handlers.

## 11. JEOS updates (before/with code — per AGENTS.md)

- `api-routes-catalog.md`: all routes in §5.
- `system-settings-catalog.md` + `control/settings_catalog.go`:
  `ai.chat.allowed_models`.
- RBAC section: `ai.byok` node.
- Feature-flag registry: `feature.ai_byok`.
- Migration-order doc: two new tables.
- Section file covering the AI gateway gets a BYOK subsection.

## 12. Testing

- Store tests: CRUD, masking, ownership scoping, cascade on user delete.
- Dispatch tests: BYOK turn hits the ephemeral provider; no fallback to
  platform providers; auth failure marks key invalid; model-not-in-catalog
  rejected; rate counters still consumed, cost counters skipped.
- Handler tests: feature-flag off → 404/403; missing `ai.byok` → 403;
  referral-click increments.
- Frontend: picker grouping renders both sections; BYOK selection
  serializes `byok_key_id` correctly.
