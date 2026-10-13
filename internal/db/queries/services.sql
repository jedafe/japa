-- =====================================================================
-- Service Directory (§98, ADR-106) + Service Packages & Bookings
-- (§111, ADR-123) queries.
--
-- Service listings live in contents (content_type='service_listing') with
-- structured fields in service_listing_meta. Packages are the same rows
-- with listing_kind='package' plus items/departures/bookings tables.
-- =====================================================================

-- ---------------------------------------------------------------------
-- service_listing_meta CRUD
-- ---------------------------------------------------------------------

-- name: CreateServiceListingMeta :one
INSERT INTO service_listing_meta (
    content_id, org_id, service_category, destination_space_id,
    city_space_id, origin_communities, is_remote, website_url, booking_url,
    price_range, is_verified, verification_type, claim_status, claimed_by_org_id,
    listing_kind, price_amount, price_currency, price_per, purchase_mode,
    capacity, available_from, available_until, faqs, steps,
    selling_points, facts, highlights, audience, included
) VALUES (
    @content_id, sqlc.narg(org_id), sqlc.narg(service_category),
    sqlc.narg(destination_space_id), sqlc.narg(city_space_id),
    sqlc.narg(origin_communities), sqlc.narg(is_remote),
    sqlc.narg(website_url), sqlc.narg(booking_url), sqlc.narg(price_range),
    sqlc.narg(is_verified), sqlc.narg(verification_type),
    COALESCE(sqlc.narg(claim_status), 'unclaimed'), sqlc.narg(claimed_by_org_id),
    COALESCE(sqlc.narg(listing_kind), 'service'), sqlc.narg(price_amount), sqlc.narg(price_currency),
    sqlc.narg(price_per), COALESCE(sqlc.narg(purchase_mode), 'inquiry'), sqlc.narg(capacity),
    sqlc.narg(available_from), sqlc.narg(available_until),
    COALESCE(sqlc.narg(faqs), '[]'::jsonb), COALESCE(sqlc.narg(steps), '[]'::jsonb),
    COALESCE(sqlc.narg(selling_points), '[]'::jsonb), COALESCE(sqlc.narg(facts), '[]'::jsonb),
    COALESCE(sqlc.narg(highlights), '[]'::jsonb), COALESCE(sqlc.narg(audience), '[]'::jsonb),
    COALESCE(sqlc.narg(included), '[]'::jsonb)
) RETURNING *;

-- name: GetServiceListingMetaByContentID :one
SELECT * FROM service_listing_meta
WHERE content_id = @content_id;

-- name: GetServiceListingMetaByID :one
SELECT * FROM service_listing_meta
WHERE id = @id;

-- Full-row update of the mutable §98/§111 fields. Verification and claim
-- columns are managed by dedicated queries below. The service layer merges
-- existing values with the incoming patch before calling this.
-- name: UpdateServiceListingMeta :one
UPDATE service_listing_meta
SET org_id = sqlc.narg(org_id),
    service_category = sqlc.narg(service_category),
    destination_space_id = sqlc.narg(destination_space_id),
    city_space_id = sqlc.narg(city_space_id),
    origin_communities = sqlc.narg(origin_communities),
    is_remote = sqlc.narg(is_remote),
    website_url = sqlc.narg(website_url),
    booking_url = sqlc.narg(booking_url),
    price_range = sqlc.narg(price_range),
    listing_kind = sqlc.narg(listing_kind),
    price_amount = sqlc.narg(price_amount),
    price_currency = sqlc.narg(price_currency),
    price_per = sqlc.narg(price_per),
    purchase_mode = sqlc.narg(purchase_mode),
    capacity = sqlc.narg(capacity),
    available_from = sqlc.narg(available_from),
    available_until = sqlc.narg(available_until),
    -- faqs/steps are replace-set jsonb arrays (nil keeps the stored set —
    -- the service layer merges the patch over the row before calling).
    faqs = COALESCE(sqlc.narg(faqs), faqs),
    steps = COALESCE(sqlc.narg(steps), steps),
    -- 000829 page-content fields — same replace-set/COALESCE semantics.
    selling_points = COALESCE(sqlc.narg(selling_points), selling_points),
    facts = COALESCE(sqlc.narg(facts), facts),
    highlights = COALESCE(sqlc.narg(highlights), highlights),
    audience = COALESCE(sqlc.narg(audience), audience),
    included = COALESCE(sqlc.narg(included), included),
    updated_at = now()
WHERE content_id = @content_id
RETURNING *;

-- ---------------------------------------------------------------------
-- Joined listing reads (contents + meta + spaces + orgs + author)
-- ---------------------------------------------------------------------

-- name: GetServiceListingBySlug :one
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at, c.featured_image_id,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       -- 000829 detail-page content fields (all listing kinds).
       sm.selling_points, sm.facts, sm.highlights, sm.included, sm.audience,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       -- 000829 mentor card fields — headline card, stats row, and the
       -- "Mentorship details" sidebar on the mentorship detail page.
       up.location, up.mentor_title, up.mentor_response_time,
       up.mentor_availability, up.mentor_session_type,
       up.mentor_languages, up.mentor_session_formats,
       up.mentor_countries, up.mentor_credentials,
       up.mentor_years_experience, up.mentor_success_label,
       -- Lifetime fulfilled bookings = "mentees helped" social proof
       -- (provider_user_id is only ever set on mentorship-kind rows).
       (SELECT COUNT(*)::int FROM service_bookings sb
        WHERE sb.provider_user_id = c.author_id
          AND sb.status = 'fulfilled') AS mentees_helped,
       fm.storage_bucket AS featured_storage_bucket,
       fm.storage_path AS featured_storage_path,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.slug = @slug AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
LIMIT 1;

-- name: GetServiceListingByContentID :one
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at, c.featured_image_id,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       -- 000829 detail-page content fields (all listing kinds).
       sm.selling_points, sm.facts, sm.highlights, sm.included, sm.audience,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       -- 000829 mentor card fields — headline card, stats row, and the
       -- "Mentorship details" sidebar on the mentorship detail page.
       up.location, up.mentor_title, up.mentor_response_time,
       up.mentor_availability, up.mentor_session_type,
       up.mentor_languages, up.mentor_session_formats,
       up.mentor_countries, up.mentor_credentials,
       up.mentor_years_experience, up.mentor_success_label,
       -- Lifetime fulfilled bookings = "mentees helped" social proof
       -- (provider_user_id is only ever set on mentorship-kind rows).
       (SELECT COUNT(*)::int FROM service_bookings sb
        WHERE sb.provider_user_id = c.author_id
          AND sb.status = 'fulfilled') AS mentees_helped,
       fm.storage_bucket AS featured_storage_bucket,
       fm.storage_path AS featured_storage_path,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.id = @content_id AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
LIMIT 1;

