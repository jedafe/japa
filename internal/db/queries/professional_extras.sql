-- Professional Profile extras — relocations, documents, and verifiers.
-- Field names mirror the frontend contract in apps/main/src/lib/api.ts.

-- =====================
-- Relocations
-- =====================

-- name: CreateProfessionalRelocation :one
INSERT INTO professional_relocations (
    tenant_id, profile_id, country, country_code, city, status, year
) VALUES (
    @tenant_id, @profile_id, @country, sqlc.narg(country_code), sqlc.narg(city), sqlc.narg(status), sqlc.narg(year)
)
RETURNING id, tenant_id, profile_id, country, country_code, city, status, year, created_at, updated_at;

-- name: GetProfessionalRelocationByID :one
SELECT id, tenant_id, profile_id, country, country_code, city, status, year, created_at, updated_at
FROM professional_relocations
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListProfessionalRelocationsByProfile :many
SELECT id, tenant_id, profile_id, country, country_code, city, status, year, created_at, updated_at
FROM professional_relocations
WHERE profile_id = @profile_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateProfessionalRelocation :one
-- PATCH semantics: omitted fields fall back to their existing values via COALESCE.
UPDATE professional_relocations
SET country = COALESCE(sqlc.narg(country), country),
    country_code = COALESCE(sqlc.narg(country_code), country_code),
    city = COALESCE(sqlc.narg(city), city),
    status = COALESCE(sqlc.narg(status), status),
    year = COALESCE(sqlc.narg(year), year),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, profile_id, country, country_code, city, status, year, created_at, updated_at;

-- name: DeleteProfessionalRelocation :exec
DELETE FROM professional_relocations
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Documents
-- =====================

-- name: CreateProfessionalDocument :one
INSERT INTO professional_documents (
    tenant_id, profile_id, title, document_type, description, file_url
) VALUES (
    @tenant_id, @profile_id, @title, @document_type,
    sqlc.narg(description), sqlc.narg(file_url)
)
RETURNING id, tenant_id, profile_id, title, document_type, description, file_url, verified, created_at, updated_at;

-- name: GetProfessionalDocumentByID :one
SELECT id, tenant_id, profile_id, title, document_type, description, file_url, verified, created_at, updated_at
FROM professional_documents
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListProfessionalDocumentsByProfile :many
SELECT id, tenant_id, profile_id, title, document_type, description, file_url, verified, created_at, updated_at
FROM professional_documents
WHERE profile_id = @profile_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateProfessionalDocument :one
-- PATCH semantics: omitted fields fall back to their existing values via COALESCE.
UPDATE professional_documents
SET title = COALESCE(sqlc.narg(title), title),
    document_type = COALESCE(sqlc.narg(document_type), document_type),
    description = COALESCE(sqlc.narg(description), description),
    file_url = COALESCE(sqlc.narg(file_url), file_url),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, profile_id, title, document_type, description, file_url, verified, created_at, updated_at;

-- name: DeleteProfessionalDocument :exec
DELETE FROM professional_documents
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Verifiers
-- =====================
-- user_name is resolved at render time via JOIN users; only user_id is stored.

-- name: CreateProfessionalVerifier :one
INSERT INTO professional_verifiers (
    tenant_id, profile_id, user_id, role, status
) VALUES (
    @tenant_id, @profile_id, @user_id, sqlc.narg(role), sqlc.narg(status)
)
ON CONFLICT (profile_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    updated_at = now()
RETURNING id, tenant_id, profile_id, user_id, role, status, created_at, updated_at;

-- name: ListProfessionalVerifiersByProfile :many
SELECT pv.id, pv.tenant_id, pv.profile_id, pv.user_id, pv.role, pv.status,
       pv.created_at, pv.updated_at, u.username AS user_name
FROM professional_verifiers pv
JOIN users u ON u.id = pv.user_id
WHERE pv.profile_id = @profile_id AND pv.tenant_id = @tenant_id
ORDER BY pv.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteProfessionalVerifier :exec
DELETE FROM professional_verifiers
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetProfessionalVerifierByID :one
SELECT id, tenant_id, profile_id, user_id, role, status, created_at, updated_at
FROM professional_verifiers
WHERE id = @id AND tenant_id = @tenant_id;
