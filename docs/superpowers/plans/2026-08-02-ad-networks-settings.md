# Ad Networks Settings — Implementation Plan

> **For agentic workers:** Use this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add a full-stack ad network settings feature — database table, backend CRUD, admin UI page, and integration with the AdvertisementBlock component — so administrators can configure external ad networks (Google AdSense, Ezoic, Media.net, custom) with API keys, client IDs, and status.

**Architecture:** New `ad_networks` table in PostgreSQL with tenant-scoped CRUD. Backend follows the existing advertising package pattern (handlers→service→store→sqlc). Admin UI uses the established DataTable + Modal pattern from slot-types. The `AdvertisementBlock` component reads network config from the backend instead of hardcoded values.

**Tech Stack:** Go (chi router, pgx/v5, sqlc, asynq), Svelte 5 (Runes, Bits UI, Tailwind v4), PostgreSQL 18, Valkey caching.

## Global Constraints

- Multi-tenant: every query carries `tenant_id`
- RBAC: all admin routes gated by `ad.network.manage` permission node
- Standard API envelope: `{"data": ...}` / `{"error": {"code": "...", "message": "..."}}`
- SQLC generated files: never hand-edit `internal/db/` — edit `internal/db/queries/advertising.sql` only
- Migration naming: `000238_ad_networks.{up,down}.sql` (next number after 000237)
- Rule 16: all frontend data from API, no hardcoded/mock data
- Rule 17: no AI attribution in commits or code
- Ref: `.jeos/sections/25-ad-placement.md`

---

## Task 1: Database Migration — ad_networks Table

**Files:**
- Create: `migrations/000238_ad_networks.up.sql`
- Create: `migrations/000238_ad_networks.down.sql`

- [ ] **Step 1: Create up migration**

```sql
CREATE TABLE ad_networks (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    provider        text NOT NULL CHECK (provider IN ('adsense', 'ezoic', 'media_net', 'custom')),
    name            text NOT NULL,
    client_id       text,
    api_key         text,
    slot_mapping    jsonb DEFAULT '{}',
    config          jsonb DEFAULT '{}',
    is_active       boolean NOT NULL DEFAULT false,
    priority        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, provider)
);

CREATE INDEX idx_ad_networks_tenant ON ad_networks (tenant_id);

CREATE OR REPLACE FUNCTION update_ad_networks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ad_networks_updated_at
    BEFORE UPDATE ON ad_networks
    FOR EACH ROW EXECUTE FUNCTION update_ad_networks_updated_at();
```

- [ ] **Step 2: Create down migration**

```sql
DROP TRIGGER IF EXISTS trg_ad_networks_updated_at ON ad_networks;
DROP FUNCTION IF EXISTS update_ad_networks_updated_at();
DROP TABLE IF EXISTS ad_networks;
```

- [ ] **Step 3: Run migration and verify**

---

## Task 2: SQLC Queries — Ad Network CRUD

**Files:**
- Modify: `internal/db/queries/advertising.sql` (append at end)

- [ ] **Step 1: Add SQL queries**

```sql
-- =====================
-- Ad Networks (Section 25 — external ad network configuration)
-- =====================

-- name: CreateAdNetwork :one
INSERT INTO ad_networks (
    tenant_id, provider, name, client_id, api_key, slot_mapping, config, is_active, priority
) VALUES (
    @tenant_id, @provider, @name, sqlc.narg(client_id), sqlc.narg(api_key),
    @slot_mapping, @config, @is_active, @priority
)
RETURNING *;

-- name: GetAdNetworkByID :one
SELECT * FROM ad_networks WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetAdNetworkByProvider :one
SELECT * FROM ad_networks WHERE tenant_id = @tenant_id AND provider = @provider;

-- name: ListAdNetworks :many
SELECT * FROM ad_networks WHERE tenant_id = @tenant_id
ORDER BY priority DESC, name LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveAdNetworks :many
SELECT * FROM ad_networks WHERE tenant_id = @tenant_id AND is_active = true
ORDER BY priority DESC, name;

-- name: UpdateAdNetwork :one
UPDATE ad_networks
SET name = @name, client_id = sqlc.narg(client_id), api_key = sqlc.narg(api_key),
    slot_mapping = @slot_mapping, config = @config, is_active = @is_active, priority = @priority
WHERE id = @id AND tenant_id = @tenant_id RETURNING *;

-- name: DeleteAdNetwork :exec
DELETE FROM ad_networks WHERE id = @id AND tenant_id = @tenant_id;

-- name: CountAdNetworks :one
SELECT COUNT(*)::int FROM ad_networks WHERE tenant_id = @tenant_id;
```