-- Mentor's shadow listing (000824): the listing_kind='mentorship' meta row
-- behind a mentor user — resolved when creating a mentorship-request
-- booking (meta_id feeds CreateServiceBooking.listing_meta_id) and when
-- PATCH /me/mentor-profile upserts the shadow. Same column shape as
-- GetServiceListingByContentID so the row maps onto the shared listing
-- domain row. Status is intentionally not filtered: preserved draft
-- shadows for non-live mentors still anchor their migrated bookings.
-- Returns no rows for users without a shadow listing.
-- name: GetMentorshipListingByAuthor :one
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at, c.featured_image_id,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       -- 000829 detail-page content fields (all listing kinds).
       sm.selling_points, sm.facts, sm.highlights, sm.included, sm.audience,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       -- 000829 mentor card fields — headline card, stats row, and the
       -- "Mentorship details" sidebar on the mentorship detail page.
       up.location, up.mentor_title, up.mentor_response_time,
       up.mentor_availability, up.mentor_session_type,
       up.mentor_languages, up.mentor_session_formats,
       up.mentor_countries, up.mentor_credentials,
       up.mentor_years_experience, up.mentor_success_label,
       -- Lifetime fulfilled bookings = "mentees helped" social proof
       -- (provider_user_id is only ever set on mentorship-kind rows).
       (SELECT COUNT(*)::int FROM service_bookings sb
        WHERE sb.provider_user_id = c.author_id
          AND sb.status = 'fulfilled') AS mentees_helped,
       fm.storage_bucket AS featured_storage_bucket,
       fm.storage_path AS featured_storage_path,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.author_id = @author_id
  AND c.content_type = 'service_listing'
  AND sm.listing_kind = 'mentorship'
  AND c.deleted_at IS NULL
-- Deterministic pick when a user somehow owns more than one shadow
-- (000824 created one per mentor, but nothing structurally forbids a
-- duplicate): prefer the row already anchoring the most bookings, then
-- the newest — an arbitrary LIMIT 1 would leave the ensure path updating
-- one shadow while the public surface serves another.
ORDER BY (
    SELECT COUNT(*) FROM service_bookings sb
    WHERE sb.listing_meta_id = sm.id
  ) DESC, c.id DESC
LIMIT 1;

-- ---------------------------------------------------------------------
-- Mentor shadow listing lifecycle (000824 / compat task). Shadow contents
-- rows are written directly — content.Service.CreateContent requires a
-- primary space for published rows and mentorship shadows are spaceless
-- by design (mirrors the 000824 migration INSERT, which also creates no
-- content_spaces row). ON CONFLICT DO NOTHING covers the partial uniques
-- on slug/content_hash (000745): a collision returns no row and the
-- service re-reads the winning shadow.
-- ---------------------------------------------------------------------

-- name: CreateMentorshipShadowContent :one
INSERT INTO contents (author_id, content_type, title, slug, body, body_html, status, published_at, content_hash)
VALUES (@author_id, 'service_listing', @title, @slug, @body, '', 'published', now(), @content_hash)
ON CONFLICT DO NOTHING
RETURNING id;

-- Refresh the mentor's shadow listing in place: title follows
-- display_name, body follows mentor_bio, content_hash follows Rule 12
-- (distinguisher 'mentor:<user_id>' matches the migration). Slug re-asserts
-- the canonical mentor-<username> form — a username change (or a shadow
-- minted with a suffix because the canonical slug was squatted) snaps
-- back once the slug frees up; the NOT EXISTS guard keeps the old slug
-- when another live row occupies the target so the update never 23505s
-- or hijacks an occupied slug. Status semantics: only a 'draft' shadow is
-- republished (preserved 000824 fallback rows for non-listed mentors); an
-- archived/moderated status is left alone so refresh can't silently
-- republish a deliberately unpublished row.
-- name: UpdateMentorshipShadowContent :execrows
UPDATE contents
SET title = @title,
    body = @body,
    content_hash = @content_hash,
    slug = CASE
        WHEN contents.slug <> @target_slug AND NOT EXISTS (
            SELECT 1 FROM contents o
            WHERE o.slug = @target_slug
              AND o.id <> @id
              AND o.deleted_at IS NULL
        ) THEN @target_slug
        ELSE contents.slug
    END,
    status = CASE WHEN status = 'draft' THEN 'published' ELSE status END,
    published_at = CASE
        WHEN status = 'draft' THEN COALESCE(published_at, now())
        ELSE published_at
    END,
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- Opt-out path: soft-delete every live mentorship shadow for the author.
-- Scoped by listing_kind='mentorship' so a mentor who also authors real
-- service listings keeps them. service_bookings keep pointing at the meta
-- row — the meta and deleted contents row survive for audit.
-- name: SoftDeleteMentorshipShadow :exec
UPDATE contents c
SET deleted_at = now(), updated_at = now()
WHERE c.author_id = @author_id
  AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM service_listing_meta sm
      WHERE sm.content_id = c.id AND sm.listing_kind = 'mentorship'
  );

-- Meta row for a freshly created shadow — NOT EXISTS-guarded so the ensure
-- path can run it inside the same tx for both create and update.
-- name: EnsureMentorshipShadowMeta :exec
INSERT INTO service_listing_meta (content_id, org_id, service_category, is_remote, listing_kind, purchase_mode)
SELECT @content_id, NULL, 'mentorship', true, 'mentorship', 'inquiry'
WHERE NOT EXISTS (
    SELECT 1 FROM service_listing_meta m WHERE m.content_id = @content_id
);

