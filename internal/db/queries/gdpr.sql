-- GDPR / NDPR Compliance — Section 19

-- =====================
-- Consent Records
-- =====================

-- name: RecordConsent :one
INSERT INTO consent_records (
    tenant_id, user_id, consent_type, granted, policy_version, ip_address, user_agent
) VALUES (
    @tenant_id, sqlc.narg(user_id), @consent_type, @granted, @policy_version,
    sqlc.narg(ip_address), sqlc.narg(user_agent)
)
RETURNING id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at;

-- name: GetUserConsents :many
SELECT id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at
FROM consent_records
WHERE user_id = @user_id
  AND (@consent_type = '' OR consent_type = @consent_type)
ORDER BY created_at DESC;

-- name: HasConsent :one
SELECT granted
FROM consent_records
WHERE user_id = @user_id AND consent_type = @consent_type
ORDER BY created_at DESC
LIMIT 1;

-- name: RevokeConsent :exec
INSERT INTO consent_records (
    tenant_id, user_id, consent_type, granted, policy_version, ip_address, user_agent
) VALUES (
    @tenant_id, @user_id, @consent_type, false, @policy_version,
    sqlc.narg(ip_address), sqlc.narg(user_agent)
);

-- name: ListConsentsByTenant :many
SELECT id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at
FROM consent_records
WHERE tenant_id = @tenant_id
  AND (@consent_type::text = '' OR consent_type = @consent_type::text)
  AND (@q::text = '' OR consent_type ILIKE '%' || @q || '%' OR ip_address::text ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteAuditLog :exec
DELETE FROM audit_logs WHERE id = @id;

-- name: DeleteConsent :one
DELETE FROM consent_records
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at;

-- =====================
-- GDPR Data Export Queries
-- =====================

-- name: GdprExportUserContent :many
SELECT id, title, slug, content_type, status, created_at
FROM contents
WHERE author_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserComments :many
SELECT id, content_id, body, parent_id, created_at
FROM comments
WHERE author_user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserSpaces :many
SELECT s.id, s.name, s.slug, s.status, sm.role, sm.joined_at
FROM space_members sm
JOIN spaces s ON s.id = sm.space_id
WHERE sm.user_id = @user_id AND sm.tenant_id = @tenant_id
ORDER BY sm.joined_at DESC;

-- name: GdprExportUserPayments :many
SELECT id, provider, amount, currency, status, purpose_type, created_at
FROM payments
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserNotifications :many
SELECT id, notification_type, title, body, read_at, created_at
FROM notifications
WHERE recipient_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- =====================
-- Data Export Requests
-- =====================

-- name: CreateDataExportRequest :one
INSERT INTO data_export_requests (
    tenant_id, user_id, status
) VALUES (
    @tenant_id, @user_id, 'pending'
)
RETURNING id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at;

-- name: GetDataExportRequest :one
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetLatestDataExportRequestByUser :one
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateDataExportRequestStatus :one
UPDATE data_export_requests
SET status = @status,
    download_url = sqlc.narg(download_url),
    expires_at = sqlc.narg(expires_at),
    completed_at = sqlc.narg(completed_at)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at;

-- name: ListPendingDataExportRequests :many
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit;

-- =====================
-- Account Deletion Requests
-- =====================

-- name: CreateAccountDeletionRequest :one
INSERT INTO account_deletion_requests (
    tenant_id, user_id, status, reason
) VALUES (
    @tenant_id, @user_id, 'pending', sqlc.narg(reason)
)
RETURNING id, tenant_id, user_id, status, reason, completed_at, created_at;

-- name: GetAccountDeletionRequest :one
SELECT id, tenant_id, user_id, status, reason, completed_at, created_at
FROM account_deletion_requests
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateAccountDeletionRequestStatus :one
UPDATE account_deletion_requests
SET status = @status,
    completed_at = sqlc.narg(completed_at)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, status, reason, completed_at, created_at;

-- name: ListPendingDeletionRequests :many
SELECT id, tenant_id, user_id, status, reason, completed_at, created_at
FROM account_deletion_requests
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: AnonymizeUser :exec
UPDATE users
SET username = 'Deleted User',
    email = 'deleted-' || id::text,
    password_hash = '',
    profile_type = NULL,
    status = 'deleted',
    updated_at = now()
WHERE id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: HardDeleteUser :exec
-- Physically deletes the anonymized user row. ON DELETE CASCADE handles most
-- child tables; callers must SET NULL or reassign non-CASCADE FK references
-- before calling this.
DELETE FROM users
WHERE id = @user_id AND tenant_id = @tenant_id AND status = 'deleted';

-- name: ReassignContentAuthor :exec
-- Reassigns content authorship before user deletion to preserve thread continuity.
UPDATE contents SET author_id = @target_user_id
WHERE author_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyPaymentUserRefs :exec
-- Nulls payment-related user references for financial audit trail preservation.
UPDATE payments SET user_id = NULL
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyTicketSubmitter :exec
-- Nulls ticket submitter reference.
UPDATE tickets SET submitter_user_id = NULL
WHERE submitter_user_id = @user_id AND tenant_id = @tenant_id;

-- =====================
-- Audit Logs
-- =====================

-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address
) VALUES (
    @tenant_id, sqlc.narg(actor_user_id), sqlc.narg(impersonated_user_id),
    @action_type, sqlc.narg(target_type), sqlc.narg(target_id),
    @metadata, sqlc.narg(ip_address)
)
RETURNING id, tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address, created_at;

-- name: ListAuditLogs :many
SELECT id, tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address, created_at
FROM audit_logs
WHERE tenant_id = @tenant_id
  AND (@actor_user_id = '00000000-0000-0000-0000-000000000000'::uuid OR actor_user_id = @actor_user_id)
  AND (@action_type = '' OR action_type = @action_type)
  AND (sqlc.narg(target_type) IS NULL OR target_type = @target_type)
  AND (sqlc.narg(start_date) IS NULL OR created_at >= @start_date)
  AND (sqlc.narg(end_date) IS NULL OR created_at <= @end_date)
  AND (@q::text = '' OR action_type ILIKE '%' || @q || '%' OR target_type ILIKE '%' || @q || '%' OR target_id::text ILIKE '%' || @q || '%' OR actor_user_id::text ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
