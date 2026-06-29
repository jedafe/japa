# Section 22 — Admin Panel

**Status:** ✅ Locked

## Decided
University CRUD, Space type + custom field management. User management: tiered warning system (admin-configurable accumulation threshold), tiered bans (time-boxed or permanent, with immediate session invalidation), appeals (`appeals` table, gated by `moderation.appeals.review`, every resolution logged to Audit Log). Unified content moderation queue combining user reports and AI moderation flags, filterable, with bulk actions. Community-health analytics dashboard (signups, DAU/MAU, content volume, top spaces, geo distribution, moderation stats, search trends) — distinct from Control Panel's infrastructure-health dashboard. Notification broadcast (`broadcast_notifications`: scoped targeting, optional scheduling, batched fan-out for large audiences to avoid overwhelming the queue/WS layer in one shot).

## Amendment — Per-User AI Context Panel (post-audit)

A per-user panel in the existing user management area shows what Honcho has actually stored/derived for that user — read-only, plus the ability to delete an individual entry, but no editing (correcting a wrong AI inference by hand-editing memory invites trust issues; deleting and letting it re-derive is the safer mechanism). Gated by new permission nodes `ai.context.admin_view` and `ai.context.admin_delete_entry`, distinct from the `ai.context.*` category nodes in Section 03 that gate what feeds the AI in the first place. Intended use: support tickets and trust & safety review. See ADR-027.

## Moderation & Admin Schemas (audit pass 2 — referenced in prose, never had CREATE TABLE)

```sql
-- User warnings: issued by moderators; accumulates toward ban threshold
CREATE TABLE user_warnings (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  issued_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  reason       text NOT NULL,
  content_id   uuid REFERENCES contents(id) ON DELETE SET NULL,  -- triggering content if any
  is_active    boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz  -- optional expiry; NULL = permanent until manually cleared
);
CREATE INDEX idx_warnings_user ON user_warnings (user_id, is_active, created_at DESC);

-- User bans: time-boxed or permanent; immediate session invalidation on create
CREATE TABLE user_bans (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  issued_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  reason       text NOT NULL,
  ban_type     text NOT NULL CHECK (ban_type IN ('temporary', 'permanent')),
  expires_at   timestamptz,   -- NULL for permanent bans
  is_active    boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  lifted_at    timestamptz,
  lifted_by    uuid REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_bans_user   ON user_bans (user_id, is_active);
CREATE INDEX idx_bans_active ON user_bans (tenant_id, is_active, expires_at)
  WHERE is_active = true;
-- On ban create: DELETE all jc:refresh:{user_id}:* keys from Valkey (force re-login)
-- maintenance:check_expired_bans (low tier, every 30 min) lifts expired bans + re-enables users

-- Appeals: users contest bans or content removals
CREATE TABLE appeals (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  user_id         uuid NOT NULL REFERENCES users(id),
  subject_type    text NOT NULL CHECK (subject_type IN ('ban','content_removal','warning')),
  subject_id      uuid,                               -- ban_id / content_id / warning_id
  reason          text NOT NULL,
  status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','upheld','overturned','dismissed')),
  reviewed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
  resolution_note text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  resolved_at     timestamptz
);
CREATE INDEX idx_appeals_status ON appeals (tenant_id, status, created_at DESC);
CREATE INDEX idx_appeals_user   ON appeals (user_id, status);
-- One open appeal per subject: UNIQUE(user_id, subject_type, subject_id) WHERE status='pending'

-- Blog editorial review feedback (Section 24)
CREATE TABLE content_reviews (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision    text NOT NULL CHECK (decision IN ('approved','changes_requested','rejected')),
  feedback    text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_content_reviews ON content_reviews (content_id, created_at DESC);

-- Admin broadcast notifications (scoped, optional scheduling)
CREATE TABLE broadcast_notifications (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  title        text NOT NULL,
  body         text NOT NULL,
  target_scope text NOT NULL DEFAULT 'all'
                 CHECK (target_scope IN ('all','user_group','geo_space')),
  target_id    uuid,            -- group_id or space_id depending on target_scope
  send_at      timestamptz,     -- NULL = send immediately on create
  status       text NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft','scheduled','sending','sent','failed')),
  created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  sent_at      timestamptz
);
-- Fan-out via batched Asynq tasks to avoid overwhelming queue — never a single massive push
```