-- Public filtered list. Optional filters are sqlc.narg — NULL means
-- "no filter". sort='top' orders by engagement; anything else is
-- latest-first. kind filters listing_kind ('service'|'package');
-- price_max is minor units; date_from filters packages with
-- available_from >= the given date — a NULL available_from means the
-- package is always available and matches any date_from.
-- name: ListServiceListings :many
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at, c.featured_image_id,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       COALESCE(
           (SELECT MIN(t.price_amount) FROM service_package_tiers t
            WHERE t.listing_meta_id = sm.id AND t.is_active),
           sm.price_amount
       ) AS min_price,
       EXISTS(
           SELECT 1 FROM service_listing_coupons lc
           JOIN coupons cp ON cp.id = lc.coupon_id
           WHERE lc.listing_meta_id = sm.id
             AND cp.status = 'active'
             AND (cp.valid_from IS NULL OR cp.valid_from <= now())
             AND (cp.expires_at IS NULL OR cp.expires_at > now())
       ) AS has_coupon,
       fm.storage_bucket AS featured_storage_bucket,
       fm.storage_path AS featured_storage_path,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND c.status = COALESCE(sqlc.narg(status), 'published')
  AND (sqlc.narg(dest_space_id)::uuid IS NULL OR sm.destination_space_id = sqlc.narg(dest_space_id)::uuid)
  AND (sqlc.narg(city_space_id)::uuid IS NULL OR sm.city_space_id = sqlc.narg(city_space_id)::uuid)
  AND (sqlc.narg(service_category)::text IS NULL OR sm.service_category = sqlc.narg(service_category)::text)
  AND (sqlc.narg(origin_community)::text IS NULL OR sqlc.narg(origin_community)::text = ANY(sm.origin_communities))
  AND (sqlc.narg(is_verified)::bool IS NULL OR sm.is_verified = sqlc.narg(is_verified)::bool)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text)
  -- exclude_listing_kind (000824): the unfiltered directory pins out
  -- flag-gated kinds ('package' while service_packages.enabled is off,
  -- 'mentorship' while mentorship.enabled is off) without collapsing
  -- the list to a single kind.
  AND (sqlc.narg(exclude_listing_kind)::text IS NULL OR sm.listing_kind <> sqlc.narg(exclude_listing_kind)::text)
  AND (sqlc.narg(price_max)::bigint IS NULL OR sm.price_amount <= sqlc.narg(price_max)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR sm.available_from IS NULL OR sm.available_from >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN sqlc.narg(sort)::text = 'top' THEN c.view_count END DESC NULLS LAST,
  CASE WHEN sqlc.narg(sort)::text = 'top' THEN c.comment_count END DESC NULLS LAST,
  c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountServiceListings :one
SELECT COUNT(*)
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND c.status = COALESCE(sqlc.narg(status), 'published')
  AND (sqlc.narg(dest_space_id)::uuid IS NULL OR sm.destination_space_id = sqlc.narg(dest_space_id)::uuid)
  AND (sqlc.narg(city_space_id)::uuid IS NULL OR sm.city_space_id = sqlc.narg(city_space_id)::uuid)
  AND (sqlc.narg(service_category)::text IS NULL OR sm.service_category = sqlc.narg(service_category)::text)
  AND (sqlc.narg(origin_community)::text IS NULL OR sqlc.narg(origin_community)::text = ANY(sm.origin_communities))
  AND (sqlc.narg(is_verified)::bool IS NULL OR sm.is_verified = sqlc.narg(is_verified)::bool)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text)
  AND (sqlc.narg(exclude_listing_kind)::text IS NULL OR sm.listing_kind <> sqlc.narg(exclude_listing_kind)::text)
  AND (sqlc.narg(price_max)::bigint IS NULL OR sm.price_amount <= sqlc.narg(price_max)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR sm.available_from IS NULL OR sm.available_from >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%');

-- Admin moderation list — same join, any status when not filtered,
-- supports q + claim_status + kind filters.
-- name: AdminListServiceListings :many
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       -- M7: the admin queue stamps the same §114 derived badge as the
       -- public list — needs the live-coupon EXISTS and the rating
       -- rollup on the row.
       EXISTS(
           SELECT 1 FROM service_listing_coupons lc
           JOIN coupons cp ON cp.id = lc.coupon_id
           WHERE lc.listing_meta_id = sm.id
             AND cp.status = 'active'
             AND (cp.valid_from IS NULL OR cp.valid_from <= now())
             AND (cp.expires_at IS NULL OR cp.expires_at > now())
       ) AS has_coupon,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(dest_space_id)::uuid IS NULL OR sm.destination_space_id = sqlc.narg(dest_space_id)::uuid)
  AND (sqlc.narg(is_verified)::bool IS NULL OR sm.is_verified = sqlc.narg(is_verified)::bool)
  AND (sqlc.narg(claim_status)::text IS NULL OR sm.claim_status = sqlc.narg(claim_status)::text)
  AND (sqlc.narg(service_category)::text IS NULL OR sm.service_category = sqlc.narg(service_category)::text)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text)
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountServiceListings :one
SELECT COUNT(*)
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(dest_space_id)::uuid IS NULL OR sm.destination_space_id = sqlc.narg(dest_space_id)::uuid)
  AND (sqlc.narg(is_verified)::bool IS NULL OR sm.is_verified = sqlc.narg(is_verified)::bool)
  AND (sqlc.narg(claim_status)::text IS NULL OR sm.claim_status = sqlc.narg(claim_status)::text)
  AND (sqlc.narg(service_category)::text IS NULL OR sm.service_category = sqlc.narg(service_category)::text)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text)
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%');

-- Org's listings. ?status= filters; the public /org/:org_id/packages
-- route passes 'published' so in-review drafts never leak.
-- name: ListServiceListingsByOrg :many
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at, c.featured_image_id,
       sm.id AS meta_id,
       sm.org_id, o.name AS org_name, o.slug AS org_slug,
       sm.service_category,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.city_space_id, cs.name AS city_space_name, cs.geo_code AS city_space_code,
       sm.origin_communities, sm.is_remote, sm.website_url, sm.booking_url,
       sm.price_range, sm.is_verified, sm.verification_type,
       sm.claim_status, sm.claimed_by_org_id, co.name AS claimed_org_name,
       sm.verified_by, sm.verified_at,
       sm.listing_kind, sm.price_amount, sm.price_currency, sm.price_per,
       sm.purchase_mode, sm.capacity, sm.available_from, sm.available_until,
       sm.faqs, sm.steps,
       u.username AS author_name, up.avatar_url AS author_avatar,
       -- 000824 mentorship fields: the mentor block on listing payloads
       -- (listing_kind='mentorship') is read off the author profile join.
       up.display_name AS author_display_name,
       up.is_mentor, up.mentor_capacity, up.active_mentee_count,
       up.mentor_areas, up.mentor_since,
       COALESCE(
           (SELECT MIN(t.price_amount) FROM service_package_tiers t
            WHERE t.listing_meta_id = sm.id AND t.is_active),
           sm.price_amount
       ) AS min_price,
       EXISTS(
           SELECT 1 FROM service_listing_coupons lc
           JOIN coupons cp ON cp.id = lc.coupon_id
           WHERE lc.listing_meta_id = sm.id
             AND cp.status = 'active'
             AND (cp.valid_from IS NULL OR cp.valid_from <= now())
             AND (cp.expires_at IS NULL OR cp.expires_at > now())
       ) AS has_coupon,
       fm.storage_bucket AS featured_storage_bucket,
       fm.storage_path AS featured_storage_path,
       ra.review_count AS rating_count,
       ra.average_rating AS rating_avg
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
LEFT JOIN organizations o ON o.id = sm.org_id
LEFT JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN spaces cs ON cs.id = sm.city_space_id
LEFT JOIN organizations co ON co.id = sm.claimed_by_org_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN review_aggregates ra ON ra.entity_type = 'service_listing' AND ra.entity_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND sm.org_id = @org_id
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text)
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- M3: count twin of ListServiceListingsByOrg — same filter set so the
-- /org/:org_id/packages total reflects the full set, not the page length.
-- name: CountServiceListingsByOrg :one
SELECT COUNT(*)
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
WHERE c.content_type = 'service_listing'
  AND c.deleted_at IS NULL
  AND sm.org_id = @org_id
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(listing_kind)::text IS NULL OR sm.listing_kind = sqlc.narg(listing_kind)::text);

-- ---------------------------------------------------------------------
-- Verification + claim workflow (§98)
-- ---------------------------------------------------------------------

-- name: SetServiceListingVerified :execrows
UPDATE service_listing_meta
SET is_verified = true,
    verification_type = @verification_type,
    verified_by = @verified_by,
    verified_at = now(),
    updated_at = now()
WHERE content_id = @content_id;

-- name: ClearServiceListingVerified :execrows
UPDATE service_listing_meta
SET is_verified = false,
    verification_type = null,
    verified_by = null,
    verified_at = null,
    updated_at = now()
WHERE content_id = @content_id;

-- name: UpdateServiceListingClaimStatus :execrows
UPDATE service_listing_meta
SET claim_status = @claim_status,
    claimed_by_org_id = sqlc.narg(claimed_by_org_id),
    updated_at = now()
WHERE content_id = @content_id;

-- Atomically initiate a claim only when unclaimed (BUG-007) and unowned.
-- The claim flow exists for platform-seeded listings with no owning org —
-- a listing that already has org_id is owned, so claim must not let a
-- different org take it over on approval (ownership hijack).
-- listing_kind <> 'mentorship': mentor shadow listings are managed only
-- by the mentor-profile ensure path (000824) — an org must never claim a
-- personal mentor shadow.
-- name: ClaimServiceListingIfUnclaimed :execrows
UPDATE service_listing_meta
SET claim_status = 'pending', claimed_by_org_id = @org_id, updated_at = now()
WHERE content_id = @content_id AND claim_status = 'unclaimed' AND org_id IS NULL
  AND listing_kind <> 'mentorship';

