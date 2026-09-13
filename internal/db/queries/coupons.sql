-- name: CreateAffiliateProvider :one
INSERT INTO affiliate_providers (name, slug, description, provider_type, logo_media_id, base_url, tracking_param, tracking_value, commission_rate, is_active, is_featured) VALUES (@name, @slug, sqlc.narg(description), @provider_type, sqlc.narg(logo_media_id), sqlc.narg(base_url), sqlc.narg(tracking_param), sqlc.narg(tracking_value), sqlc.narg(commission_rate), @is_active, @is_featured)
RETURNING id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, created_at, updated_at;

-- name: GetAffiliateProviderByID :one
SELECT id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, created_at, updated_at
FROM affiliate_providers
WHERE id = @id;

-- name: GetAffiliateProviderBySlug :one
SELECT id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, created_at, updated_at
FROM affiliate_providers
WHERE slug = @slug;

-- name: ListAffiliateProvidersByTenant :many
SELECT id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, created_at, updated_at
FROM affiliate_providers
WHERE (@is_active::bool IS NULL OR is_active = @is_active)
ORDER BY name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAffiliateProvider :one
UPDATE affiliate_providers
SET name = @name, description = sqlc.narg(description), provider_type = @provider_type,
 logo_media_id = sqlc.narg(logo_media_id), base_url = sqlc.narg(base_url),
 tracking_param = sqlc.narg(tracking_param), tracking_value = sqlc.narg(tracking_value),
 commission_rate = sqlc.narg(commission_rate), is_active = @is_active,
 is_featured = @is_featured, updated_at = now()
WHERE id = @id
RETURNING id, name, slug, description, provider_type, logo_media_id,
 base_url, tracking_param, tracking_value, commission_rate,
 is_active, is_featured, author_commission_pct, created_at, updated_at;

-- name: DeleteAffiliateProvider :exec
DELETE FROM affiliate_providers
WHERE id = @id;

-- name: CountCouponsByProvider :one
SELECT COUNT(*) AS count
FROM coupons
WHERE provider_id = @provider_id;

-- =====================
-- Coupons CRUD
-- =====================

-- name: CreateCoupon :one
INSERT INTO coupons (provider_id, coupon_type, title, description, code, discount_type, discount_value, currency, terms_url, referral_url, is_verified, status, valid_from, expires_at, created_by) VALUES (sqlc.narg(provider_id), @coupon_type, @title, sqlc.narg(description), @code, sqlc.narg(discount_type), sqlc.narg(discount_value), sqlc.narg(currency), sqlc.narg(terms_url), sqlc.narg(referral_url), @is_verified, @status, sqlc.narg(valid_from), sqlc.narg(expires_at), sqlc.narg(created_by))
RETURNING id, provider_id, coupon_type, title, description, code,
 discount_type, discount_value, currency, terms_url, referral_url,
 is_verified, status, valid_from, expires_at, total_clicks, total_copies,
 created_by, created_at, updated_at;

-- name: GetCouponByID :one
SELECT c.id, c.provider_id, c.coupon_type, c.title, c.description,
 c.code, c.discount_type, c.discount_value, c.currency, c.terms_url,
 c.referral_url, c.is_verified, c.status, c.valid_from, c.expires_at,
 c.total_clicks, c.total_copies, c.created_by, c.created_at, c.updated_at,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.id = @id;

-- name: GetCouponByCode :one
SELECT c.id, c.provider_id, c.coupon_type, c.title, c.description,
 c.code, c.discount_type, c.discount_value, c.currency, c.terms_url,
 c.referral_url, c.is_verified, c.status, c.valid_from, c.expires_at,
 c.total_clicks, c.total_copies, c.created_by, c.created_at, c.updated_at,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.code = @code;

-- name: ListCouponsByTenant :many
SELECT c.id, c.provider_id, c.coupon_type, c.title, c.description,
 c.code, c.discount_type, c.discount_value, c.currency, c.terms_url,
 c.referral_url, c.is_verified, c.status, c.valid_from, c.expires_at,
 c.total_clicks, c.total_copies, c.created_by, c.created_at, c.updated_at,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
 AND (sqlc.narg(coupon_type)::text IS NULL OR c.coupon_type = sqlc.narg(coupon_type))
 AND (sqlc.narg(provider_id)::uuid IS NULL OR c.provider_id = sqlc.narg(provider_id))
 AND (sqlc.narg(search)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(search) || '%' OR c.code ILIKE '%' || sqlc.narg(search) || '%')
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveCouponsByTenant :many
SELECT c.id, c.provider_id, c.coupon_type, c.title, c.description,
 c.code, c.discount_type, c.discount_value, c.currency, c.terms_url,
 c.referral_url, c.is_verified, c.status, c.valid_from, c.expires_at,
 c.total_clicks, c.total_copies, c.created_by, c.created_at, c.updated_at,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.status = 'active'
 AND (c.expires_at IS NULL OR c.expires_at > now())
 AND (sqlc.narg(provider_id)::uuid IS NULL OR c.provider_id = sqlc.narg(provider_id))
 AND (sqlc.narg(coupon_type)::text IS NULL OR c.coupon_type = sqlc.narg(coupon_type))
 AND (sqlc.narg(search)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(search) || '%')
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateCoupon :one
UPDATE coupons
SET title = @title, description = sqlc.narg(description), code = @code,
 provider_id = sqlc.narg(provider_id),
 discount_type = sqlc.narg(discount_type), discount_value = sqlc.narg(discount_value),
 currency = sqlc.narg(currency), terms_url = sqlc.narg(terms_url),
 referral_url = sqlc.narg(referral_url), is_verified = @is_verified,
 status = @status, valid_from = sqlc.narg(valid_from),
 expires_at = sqlc.narg(expires_at), updated_at = now()