- [ ] **Step 2: Run `make sqlc-gen`**
- [ ] **Step 3: Verify `go build ./internal/db/gen/...`**

---

## Task 3: Backend Store Layer

**Files:**
- Modify: `internal/advertising/store.go` (append)

- [ ] **Step 1: Add type alias and store methods**

```go
type AdNetwork = gen.AdNetwork

func (s *Store) createAdNetwork(ctx context.Context, tenantID pgtype.UUID, input gen.CreateAdNetworkParams) (AdNetwork, error) {
    return s.queries.CreateAdNetwork(ctx, input)
}
func (s *Store) getAdNetworkByID(ctx context.Context, id, tenantID pgtype.UUID) (AdNetwork, error) {
    return s.queries.GetAdNetworkByID(ctx, gen.GetAdNetworkByIDParams{ID: id, TenantID: tenantID})
}
func (s *Store) getAdNetworkByProvider(ctx context.Context, tenantID pgtype.UUID, provider string) (AdNetwork, error) {
    return s.queries.GetAdNetworkByProvider(ctx, gen.GetAdNetworkByProviderParams{TenantID: tenantID, Provider: provider})
}
func (s *Store) listAdNetworks(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdNetwork, error) {
    if limit <= 0 || limit > MaxPageSize { limit = DefaultPageSize }
    return s.queries.ListAdNetworks(ctx, gen.ListAdNetworksParams{TenantID: tenantID, PageOffset: offset, PageLimit: limit})
}
func (s *Store) listActiveAdNetworks(ctx context.Context, tenantID pgtype.UUID) ([]AdNetwork, error) {
    return s.queries.ListActiveAdNetworks(ctx, tenantID)
}
func (s *Store) updateAdNetwork(ctx context.Context, id, tenantID pgtype.UUID, input gen.UpdateAdNetworkParams) (AdNetwork, error) {
    return s.queries.UpdateAdNetwork(ctx, input)
}
func (s *Store) deleteAdNetwork(ctx context.Context, id, tenantID pgtype.UUID) error {
    return s.queries.DeleteAdNetwork(ctx, gen.DeleteAdNetworkParams{ID: id, TenantID: tenantID})
}
func (s *Store) countAdNetworks(ctx context.Context, tenantID pgtype.UUID) (int, error) {
    return s.queries.CountAdNetworks(ctx, tenantID)
}
```

- [ ] **Step 2: Verify `go build ./internal/advertising/...`**

---

## Task 4: Backend Service Layer

**Files:**
- Modify: `internal/advertising/service.go` (append)

- [ ] **Step 1: Add input structs and service methods**

```go
type CreateAdNetworkInput struct {
    Provider    string          `json:"provider"`
    Name        string          `json:"name"`
    ClientID    *string         `json:"client_id,omitempty"`
    APIKey      *string         `json:"api_key,omitempty"`
    SlotMapping json.RawMessage `json:"slot_mapping"`
    Config      json.RawMessage `json:"config"`
    IsActive    bool            `json:"is_active"`
    Priority    int32           `json:"priority"`
}

type UpdateAdNetworkInput struct {
    Name        *string         `json:"name,omitempty"`
    ClientID    *string         `json:"client_id,omitempty"`
    APIKey      *string         `json:"api_key,omitempty"`
    SlotMapping json.RawMessage `json:"slot_mapping,omitempty"`
    Config      json.RawMessage `json:"config,omitempty"`
    IsActive    *bool           `json:"is_active,omitempty"`
    Priority    *int32          `json:"priority,omitempty"`
}

var validProviders = map[string]bool{
    "adsense": true, "ezoic": true, "media_net": true, "custom": true,
}
```

Service methods: CreateAdNetwork, GetAdNetwork, ListAdNetworks, ListActiveAdNetworks, UpdateAdNetwork, DeleteAdNetwork, CountAdNetworks. Validation at service layer, store access via s.store.xxx().

- [ ] **Step 2: Verify `go build ./internal/advertising/...`**

---

## Task 5: Backend HTTP Handlers

**Files:**
- Modify: `internal/advertising/handlers.go` (append)

- [ ] **Step 1: Add handler methods**

CreateAdNetwork, ListAdNetworks, GetAdNetwork, UpdateAdNetwork, DeleteAdNetwork. Each: tenant.FromContext(ctx), JSON decode, call service, writeJSON with envelope.

- [ ] **Step 2: Verify `go build ./internal/advertising/...`**

---