-- Atomically resolve a pending claim (BUG-015). Rejection clears the
-- claimant; approval keeps claimed_by_org_id on the org AND links org_id —
-- the approved claim is the ownership transfer (§07 claim pattern):
-- without org_id the claiming org cannot edit the listing
-- (loadListingForEdit), receive inquiries/bookings (service_bookings.org_id
-- is NOT NULL), or appear as the provider on public payloads.
-- name: ApproveServiceClaimIfPending :execrows
UPDATE service_listing_meta
SET claim_status = 'approved', claimed_by_org_id = @org_id, org_id = @org_id, updated_at = now()
WHERE content_id = @content_id AND claim_status = 'pending'
  AND listing_kind <> 'mentorship';

-- name: RejectServiceClaimIfPending :execrows
-- Rejection keeps claimed_by_org_id: the claimant identity drives §22
-- appeal eligibility (members of the rejected org may appeal) and the
-- overturn path re-approves without re-capturing the org.
UPDATE service_listing_meta
SET claim_status = 'rejected', updated_at = now()
WHERE content_id = @content_id AND claim_status = 'pending';

-- Overturn a rejected claim via the §22 appeal flow — grants the claim
-- (approved + org_id transfer) only when the row is still rejected and
-- still carries the claimant.
-- name: ApproveServiceClaimOnAppeal :execrows
UPDATE service_listing_meta
SET claim_status = 'approved', org_id = claimed_by_org_id, updated_at = now()
WHERE content_id = @content_id
  AND claim_status = 'rejected'
  AND claimed_by_org_id IS NOT NULL
  AND listing_kind <> 'mentorship';

-- ---------------------------------------------------------------------
-- Org helpers
-- ---------------------------------------------------------------------

-- name: IsServiceOrgMember :one
SELECT EXISTS(
    SELECT 1 FROM organization_members
    WHERE organization_id = @org_id AND user_id = @user_id
) AS is_member;

-- §111 verified-org gate: packages require verified=true AND a
-- business/professional_body verification type.
-- name: GetServiceOrgVerification :one
SELECT verified, verification_type FROM organizations
WHERE id = @org_id;

-- Validate that a component link points to a service listing owned by
-- the same org (§111 package items). Returns kind too — a package may
-- not nest another package as a component.
-- name: GetServiceListingOrgIDByContentID :one
SELECT sm.org_id, sm.listing_kind
FROM service_listing_meta sm
JOIN contents c ON c.id = sm.content_id
WHERE sm.content_id = @content_id
  AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL;

-- ---------------------------------------------------------------------
-- Package items (§111) — ordered line items, replace-on-save
-- ---------------------------------------------------------------------

-- name: DeleteServicePackageItems :exec
DELETE FROM service_package_items
WHERE package_meta_id = @package_meta_id;

-- name: CreateServicePackageItem :one
INSERT INTO service_package_items (
    package_meta_id, position, title, description,
    component_content_id, included, optional
) VALUES (
    @package_meta_id, @position, @title,
    sqlc.narg(description), sqlc.narg(component_content_id),
    sqlc.narg(included), sqlc.narg(optional)
) RETURNING *;

-- name: ListServicePackageItems :many
SELECT * FROM service_package_items
WHERE package_meta_id = @package_meta_id
ORDER BY position, created_at;

-- ---------------------------------------------------------------------
-- Package departures (§111)
-- ---------------------------------------------------------------------

-- name: DeleteServicePackageDepartures :exec
DELETE FROM service_package_departures
WHERE package_meta_id = @package_meta_id;

-- Replace-on-save helper: delete departures EXCEPT those that already
-- carry bookings (their departure_id must survive — bookings keep the
-- slot association via ON DELETE SET NULL, but we preserve the row so the
-- booked_count/capacity accounting stays intact).
-- name: DeleteServicePackageDeparturesExcept :exec
DELETE FROM service_package_departures
WHERE package_meta_id = @package_meta_id
  AND id != ALL(sqlc.arg(keep_ids)::uuid[]);

-- name: ListDeparturesWithBookings :many
SELECT DISTINCT departure_id FROM service_bookings sb
JOIN service_package_departures d ON d.id = sb.departure_id
WHERE d.package_meta_id = @package_meta_id
  AND sb.departure_id IS NOT NULL;

-- name: CreateServicePackageDeparture :one
INSERT INTO service_package_departures (
    package_meta_id, starts_on, ends_on, capacity, status
) VALUES (
    @package_meta_id, @starts_on,
    sqlc.narg(ends_on), sqlc.narg(capacity), sqlc.narg(status)
) RETURNING *;

-- name: ListServicePackageDepartures :many
SELECT * FROM service_package_departures
WHERE package_meta_id = @package_meta_id
ORDER BY starts_on;

-- Open departures for the public detail page: open, not yet started.
-- name: ListOpenServicePackageDepartures :many
SELECT * FROM service_package_departures
WHERE package_meta_id = @package_meta_id
  AND status = 'open'
  AND starts_on >= CURRENT_DATE
ORDER BY starts_on;

-- name: GetServicePackageDeparture :one
SELECT * FROM service_package_departures
WHERE id = @id;

-- In-place departure update (§111 edit): keeps the row id so bookings
-- holding departure_id stay attached — delete+insert would SET NULL them.
-- Scoped to the owning package meta so a caller can't move rows between
-- packages. Returns no rows when the id doesn't belong to the package.
-- name: UpdateServicePackageDeparture :one
UPDATE service_package_departures
SET starts_on = @starts_on,
    ends_on = sqlc.narg(ends_on),
    capacity = sqlc.narg(capacity),
    status = sqlc.narg(status)
WHERE id = @id AND package_meta_id = @package_meta_id
RETURNING *;

-- Row lock for the capacity-guarded confirm path (§111).
-- name: GetServicePackageDepartureForUpdate :one
SELECT * FROM service_package_departures
WHERE id = @id
FOR UPDATE;

-- Atomic capacity check + increment + 'full' flip. Returns no rows when
-- the departure is not open or lacks capacity → service layer maps to
-- ErrDepartureFull (§111: 409 departure_full). §111 schema: a NULL
-- departure capacity inherits the package-level capacity (meta.capacity);
-- both NULL means unlimited.
-- name: IncrementDepartureBookedCount :one
UPDATE service_package_departures d
SET booked_count = d.booked_count + @party_size,
    status = CASE
        WHEN eff.capacity IS NOT NULL AND d.booked_count + @party_size >= eff.capacity
            THEN 'full'
        ELSE d.status
    END
FROM (SELECT d2.id, COALESCE(d2.capacity, sm.capacity) AS capacity
      FROM service_package_departures d2
      JOIN service_listing_meta sm ON sm.id = d2.package_meta_id
      WHERE d2.id = @id) eff
WHERE d.id = @id
  AND d.id = eff.id
  AND d.status = 'open'
  AND (eff.capacity IS NULL OR d.booked_count + @party_size <= eff.capacity)
RETURNING d.*;

-- name: DecrementDepartureBookedCount :execrows
UPDATE service_package_departures
SET booked_count = GREATEST(0, booked_count - @party_size),
    status = CASE
        WHEN status = 'full' AND (capacity IS NULL OR booked_count - @party_size < capacity)
            THEN 'open'
        ELSE status
    END
WHERE id = @id;

-- ---------------------------------------------------------------------
-- service_bookings (§111) — unified leads + bookings pipeline
-- ---------------------------------------------------------------------

