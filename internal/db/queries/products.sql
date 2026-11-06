-- Public product directory queries (§80 Product Domain). Products are
-- comparison entities with dedicated surfaces (/products, /product/{slug},
-- /compare) — not wiki kinds — so these power the de-wikied read surface.

-- name: CountProducts :one
SELECT count(*)
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE p.deleted_at IS NULL
  AND ($1::text = '' OR t.slug = $1)
  AND ($2::text = '' OR p.name ILIKE '%' || $2 || '%');

-- name: ListProductTypesWithCounts :many
SELECT t.slug, t.label, t.icon, count(p.id) AS entity_count
FROM product_types t
LEFT JOIN products p ON p.product_type_id = t.id AND p.deleted_at IS NULL
WHERE t.is_enabled = true
GROUP BY t.id
ORDER BY t.label;

-- name: GetPublicProductBySlug :one
SELECT p.id, p.product_type_id, p.slug, p.name, p.tagline, p.website_url,
       p.pricing_model, p.price_from, p.rating_avg, p.review_count,
       p.geo_space_id, t.slug AS type_slug, t.label AS type_label,
       m.storage_path AS logo_storage_path, m.storage_bucket AS logo_storage_bucket
FROM products p
JOIN product_types t ON t.id = p.product_type_id
LEFT JOIN media m ON m.id = p.logo_id
WHERE p.slug = @slug AND p.deleted_at IS NULL;

-- ── Admin product management (§80) ──────────────────────────────────────

-- name: AdminListProducts :many
SELECT p.id, p.slug, p.name, p.tagline, p.website_url, p.pricing_model,
       p.price_from, p.rating_avg, p.review_count, p.deleted_at,
       p.created_at, p.updated_at,
       t.slug AS type_slug, t.label AS type_label,
       m.storage_path AS logo_storage_path, m.storage_bucket AS logo_storage_bucket,
       (SELECT count(*) FROM product_providers pp WHERE pp.product_id = p.id) AS provider_count,
       (SELECT count(*) FROM product_deals pd WHERE pd.product_id = p.id) AS deal_count
