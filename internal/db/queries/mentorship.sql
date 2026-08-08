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
VALUES (@tenant_id, @name, @description, @is_active)
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: UpdateMentorshipProgram :one
UPDATE mentorship_programs
SET name = @name, description = @description, is_active = @is_active, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: ListMentorshipMatches :many
SELECT m.id, m.tenant_id, m.program_id, m.mentor_id, m.mentee_id, m.status, m.created_at, m.updated_at,
       p.name AS program_name,
       mu.username AS mentor_username,
       eu.username AS mentee_username
FROM mentorship_matches m
LEFT JOIN mentorship_programs p ON p.id = m.program_id
LEFT JOIN users mu ON mu.id = m.mentor_id
LEFT JOIN users eu ON eu.id = m.mentee_id
WHERE m.tenant_id = @tenant_id
ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateMentorshipMatch :one
UPDATE mentorship_matches
SET status = @status, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at;

-- name: DeleteMentorshipProgram :execrows
DELETE FROM mentorship_programs
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListMentorshipProgramsWithCounts :many
SELECT p.id, p.tenant_id, p.name, p.description, p.is_active, p.created_at, p.updated_at,
       COALESCE(mentor_counts.cnt, 0)::int AS mentor_count,
       COALESCE(mentee_counts.cnt, 0)::int AS mentee_count,
       COALESCE(match_counts.cnt, 0)::int AS match_count
FROM mentorship_programs p
LEFT JOIN (
    SELECT mm.program_id, COUNT(DISTINCT mm.mentor_id) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) mentor_counts ON mentor_counts.program_id = p.id
LEFT JOIN (
    SELECT mm.program_id, COUNT(DISTINCT mm.mentee_id) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) mentee_counts ON mentee_counts.program_id = p.id
LEFT JOIN (
    SELECT mm.program_id, COUNT(*) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) match_counts ON match_counts.program_id = p.id
WHERE p.tenant_id = @tenant_id
ORDER BY p.created_at DESC
LIMIT @page_limit OFFSET @page_offset;