-- name: CreateServiceBooking :one
-- provider_user_id/destination_space_id/meta are the mentorship-unification
-- columns (000824): provider-owned bookings (listing_kind='mentorship')
-- carry provider_user_id instead of org_id. All three are narg so legacy
-- org-path callers pass NULL/empty and keep working.
INSERT INTO service_bookings (
    listing_meta_id, departure_id, booker_user_id, org_id, intent,
    party_size, preferred_dates, note, status, purchase_mode,
    payment_id, total_price, fee_amount, fee_currency, agent_balance_due,
    expires_at, tier_id, coupon_id, coupon_code, discount_amount,
    tier_name, tier_price_amount,
    provider_user_id, destination_space_id, meta
) VALUES (
    @listing_meta_id, sqlc.narg(departure_id), @booker_user_id, sqlc.narg(org_id),
    @intent, sqlc.narg(party_size), sqlc.narg(preferred_dates),
    sqlc.narg(note), sqlc.narg(status), @purchase_mode,
    sqlc.narg(payment_id), sqlc.narg(total_price), sqlc.narg(fee_amount),
    sqlc.narg(fee_currency), sqlc.narg(agent_balance_due),
    sqlc.narg(expires_at), sqlc.narg(tier_id), sqlc.narg(coupon_id),
    sqlc.narg(coupon_code), sqlc.narg(discount_amount),
    sqlc.narg(tier_name), sqlc.narg(tier_price_amount),
    sqlc.narg(provider_user_id), sqlc.narg(destination_space_id),
    COALESCE(sqlc.narg(meta)::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetServiceBooking :one
SELECT * FROM service_bookings
WHERE id = @id;

-- Detail join for display: listing title/slug/content_id, org name,
-- departure dates. content_id feeds the jc:svc_package cache key so
-- seat-count transitions can invalidate the package detail cache.
-- name: GetServiceBookingDetail :one
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title,
       c.slug AS listing_slug,
       sm.listing_kind, sm.capacity AS package_capacity,
       o.name AS org_name, o.slug AS org_slug,
       p.status AS payment_status, p.provider AS payment_provider,
       d.starts_on AS departure_starts_on, d.ends_on AS departure_ends_on,
       u.username AS booker_username, up.display_name AS booker_display_name,
       up.avatar_url AS booker_avatar,
       pu.username AS provider_username, pup.display_name AS provider_display_name,
       pup.avatar_url AS provider_avatar,
       dsp.name AS destination_space_name,
       COALESCE(sb.tier_name, t.name, '') AS effective_tier_name
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
LEFT JOIN organizations o ON o.id = sb.org_id
LEFT JOIN payments p ON p.id = sb.payment_id
LEFT JOIN service_package_departures d ON d.id = sb.departure_id
LEFT JOIN service_package_tiers t ON t.id = sb.tier_id
LEFT JOIN users u ON u.id = sb.booker_user_id
LEFT JOIN user_profiles up ON up.user_id = sb.booker_user_id
-- Provider dimension (000824): user-owned (mentorship) bookings carry
-- provider_user_id instead of org_id — NULL columns for org-owned rows.
LEFT JOIN users pu ON pu.id = sb.provider_user_id
LEFT JOIN user_profiles pup ON pup.user_id = pu.id
LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id
WHERE sb.id = @id;

-- name: AttachServiceBookingPayment :one
UPDATE service_bookings
SET payment_id = @payment_id, updated_at = now()
WHERE id = @id
RETURNING *;

-- Backfill for the payment-success path: the booking may already have
-- left 'pending' (expired/cancelled during checkout) so there is no
-- status guard — only never overwrite an existing link.
-- name: BackfillServiceBookingPayment :one
UPDATE service_bookings
SET payment_id = @payment_id, updated_at = now()
WHERE id = @id AND payment_id IS NULL
RETURNING *;

-- Booker history. kind filters sm.listing_kind ('service'|'package'|
-- 'mentorship'); NULL means all kinds.
-- name: ListServiceBookingsByUser :many
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title, c.slug AS listing_slug,
       sm.listing_kind, o.name AS org_name, o.slug AS org_slug,
       p.status AS payment_status, p.provider AS payment_provider,
       d.starts_on AS departure_starts_on, d.ends_on AS departure_ends_on,
       pu.username AS provider_username, pup.display_name AS provider_display_name,
       pup.avatar_url AS provider_avatar,
       dsp.name AS destination_space_name,
       COALESCE(sb.tier_name, t.name, '') AS effective_tier_name
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
LEFT JOIN organizations o ON o.id = sb.org_id
LEFT JOIN payments p ON p.id = sb.payment_id
LEFT JOIN service_package_departures d ON d.id = sb.departure_id
LEFT JOIN service_package_tiers t ON t.id = sb.tier_id
LEFT JOIN users pu ON pu.id = sb.provider_user_id
LEFT JOIN user_profiles pup ON pup.user_id = pu.id
LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id
WHERE sb.booker_user_id = @booker_user_id
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text)
ORDER BY sb.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountServiceBookingsByUser :one
SELECT COUNT(*) FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
WHERE sb.booker_user_id = @booker_user_id
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text);

-- Org inbox — paid bookings only appear once money actually moved
-- (succeeded or refunded: a pending/failed payment never lands in the
-- org's work queue, but a refunded booking is still the org's audit
-- record of work that happened).
-- name: ListServiceBookingsByOrg :many
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title, c.slug AS listing_slug,
       sm.listing_kind, o.name AS org_name, o.slug AS org_slug,
       d.starts_on AS departure_starts_on, d.ends_on AS departure_ends_on,
       u.username AS booker_username, up.display_name AS booker_display_name,
       up.avatar_url AS booker_avatar,
       pu.username AS provider_username, pup.display_name AS provider_display_name,
       pup.avatar_url AS provider_avatar,
       dsp.name AS destination_space_name,
       COALESCE(sb.tier_name, t.name, '') AS effective_tier_name
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
LEFT JOIN organizations o ON o.id = sb.org_id
LEFT JOIN service_package_departures d ON d.id = sb.departure_id
LEFT JOIN payments p ON p.id = sb.payment_id
LEFT JOIN users u ON u.id = sb.booker_user_id
LEFT JOIN user_profiles up ON up.user_id = sb.booker_user_id
LEFT JOIN users pu ON pu.id = sb.provider_user_id
LEFT JOIN user_profiles pup ON pup.user_id = pu.id
LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id
LEFT JOIN service_package_tiers t ON t.id = sb.tier_id
-- Note: anonymized rows are NOT filtered out — §111 GDPR says the row
-- survives erasure as the org's financial/audit record (booker PII is
-- already wiped by AnonymizeServiceBookings, and the user join just
-- yields NULL names).
WHERE sb.org_id = @org_id
  AND (sb.intent = 'inquiry' OR sb.fee_amount = 0 OR p.status IN ('succeeded','refunded'))
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(listing_content_id)::uuid IS NULL OR sb.listing_meta_id =
       (SELECT id FROM service_listing_meta WHERE content_id = sqlc.narg(listing_content_id)::uuid))
ORDER BY sb.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountServiceBookingsByOrg :one
SELECT COUNT(*) FROM service_bookings sb
LEFT JOIN payments p ON p.id = sb.payment_id
WHERE sb.org_id = @org_id
  AND (sb.intent = 'inquiry' OR sb.fee_amount = 0 OR p.status IN ('succeeded','refunded'))
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(listing_content_id)::uuid IS NULL OR sb.listing_meta_id =
       (SELECT id FROM service_listing_meta WHERE content_id = sqlc.narg(listing_content_id)::uuid));

