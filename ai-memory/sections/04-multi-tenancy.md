# Section 04 — Multi-Tenancy

**Status:** ✅ Locked (expanded audit pass — tenants table schema added)

## Decided
`tenant_id` present on every scoped table from the first migration. Single tenant seeded at launch. This is baseline schema convention going forward — every new table needs `tenant_id` unless there's a documented reason it's intentionally global (e.g. `permissions`, `notification_types`, `known_bots`).

## Tenants Table

```sql
CREATE TABLE tenants (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL,
  slug        text NOT NULL UNIQUE,
  domain      text UNIQUE,              -- custom domain; NULL = use default domain
  plan        text NOT NULL DEFAULT 'launch',  -- 'launch'|'growth'|'enterprise'
  status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'trial')),
  meta        jsonb NOT NULL DEFAULT '{}',     -- arbitrary per-tenant config overrides
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- system_settings: per-tenant key/value config store
CREATE TABLE system_settings (
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  key         text NOT NULL,
  value       text NOT NULL,
  updated_by  uuid,          -- no FK to users: seeded before users exist
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, key)
);
-- Keys follow dot-notation namespaces: auth.password.min_length, feed.fanout_threshold, etc.
-- Valkey cache: jc:settings:{tenant_id} → JSON map of all keys. TTL 5 min, invalidated on write.
```

The single launch tenant is seeded in migration `00001_initial_setup.sql`. All subsequent tables FK into this tenant via `tenant_id`. Multi-tenant activation (adding a second tenant) requires no code changes — the schema is ready from day one.

## See Also
ADR-004. Section 02 (users table — tenant_id FK here), Section 03 (user_groups — tenant_id FK here).
