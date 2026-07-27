-- university.sql — Japa University creator economy queries (ADR-063).
-- Covers instructor_profiles, revenue_splits, instructor_payouts, and the
-- marketplace extensions to learning_tracks / enrollments. Course reviews
-- reuse the polymorphic reviews table (entity_type='learning_track') and are
-- served by the existing reviews package; only the aggregate read is here.
-- Ref: japaconnect-engineering/sections/36-learning-platform.md (Japa University)
--      japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md

-- =====================================================================
-- Instructor profiles
-- =====================================================================

-- name: UpsertInstructorApplication :one
-- A user may re-submit while pending/rejected; approved/suspended are frozen
-- from the self-service path (admin manages those states).
INSERT INTO instructor_profiles (
    tenant_id, user_id, display_name, headline, bio, social_links,
    tier, status, application_note
)
VALUES (@tenant_id, @user_id, @display_name, sqlc.narg(headline), sqlc.narg(bio), sqlc.narg(social_links),
        'standard', 'pending', sqlc.narg(application_note))
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET display_name      = EXCLUDED.display_name,
    headline          = COALESCE(EXCLUDED.headline, instructor_profiles.headline),
    bio               = COALESCE(EXCLUDED.bio, instructor_profiles.bio),
    social_links      = COALESCE(EXCLUDED.social_links, instructor_profiles.social_links),
    application_note  = COALESCE(EXCLUDED.application_note, instructor_profiles.application_note),
    status            = CASE WHEN instructor_profiles.status IN ('pending','rejected')
                             THEN 'pending' ELSE instructor_profiles.status END,
    updated_at        = now()
RETURNING *;

-- name: GetInstructorProfileByUser :one
SELECT * FROM instructor_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetInstructorProfileByID :one
SELECT * FROM instructor_profiles
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListInstructorProfilesByStatus :many
SELECT * FROM instructor_profiles
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountInstructorProfilesByStatus :one
SELECT COUNT(*) FROM instructor_profiles
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- name: UpdateInstructorProfileSelf :one
-- Creator-editable fields only. tier/status/commission override are admin-only.
UPDATE instructor_profiles
SET display_name = COALESCE(@display_name, display_name),
    headline     = COALESCE(sqlc.narg(headline), headline),
    bio          = COALESCE(sqlc.narg(bio), bio),
    social_links = COALESCE(sqlc.narg(social_links), social_links),
    payout_method = COALESCE(sqlc.narg(payout_method), payout_method),
    updated_at   = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id
  AND status IN ('pending','approved')
RETURNING *;

-- name: ApproveInstructorApplication :one
UPDATE instructor_profiles
SET status      = 'approved',
    reviewed_by = sqlc.narg(reviewed_by),
    reviewed_at = now(),
    tier        = COALESCE(sqlc.narg(tier), tier),
    commission_pct_override = sqlc.narg(commission_pct_override),
    updated_at  = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'pending'
RETURNING *;

-- name: RejectInstructorApplication :one
UPDATE instructor_profiles
SET status      = 'rejected',
    reviewed_by = sqlc.narg(reviewed_by),
    reviewed_at = now(),
    updated_at  = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'pending'
RETURNING *;

-- name: SuspendInstructor :one
UPDATE instructor_profiles
SET status      = 'suspended',
    reviewed_by = sqlc.narg(reviewed_by),
    reviewed_at = now(),
    updated_at  = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'approved'
RETURNING *;

-- name: ReinstateInstructor :one
-- Reverse of suspend: an admin restores a suspended instructor to approved.
UPDATE instructor_profiles
SET status      = 'approved',
    reviewed_by = sqlc.narg(reviewed_by),
    reviewed_at = now(),
    updated_at  = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'suspended'
RETURNING *;

-- name: GetPublicInstructorProfile :one
-- Public marketplace view: only approved instructors, no payout_method/KYC.
SELECT id, tenant_id, user_id, display_name, headline, bio, social_links,
       tier, status, created_at, updated_at
FROM instructor_profiles
WHERE tenant_id = @tenant_id AND id = @id AND status = 'approved';

-- =====================================================================
-- Marketplace tracks (learning_tracks extensions)
-- =====================================================================