-- User-provider inbox (000824): the counterpart of ListServiceBookingsByOrg
-- for user-owned listings — mentorship requests land here
-- (provider_user_id = mentor). Same DTO shape as ListServiceBookingsByUser
-- plus the booker display joins the provider needs, plus the same
-- money-moved gate as the org inbox so unpaid paid-intent rows never reach
-- the provider's work queue. kind filters sm.listing_kind.
-- name: ListServiceBookingsByProviderUser :many
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title, c.slug AS listing_slug,
       sm.listing_kind, o.name AS org_name, o.slug AS org_slug,
       p.status AS payment_status, p.provider AS payment_provider,
       d.starts_on AS departure_starts_on, d.ends_on AS departure_ends_on,
       u.username AS booker_username, up.display_name AS booker_display_name,
       up.avatar_url AS booker_avatar,
       pu.username AS provider_username, pup.display_name AS provider_display_name,
       pup.avatar_url AS provider_avatar,
       dsp.name AS destination_space_name,
       COALESCE(sb.tier_name, t.name, '') AS effective_tier_name
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
LEFT JOIN organizations o ON o.id = sb.org_id
LEFT JOIN payments p ON p.id = sb.payment_id
LEFT JOIN service_package_departures d ON d.id = sb.departure_id
LEFT JOIN service_package_tiers t ON t.id = sb.tier_id
LEFT JOIN users u ON u.id = sb.booker_user_id
LEFT JOIN user_profiles up ON up.user_id = sb.booker_user_id
LEFT JOIN users pu ON pu.id = sb.provider_user_id
LEFT JOIN user_profiles pup ON pup.user_id = pu.id
LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id
WHERE sb.provider_user_id = @provider_user_id
  AND (sb.intent = 'inquiry' OR sb.fee_amount = 0 OR p.status IN ('succeeded','refunded'))
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text)
ORDER BY sb.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountServiceBookingsByProviderUser :one
SELECT COUNT(*) FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
LEFT JOIN payments p ON p.id = sb.payment_id
WHERE sb.provider_user_id = @provider_user_id
  AND (sb.intent = 'inquiry' OR sb.fee_amount = 0 OR p.status IN ('succeeded','refunded'))
  AND (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text);

-- Mentorship dedupe (000824) — replaces the dropped
-- GetActiveOrPendingConnection: a booker may hold at most one OPEN request
-- (pending awaiting mentor, or confirmed = active connection) against a
-- MENTOR, keyed on (booker_user_id, provider_user_id) — the same pair the
-- idx_svc_bookings_mentorship_open_uniq partial index enforces (000828).
-- Keying on the provider user, not the listing meta, keeps the gate closed
-- across shadow re-mints; org_id IS NULL mirrors the index predicate.
-- Returns no rows when none is open — callers treat pgx.ErrNoRows as
-- "no open request" (same contract the old connection query had).
-- name: GetOpenMentorshipBooking :one
SELECT sb.id FROM service_bookings sb
WHERE sb.booker_user_id = @booker_user_id
  AND sb.provider_user_id = @provider_user_id
  AND sb.org_id IS NULL
  AND sb.status IN ('pending','confirmed')
ORDER BY sb.created_at DESC
LIMIT 1;

-- Legacy connection-id fallback (000824): migrated rows keep their
-- pre-unification mentorship_connections UUID in
-- meta->>'connection_id'. Compat callers (the mentorship.Service row
-- lookup, tools.read) still arrive holding that old id — resolve it to
-- the live booking id so a migrated connection stays addressable.
-- name: GetMentorshipBookingIDByConnectionID :one
SELECT sb.id FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
WHERE sb.meta->>'connection_id' = @connection_id::text
  AND sm.listing_kind = 'mentorship'
LIMIT 1;

-- Mentor capacity probe (000824): the mentee-count trigger
-- trg_service_booking_mentee_count keeps user_profiles.active_mentee_count
-- in sync with confirmed mentorship-kind bookings, so the create-booking
-- capacity check reads both columns straight from the profile. is_mentor
-- rides along so the create path can re-verify the mentor flag — a live
-- shadow left behind when mentorship is switched off must not keep
-- minting bookings (orphan-shadow guard). Deliberately
-- NOT the legacy GetMentorProfile (mentorship.sql is dead — Task 6 removes
-- it); this lives in servicedir's own query file.
-- name: GetMentorCapacity :one
SELECT mentor_capacity, active_mentee_count, is_mentor
FROM user_profiles
WHERE user_id = @user_id;

-- Admin view — all bookings, any payment state. kind/provider_user_id/
-- booker_user_id are the mentorship-compat filters (000824): the legacy
-- admin connections list filters by mentor (provider) and mentee
-- (booker) over kind='mentorship' rows.
-- name: AdminListServiceBookings :many
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title, c.slug AS listing_slug,
       sm.listing_kind, o.name AS org_name, o.slug AS org_slug,
       p.status AS payment_status, p.provider AS payment_provider,
       d.starts_on AS departure_starts_on, d.ends_on AS departure_ends_on,
       u.username AS booker_username, up.display_name AS booker_display_name,
       up.avatar_url AS booker_avatar,
       pu.username AS provider_username, pup.display_name AS provider_display_name,
       pup.avatar_url AS provider_avatar,
       dsp.name AS destination_space_name,
       COALESCE(sb.tier_name, t.name, '') AS effective_tier_name
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
LEFT JOIN organizations o ON o.id = sb.org_id
LEFT JOIN payments p ON p.id = sb.payment_id
LEFT JOIN service_package_departures d ON d.id = sb.departure_id
LEFT JOIN service_package_tiers t ON t.id = sb.tier_id
LEFT JOIN users u ON u.id = sb.booker_user_id
LEFT JOIN user_profiles up ON up.user_id = sb.booker_user_id
LEFT JOIN users pu ON pu.id = sb.provider_user_id
LEFT JOIN user_profiles pup ON pup.user_id = pu.id
LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id
WHERE (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(org_id)::uuid IS NULL OR sb.org_id = sqlc.narg(org_id)::uuid)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(provider_user_id)::uuid IS NULL OR sb.provider_user_id = sqlc.narg(provider_user_id)::uuid)
  AND (sqlc.narg(booker_user_id)::uuid IS NULL OR sb.booker_user_id = sqlc.narg(booker_user_id)::uuid)
ORDER BY sb.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountServiceBookings :one
SELECT COUNT(*) FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
WHERE (sqlc.narg(status)::text IS NULL OR sb.status = sqlc.narg(status)::text)
  AND (sqlc.narg(intent)::text IS NULL OR sb.intent = sqlc.narg(intent)::text)
  AND (sqlc.narg(org_id)::uuid IS NULL OR sb.org_id = sqlc.narg(org_id)::uuid)
  AND (sqlc.narg(kind)::text IS NULL OR sm.listing_kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(provider_user_id)::uuid IS NULL OR sb.provider_user_id = sqlc.narg(provider_user_id)::uuid)
  AND (sqlc.narg(booker_user_id)::uuid IS NULL OR sb.booker_user_id = sqlc.narg(booker_user_id)::uuid);

-- ---------------------------------------------------------------------
-- Booking state machine (§111). Each transition is guarded on its
-- legal source status(es); no rows → the service layer maps to
-- ErrInvalidBookingTransition.
-- ---------------------------------------------------------------------

