-- Control Panel: tenant management (catalog: /control/tenants/*, super-admin only).

-- name: ListTenants :many
SELECT id, name, slug, domain, status, created_at, updated_at, deleted_at
FROM tenants
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: GetTenantByID :one
SELECT id, name, slug, domain, status, created_at, updated_at, deleted_at
FROM tenants
WHERE id = @id;

-- name: CreateTenant :one
INSERT INTO tenants (name, slug, domain)
VALUES (@name, @slug, @domain)
RETURNING id, name, slug, domain, status, created_at, updated_at, deleted_at;

-- name: UpdateTenant :one
UPDATE tenants
SET name = @name, domain = @domain, updated_at = now()
WHERE id = @id
RETURNING id, name, slug, domain, status, created_at, updated_at, deleted_at;

-- name: DeleteTenant :one
UPDATE tenants
SET status = 'deleted', deleted_at = now(), updated_at = now()
WHERE id = @id AND status != 'deleted'
RETURNING id, name, slug, domain, status, created_at, updated_at, deleted_at;

-- name: SetTenantStatus :one
UPDATE tenants
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING id, name, slug, domain, status, created_at, updated_at, deleted_at;

-- name: ListActiveTenantIDs :many
SELECT id FROM tenants WHERE status = 'active' ORDER BY id;

-- name: GetTenantStatus :one
SELECT status FROM tenants WHERE id = @id;

-- name: HardDeleteTenant :exec
DELETE FROM tenants WHERE id = @id;

-- name: DeleteTenantData :execrows
-- Deletes all tenant-scoped rows from tables WITHOUT ON DELETE CASCADE on
-- tenants(id), in FK dependency order. Called before HardDeleteTenant to
-- avoid FK violations. Tables with CASCADE from tenants (contents, feeds,
-- spaces, notifications, tickets, payments, enrollments, coupons, etc.)
-- are auto-deleted by PostgreSQL and intentionally omitted.
DELETE FROM learning_certificates WHERE tenant_id = @tenant_id;
DELETE FROM ownership_claims WHERE tenant_id = @tenant_id;
DELETE FROM ai_hitl_reviews WHERE tenant_id = @tenant_id;
DELETE FROM affiliate_link_events WHERE tenant_id = @tenant_id;
DELETE FROM affiliate_program_members WHERE tenant_id = @tenant_id;
DELETE FROM api_keys WHERE tenant_id = @tenant_id;
DELETE FROM doc_pages WHERE tenant_id = @tenant_id;
DELETE FROM lesson_progress WHERE tenant_id = @tenant_id;
DELETE FROM learning_lessons WHERE tenant_id = @tenant_id;
DELETE FROM learning_modules WHERE tenant_id = @tenant_id;
DELETE FROM learning_tracks WHERE tenant_id = @tenant_id;
DELETE FROM moderation_alerts WHERE tenant_id = @tenant_id;
DELETE FROM onboarding_state WHERE tenant_id = @tenant_id;
DELETE FROM organization_invitations WHERE tenant_id = @tenant_id;
DELETE FROM organizations WHERE tenant_id = @tenant_id;
DELETE FROM search_analytics WHERE tenant_id = @tenant_id;
DELETE FROM trending_snapshots WHERE tenant_id = @tenant_id;
DELETE FROM user_affiliate_links WHERE tenant_id = @tenant_id;
DELETE FROM user_preferences WHERE tenant_id = @tenant_id;
DELETE FROM user_profiles WHERE tenant_id = @tenant_id;
DELETE FROM users WHERE tenant_id = @tenant_id;
DELETE FROM webhook_deliveries WHERE tenant_id = @tenant_id;
DELETE FROM webhook_endpoints WHERE tenant_id = @tenant_id;
