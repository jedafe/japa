# Section 47 — Public API, OpenAPI & Developer Portal

**Status:** ✅ Locked

## Overview

JapaConnect exposes a versioned public API for external developers and partner integrations. The public API is the same Go service that powers the SvelteKit apps — no separate gateway — but with dedicated middleware layers (API key auth, rate limiting, scope enforcement) applied only to public-facing routes. The OpenAPI 3.x spec covers public routes only; internal routes used solely by the SvelteKit apps are not included in V1.

**New subdomain:** `api.japaconnect.com` — served by the Go service via a dedicated Nginx virtual host. Swagger UI lives at `api.japaconnect.com/docs`.

## Tech: oapi-codegen + Chi

The locked Chi router pairs naturally with `oapi-codegen`. Workflow is spec-first: write the OpenAPI YAML → generate Go types + server interface → implement the interface:

```
api/
  openapi/
    api.yaml          ← hand-authored OpenAPI 3.x spec (public routes only)
    config.yaml       ← oapi-codegen config
    generated/        ← git-committed generated output (do not hand-edit)
      api.gen.go      ← Go types, ServerInterface, ChiServerOptions
```

```yaml
# api/openapi/config.yaml
package: openapi
generate:
  chi-server: true
  models: true
  spec: true
  strict-server: true   # enforces all handler return types at compile time
output: api/openapi/generated/api.gen.go
```

```go
// Middleware stack for api.japaconnect.com routes only:
// 1. APIKeyMiddleware   — extract + validate key, inject scopes into ctx
// 2. RateLimitMiddleware — per-key sliding window via Valkey
// 3. ScopeMiddleware    — check key scopes against required node for this route
// 4. OapiRequestValidator — generated spec validation, drops malformed payloads

r.Group(func(r chi.Router) {
    swagger, _ := openapi.GetSwagger()
    swagger.Servers = openapi_types.Servers{}  // disable server URL validation in middleware

    r.Use(APIKeyMiddleware(queries, valkey))
    r.Use(RateLimitMiddleware(valkey))
    r.Use(ScopeMiddleware)
    r.Use(oapimiddleware.OapiRequestValidator(swagger))
    openapi.HandlerWithOptions(&PublicAPIServer{}, openapi.ChiServerOptions{BaseRouter: r})
})

// Swagger UI — embedded, not CDN (consistent with self-hosted posture)
//go:embed openapi/swagger-ui
var swaggerUIFS embed.FS

r.Get("/docs", swaggerUIHandler(swaggerUIFS))
r.Handle("/docs/*", http.StripPrefix("/docs", http.FileServer(http.FS(swaggerSubFS))))
```

## API Key Schema

```sql
CREATE TABLE api_keys (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE,  -- NULL for admin/system keys
  app_name        text NOT NULL,
  environment     text NOT NULL DEFAULT 'live' CHECK (environment IN ('live','test')),
  key_prefix      text NOT NULL UNIQUE,      -- first 12 chars, shown in UI (e.g. "jc_live_aBcD")
  key_hash        text NOT NULL UNIQUE,      -- SHA-256 of full key; full key shown ONCE, never stored
  scopes          text[] NOT NULL DEFAULT '{}',  -- array of API permission nodes (see below)
  rate_limit_tier text NOT NULL DEFAULT 'basic', -- 'basic'|'standard'|'premium'|'unlimited'
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','revoked')),
  last_used_at    timestamptz,
  request_count   bigint NOT NULL DEFAULT 0,
  expires_at      timestamptz,               -- NULL = never expires
  created_at      timestamptz NOT NULL DEFAULT now(),
  revoked_at      timestamptz,
  revoked_by      uuid REFERENCES users(id) ON DELETE SET NULL,
  notes           text                       -- admin-visible notes field
);
CREATE INDEX idx_api_keys_user   ON api_keys (user_id, status);
CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id, status, created_at DESC);
CREATE INDEX idx_api_keys_prefix ON api_keys (key_prefix); -- lookup on auth
```

**Key format:** `jc_live_<32-hex-chars>` or `jc_test_<32-hex-chars>`
Only `key_prefix` (first 12 chars) is shown in list views. Full key revealed once on creation modal and never again. Stored hash is `hex(sha256(full_key))`.

## API Key Authentication Middleware

```go
func APIKeyMiddleware(q *db.Queries, v *valkey.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Accept via Authorization: Bearer jc_live_... OR X-API-Key header
            raw := extractAPIKey(r)
            if raw == "" {
                renderAPIError(w, http.StatusUnauthorized, "api_key_missing", "API key required")
                return
            }

            hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
            key, err := q.GetAPIKeyByHash(r.Context(), hash)
            if err != nil || key.Status != "active" {
                renderAPIError(w, http.StatusUnauthorized, "api_key_invalid", "Invalid or revoked API key")
                return
            }
            if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
                renderAPIError(w, http.StatusUnauthorized, "api_key_expired", "API key has expired")
                return
            }

            // Fire-and-forget usage update (non-blocking): last_used_at + request_count increment
            go func(keyID uuid.UUID) {
                _ = q.IncrementAPIKeyUsage(context.Background(), keyID)
                // sqlc query: UPDATE api_keys SET last_used_at=now(), request_count=request_count+1 WHERE id=$1
            }(key.ID)

            ctx := context.WithValue(r.Context(), ctxKeyAPIKey, key)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## Rate Limiting (Valkey Sliding Window)

```go
// Tiers (configurable via system_settings api.rate_limit.{tier}):
// basic:     100  req/min
// standard:  1000 req/min
// premium:   10000 req/min
// unlimited: no limit (admin/system keys only)