-- pending → confirmed (org accepts). Capacity increment happens in the
-- same tx via IncrementDepartureBookedCount.
-- name: ConfirmServiceBooking :one
UPDATE service_bookings
SET status = 'confirmed',
    org_response_note = sqlc.narg(org_response_note),
    confirmed_at = now(),
    updated_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- pending → declined (org rejects). meta.responded_at preserves the
-- legacy mentorship_connections.responded_at semantics — service_bookings
-- has no declined_at column, so the response timestamp rides meta (same
-- shape the 000824 migration wrote for declined rows).
-- name: DeclineServiceBooking :one
UPDATE service_bookings
SET status = 'declined',
    org_response_note = sqlc.narg(org_response_note),
    meta = meta || jsonb_build_object('responded_at', now()),
    updated_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- confirmed → fulfilled (org marks delivery complete; fee settles).
-- name: FulfillServiceBooking :one
UPDATE service_bookings
SET status = 'fulfilled',
    fulfilled_at = now(),
    updated_at = now()
WHERE id = @id AND status = 'confirmed'
RETURNING *;

-- booker-initiated cancel (pending or confirmed).
-- name: CancelServiceBooking :one
UPDATE service_bookings
SET status = 'cancelled',
    cancel_reason = sqlc.narg(cancel_reason),
    cancelled_at = now(),
    updated_at = now()
WHERE id = @id AND status IN ('pending','confirmed')
RETURNING *;

-- org-initiated cancel (pending or confirmed).
-- name: OrgCancelServiceBooking :one
UPDATE service_bookings
SET status = 'cancelled',
    org_response_note = sqlc.narg(org_response_note),
    cancel_reason = sqlc.narg(cancel_reason),
    cancelled_at = now(),
    updated_at = now()
WHERE id = @id AND status IN ('pending','confirmed')
RETURNING *;

-- Booker flags a confirmed|fulfilled booking as disputed — the booker's
-- reason lands in dispute_reason (org_response_note stays org/admin-only).
-- org_id IS NOT NULL: mentorship-kind rows (user-owned, org_id NULL) are
-- inquiry-only and non-disputable — the service layer rejects them ahead
-- of this query; the predicate is defence-in-depth so a direct query call
-- can never push a mentor request into the dispute flow.
-- name: DisputeServiceBooking :one
UPDATE service_bookings
SET status = 'disputed',
    dispute_reason = sqlc.narg(dispute_reason),
    updated_at = now()
WHERE id = @id AND status IN ('confirmed','fulfilled')
  AND org_id IS NOT NULL
RETURNING *;

-- Admin dispute resolution / forced transition.
-- name: AdminSetServiceBookingStatus :one
UPDATE service_bookings
SET status = @status,
    org_response_note = COALESCE(sqlc.narg(org_response_note), org_response_note),
    cancelled_at = CASE WHEN @status = 'cancelled' THEN now() ELSE cancelled_at END,
    fulfilled_at = CASE WHEN @status = 'fulfilled' THEN now() ELSE fulfilled_at END,
    confirmed_at = CASE WHEN @status = 'confirmed' AND confirmed_at IS NULL THEN now() ELSE confirmed_at END,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- Mark fee refunded (set after a successful refund via §17).
-- name: MarkServiceBookingRefunded :one
UPDATE service_bookings
SET status = 'refunded', updated_at = now()
WHERE id = @id AND status IN ('declined','cancelled','disputed')
RETURNING *;

-- Row lock + lookup for the provider-initiated refund path (§17
-- reversal arm for §111): a Stripe dashboard refund / chargeback webhook
-- flips payments.status to 'refunded' out-of-band, and the booking must
-- follow regardless of its current state.
-- name: GetServiceBookingByPaymentForUpdate :one
SELECT * FROM service_bookings
WHERE payment_id = @payment_id
FOR UPDATE;

-- Provider-side refund: mark the booking refunded from any live status.
-- (Terminal 'refunded' is excluded so re-delivered webhooks no-op.)
-- name: MarkServiceBookingRefundedByProvider :one
UPDATE service_bookings
SET status = 'refunded', updated_at = now()
WHERE id = @id AND status <> 'refunded'
RETURNING *;

-- SLA sweep source rows (service:booking_expire_pending).
-- name: ListExpiredPendingServiceBookings :many
SELECT * FROM service_bookings
WHERE status = 'pending'
  AND expires_at IS NOT NULL
  AND expires_at < now()
ORDER BY expires_at
LIMIT @page_limit;

-- §111 SLA nudge — pending bookings past the midpoint of their response
-- window that haven't been reminded yet. expires_at - created_at is the
-- org_response_sla_hours window stamped at creation.
-- Org-owned rows only: 000824 user-provider (mentorship) bookings are not
-- SLA-bound (expires_at stays NULL) and get no reminder — the explicit
-- org_id predicate documents that policy rather than relying on the
-- expires_at filter or the inner join to incidentally exclude them.
-- name: ListPendingBookingsDueReminder :many
SELECT sb.*, c.id AS listing_content_id, c.title AS listing_title, c.slug AS listing_slug,
       sm.listing_kind, o.name AS org_name, o.slug AS org_slug
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
JOIN contents c ON c.id = sm.content_id
JOIN organizations o ON o.id = sb.org_id
WHERE sb.status = 'pending'
  AND sb.org_id IS NOT NULL
  AND sb.reminded_at IS NULL
  AND sb.expires_at IS NOT NULL
  AND sb.expires_at > now()
  AND now() >= sb.created_at + (sb.expires_at - sb.created_at) / 2
ORDER BY sb.expires_at
LIMIT @page_limit;

-- name: MarkServiceBookingReminded :execrows
UPDATE service_bookings
SET reminded_at = now(), updated_at = now()
WHERE id = @id AND status = 'pending' AND reminded_at IS NULL;

-- GDPR (§19): anonymize booker/provider PII on confirmed erasure. The row
-- survives for the org's financial/audit record; both owner pointers are
-- nulled so the subsequent HardDeleteUser is not blocked by the FKs, and
-- every free-text/PII column is wiped (note — which carries mentorship
-- goals, org_response_note, cancel_reason, dispute_reason, preferred_dates,
-- destination_space_id, meta — which carries goals/responded_at).
-- (000824) The provider arm covers mentorship-kind bookings where the
-- erased user was the mentor; the provider XOR CHECK has an anonymized_at
-- escape so a mentorship row may legally end up with org_id and
-- provider_user_id both NULL.
-- name: AnonymizeServiceBookings :exec
UPDATE service_bookings
SET booker_user_id = NULL,
    provider_user_id = NULL,
    note = NULL,
    org_response_note = NULL,
    preferred_dates = NULL,
    destination_space_id = NULL,
    meta = '{}'::jsonb,
    dispute_reason = NULL,
    cancel_reason = NULL,
    anonymized_at = now(),
    updated_at = now()
WHERE (booker_user_id = @user_id OR provider_user_id = @user_id)
  AND anonymized_at IS NULL;

-- ---------------------------------------------------------------------
-- service_categories (dynamic taxonomy — marketplace upgrade)
-- ---------------------------------------------------------------------

-- name: ListServiceCategories :many
SELECT id, slug, label, description, icon, accent_color, applies_to,
       position, is_active, created_at, updated_at
FROM service_categories
ORDER BY position ASC, label ASC;

-- name: GetServiceCategoryBySlug :one
SELECT id, slug, label, description, icon, accent_color, applies_to,
       position, is_active, created_at, updated_at
