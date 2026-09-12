-- referral_v2.sql — Referral System V2 queries (Section 60, ADR-078).
-- Covers referral code CRUD, referral tracking, and admin oversight.
-- Ref: japaconnect-engineering/sections/60-referral-system.md
--      japaconnect-engineering/adr/ADR-078-referral-system.md

-- =====================================================================
-- Referral Codes
-- =====================================================================

-- name: GetReferralCodeByUserID :one
SELECT * FROM referral_codes
WHERE user_id = @user_id;

-- name: GetReferralCodeByCode :one
SELECT * FROM referral_codes
WHERE code = @code;

-- name: CreateReferralCode :one
INSERT INTO referral_codes (user_id, code) VALUES (@user_id, @code)

ON CONFLICT (user_id) DO NOTHING
RETURNING *;

-- name: IncrementReferralClickCount :exec
UPDATE referral_codes
SET click_count = click_count + 1
WHERE code = @code;

-- name: ListUserReferralCodes :many
SELECT * FROM referral_codes

ORDER BY created_at DESC;

-- =====================================================================
-- Referrals — tracking and status transitions
-- =====================================================================

-- name: CreateReferral :one
INSERT INTO referrals (referrer_id, referee_id, referral_code, status) VALUES (@referrer_id, @referee_id, @referral_code, 'pending')

ON CONFLICT (referee_id) DO NOTHING
RETURNING *;

-- name: GetReferralByID :one
SELECT * FROM referrals
WHERE id = @id;

-- name: GetReferralByReferee :one
SELECT * FROM referrals
WHERE referee_id = @referee_id;

-- name: ListReferralsByReferrer :many
SELECT * FROM referrals
WHERE referrer_id = @referrer_id
 AND (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountReferralsByReferrer :one
SELECT COUNT(*) AS count
FROM referrals
WHERE referrer_id = @referrer_id
 AND (@status::text = '' OR status = @status);

-- name: CountCreditedReferralsByReferrerThisPeriod :one
SELECT COUNT(*) AS count
FROM referrals
WHERE referrer_id = @referrer_id
 AND status = 'credited'
 AND credited_at >= @period_start;

-- name: QualifyReferral :one
UPDATE referrals
SET
 status = 'qualified',
 qualified_payment_id = @qualified_payment_id,
 qualified_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- name: CreditReferral :one
UPDATE referrals
SET
 status = 'credited',
 reward_amount = @reward_amount,
 credited_at = now()
WHERE id = @id AND status = 'qualified'
RETURNING *;

-- name: CreditFlaggedReferral :one
-- Transitions a flagged referral to credited after admin review (§60
-- PATCH /admin/referrals/:id/resolve action=credit). CreditReferral only
-- accepts status='qualified'; flagged referrals need their own transition.
UPDATE referrals
SET
 status = 'credited',
 reward_amount = @reward_amount,
 credited_at = now(),
 flagged_reason = NULL
WHERE id = @id AND status = 'flagged'
RETURNING *;

-- name: FlagReferral :one
-- Status guard: only pending or qualified referrals can be flagged. This
-- prevents reverting an already-credited or rejected referral (§60 status
-- machine, BUG-012).
UPDATE referrals
SET
 status = 'flagged',
 flagged_reason = @flagged_reason
WHERE id = @id AND status IN ('pending', 'qualified')
RETURNING *;

-- name: RejectReferral :exec
UPDATE referrals
SET status = 'rejected'
WHERE id = @id;

-- =====================================================================
-- Admin oversight
-- =====================================================================

-- name: AdminListReferrals :many
SELECT * FROM referrals
WHERE (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountReferrals :one
SELECT COUNT(*) AS count
FROM referrals
WHERE (@status::text = '' OR status = @status);

-- name: AdminReferralStats :many
SELECT status, COUNT(*) AS count
FROM referrals

GROUP BY status
ORDER BY count DESC;

-- name: AdminListFlaggedReferrals :many
SELECT * FROM referrals
WHERE status = 'flagged'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountFlaggedReferrals :one
SELECT COUNT(*) AS count
FROM referrals
WHERE status = 'flagged';