## Task 6: Route Registration

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add routes in advertising section**

```go
// Ad Networks (Section 25 — external ad network configuration)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
    Get("/admin/ads/networks", adHandlers.ListAdNetworks)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
    Post("/admin/ads/networks", adHandlers.CreateAdNetwork)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
    Get("/admin/ads/networks/{id}", adHandlers.GetAdNetwork)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
    Put("/admin/ads/networks/{id}", adHandlers.UpdateAdNetwork)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
    Delete("/admin/ads/networks/{id}", adHandlers.DeleteAdNetwork)
```

- [ ] **Step 2: Verify `go build ./...`**

---

## Task 7: RBAC Permission Node

**Files:**
- Modify: RBAC permissions seed

- [ ] **Step 1: Add `ad.network.manage` permission node**

- [ ] **Step 2: Verify build**

---

## Task 8: API Routes Catalog Update

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`

- [ ] **Step 1: Add routes**

```
-- Ad Networks
GET    /admin/ads/networks          — list all networks
POST   /admin/ads/networks          — create network
GET    /admin/ads/networks/{id}     — get network detail
PUT    /admin/ads/networks/{id}     — update network
DELETE /admin/ads/networks/{id}     — delete network
```

---

## Task 9: Admin API Client

**Files:**
- Modify: `apps/admin/src/lib/api.ts`

- [ ] **Step 1: Add adNetworksApi export**

```typescript
export const adNetworksApi = {
    list: (params?: string) => api<unknown>(`/admin/ads/networks${params ? `?${params}` : ''}`),
    get: (id: string) => api<unknown>(`/admin/ads/networks/${id}`),
    create: (body: { provider: string; name: string; client_id?: string; api_key?: string; slot_mapping?: Record<string,string>; config?: Record<string,unknown>; is_active?: boolean; priority?: number }) =>
        api('/admin/ads/networks', { method: 'POST', body }),
    update: (id: string, body: { name?: string; client_id?: string; api_key?: string; slot_mapping?: Record<string,string>; config?: Record<string,unknown>; is_active?: boolean; priority?: number }) =>
        api(`/admin/ads/networks/${id}`, { method: 'PUT', body }),
    delete: (id: string) => api(`/admin/ads/networks/${id}`, { method: 'DELETE' }),
};
```

---

## Task 10: Admin Sidebar Nav Link

**Files:**
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte`

- [ ] **Step 1: Add nav item after /ads/slot-types**

```typescript
{ path: "/ads/networks", label: t("nav.ad_networks", "Ad Networks"), icon: Globe, permission: "ad.network.manage" }
```

Import Globe from lucide-svelte if not already imported.

---

## Task 11: Admin Ad Networks Page

**Files:**
- Create: `apps/admin/src/routes/(app)/ads/networks/+page.svelte`

- [ ] **Step 1: Build DataTable page**

Columns: Provider (badge), Name, Client ID, Status, Priority, Actions
Create/Edit modal: Provider dropdown, Name, Client ID, API Key (masked), Slot Mapping (JSON), Config (JSON), Active toggle, Priority
Delete confirmation, bulk delete, permission gating (ad.network.manage), loading skeleton, Rule 16 compliant.

---

## Task 12: Public Ad Network Config Endpoint

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/advertising/handlers.go`

- [ ] **Step 1: Add `GET /ads/networks/config`**

Public endpoint (OptionalAuth) returning active networks' client_id, provider, slot_mapping, config (no api_key). Used by AdvertisementBlock.

---

## Task 13: AdvertisementBlock Integration

**Files:**
- Modify: `apps/main/src/lib/components/pagebuilder/blocks/AdvertisementBlock.svelte`

- [ ] **Step 1: Fetch from /ads/networks/config instead of hardcoded values**

Replace hardcoded ca-pub-0000000000000000 with dynamic client_id from API. Use slot_mapping to resolve slot IDs.

---

## Task 14: JEOS Spec Update

**Files:**
- Modify: `.jeos/sections/25-ad-placement.md`

- [ ] **Step 1: Add Ad Networks schema, admin routes, and public route docs**

---

## Task 15: Documentation Update

**Files:**
- Modify: relevant docs

- [ ] **Step 1: Document ad network configuration, slot_mapping, priority ordering**

---

## Verification

- [ ] `go build ./...` — clean
- [ ] `make sqlc-gen` — clean
- [ ] Admin panel: /ads/networks page loads
- [ ] Admin panel: CRUD operations work
- [ ] Main app: AdvertisementBlock renders with configured network
- [ ] JEOS spec and routes catalog updated