-- name: ListUniversityCatalog :many
SELECT lt.id, lt.tenant_id, lt.title, lt.slug, lt.description,
       lt.difficulty, lt.cover_media_id, lt.instructor_id, lt.price_kobo,
       lt.currency, lt.category, lt.language, lt.geo_space_id, lt.total_enrolled,
       lt.created_at, lt.updated_at,
       ip.display_name AS instructor_name,
       COALESCE(ra.average_rating, 0)  AS avg_rating,
       COALESCE(ra.review_count, 0) AS review_count
FROM learning_tracks lt
LEFT JOIN instructor_profiles ip ON ip.id = lt.instructor_id
LEFT JOIN review_aggregates ra ON ra.entity_type = 'learning_track'
                              AND ra.entity_id = lt.id
                              AND ra.tenant_id = lt.tenant_id
WHERE lt.tenant_id = @tenant_id
  AND lt.is_published_to_university = true
  AND (@category::text = '' OR lt.category = @category)
  AND (@language::text = '' OR lt.language = @language)
  AND (@is_free::bool = false OR lt.price_kobo IS NULL)
ORDER BY lt.total_enrolled DESC, lt.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUniversityCatalog :one
SELECT COUNT(*) FROM learning_tracks
WHERE tenant_id = @tenant_id
  AND is_published_to_university = true
  AND (@category::text = '' OR category = @category)
  AND (@language::text = '' OR language = @language)
  AND (@is_free::bool = false OR price_kobo IS NULL);

-- name: GetUniversityTrackBySlug :one
SELECT lt.id, lt.tenant_id, lt.title, lt.slug, lt.description,
       lt.difficulty, lt.cover_media_id, lt.instructor_id, lt.price_kobo,
       lt.currency, lt.category, lt.language, lt.geo_space_id, lt.total_enrolled,
       lt.is_published, lt.is_published_to_university,
       lt.created_at, lt.updated_at,
       ip.display_name  AS instructor_name,
       ip.headline      AS instructor_headline,
       ip.tier          AS instructor_tier,
       COALESCE(ra.average_rating, 0)   AS avg_rating,
       COALESCE(ra.review_count, 0) AS review_count
FROM learning_tracks lt
LEFT JOIN instructor_profiles ip ON ip.id = lt.instructor_id
LEFT JOIN review_aggregates ra ON ra.entity_type = 'learning_track'
                              AND ra.entity_id = lt.id
                              AND ra.tenant_id = lt.tenant_id
WHERE lt.tenant_id = @tenant_id AND lt.slug = @slug
  AND lt.is_published_to_university = true;

-- name: CreateUniversityTrack :one
INSERT INTO learning_tracks (
    tenant_id, title, slug, description, difficulty, cover_media_id,
    is_published, sort_order, instructor_id, price_kobo, currency,
    is_published_to_university, category, language, geo_space_id
)
VALUES (
    @tenant_id, @title, @slug, sqlc.narg(description), @difficulty, sqlc.narg(cover_media_id),
    false, 0, sqlc.narg(instructor_id), sqlc.narg(price_kobo), COALESCE(sqlc.narg(currency), 'NGN'),
    false, sqlc.narg(category), COALESCE(sqlc.narg(language), 'en'), sqlc.narg(geo_space_id)
)
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id,
          is_published, sort_order, instructor_id, price_kobo, currency,
          is_published_to_university, category, language, geo_space_id, total_enrolled,
          created_at, updated_at;

-- name: UpdateUniversityTrack :one
-- Creator-editable fields. is_published_to_university is controlled by the
-- submit/approve flow, not by this query.
UPDATE learning_tracks
SET title       = COALESCE(@title, title),
    description = COALESCE(sqlc.narg(description), description),
    difficulty  = COALESCE(@difficulty, difficulty),
    cover_media_id = COALESCE(sqlc.narg(cover_media_id), cover_media_id),
    price_kobo  = COALESCE(sqlc.narg(price_kobo), price_kobo),
    currency    = COALESCE(sqlc.narg(currency), currency),
    category    = COALESCE(sqlc.narg(category), category),
    language    = COALESCE(sqlc.narg(language), language),
    geo_space_id = COALESCE(sqlc.narg(geo_space_id), geo_space_id),
    updated_at  = now()
WHERE tenant_id = @tenant_id AND id = @id AND instructor_id = @instructor_id
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id,
          is_published, sort_order, instructor_id, price_kobo, currency,
          is_published_to_university, category, language, geo_space_id, total_enrolled,
          created_at, updated_at;