FROM products p
JOIN product_types t ON t.id = p.product_type_id
LEFT JOIN media m ON m.id = p.logo_id
WHERE (@type_slug::text = '' OR t.slug = @type_slug)
  AND (@q::text = '' OR p.name ILIKE '%' || @q || '%' ESCAPE '\' OR p.slug ILIKE '%' || @q || '%' ESCAPE '\')
  AND (@status::text = 'all'
       OR (@status = 'deleted' AND p.deleted_at IS NOT NULL)
       OR (@status <> 'deleted' AND @status <> 'all' AND p.deleted_at IS NULL))
ORDER BY
  CASE WHEN @sort_key::text = 'name:asc' THEN p.name END ASC,
  CASE WHEN @sort_key = 'name:desc' THEN p.name END DESC,
  CASE WHEN @sort_key = 'created_at:asc' THEN p.created_at END ASC,
  p.created_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: AdminCountProducts :one
SELECT count(*)
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE (@type_slug::text = '' OR t.slug = @type_slug)
  AND (@q::text = '' OR p.name ILIKE '%' || @q || '%' ESCAPE '\' OR p.slug ILIKE '%' || @q || '%' ESCAPE '\')
  AND (@status::text = 'all'
       OR (@status = 'deleted' AND p.deleted_at IS NOT NULL)
       OR (@status <> 'deleted' AND @status <> 'all' AND p.deleted_at IS NULL));

-- Admin detail — includes deleted rows so the edit page can restore.
-- name: AdminGetProductBySlug :one
SELECT p.id, p.product_type_id, p.slug, p.name, p.tagline, p.description,
       p.website_url, p.logo_id, p.pricing_model,
       p.price_from, p.rating_avg, p.review_count,
       p.geo_space_id, p.deleted_at, p.created_at, p.updated_at,
       t.slug AS type_slug, t.label AS type_label,
       m.storage_path AS logo_storage_path, m.storage_bucket AS logo_storage_bucket
FROM products p
JOIN product_types t ON t.id = p.product_type_id
LEFT JOIN media m ON m.id = p.logo_id
WHERE p.slug = @slug;

-- Product gallery (product_gallery join — §80 product management).
-- name: ListProductGallery :many
SELECT pg.media_id, pg.position, m.storage_bucket, m.storage_path,
       m.original_filename, m.mime_type
FROM product_gallery pg
JOIN media m ON m.id = pg.media_id
WHERE pg.product_id = @product_id
ORDER BY pg.position;

-- name: UpsertProductGalleryImage :exec
INSERT INTO product_gallery (product_id, media_id, position)
VALUES (@product_id, @media_id, @position)
ON CONFLICT (product_id, media_id) DO UPDATE SET position = EXCLUDED.position;

-- name: DeleteProductGalleryNotIn :exec
DELETE FROM product_gallery
WHERE product_id = @product_id AND media_id <> ALL(@keep::uuid[]);

-- name: RestoreProduct :execrows
UPDATE products SET deleted_at = NULL, updated_at = now()
WHERE slug = @slug AND deleted_at IS NOT NULL;

-- name: HardDeleteProduct :execrows
DELETE FROM products WHERE slug = @slug AND deleted_at IS NOT NULL;

-- Product deals (product_deals join — §80 product management).
-- name: ListProductDeals :many
SELECT pd.coupon_id, pd.position, c.slug, c.title, c.discount_type, c.discount_value,
       c.currency, c.status, c.expires_at, ap.name AS provider_name
FROM product_deals pd
JOIN coupons c ON c.id = pd.coupon_id
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE pd.product_id = @product_id
ORDER BY pd.position, c.title;

-- name: UpsertProductDeal :exec
INSERT INTO product_deals (product_id, coupon_id, position)
VALUES (@product_id, @coupon_id, @position)
ON CONFLICT (product_id, coupon_id) DO UPDATE SET position = EXCLUDED.position;

-- name: RemoveProductDeal :exec
DELETE FROM product_deals WHERE product_id = @product_id AND coupon_id = @coupon_id;

-- name: DeleteProductDealsNotIn :exec
DELETE FROM product_deals
WHERE product_id = @product_id AND coupon_id <> ALL(@keep_ids::uuid[]);

-- ── Admin product types ─────────────────────────────────────────────────

-- name: AdminListProductTypes :many
SELECT t.id, t.slug, t.label, t.icon, t.is_enabled, t.config, t.created_at, t.updated_at,
       (SELECT count(*) FROM products p WHERE p.product_type_id = t.id AND p.deleted_at IS NULL) AS product_count
FROM product_types t
WHERE (@q::text = '' OR t.label ILIKE '%' || @q || '%' ESCAPE '\' OR t.slug ILIKE '%' || @q || '%' ESCAPE '\')
ORDER BY t.label
LIMIT @limit_count OFFSET @offset_count;

-- name: AdminCountProductTypes :one
SELECT count(*) FROM product_types t
WHERE (@q::text = '' OR t.label ILIKE '%' || @q || '%' ESCAPE '\' OR t.slug ILIKE '%' || @q || '%' ESCAPE '\');

-- name: AdminCountProductsByType :one
SELECT count(*) FROM products WHERE product_type_id = @product_type_id AND deleted_at IS NULL;

-- name: UpdateProductType :one
UPDATE product_types SET
    label = COALESCE(sqlc.narg(label), label),
    icon = CASE WHEN 'icon' = ANY(@clear_fields::text[]) THEN NULL ELSE COALESCE(sqlc.narg(icon), icon) END,
    is_enabled = COALESCE(sqlc.narg(is_enabled), is_enabled),
    config = COALESCE(sqlc.narg(config), config),
    updated_at = now()
WHERE slug = @slug
RETURNING id, slug, label, icon, is_enabled, config;

-- name: DeleteProductType :execrows
DELETE FROM product_types WHERE slug = @slug;

-- Field schema for a product type: groups bound via
-- custom_field_group_assignments(entity_class = type slug), fields joined in
-- one read for the /products/new + admin product forms.
-- name: ListProductTypeGroupsWithFields :many
SELECT g.id AS group_id, g.slug AS group_slug, g.name AS group_name,
       cfga.position AS group_position,
       f.id AS field_id, f.key, f.label, f.field_type, f.options, f.placeholder,
       f.default_value, f.is_required, f.position AS field_position, f.section
FROM custom_field_group_assignments cfga
JOIN custom_field_groups g ON g.id = cfga.group_id
LEFT JOIN custom_fields f ON f.group_id = g.id
WHERE cfga.entity_class = @entity_class
ORDER BY cfga.position, g.created_at, f.position, f.created_at;

-- name: ListCouponsForProviders :many
-- Deal picker source: active coupons, optionally scoped to a provider set
-- (empty array → all active deals).
SELECT c.id, c.slug, c.title, c.discount_type, c.discount_value, c.currency,
       c.status, c.provider_id, ap.name AS provider_name
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.status = 'active'
  AND (cardinality(@provider_ids::uuid[]) = 0 OR c.provider_id = ANY(@provider_ids::uuid[]))
ORDER BY c.title;