func RateLimitMiddleware(v *valkey.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := APIKeyFromContext(r.Context())
            if key.RateLimitTier == "unlimited" {
                next.ServeHTTP(w, r)
                return
            }
            limit := getRateLimit(key.RateLimitTier)
            bucket := fmt.Sprintf("jc:api_rate:%s:%d", key.KeyPrefix, time.Now().Unix()/60)

            count, _ := v.Incr(r.Context(), bucket)
            if count == 1 {
                v.Expire(r.Context(), bucket, 70*time.Second) // slightly over 60s for safety
            }

            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, limit-int(count))))

            if int(count) > limit {
                w.Header().Set("Retry-After", "60")
                renderAPIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

## API Permission Scopes

API scope nodes are a subset of the RBAC permission tree, prefixed with `api.` to distinguish them from internal permissions:

```
api.read.spaces          — list/read spaces
api.read.content         — read published content
api.read.users.public    — read public user profiles
api.read.tags            — read tags
api.write.content        — create/update content (via API key)
api.write.comments       — post comments
api.read.me              — read authenticated user's own data
api.write.me             — update authenticated user's own profile
```

V1 public API is intentionally read-heavy. Write scopes are approved by admin on a per-key basis.

## Swagger UI (go:embed, no CDN)

```go
//go:embed openapi/swagger-ui
var swaggerUIFS embed.FS

// Single handler — serves embedded Swagger UI HTML/CSS/JS
// No CDN dependency, works in air-gapped environments, binary stays <10MB impact
// Accessible at api.japaconnect.com/docs — not behind OapiRequestValidator
func swaggerUIHandler(assets embed.FS) http.HandlerFunc {
    subFS, _ := fs.Sub(assets, "openapi/swagger-ui")
    fileServer := http.FileServer(http.FS(subFS))
    return func(w http.ResponseWriter, r *http.Request) {
        // CSP: self + inline (Swagger UI requires inline scripts for its init block)
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        fileServer.ServeHTTP(w, r)
    }
}
```

## Developer Self-Service (Main Site)

Route: `/dashboard/developer` (gated by `api.keys.self_manage` permission, on by default for all verified members)

```
/dashboard/developer
  ├─ My Apps           — list of user's api_keys (app_name, key_prefix, scopes, status, last_used_at)
  ├─ Create App        — choose name, environment, request scopes (subset of user's own max permission set)
  └─ Revoke            — user can revoke own keys; cannot change rate limit tier (admin only)
```

Max keys per user: `system_settings api.keys.max_per_user` (default 5). Admin can override per user.

## Control Panel — API Key Management

Route: `/control/api-keys`

| Panel | What admin can do |
|---|---|
| All Keys | Filter by user, tier, status, date. Export CSV. |
| Key Detail | View app_name, user, scopes, rate_limit_tier, request_count, last_used_at, notes |
| Actions | Suspend (reversible) / Revoke (permanent) / Change rate limit tier / Edit notes |
| Create Key | Create a key on behalf of any user or as a system key (user_id = NULL) |
| Rate Tiers | View/edit tier limits in system_settings (no redeploy needed) |
| Usage Analytics | Per-key request volume chart, top endpoints called, error rate |

Permission nodes for admin actions:
```
api.keys.view.all         — see all keys across all users
api.keys.manage           — suspend, revoke, change tier, create system keys
api.keys.analytics.view   — view usage analytics
```

## Error Response Format (consistent across all public API errors)

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Rate limit exceeded. Upgrade your plan or retry after 60 seconds.",
    "status": 429,
    "docs": "https://api.japaconnect.com/docs#section/Errors"
  }
}
```

## Release Placement

V1.2.0 — Operations & Safety phase (same release as Admin Impersonation). Depends on: Section 02 (auth), Section 03 (RBAC), Section 11 (Asynq — for async key metrics), Section 21 (Control Panel hosting the admin UI).

The `api.japaconnect.com` subdomain is the sixth Nginx virtual host. The Swagger UI `swagger-ui/` assets are pre-built from the official Swagger UI dist package and committed to the repo — no build step needed.

## See Also

Section 02 (auth token model), Section 03 (RBAC), Section 11 (Asynq analytics tasks), Section 21 (Control Panel API key panel), Section 44 (API Playground in docs.japaconnect.com). ADR-050.