-- name: SubmitUniversityTrack :one
-- Creator submits a track for marketplace approval. Resets to
-- is_published_to_university=false so a re-submit after a reject re-enters
-- the queue. The actual publish flag is set by ApproveUniversityTrack.
UPDATE learning_tracks
SET is_published_to_university = false,
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND instructor_id = @instructor_id
  AND is_published_to_university = false
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id,
          is_published, sort_order, instructor_id, price_kobo, currency,
          is_published_to_university, category, language, total_enrolled,
          created_at, updated_at;

-- name: ListUniversityTracksByInstructor :many
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, instructor_id, price_kobo, currency,
       is_published_to_university, category, language, geo_space_id, total_enrolled,
       created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id
ORDER BY created_at DESC;

-- name: ListPendingUniversityTracks :many
-- Tracks submitted for approval: a submitted track is one with an
-- instructor_id set, is_published_to_university=false, and is_published=true
-- (the creator has published the underlying track content; the marketplace
-- gate is the only thing pending).
SELECT lt.id, lt.tenant_id, lt.title, lt.slug, lt.description,
       lt.difficulty, lt.cover_media_id, lt.instructor_id, lt.price_kobo,
       lt.currency, lt.category, lt.language, lt.geo_space_id, lt.total_enrolled,
       lt.is_published, lt.is_published_to_university,
       lt.created_at, lt.updated_at,
       ip.display_name AS instructor_name
FROM learning_tracks lt
JOIN instructor_profiles ip ON ip.id = lt.instructor_id
WHERE lt.tenant_id = @tenant_id
  AND lt.instructor_id IS NOT NULL
  AND lt.is_published_to_university = false
  AND lt.is_published = true
  AND ip.status = 'approved'
ORDER BY lt.updated_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ApproveUniversityTrack :one
-- Admin approves a submitted track to the marketplace.
UPDATE learning_tracks
SET is_published_to_university = true,
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
  AND instructor_id IS NOT NULL
  AND is_published_to_university = false
RETURNING id, tenant_id, title, slug, description, difficulty, cover_media_id,
          is_published, sort_order, instructor_id, price_kobo, currency,
          is_published_to_university, category, language, geo_space_id, total_enrolled,
          created_at, updated_at;

-- name: GetUniversityTrackByID :one
SELECT id, tenant_id, title, slug, description, difficulty, cover_media_id,
       is_published, sort_order, instructor_id, price_kobo, currency,
       is_published_to_university, category, language, geo_space_id, total_enrolled,
       created_at, updated_at
FROM learning_tracks
WHERE tenant_id = @tenant_id AND id = @id;

-- name: IncrementTrackTotalEnrolled :exec
UPDATE learning_tracks
SET total_enrolled = total_enrolled + 1,
    updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id;

-- =====================================================================
-- Enrollments (paid + Pro-stream)
-- =====================================================================

-- name: UpsertEnrollmentWithPayment :one
-- Idempotent on (user_id, track_id): a re-process of the same payment does
-- not create a duplicate enrollment. payment_id is set only when provided.
INSERT INTO enrollments (tenant_id, user_id, track_id, access_type, payment_id)
VALUES (@tenant_id, @user_id, @track_id, @access_type, sqlc.narg(payment_id))
ON CONFLICT (user_id, track_id) DO UPDATE
SET payment_id = COALESCE(EXCLUDED.payment_id, enrollments.payment_id),
    access_type = CASE WHEN enrollments.access_type = 'purchase' THEN enrollments.access_type
                       ELSE EXCLUDED.access_type END
RETURNING id, tenant_id, user_id, track_id, access_type, progress_pct,
          enrolled_at, completed_at, payment_id;

-- name: GetEnrollmentByID :one
SELECT id, tenant_id, user_id, track_id, access_type, progress_pct,
       enrolled_at, completed_at, payment_id
FROM enrollments
WHERE tenant_id = @tenant_id AND id = @id;

-- name: HasEnrollmentForTrack :one
SELECT EXISTS (
    SELECT 1 FROM enrollments
    WHERE tenant_id = @tenant_id AND user_id = @user_id AND track_id = @track_id
) AS has_enrollment;

-- =====================================================================
-- Revenue splits (immutable per-event ledger)
-- =====================================================================

