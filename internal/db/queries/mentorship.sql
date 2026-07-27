-- Mentorship program and match queries (DB-001: migrated from raw SQL to sqlc).

-- name: ListMentorshipPrograms :many
SELECT id, tenant_id, name, description, is_active, created_at, updated_at
FROM mentorship_programs
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMentorshipProgram :one
SELECT id, tenant_id, name, description, is_active, created_at, updated_at
FROM mentorship_programs
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateMentorshipProgram :one
INSERT INTO mentorship_programs (tenant_id, name, description, is_active)
VALUES (@tenant_id, @name, @description, true)
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: UpdateMentorshipProgram :one
UPDATE mentorship_programs
SET name = @name, description = @description, is_active = @is_active, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: ListMentorshipMatches :many
SELECT id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at
FROM mentorship_matches
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateMentorshipMatch :one
UPDATE mentorship_matches
SET status = @status, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at;