FROM service_categories
WHERE slug = @slug;

-- name: CreateServiceCategory :one
INSERT INTO service_categories (slug, label, description, icon, accent_color,
    applies_to, position, is_active)
VALUES (@slug, @label, sqlc.narg(description), sqlc.narg(icon),
    sqlc.narg(accent_color), COALESCE(sqlc.narg(applies_to), 'both'),
    COALESCE(sqlc.narg(position), 0), COALESCE(sqlc.narg(is_active), true))
RETURNING *;

-- name: UpdateServiceCategory :one
UPDATE service_categories
SET label = @label,
    description = sqlc.narg(description),
    icon = sqlc.narg(icon),
    accent_color = sqlc.narg(accent_color),
    applies_to = COALESCE(sqlc.narg(applies_to), applies_to),
    position = COALESCE(sqlc.narg(position), position),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    updated_at = now()
WHERE slug = @slug
RETURNING *;

-- name: DeleteServiceCategory :exec
DELETE FROM service_categories WHERE slug = @slug;

-- name: CountServiceListingsByCategory :one
SELECT COUNT(*) FROM service_listing_meta WHERE service_category = @slug;

-- ---------------------------------------------------------------------
-- service_listing_coupons / coupon_redemptions / service_package_tiers
-- (Services Marketplace — servicedir owns attach/redeem/tiers so a
-- booking + redemption commit in one WithTxResult)
-- ---------------------------------------------------------------------

-- name: ListServiceListingCoupons :many
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.valid_from, c.expires_at,
       c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
ORDER BY lc.position ASC;

-- name: ListActiveServiceListingCoupons :many
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.valid_from, c.expires_at,
       c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
  AND c.status = 'active'
  AND (c.valid_from IS NULL OR c.valid_from <= now())
  AND (c.expires_at IS NULL OR c.expires_at > now())
ORDER BY lc.position ASC;

-- name: GetListingCouponByCode :one
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.valid_from, c.expires_at,
       c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
  AND lower(c.code) = lower(@code);

-- Lock the coupon row for a redemption commit — the booking create path
-- takes this inside the booking transaction so concurrent checkouts
-- against the same coupon serialize on the row lock instead of
-- oversubscribing max_redemptions / per_user_limit.
-- name: LockCouponForRedemption :one
SELECT c.id, c.title, c.code, c.discount_type, c.discount_value, c.currency,
       c.terms_url, c.referral_url, c.status, c.valid_from, c.expires_at,
       c.applies_to,
       c.max_redemptions, c.per_user_limit, c.min_spend, c.first_time_only,
       c.organization_id, lc.position, lc.overrides
FROM service_listing_coupons lc
JOIN coupons c ON c.id = lc.coupon_id
WHERE lc.listing_meta_id = @listing_meta_id
  AND lower(c.code) = lower(@code)
FOR UPDATE OF c;

-- name: DeleteServiceListingCoupons :exec
DELETE FROM service_listing_coupons WHERE listing_meta_id = @listing_meta_id;

-- name: CreateServiceListingCouponLink :one
INSERT INTO service_listing_coupons (coupon_id, listing_meta_id, position)
VALUES (@coupon_id, @listing_meta_id, COALESCE(sqlc.narg(position), 0))
RETURNING *;

-- name: CountListingCouponLinks :one
SELECT COUNT(*) FROM service_listing_coupons WHERE listing_meta_id = @listing_meta_id;

-- name: CountCouponRedemptions :one
-- Dead bookings don't burn caps: cancel/decline/refund auto-releases.
SELECT COUNT(*) FROM coupon_redemptions r
JOIN service_bookings b ON b.id = r.service_booking_id
WHERE r.coupon_id = @coupon_id
  AND b.status NOT IN ('cancelled','declined','refunded');

-- name: CountCouponRedemptionsByUser :one
SELECT COUNT(*) FROM coupon_redemptions r
JOIN service_bookings b ON b.id = r.service_booking_id
WHERE r.coupon_id = @coupon_id AND r.user_id = @user_id
  AND b.status NOT IN ('cancelled','declined','refunded');

-- name: CreateCouponRedemption :one
INSERT INTO coupon_redemptions (coupon_id, user_id, service_booking_id, listing_meta_id, amount_saved, currency)
VALUES (@coupon_id, @user_id, @service_booking_id, @listing_meta_id, @amount_saved, sqlc.narg(currency))
RETURNING *;

-- name: ListingHasActiveCoupon :one
-- Mirrors ListActiveServiceListingCoupons' active-window filters (incl.
-- valid_from) so a future-dated coupon does not flip the badge.
SELECT EXISTS(
    SELECT 1 FROM service_listing_coupons lc
    JOIN coupons c ON c.id = lc.coupon_id
    WHERE lc.listing_meta_id = @listing_meta_id
      AND c.status = 'active'
      AND (c.valid_from IS NULL OR c.valid_from <= now())
      AND (c.expires_at IS NULL OR c.expires_at > now())
) AS has_coupon;

-- name: CreateServicePackageTier :one
INSERT INTO service_package_tiers (listing_meta_id, name, badge, description,
    icon, price_amount, price_currency, features, position, is_featured, is_active)
VALUES (@listing_meta_id, @name, sqlc.narg(badge), sqlc.narg(description),
    sqlc.narg(icon), @price_amount, sqlc.narg(price_currency), @features,
    COALESCE(sqlc.narg(position), 0), COALESCE(sqlc.narg(is_featured), false),
    COALESCE(sqlc.narg(is_active), true))
RETURNING *;

-- name: ListServicePackageTiers :many
SELECT * FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id AND is_active = true
ORDER BY position ASC;

-- name: ListAllServicePackageTiers :many
SELECT * FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id
ORDER BY position ASC;

-- name: GetServicePackageTier :one
SELECT * FROM service_package_tiers
WHERE id = @id AND listing_meta_id = @listing_meta_id;

-- name: DeleteServicePackageTiers :exec
DELETE FROM service_package_tiers WHERE listing_meta_id = @listing_meta_id;

-- name: DeleteServicePackageTiersExcept :exec
DELETE FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id
  AND id <> ALL(@keep_ids::uuid[]);
-- name: GetMinActiveTierPrice :one
SELECT MIN(price_amount) FROM service_package_tiers
WHERE listing_meta_id = @listing_meta_id AND is_active = true;

-- name: CountServiceBookingsByUserOrg :one
SELECT COUNT(*) FROM service_bookings
WHERE booker_user_id = @user_id AND org_id = @org_id
  AND status NOT IN ('cancelled','declined','refunded');

-- name: UpdateMentorShadowMetaContent :exec
-- §114 2026-10-10: mentor-profile PATCH syncs the shadow listing's page
-- content the same way UpdateMentorShadowMeta syncs the listing fields —
-- COALESCE so nil preserves the stored set, '[]' clears.
UPDATE service_listing_meta
SET selling_points = COALESCE(sqlc.narg(selling_points), selling_points),
    facts = COALESCE(sqlc.narg(facts), facts),
    highlights = COALESCE(sqlc.narg(highlights), highlights),
    audience = COALESCE(sqlc.narg(audience), audience),
    included = COALESCE(sqlc.narg(included), included),
    faqs = COALESCE(sqlc.narg(faqs), faqs),
    steps = COALESCE(sqlc.narg(steps), steps),
    updated_at = now()
WHERE content_id = @content_id;