-- name: InsertRevenueSplit :one
INSERT INTO revenue_splits (
    tenant_id, instructor_id, track_id, enrollment_id, payment_id,
    source, gross_kobo, creator_pct, platform_pct, creator_kobo,
    platform_kobo, currency
)
VALUES (
    @tenant_id, @instructor_id, @track_id, sqlc.narg(enrollment_id), sqlc.narg(payment_id),
    @source, @gross_kobo, @creator_pct, @platform_pct, @creator_kobo,
    @platform_kobo, COALESCE(sqlc.narg(currency), 'NGN')
)
RETURNING *;

-- name: HasRevenueSplitForPayment :one
-- Idempotency guard for university:process_enrollment (AUDIT-BE-005).
SELECT EXISTS (
    SELECT 1 FROM revenue_splits
    WHERE tenant_id = @tenant_id AND payment_id = @payment_id
) AS has_split;

-- name: ListOpenRevenueSplitsByInstructor :many
SELECT * FROM revenue_splits
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id AND payout_id IS NULL
ORDER BY created_at ASC;

-- name: ListRevenueSplitsByInstructor :many
SELECT * FROM revenue_splits
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: SumOpenSplitsByInstructor :one
SELECT
    COALESCE(SUM(creator_kobo), 0)::bigint AS creator_kobo_total,
    COALESCE(SUM(platform_kobo), 0)::bigint AS platform_kobo_total,
    COALESCE(SUM(gross_kobo), 0)::bigint AS gross_kobo_total
FROM revenue_splits
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id AND payout_id IS NULL;

-- name: SumOpenSplitsByAllInstructors :many
SELECT
    instructor_id,
    COALESCE(SUM(creator_kobo), 0)::bigint AS creator_kobo_total,
    COALESCE(SUM(platform_kobo), 0)::bigint AS platform_kobo_total,
    COALESCE(SUM(gross_kobo), 0)::bigint AS gross_kobo_total
FROM revenue_splits
WHERE tenant_id = @tenant_id AND payout_id IS NULL
GROUP BY instructor_id;

-- name: StampRevenueSplitsWithPayout :execrows
UPDATE revenue_splits
SET payout_id = @payout_id
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id
  AND payout_id IS NULL
  AND created_at >= @period_start AND created_at < @period_end;

-- =====================================================================
-- Instructor payouts
-- =====================================================================

-- name: CreateInstructorPayout :one
INSERT INTO instructor_payouts (
    tenant_id, instructor_id, period_start, period_end,
    gross_kobo, platform_commission_kobo, net_kobo, currency, status
)
VALUES (
    @tenant_id, @instructor_id, @period_start, @period_end,
    @gross_kobo, @platform_commission_kobo, @net_kobo,
    COALESCE(sqlc.narg(currency), 'NGN'), 'pending'
)
ON CONFLICT (tenant_id, instructor_id, period_start, period_end) DO NOTHING
RETURNING *;

-- name: GetInstructorPayoutByID :one
SELECT * FROM instructor_payouts
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListInstructorPayoutsByInstructor :many
SELECT * FROM instructor_payouts
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id
ORDER BY period_start DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListInstructorPayoutsByStatus :many
SELECT * FROM instructor_payouts
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status)
ORDER BY created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdatePayoutStatus :one
UPDATE instructor_payouts
SET status            = @status,
    payout_reference  = COALESCE(sqlc.narg(payout_reference), payout_reference),
    failure_reason    = COALESCE(sqlc.narg(failure_reason), failure_reason),
    processed_at      = CASE WHEN @status IN ('paid','failed') THEN now() ELSE processed_at END
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- =====================================================================
-- Creator earnings summary (self)
-- =====================================================================

-- name: GetInstructorEarningsSummary :one
SELECT
    COALESCE(SUM(creator_kobo) FILTER (WHERE payout_id IS NULL), 0)::bigint  AS open_kobo,
    COALESCE(SUM(creator_kobo) FILTER (WHERE payout_id IS NOT NULL), 0)::bigint AS paid_kobo,
    COUNT(*)::bigint AS total_splits
FROM revenue_splits
WHERE tenant_id = @tenant_id AND instructor_id = @instructor_id;

-- =====================================================================
-- Geo personalization (ADR-064) — user override column
-- =====================================================================

-- name: GetUserContentCountryPreference :one
SELECT content_country_preference
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: SetUserContentCountryPreference :exec
UPDATE user_profiles
SET content_country_preference = sqlc.narg(content_country_preference),
    updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id;
