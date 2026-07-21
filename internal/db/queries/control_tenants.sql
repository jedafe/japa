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
