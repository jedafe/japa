-- Professional Profiles — type definitions, profiles, members, invitations.

-- =====================
-- Profile Type Definitions CRUD
-- =====================

-- name: CreateProfessionalProfileType :one
INSERT INTO professional_profile_types (
    tenant_id, name, slug, icon, field_group_id, is_system
) VALUES (
    @tenant_id, @name, @slug, sqlc.narg(icon), sqlc.narg(field_group_id), @is_system
)
RETURNING id, tenant_id, name, slug, icon, field_group_id, is_system, created_at;

-- name: GetProfessionalProfileTypeByID :one
SELECT id, tenant_id, name, slug, icon, field_group_id, is_system, created_at
FROM professional_profile_types
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetProfessionalProfileTypeBySlug :one
SELECT id, tenant_id, name, slug, icon, field_group_id, is_system, created_at
FROM professional_profile_types
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: ListProfessionalProfileTypesByTenant :many
SELECT id, tenant_id, name, slug, icon, field_group_id, is_system, created_at
FROM professional_profile_types
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateProfessionalProfileType :one
UPDATE professional_profile_types
SET name = @name, icon = sqlc.narg(icon), field_group_id = sqlc.narg(field_group_id)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, name, slug, icon, field_group_id, is_system, created_at;

-- name: DeleteProfessionalProfileType :exec
DELETE FROM professional_profile_types
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Professional Profiles CRUD
-- =====================

-- name: CreateProfessionalProfile :one
INSERT INTO professional_profiles (
    tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verification_type, field_values
) VALUES (
    @tenant_id, @owner_user_id, @type_id, @name, @slug, sqlc.narg(tagline),
    sqlc.narg(description), sqlc.narg(logo_media_id), sqlc.narg(header_media_id),
    sqlc.narg(website_url), sqlc.narg(verification_type), @field_values
)
RETURNING id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at;

-- name: GetProfessionalProfileByID :one
SELECT id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at
FROM professional_profiles
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetProfessionalProfileBySlug :one
SELECT id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at
FROM professional_profiles
WHERE slug = @slug;

-- name: ListProfessionalProfilesByOwner :many
SELECT id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at
FROM professional_profiles
WHERE owner_user_id = @owner_user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListProfessionalProfilesByType :many
SELECT id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at
FROM professional_profiles
WHERE type_id = @type_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateProfessionalProfile :one
UPDATE professional_profiles
SET name = @name, tagline = sqlc.narg(tagline), description = sqlc.narg(description),
    logo_media_id = sqlc.narg(logo_media_id), header_media_id = sqlc.narg(header_media_id),
    website_url = sqlc.narg(website_url), verification_type = sqlc.narg(verification_type),
    status = @status, field_values = @field_values
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, owner_user_id, type_id, name, slug, tagline, description,
    logo_media_id, header_media_id, website_url, verified, verification_type,
    follower_count, status, field_values, created_at;

-- name: DeleteProfessionalProfile :exec
DELETE FROM professional_profiles
WHERE id = @id AND tenant_id = @tenant_id;

-- name: IncrementFollowerCount :exec
UPDATE professional_profiles
SET follower_count = follower_count + @delta
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Professional Profile Members
-- =====================

-- name: AddProfessionalProfileMember :exec
INSERT INTO professional_profile_members (profile_id, user_id, role, invited_by)
VALUES (@profile_id, @user_id, @role, sqlc.narg(invited_by))
ON CONFLICT (profile_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- name: RemoveProfessionalProfileMember :exec
DELETE FROM professional_profile_members
WHERE profile_id = @profile_id AND user_id = @user_id;

-- name: ListProfessionalProfileMembers :many
SELECT ppm.profile_id, ppm.user_id, ppm.role, ppm.invited_by, ppm.joined_at,
       u.username, u.email
FROM professional_profile_members ppm
JOIN users u ON u.id = ppm.user_id
WHERE ppm.profile_id = @profile_id
ORDER BY ppm.joined_at ASC;

-- name: UpdateProfessionalProfileMemberRole :exec
UPDATE professional_profile_members
SET role = @role
WHERE profile_id = @profile_id AND user_id = @user_id;

-- name: GetProfessionalProfileMember :one
SELECT profile_id, user_id, role, invited_by, joined_at
FROM professional_profile_members
WHERE profile_id = @profile_id AND user_id = @user_id;

-- name: IsProfessionalProfileMember :one
SELECT EXISTS(
    SELECT 1 FROM professional_profile_members
    WHERE profile_id = @profile_id AND user_id = @user_id
);

-- name: IsProfessionalProfileOwner :one
SELECT EXISTS(
    SELECT 1 FROM professional_profiles
    WHERE id = @profile_id AND owner_user_id = @user_id AND tenant_id = @tenant_id
);

-- =====================
-- Professional Profile Invitations
-- =====================

-- name: CreateProfessionalProfileInvitation :one
INSERT INTO professional_profile_invitations (
    profile_id, email, role, invited_by, token, expires_at
) VALUES (
    @profile_id, @email, @role, sqlc.narg(invited_by), @token, @expires_at
)
RETURNING id, profile_id, email, role, invited_by, token, expires_at, accepted_at, created_at;

-- name: GetProfessionalProfileInvitationByToken :one
SELECT id, profile_id, email, role, invited_by, token, expires_at, accepted_at, created_at
FROM professional_profile_invitations
WHERE token = @token AND accepted_at IS NULL;

-- name: AcceptProfessionalProfileInvitation :exec
UPDATE professional_profile_invitations
SET accepted_at = now()
WHERE id = @id AND accepted_at IS NULL;

-- name: ListProfessionalProfileInvitations :many
SELECT id, profile_id, email, role, invited_by, token, expires_at, accepted_at, created_at
FROM professional_profile_invitations
WHERE profile_id = @profile_id AND accepted_at IS NULL
ORDER BY created_at DESC;

-- name: DeleteExpiredInvitations :exec
DELETE FROM professional_profile_invitations
WHERE expires_at < now() AND accepted_at IS NULL;