WHERE id = @id
RETURNING id, provider_id, coupon_type, title, description, code,
 discount_type, discount_value, currency, terms_url, referral_url,
 is_verified, status, valid_from, expires_at, total_clicks, total_copies,
 created_by, created_at, updated_at;

-- name: DeleteCoupon :exec
DELETE FROM coupons
WHERE id = @id;

-- name: ExpireCoupons :exec
UPDATE coupons
SET status = 'expired', updated_at = now()
WHERE status = 'active'
 AND expires_at IS NOT NULL
 AND expires_at < now();

-- name: InvalidateCouponCache :exec
-- Used by Asynq task to signal cache invalidation
SELECT 1;

-- =====================
-- Coupon Events
-- =====================

-- name: CreateCouponEvent :one
INSERT INTO coupon_events (coupon_id, event_type, user_id, content_id) VALUES (@coupon_id, @event_type, sqlc.narg(user_id), sqlc.narg(content_id))

RETURNING id, coupon_id, event_type, user_id, content_id, created_at;

-- name: ListCouponEvents :many
SELECT id, coupon_id, event_type, user_id, content_id, created_at
FROM coupon_events
WHERE coupon_id = @coupon_id
 AND (@event_type::text IS NULL OR event_type = @event_type)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCouponEventsByType :one
SELECT COUNT(*) AS count
FROM coupon_events
WHERE coupon_id = @coupon_id AND event_type = @event_type;

-- name: BatchFlushCouponEvents :exec
-- Increment coupon counters from events, then delete processed events
UPDATE coupons
SET total_clicks = total_clicks + (
 SELECT COUNT(*) FROM coupon_events ce
 WHERE ce.coupon_id = coupons.id AND ce.event_type = 'click'
 ),
 total_copies = total_copies + (
 SELECT COUNT(*) FROM coupon_events ce
 WHERE ce.coupon_id = coupons.id AND ce.event_type = 'copy'
 )
WHERE coupons.id = @coupon_id;

-- name: DeleteProcessedCouponEvents :exec
DELETE FROM coupon_events
WHERE coupon_id = @coupon_id AND event_type = @event_type;

-- =====================
-- Render Helpers
-- =====================

-- name: GetCouponForRender :one
-- BE-LOW-001: tenant-scoped so a render request cannot resolve a coupon
-- from another tenant. The InternalGuard on the render routes is defense-
-- in-depth; this query is the primary tenant isolation control.
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value,
 c.referral_url, c.status, c.expires_at,
 ap.name AS provider_name, ap.slug AS provider_slug, ap.logo_media_id AS provider_logo_media_id
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.id = @id;

-- name: GetAffiliateProviderForRender :one
-- BE-LOW-001: tenant-scoped render resolution.
SELECT id, base_url, tracking_param, tracking_value, is_active
FROM affiliate_providers
WHERE id = @id AND is_active = true;

-- name: CountCouponsByTenant :one
SELECT COUNT(*) AS count
FROM coupons
;

-- name: UpdateCouponClickCounts :exec
UPDATE coupons c
SET total_clicks = (
 SELECT COUNT(*)::int FROM coupon_events ce
 WHERE ce.coupon_id = c.id AND ce.event_type = 'click'
),
 updated_at = now()
WHERE EXISTS (
 SELECT 1 FROM coupon_events ce
 WHERE ce.coupon_id = c.id AND ce.event_type = 'click'
);

-- name: UpdateCouponCopyCounts :exec
UPDATE coupons c
SET total_copies = (
 SELECT COUNT(*)::int FROM coupon_events ce
 WHERE ce.coupon_id = c.id AND ce.event_type = 'copy'
),
 updated_at = now()
WHERE EXISTS (
 SELECT 1 FROM coupon_events ce
 WHERE ce.coupon_id = c.id AND ce.event_type = 'copy'
);
