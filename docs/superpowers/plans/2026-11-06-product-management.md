# Product Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship full product management — admin CRUD for products + product types with per-type custom field groups, provider/deal links, and `/products/new` create forms in the admin and main apps.

**Architecture:** Extends the existing §80 product domain (`internal/wiki` package — products were de-wikied but the package still owns them). New canonical routes under `/admin/products` + `/admin/product-types` gated by new `products.*`/`product_types.*` RBAC nodes; legacy `/admin/wiki/products*` routes stay wired to the same handlers. `product_deals` join table links products↔coupons; product-type field groups get `custom_field_group_assignments` rows (`entity_class = type slug`).

**Tech Stack:** Go + Chi + sqlc/pgx (backend, `internal/wiki`), PostgreSQL migrations (golang-migrate, next number **001081**), SvelteKit 2 + Svelte 5 runes (admin SPA `apps/admin`, main SSR app `apps/main`), bun tooling.

## Global Constraints

- Never hand-edit `internal/db/gen/*` — edit `.sql` query files, run `make sqlc-gen` (Rule 2).
- Bun only for JS tooling at repo root (Rule 18, AGENTS.md).
- Commit only files this plan touches — the working tree has unrelated dirty files. Never `git add -A`/`git add .`.
- No AI/agent attribution anywhere in code, comments, or commits (Rule 17).
- Response contract: `errpkg.WriteJSON(w, status, {...})` / `errpkg.WriteError(w, status, code, message)`. Admin list shape: `{"data": rows, "total": n, "page": p, "page_size": s}`.
- New permission nodes must be fully seeded (BE-005 regression anchor).
- Every new route goes in `.jeos/conventions/api-routes-catalog.md`; every node in `.jeos/sections/03-rbac.md`; §80 amendment in `.jeos/sections/80-destination-wiki.md` — same PR.
- Soft delete for products (`deleted_at`); hard delete only via `?force=true` gated `admin.hard_delete` (Rule 11).
- Key existing identifiers: `gen.CreateAssignment`, `gen.UpsertCustomFieldValue`, `gen.DeleteCustomFieldValue`, `gen.ListCustomFieldValuesWithDefsByEntity`, `gen.ListCustomFieldsByGroup`, `gen.CreateCustomFieldEntityType`; wiki service uses `s.store.read`, `s.store.write`, `s.store.pool.Writer` (pgx tx); handlers in `internal/wiki/product_admin.go`, `product_directory.go`, `entity_directory.go`; routes registered in `internal/server/server.go` near line 6667–6685.

---

### Task 1: Migration `001081` — product_deals + CFGA backfill + entity type + RBAC seed

**Files:**
- Create: `migrations/001081_product_management.up.sql`
- Create: `migrations/001081_product_management.down.sql`

**Interfaces:**
- Produces: `product_deals(product_id, coupon_id, position, created_at)`; `custom_field_group_assignments` rows `entity_class = product type slug, entity_id NULL` for every existing `wiki_{type}_{cat}` group; `custom_field_entity_types` row `('product','Product')`; permission nodes `products.{list,view,create,update,delete}` + `product_types.{list,view,create,update,delete}` granted to `super_admin`/`super_admin_role`/`admin` group-backed roles.

- [ ] **Step 1: Write the up migration**

```sql
-- 001081_product_management.up.sql
-- Product management (§80): product↔deal links, product-type field-group
-- assignments (entity_class = type slug), 'product' cfv entity type, and
-- dedicated products.* / product_types.* RBAC nodes.
BEGIN;

CREATE TABLE product_deals (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    coupon_id  uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, coupon_id)
);
CREATE INDEX idx_product_deals_coupon ON product_deals (coupon_id);

-- Type → group association: every wiki_{type}_{category} group bound to its
-- product type slug so GET /custom-field-groups?entity_class={slug} resolves
-- the type's field schema (§80: "entity_class = product type slug").
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, pt.slug, NULL, 0
FROM product_types pt
JOIN custom_field_groups g
  ON g.slug LIKE 'wiki\_' || replace(pt.slug, '-', '_') || '\_%' ESCAPE '\'
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- Register 'product' as a cfv entity type.
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('product', 'Product', true, 8)
ON CONFLICT (value) DO NOTHING;

-- RBAC nodes (BE-005: full seed).
INSERT INTO permissions (key, description) VALUES
    ('products.list',    'List products (admin)'),
    ('products.view',    'View product detail incl. providers/deals/custom fields'),
    ('products.create',  'Create products'),
    ('products.update',  'Update products incl. provider/deal links'),
    ('products.delete',  'Delete/restore products'),
    ('product_types.list',   'List product types'),
    ('product_types.view',   'View product type incl. bound field groups'),
    ('product_types.create', 'Create product types (provisions field groups + comparison type)'),
    ('product_types.update', 'Update product types (label/icon/enabled/categories)'),
    ('product_types.delete', 'Delete product types with no products')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN (
    'products.list','products.view','products.create','products.update','products.delete',
    'product_types.list','product_types.view','product_types.create','product_types.update','product_types.delete'
  )
ON CONFLICT DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 001081_product_management.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
      'products.list','products.view','products.create','products.update','products.delete',
      'product_types.list','product_types.view','product_types.create','product_types.update','product_types.delete'));
DELETE FROM permissions WHERE key IN (
      'products.list','products.view','products.create','products.update','products.delete',
      'product_types.list','product_types.view','product_types.create','product_types.update','product_types.delete');

DELETE FROM custom_field_entity_types WHERE value = 'product';

DELETE FROM custom_field_group_assignments a
USING product_types pt, custom_field_groups g
WHERE a.group_id = g.id AND a.entity_class = pt.slug AND a.entity_id IS NULL
  AND g.slug LIKE 'wiki\_' || replace(pt.slug, '-', '_') || '\_%' ESCAPE '\';

DROP TABLE product_deals;

COMMIT;
```

- [ ] **Step 3: Sanity-check SQL**

```bash
psql "$JC_DB_URL" -c '\d custom_field_group_assignments' # confirm unique key on (group_id, entity_class, entity_id)
```
If the unique constraint differs, adjust the `ON CONFLICT` target.
Commit: `git add migrations/001081_product_management.*.sql && git commit -m "db: product management — product_deals, type field-group assignments, products.* RBAC nodes"`

---

### Task 2: sqlc queries for admin product management

**Files:**
- Modify: `internal/db/queries/products.sql` (append all queries below)
- Regenerate: `make sqlc-gen`

**Interfaces:**
- Produces (used in Tasks 3–4): `AdminListProducts`, `AdminCountProducts`, `AdminGetProductBySlug`, `AdminListProductTypes`, `AdminCountProductsByType`, `UpdateProductType`, `DeleteProductType`, `ListProductDeals`, `UpsertProductDeal`, `RemoveProductDeal`, `DeleteProductDealsNotIn`, `ListProductTypeGroupsWithFields`, `RestoreProduct`, `HardDeleteProduct`, `ListProvidersForProductIDs` is **not** needed (detail uses existing `ListProductProviders`).

- [ ] **Step 1: Append queries to `internal/db/queries/products.sql`**

```sql
-- Admin list: q on name/slug, type filter by product_types.slug, status
-- active|deleted|all, column allow-list sort.
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
WHERE ($1::text = '' OR t.slug = $1)
  AND ($2::text = '' OR p.name ILIKE '%' || $2 || '%' OR p.slug ILIKE '%' || $2 || '%')
  AND ($3::text = 'all'
       OR ($3::text = 'deleted' AND p.deleted_at IS NOT NULL)
       OR ($3::text <> 'deleted' AND $3::text <> 'all' AND p.deleted_at IS NULL))
ORDER BY
  CASE WHEN $4::text = 'name:asc' THEN p.name END ASC,
  CASE WHEN $4::text = 'name:desc' THEN p.name END DESC,
  CASE WHEN $4::text = 'created_at:asc' THEN p.created_at END ASC,
  p.created_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: AdminCountProducts :one
SELECT count(*)
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE ($1::text = '' OR t.slug = $1)
  AND ($2::text = '' OR p.name ILIKE '%' || $2 || '%' OR p.slug ILIKE '%' || $2 || '%')
  AND ($3::text = 'all'
       OR ($3::text = 'deleted' AND p.deleted_at IS NOT NULL)
       OR ($3::text <> 'deleted' AND $3::text <> 'all' AND p.deleted_at IS NULL));

-- Admin detail — includes deleted rows so the edit page can restore.
-- name: AdminGetProductBySlug :one
SELECT p.id, p.product_type_id, p.slug, p.name, p.tagline, p.website_url,
       p.logo_id, p.pricing_model, p.price_from, p.rating_avg, p.review_count,
       p.geo_space_id, p.deleted_at, p.created_at, p.updated_at,
       t.slug AS type_slug, t.label AS type_label
FROM products p
JOIN product_types t ON t.id = p.product_type_id
WHERE p.slug = @slug;

-- name: RestoreProduct :exec
UPDATE products SET deleted_at = NULL, updated_at = now() WHERE slug = @slug AND deleted_at IS NOT NULL;

-- name: HardDeleteProduct :exec
DELETE FROM products WHERE slug = @slug AND deleted_at IS NOT NULL;

-- Product deals (product_deals join).
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
DELETE FROM product_deals WHERE product_id = @product_id AND coupon_id <> ALL(@keep_ids::uuid[]);

-- name: ListProductProvidersNotIn is unnecessary; use RemoveProductProvider per-row.

-- Admin product types.
-- name: AdminListProductTypes :many
SELECT t.id, t.slug, t.label, t.icon, t.is_enabled, t.config, t.created_at, t.updated_at,
       (SELECT count(*) FROM products p WHERE p.product_type_id = t.id AND p.deleted_at IS NULL) AS product_count
FROM product_types t
ORDER BY t.label;

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

-- Field schema for a product type: groups bound via cfga(entity_class=slug),
-- fields joined in one read for the /products/new + admin forms.
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
-- Deal picker: active coupons across a set of provider ids (empty → all).
SELECT c.id, c.slug, c.title, c.discount_type, c.discount_value, c.currency,
       c.status, c.provider_id, ap.name AS provider_name
FROM coupons c
LEFT JOIN affiliate_providers ap ON ap.id = c.provider_id
WHERE c.status = 'active'
  AND (cardinality(@provider_ids::uuid[]) = 0 OR c.provider_id = ANY(@provider_ids::uuid[]))
ORDER BY c.title;
```

- [ ] **Step 2: Regenerate and compile**

```bash
make sqlc-gen && go build ./internal/db/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/products.sql internal/db/gen/products.sql.go internal/db/gen/models.go
git commit -m "db: product management sqlc — admin list/detail, product_deals, product types, field schema"
```

---

### Task 3: Service layer — extended create/update tx, links, type CRUD

**Files:**
- Modify: `internal/wiki/product_admin.go` (extend `ProductInput`, `CreateProduct`, `UpdateProduct`; add list/detail/restore/link/type methods)

**Interfaces:**
- Consumes: all Task 2 queries; `gen.CreateAssignment`, `gen.UpsertCustomFieldValue`, `gen.DeleteCustomFieldValue`, `gen.ListCustomFieldValuesWithDefsByEntity`, `gen.ListCustomFieldsByGroup` (already generated).
- Produces (handlers in Task 4 call these):
  - `(s *Service) AdminListProducts(ctx, typeSlug, q, status, sort string, limit, offset int32) ([]gen.AdminListProductsRow, int64, error)`
  - `(s *Service) AdminGetProduct(ctx, slug) (*ProductDetail, error)` — detail + providers + deals + cfv
  - `(s *Service) CreateProduct(ctx, ProductInput) (string, error)` — extended
  - `(s *Service) UpdateProduct(ctx, slug, ProductInput) error` — extended
  - `(s *Service) RestoreProduct(ctx, slug) error`
  - `(s *Service) HardDeleteProduct(ctx, slug) error`
  - `(s *Service) SetProductDeal(ctx, productSlug, couponID string, position int32, unlink bool) error`
  - `(s *Service) AdminListProductTypes(ctx) ([]gen.AdminListProductTypesRow, error)`
  - `(s *Service) AdminGetProductType(ctx, slug) (*ProductTypeDetail, error)`
  - `(s *Service) UpdateProductType(ctx, slug string, in ProductTypeInput) error`
  - `(s *Service) DeleteProductType(ctx, slug) error` — 409 when products exist
  - `(s *Service) ProductFieldSchema(ctx, typeSlug) ([]FieldSchemaGroup, error)`

- [ ] **Step 1: Extend `ProductInput` + write `CreateProduct` tx**

In `internal/wiki/product_admin.go`, extend `ProductInput`:

```go
type ProductProviderInput struct {
	ProviderID string `json:"provider_id"`
	PriceFrom  string `json:"price_from"`
	OfferURL   string `json:"offer_url"`
	Position   int32  `json:"position"`
}

type ProductFieldValueInput struct {
	FieldID     string           `json:"field_id"`
	ValueText   *string          `json:"value_text"`
	ValueNumber *float64         `json:"value_number"`
	ValueBool   *bool            `json:"value_bool"`
	ValueDate   *string          `json:"value_date"` // RFC 3339
	ValueJSON   *json.RawMessage `json:"value_json"`
}

type ProductInput struct {
	TypeSlug      string                  `json:"type_slug"`
	Name          string                  `json:"name"`
	Slug          string                  `json:"slug"`
	Tagline       *string                 `json:"tagline"`
	WebsiteURL    *string                 `json:"website_url"`
	PricingModel  *string                 `json:"pricing_model"`
	PriceFrom     *float64                `json:"price_from"`
	Providers     []ProductProviderInput  `json:"providers"`
	DealIDs       []string                `json:"deal_ids"`
	CustomFields  []ProductFieldValueInput `json:"custom_fields"`
	ClearFieldIDs []string                `json:"clear_field_ids"`
}
```

Rewrite `CreateProduct` to run one tx (product → providers → deals → cfv):

```go
func (s *Service) CreateProduct(ctx context.Context, in ProductInput) (string, error) {
	if in.TypeSlug == "" || in.Slug == "" || in.Name == "" {
		return "", fmt.Errorf("type_slug, slug and name are required")
	}
	typeID, err := s.productTypeID(ctx, in.TypeSlug)
	if err != nil {
		return "", fmt.Errorf("product type %q not found", in.TypeSlug)
	}
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.store.write.WithTx(tx)

	row, err := q.CreateProduct(ctx, gen.CreateProductParams{
		ProductTypeID: typeID, Slug: in.Slug, Name: in.Name,
		Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom),
	})
	if err != nil {
		return "", err
	}
	if err := s.syncProductLinks(ctx, q, row.ID, in); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return row.Slug, nil
}
```

New helpers (same file):

```go
// syncProductLinks writes provider links, deal links and custom field values
// for a product inside q's tx. providers/deal_ids are full-replace sets.
func (s *Service) syncProductLinks(ctx context.Context, q *gen.Queries, productID pgtype.UUID, in ProductInput) error {
	for i, pv := range in.Providers {
		var pf pgtype.Numeric
		if pv.PriceFrom != "" {
			_ = pf.Scan(pv.PriceFrom)
		}
		var ou *string
		if pv.OfferURL != "" {
			ou = &pv.OfferURL
		}
		pos := pv.Position
		if pos == 0 {
			pos = int32(i)
		}
		if err := q.UpsertProductProvider(ctx, gen.UpsertProductProviderParams{
			ProductID: productID, ProviderID: uuidOf(pv.ProviderID),
			PriceFrom: pf, OfferUrl: ou, Position: pos,
		}); err != nil {
			return fmt.Errorf("link provider: %w", err)
		}
	}
	for i, did := range in.DealIDs {
		if err := q.UpsertProductDeal(ctx, gen.UpsertProductDealParams{
			ProductID: productID, CouponID: uuidOf(did), Position: int32(i),
		}); err != nil {
			return fmt.Errorf("link deal: %w", err)
		}
	}
	return s.writeProductFieldValues(ctx, q, productID, in.CustomFields, in.ClearFieldIDs)
}

// writeProductFieldValues upserts cfv rows (entity_type='product') and deletes
// any ids in clearFieldIDs.
func (s *Service) writeProductFieldValues(ctx context.Context, q *gen.Queries, productID pgtype.UUID, fields []ProductFieldValueInput, clearIDs []string) error {
	for _, f := range fields {
		var num pgtype.Numeric
		if f.ValueNumber != nil {
			_ = num.Scan(fmt.Sprintf("%f", *f.ValueNumber))
		}
		var dt pgtype.Timestamptz
		if f.ValueDate != nil {
			_ = dt.Scan(*f.ValueDate)
		}
		var vb pgtype.Bool
		if f.ValueBool != nil {
			vb = pgtype.Bool{Bool: *f.ValueBool, Valid: true}
		}
		var vj []byte
		if f.ValueJSON != nil {
			vj = *f.ValueJSON
		}
		if _, err := q.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
			FieldID: uuidOf(f.FieldID), EntityType: "product", EntityID: productID,
			ValueText: f.ValueText, ValueNumber: num, ValueBool: vb,
			ValueDate: dt, ValueJson: vj,
		}); err != nil {
			return fmt.Errorf("set field value: %w", err)
		}
	}
	for _, id := range clearIDs {
		if err := q.DeleteCustomFieldValue(ctx, gen.DeleteCustomFieldValueParams{
			FieldID: uuidOf(id), EntityType: "product", EntityID: productID,
		}); err != nil {
			return fmt.Errorf("clear field value: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Extend `UpdateProduct` with replace semantics**

`UpdateProduct` gains tx + replace-diff for providers and deals:

```go
func (s *Service) UpdateProduct(ctx context.Context, slug string, in ProductInput) error {
	var name *string
	if in.Name != "" {
		name = &in.Name
	}
	clear := []string{}
	for _, kv := range []struct {
		key string
		val *string
	}{{"tagline", in.Tagline}, {"website_url", in.WebsiteURL}, {"pricing_model", in.PricingModel}} {
		if kv.val != nil && *kv.val == "" {
			clear = append(clear, kv.key)
		}
	}
	tx, err := s.store.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.store.write.WithTx(tx)

	if err := q.UpdateProduct(ctx, gen.UpdateProductParams{
		Name: name, Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom), Slug: slug,
		ClearFields: clear,
	}); err != nil {
		return err
	}
	p, err := q.GetProductBySlug(ctx, slug)
	if err != nil {
		return err
	}
	// Providers full-replace: remove links absent from in.Providers, upsert rest.
	if in.Providers != nil {
		existing, err := q.ListProductProviders(ctx, p.ID)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, pv := range in.Providers {
			keep[pv.ProviderID] = true
		}
		for _, row := range existing {
			if !keep[row.ProviderID.String()] {
				if err := q.RemoveProductProvider(ctx, gen.RemoveProductProviderParams{
					ProductID: p.ID, ProviderID: row.ProviderID,
				}); err != nil {
					return err
				}
			}
		}
	}
	// Deals full-replace.
	if in.DealIDs != nil {
		keep := make([]pgtype.UUID, 0, len(in.DealIDs))
		for _, d := range in.DealIDs {
			keep = append(keep, uuidOf(d))
		}
		if err := q.DeleteProductDealsNotIn(ctx, gen.DeleteProductDealsNotInParams{
			ProductID: p.ID, KeepIds: keep,
		}); err != nil {
			return err
		}
	}
	if err := s.syncProductLinks(ctx, q, p.ID, in); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

(Verify generated param names after `sqlc generate` — e.g. `KeepIds`, `ValueJson`.)

- [ ] **Step 3: Add list/detail/restore/link/type/schema service methods**

```go
func (s *Service) AdminListProducts(ctx context.Context, typeSlug, q, status, sort string, limit, offset int32) ([]gen.AdminListProductsRow, int64, error) {
	if status == "" {
		status = "active"
	}
	sortArg := ""
	switch sort {
	case "name:asc", "name:desc", "created_at:asc":
		sortArg = sort
	}
	rows, err := s.store.read.AdminListProducts(ctx, gen.AdminListProductsParams{
		Column1: typeSlug, Column2: q, Column3: status, Column4: sortArg,
		LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.AdminCountProducts(ctx, gen.AdminCountProductsParams{
		Column1: typeSlug, Column2: q, Column3: status,
	})
	return rows, total, err
}

type ProductDetail struct {
	Product  gen.AdminGetProductBySlugRow   `json:"product"`
	Providers []map[string]any             `json:"providers"`
	Deals    []gen.ListProductDealsRow     `json:"deals"`
	Fields   []gen.ListCustomFieldValuesWithDefsByEntityRow `json:"field_values"`
}

func (s *Service) AdminGetProduct(ctx context.Context, slug string) (*ProductDetail, error) {
	p, err := s.store.read.AdminGetProductBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	d := &ProductDetail{Product: p}
	provs, err := s.store.read.ListProductProviders(ctx, p.ID)
	if err == nil {
		for _, pp := range provs {
			d.Providers = append(d.Providers, map[string]any{
				"provider_id": pp.ProviderID.String(), "provider_slug": pp.ProviderSlug,
				"name": pp.Name, "price_from": pp.PriceFrom, "offer_url": pp.OfferUrl,
				"has_deal": pp.HasDeal,
			})
		}
	}
	deals, err := s.store.read.ListProductDeals(ctx, p.ID)
	if err == nil {
		d.Deals = deals
	}
	fv, err := s.store.read.ListCustomFieldValuesWithDefsByEntity(ctx, gen.ListCustomFieldValuesWithDefsByEntityParams{
		EntityType: "product", EntityID: p.ID,
	})
	if err == nil {
		d.Fields = fv
	}
	return d, nil
}
```

Plus thin wrappers `RestoreProduct`, `HardDeleteProduct` (both `s.store.write.*`), `SetProductDeal` (mirrors `SetProductProvider`), `AdminListProductTypes`, `AdminGetProductType` (type + `ListAssignmentsByEntityClass` + `ListCustomFieldsByGroup` per group), `UpdateProductType` (label/icon/is_enabled/config + append-category provisioning — reuse the categories→groups→pool block extracted from `CreateProductType` into a helper `provisionTypeCategories(ctx, q, slug, label, icon, categories)` called by both create and patch), `DeleteProductType` (pre-check `AdminCountProductsByType` → `fmt.Errorf("type has %d products", n)` mapped to 409 in handler), and:

```go
type FieldSchemaField struct {
	ID, Key, Label, FieldType string
	Options                   json.RawMessage
	Placeholder, DefaultValue *string
	IsRequired                bool
	Position                  int32
	Section                   *string
}
type FieldSchemaGroup struct {
	ID, Slug, Name string
	Position       int32
	Fields         []FieldSchemaField
}

func (s *Service) ProductFieldSchema(ctx context.Context, typeSlug string) ([]FieldSchemaGroup, error) {
	rows, err := s.store.read.ListProductTypeGroupsWithFields(ctx, typeSlug)
	if err != nil {
		return nil, err
	}
	var out []FieldSchemaGroup
	idx := map[string]int{}
	for _, r := range rows {
		i, ok := idx[r.GroupID.String()]
		if !ok {
			out = append(out, FieldSchemaGroup{
				ID: r.GroupID.String(), Slug: r.GroupSlug, Name: r.GroupName, Position: r.GroupPosition,
			})
			i = len(out) - 1
			idx[r.GroupID.String()] = i
		}
		if r.FieldID.Valid {
			out[i].Fields = append(out[i].Fields, FieldSchemaField{
				ID: r.FieldID.String(), Key: r.Key.String, Label: r.Label.String,
				FieldType: r.FieldType.String, Options: r.Options,
				Placeholder: r.Placeholder, IsRequired: r.IsRequired.Bool,
				Position: r.FieldPosition.Int32, Section: r.Section,
			})
		}
	}
	return out, nil
}
```

(Adjust `.Valid`/`.String` per generated row types after sqlc-gen.)

- [ ] **Step 4: Also update `CreateProductType`** to write `CreateAssignment` rows per category group:

Inside the existing per-category loop in `entity_directory.go` after `groups = append(...)`:

```go
if _, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
	GroupID: pgtype.UUID{Bytes: g.ID.Bytes, Valid: true},
	EntityClass: slug, Position: int32(i),
}); err != nil {
	return fmt.Errorf("assigning group %s: %w", gslug, err)
}
```

- [ ] **Step 5: Build + commit**

```bash
go build ./internal/wiki/ && git add internal/wiki/product_admin.go internal/wiki/entity_directory.go
git commit -m "products: service layer — tx create/update with providers+deals+cfv, type CRUD, field schema"
```

---

### Task 4: Handlers + route registration + aliases

**Files:**
- Modify: `internal/wiki/product_admin.go` (new handlers)
- Modify: `internal/server/server.go` (route block near line 6664–6685)

**Interfaces:**
- Consumes: Task 3 service methods.
- Produces: HTTP contract per spec — `GET/POST/PATCH/DELETE /admin/products*`, `/admin/product-types*`, `GET /products/types/{slug}/field-schema`; `/admin/wiki/*` legacy paths kept on same handlers.

- [ ] **Step 1: Add handlers in `product_admin.go`**

```go
// HandleAdminListProducts serves GET /admin/products — DataTable contract.
func (h *Handlers) HandleAdminListProducts(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	typeSlug, q, status := qp.Get("type"), qp.Get("q"), qp.Get("status")
	sort := qp.Get("sort")
	if dir := qp.Get("dir"); sort != "" && dir != "" {
		sort += ":" + dir
	}
	page, _ := strconv.Atoi(qp.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(qp.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 25
	}
	rows, total, err := h.service.AdminListProducts(r.Context(), typeSlug, q, status, sort, int32(limit), int32((page-1)*limit))
	if err != nil {
		slog.Error("products: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list products")
		return
	}
	if rows == nil {
		rows = []gen.AdminListProductsRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows, "total": total, "page": page, "page_size": limit,
	})
}

// HandleAdminGetProduct serves GET /admin/products/{slug}.
func (h *Handlers) HandleAdminGetProduct(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.AdminGetProduct(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": d})
}

// HandleRestoreProduct serves POST /admin/products/{slug}/restore.
func (h *Handlers) HandleRestoreProduct(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RestoreProduct(r.Context(), chi.URLParam(r, "slug")); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "restore_failed", "Failed to restore product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleBulkProducts serves POST /admin/products/bulk — {action, ids[]}
// where ids are slugs (products are slug-addressed).
func (h *Handlers) HandleBulkProducts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	failed := []string{}
	for _, slug := range body.IDs {
		var err error
		switch body.Action {
		case "delete":
			err = h.service.DeleteProduct(r.Context(), slug)
		case "restore":
			err = h.service.RestoreProduct(r.Context(), slug)
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "unknown_action", "Unsupported bulk action")
			return
		}
		if err != nil {
			failed = append(failed, slug)
		}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"failed": failed}})
}
```

Extend `HandleDeleteProduct` for `?force=true` hard delete (check the `admin.hard_delete` permission via the same mechanism other hard-delete paths use — look at how deletions handlers check it, e.g. `internal/admin` or deletions surface; simplest correct check: `rbacResolver.HasPermission(ctx, userID, "admin.hard_delete")` — match whatever helper existing code uses; search for `"admin.hard_delete"` in `internal/` for the call pattern). Inside `HandleDeleteProduct`:

```go
if r.URL.Query().Get("force") == "true" {
	if err := h.service.HardDeleteProduct(r.Context(), slug); ...
}
```

Add `HandleAdminProductTypes` (list), `HandleAdminGetProductType`, `HandleUpdateProductType` (decodes `ProductTypeInput{Label, Icon, IsEnabled, Config, Categories}` → `UpdateProductType` + append-provision), `HandleDeleteProductType` (map the "type has products" error → 409 `type_has_products`), `HandleProductFieldSchema` (`GET /products/types/{slug}/field-schema`, auth-only), `HandleSetProductDeal` (mirror of `HandleSetProductProvider`).

- [ ] **Step 2: Register routes in `server.go`** (inside the same authenticated mux block, replacing/augmenting the `/admin/wiki/products` block):

```go
// §80 Product Domain — canonical admin CRUD (products.* / product_types.* nodes).
// Legacy /admin/wiki/products* paths below stay wired to the same handlers.
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.list")).Get("/admin/products", wikiHandlers.HandleAdminListProducts)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.view")).Get("/admin/products/{slug}", wikiHandlers.HandleAdminGetProduct)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.create")).Post("/admin/products", wikiHandlers.HandleCreateProduct)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Patch("/admin/products/{slug}", wikiHandlers.HandleUpdateProduct)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.delete")).Delete("/admin/products/{slug}", wikiHandlers.HandleDeleteProduct)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Post("/admin/products/{slug}/restore", wikiHandlers.HandleRestoreProduct)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.delete")).Post("/admin/products/bulk", wikiHandlers.HandleBulkProducts)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Put("/admin/products/{slug}/providers/{provider_id}", wikiHandlers.HandleSetProductProvider)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Delete("/admin/products/{slug}/providers/{provider_id}", wikiHandlers.HandleSetProductProvider)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Put("/admin/products/{slug}/deals/{coupon_id}", wikiHandlers.HandleSetProductDeal)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "products.update")).Delete("/admin/products/{slug}/deals/{coupon_id}", wikiHandlers.HandleSetProductDeal)

r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "product_types.list")).Get("/admin/product-types", wikiHandlers.HandleAdminProductTypes)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "product_types.view")).Get("/admin/product-types/{slug}", wikiHandlers.HandleAdminGetProductType)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "product_types.create")).Post("/admin/product-types", wikiHandlers.HandleCreateProductType)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "product_types.update")).Patch("/admin/product-types/{slug}", wikiHandlers.HandleUpdateProductType)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "product_types.delete")).Delete("/admin/product-types/{slug}", wikiHandlers.HandleDeleteProductType)

// Field schema for the product forms — auth-only like /custom-field-groups.
r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/products/types/{slug}/field-schema", wikiHandlers.HandleProductFieldSchema)
```

Route-order note: `/admin/products/bulk` must be registered before `/admin/products/{slug}` GET in chi, or chi will treat `bulk` as a slug — in chi, static segments win over params at the same level, so registration order is safe either way; verify no `{slug}` pattern swallows `bulk`.

Also update the legacy block's comments: keep `/admin/wiki/products*` wired (aliases) but note "deprecated aliases — canonical paths are /admin/products*". Keep their `wiki.manage` gates as-is (legacy contract).

- [ ] **Step 3: Build + vet**

```bash
go build ./... && go vet ./internal/wiki ./internal/server
```

- [ ] **Step 4: Commit** — `git commit -m "products: admin CRUD routes + handlers, product-types CRUD, field-schema endpoint"`

---

### Task 5: Backend tests

**Files:**
- Create: `internal/wiki/product_admin_test.go`

**Interfaces:** consumes Task 3 service + Task 4 handlers.

- [ ] **Step 1: Write tests** (follow existing wiki test style — check `internal/wiki/*_test.go` for the store/test harness; use `internal/testdb` if present). Cover:
  - `CreateProduct` with providers + deal_ids + custom_fields writes all rows (testdb).
  - `UpdateProduct` replace semantics: removing a provider/deal deletes the link row.
  - `clear_field_ids` deletes the cfv row.
  - `CreateProductType` writes `custom_field_group_assignments` per category.
  - `UpdateProductType` with new category provisions group + assignment + pool row.
  - `DeleteProductType` errors when products exist (→ 409 in handler test).
  - Handler RBAC denial: unauthenticated/no-node → 401/403 on `GET /admin/products`.
  - `?force=true` without `admin.hard_delete` → 403.
- [ ] **Step 2: `go test ./internal/wiki/` — green.**
- [ ] **Step 3: Commit** — `git commit -m "products: service+handler tests"`

---

### Task 6: JEOS spec + catalog updates

**Files:**
- Modify: `.jeos/sections/80-destination-wiki.md` (Product Domain amendment)
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/sections/03-rbac.md` (node list)
- Modify: `docs/superpowers/specs/2026-11-06-wiki-dynamic-entity-kinds.md` (pointer to new spec)

- [ ] **Step 1:** §80 amendment — `product_deals` table, `/admin/products*` + `/admin/product-types*` contract, `products.*`/`product_types.*` nodes, `entity_class = type slug` assignments, `GET /products/types/{slug}/field-schema`, `/products/new` surfaces (admin + main), legacy `/admin/wiki/products*` = deprecated aliases.
- [ ] **Step 2:** Routes catalog — add every route from Task 4 under the admin section; mark `/admin/wiki/products*` deprecated aliases.
- [ ] **Step 3:** §03 RBAC — add the 10 nodes.
- [ ] **Step 4:** Commit — `git commit -m "jeos: §80 product management amendment + catalog/RBAC updates"`

---

### Task 7: Admin app — API client + sidebar + `/products` list page

**Files:**
- Modify: `apps/admin/src/lib/api.ts` (add `productsAdminApi`, `productTypesAdminApi`)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (Monetization section)
- Modify: `apps/admin/src/lib/locales/en.json`, `fr.json` (nav keys `nav.products`, `nav.product_types` + page strings)
- Create: `apps/admin/src/routes/(app)/products/+page.svelte`

**Interfaces:**
- Consumes: `GET /admin/products` list contract; `productsAdminApi` used by Tasks 8.
- Produces: `productsAdminApi` shape used by form pages:
  ```ts
  productsAdminApi.list(params), .get(slug), .create(body), .update(slug, body),
  .delete(slug, force?), .restore(slug), .bulk(body),
  .setProvider(slug, providerId, body|null), .setDeal(slug, couponId, body|null)
  productTypesAdminApi.list(), .get(slug), .create(body), .update(slug, body), .delete(slug)
  ```

- [ ] **Step 1: API client** — append to `api.ts` matching the `affiliateProvidersAdminApi` style:

```ts
export const productsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/products${params ? `?${params}` : ''}`),
	get: (slug: string) => api<unknown>(`/admin/products/${slug}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/products', { method: 'POST', body }),
	update: (slug: string, body: Record<string, unknown>) => api<unknown>(`/admin/products/${slug}`, { method: 'PATCH', body }),
	delete: (slug: string, force = false) =>
		api<unknown>(`/admin/products/${slug}${force ? '?force=true' : ''}`, { method: 'DELETE' }),
	restore: (slug: string) => api<unknown>(`/admin/products/${slug}/restore`, { method: 'POST', body: {} }),
	bulk: (body: { action: string; ids: string[] }) =>
		api<unknown>('/admin/products/bulk', { method: 'POST', body }),
	setProvider: (slug: string, providerId: string, body?: Record<string, unknown>) =>
		api<unknown>(`/admin/products/${slug}/providers/${providerId}`,
			body ? { method: 'PUT', body } : { method: 'DELETE' }),
	setDeal: (slug: string, couponId: string, body?: Record<string, unknown>) =>
		api<unknown>(`/admin/products/${slug}/deals/${couponId}`,
			body ? { method: 'PUT', body } : { method: 'DELETE' }),
};

export const productTypesAdminApi = {
	list: () => api<unknown>('/admin/product-types'),
	get: (slug: string) => api<unknown>(`/admin/product-types/${slug}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/product-types', { method: 'POST', body }),
	update: (slug: string, body: Record<string, unknown>) => api<unknown>(`/admin/product-types/${slug}`, { method: 'PATCH', body }),
	delete: (slug: string) => api<unknown>(`/admin/product-types/${slug}`, { method: 'DELETE' }),
};
```

- [ ] **Step 2: Sidebar** — in the `monetization` section after the `/deals` and `/coupon-feeds` items:

```ts
{ path: "/products", label: t("nav.products", "Products"), icon: Package, permission: "products.list" },
{ path: "/product-types", label: t("nav.product_types", "Product Types"), icon: ListOrdered, permission: "product_types.list" },
```

(`Package`, `ListOrdered` — add to lucide imports if absent.)

- [ ] **Step 3: `/products` list page** — `EntityPage` scaffold:

```svelte
<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { productsAdminApi, productTypesAdminApi } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	type Row = {
		id: string; slug: string; name: string; type_slug: string; type_label: string;
		pricing_model: string | null; price_from: number | null;
		rating_avg: number | null; provider_count: number; deal_count: number;
		deleted_at: string | null; created_at: string;
	};

	let typeOptions = $state<Array<{ value: string; label: string }>>([]);
	onMount(async () => {
		const res = await productTypesAdminApi.list();
		const items = (res.data as any)?.items ?? res.data ?? [];
		if (res.ok) typeOptions = items.map((tp: any) => ({ value: tp.slug, label: tp.label }));
	});

	const config = $derived<EntityPageConfig>({
		title: t('products.title', 'Products'),
		description: t('products.description', 'Manage the product catalog, providers and deals.'),
		hideCreate: true, // dedicated page at /products/new
		createPermission: 'products.create',
		editPermission: 'products.update',
		deletePermission: 'products.delete',
		columns: [
			{ key: 'name', label: t('products.col_name', 'Name'), sortable: true },
			{ key: 'type_label', label: t('products.col_type', 'Type'), sortable: false },
			{ key: 'price_from', label: t('products.col_price', 'From'), sortable: false, format: (v) => (v == null ? '—' : String(v)) },
			{ key: 'rating_avg', label: t('products.col_rating', 'Rating'), sortable: false },
			{ key: 'provider_count', label: t('products.col_providers', 'Providers'), sortable: false },
			{ key: 'deal_count', label: t('products.col_deals', 'Deals'), sortable: false },
			{ key: 'created_at', label: t('products.col_created', 'Created'), sortable: true, format: (v) => new Date(v).toLocaleDateString() },
		],
		filters: [
			{ key: 'type', label: t('products.filter_type', 'Type'), type: 'select', options: typeOptions },
			{ key: 'status', label: t('products.filter_status', 'Status'), type: 'select', options: [
				{ value: 'active', label: t('common.active', 'Active') },
				{ value: 'deleted', label: t('common.deleted', 'Deleted') },
				{ value: 'all', label: t('common.all', 'All') },
			] },
		],
		fields: [
			{ key: 'name', label: t('products.fld_name', 'Name'), type: 'text', required: true },
			{ key: 'slug', label: t('products.fld_slug', 'Slug'), type: 'text', required: true },
		],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({ page: String(page), limit: String(pageSize) });
			if (q) params.set('q', q);
			if (sort) params.set('sort', sort.key);
			if (sort?.dir) params.set('dir', sort.dir);
			if (filter?.type) params.set('type', filter.type);
			if (filter?.status) params.set('status', filter.status);
			const res = await productsAdminApi.list(params.toString());
			const d = res.data as any;
			return { data: d?.data ?? d ?? [], total: d?.total ?? 0, totalPages: Math.ceil((d?.total ?? 0) / pageSize) };
		},
		remove: async (row: Row) => {
			const res = await productsAdminApi.delete(row.slug);
			return { ok: res.ok, error: res.error?.message };
		},
		restore: async (row: Row) => {
			const res = await productsAdminApi.restore(row.slug);
			return { ok: res.ok, error: res.error?.message };
		},
		hardRemove: async (row: Row) => {
			const res = await productsAdminApi.delete(row.slug, true);
			return { ok: res.ok, error: res.error?.message };
		},
		isDeleted: (row: Row) => row.deleted_at != null,
		actions: [
			{ label: t('common.edit', 'Edit'), onclick: (row: Row) => goto(`/products/${row.slug}`) },
			{ label: t('common.delete', 'Delete'), variant: 'destructive', show: (row: Row) => row.deleted_at == null, onclick: (row: Row, ctx) => ctx?.openDelete(row) },
		],
		exportable: true,
		exportFilename: 'products',
	});
</script>

<EntityPage {config} />
```

Match the exact `Column`/`ActionItem`/`FilterDef` field names against `apps/admin/src/lib/components/ui/data-table/types.ts` during implementation (e.g. `format` vs `render`, `sort` param shape in `load`). The header create action must navigate to `/products/new` — check `EntityPageConfig` for a `createHref`/`onCreate` hook; if absent use a page-level header button via the actions snippet (look at how `/spaces` does it, which "navigates to a dedicated page" per AGENTS.md).

- [ ] **Step 4: Locale strings** for all new `t()` keys (en + fr).
- [ ] **Step 5: Commit** — `git commit -m "admin: products list page, api client, monetization nav"`

---

### Task 8: Admin app — `ProductForm` + `/products/new` + `/products/[slug]`

**Files:**
- Create: `apps/admin/src/lib/components/products/product-form.svelte`
- Create: `apps/admin/src/routes/(app)/products/new/+page.svelte`
- Create: `apps/admin/src/routes/(app)/products/[slug]/+page.svelte`

**Interfaces:**
- Consumes: `productsAdminApi`, `productTypesAdminApi`, `affiliateProvidersAdminApi`, `GET /products/types/{slug}/field-schema`, `ListCouponsForProviders` (deal options — fetch via `couponsAdminApiExtended.list` filtered or a small `api()` call to `/admin/coupons?status=active&provider_id=` per selected provider; simplest: fetch all active deals via existing coupons admin list `status=active&page_size=200` and filter client-side by selected provider ids).
- Produces: `ProductForm` props `{ mode: 'create' | 'edit', initial?: ProductDetail, onsaved: (slug) => void, oncancel: () => void }`.

- [ ] **Step 1: `product-form.svelte`** — sections:
  - **Type picker** (select from `productTypesAdminApi.list()`); `onchange` → `api('/products/types/{slug}/field-schema')` → `schema` state; disabled in edit mode.
  - **Core fields**: name, slug (auto-slugify from name on create, editable), tagline, website_url, pricing_model (select: free/freemium/subscription/one-time/usage — load from product type `config.pricing_models` if present, else free text), price_from, logo (media picker — reuse the `/tags` page's media picker pattern).
  - **Custom fields**: per `schema` group render inputs by `field_type` (text→input, number→number input, checkbox→checkbox, select→options select, date→date input, json→JsonValueEditor from `@japaconnect/ui`, rich_text/textarea→textarea). Bind into `fieldValues: Record<field_id, value>`; on submit map to `custom_fields[{field_id, value_<kind>}]` by field_type.
  - **Providers repeater**: rows `{provider_id (autocomplete select via affiliateProvidersAdminApi.list('page_size=100')), price_from, offer_url, position}` + add/remove buttons; show `has_deal` hint from provider data if present.
  - **Deals multi-select**: options = active coupons filtered to selected provider ids (fallback: all active when none selected), checkbox list or multi-autocomplete; binds `deal_ids`.
  - Submit: `productsAdminApi.create`/`update` with the spec body; error → inline alert.
- [ ] **Step 2: `/products/new/+page.svelte`** — `<ProductForm mode="create" onsaved={(slug) => goto(`/products/${slug}`)} oncancel={() => goto('/products')} />` inside a `PageHeader`/breadcrumb shell matching `/deals` page chrome. Permission guard: hide/forbid when lacking `products.create` (check how admin guards pages — likely a `hasPerm` store on the user session; mirror an existing gated page).
- [ ] **Step 3: `/products/[slug]/+page.svelte`** — `onMount` → `productsAdminApi.get(slug)` → map `data.product` + `field_values` + `providers` + `deals` into `initial` → `<ProductForm mode="edit" {initial} .../>` + Delete/Restore buttons (deleted banner when `deleted_at`).
- [ ] **Step 4: Commit** — `git commit -m "admin: product create/edit form with type fields, providers, deals"`

---

### Task 9: Admin app — `/product-types` pages

**Files:**
- Create: `apps/admin/src/routes/(app)/product-types/+page.svelte`
- Create: `apps/admin/src/routes/(app)/product-types/[slug]/+page.svelte`

**Interfaces:** consumes `productTypesAdminApi`, `custom-field-groups/{id}/fields` endpoints (`api('/admin/field-groups')`, `api('/custom-field-groups/{id}/fields')`, `api('/custom-fields/{id}', {method:'PATCH'})` etc. — check `api.ts` for an existing `customFieldsApi`/`fieldGroupsApi`).

- [ ] **Step 1: `/product-types`** — `EntityPage`: columns slug, label, icon, is_enabled, product_count, created_at; create = `formSnippet` w/ label, slug, icon, categories repeater (`[{slug,label}]`); row action Edit → `/product-types/{slug}`; delete via `productTypesAdminApi.delete` (surfaces 409 message when products exist); enable/disable quick action via `update`.
- [ ] **Step 2: `/product-types/[slug]`** — detail: core fields edit (label, icon, is_enabled toggle, config JSON editor), "Add category" (posts `update` with appended categories → backend provisions group+assignment+pool), and a per-group field list (`GET /admin/product-types/{slug}` response carries groups+fields) with inline add/edit/delete via `/custom-field-groups/{id}/fields` + `/custom-fields/{id}` endpoints; link out to `/custom-fields` full editor for advanced field config.
- [ ] **Step 3: Commit** — `git commit -m "admin: product types management"`

---

### Task 10: Main app — `/products/new` + `/products` CTA rewire

**Files:**
- Create: `apps/main/src/routes/(app)/products/new/+page.server.ts`
- Create: `apps/main/src/routes/(app)/products/new/+page.svelte`
- Modify: `apps/main/src/routes/(app)/products/+page.server.ts` (canCreate → `products.create`)
- Modify: `apps/main/src/routes/(app)/products/+page.svelte` (remove inline form; PageAction → `/products/new`)
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json` (new keys)

**Interfaces:**
- Consumes: `GET /products/types`, `GET /products/types/{slug}/field-schema`, `GET /deals/partners?page_size=100` (provider options), `GET /coupons?status=active` (deal options — verify param names on the public coupons list; filter client-side by provider), `POST /admin/products`.

- [ ] **Step 1: `+page.server.ts`** — mirror `/products/deals/new/+page.server.ts`:

```ts
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const perms = event.locals.user.permissions ?? [];
	if (!hasPerm(perms, 'products.create')) throw error(403);
	const ctx = apiContext(event);
	const [types, providers, deals] = await Promise.all([
		apiGet<{ items: { slug: string; label: string }[] }>(ctx, '/products/types').catch(() => null),
		apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100').catch(() => ({ items: [] })),
		apiList<{ id: string; title: string; provider_id: string | null }>(ctx, '/coupons?status=active&page_size=200').catch(() => ({ items: [] })),
	]);
	return { types: types?.items ?? [], providers: providers.items, deals: deals.items };
};
```

- [ ] **Step 2: `+page.svelte`** — port `ProductForm` structure to main-app conventions (use `AutocompleteSelect` like deals/new, `.inp` styles, `t()` i18n): type select → `api.get('/products/types/{slug}/field-schema')` → dynamic fields → providers repeater → deals checkbox list → `api.post('/admin/products', body)` → `goto('/product/' + slug)` on success. Error surfacing per `res.error.message`.
- [ ] **Step 3: `/products` page changes** — delete the `showCreate`/`form`/`submitCreate` block (lines ~100–132 + template card ~161–211); `PageAction onclick` → `goto('/products/new')`; `+page.server.ts` `canCreate: hasPerm(perms, 'products.create')`.
- [ ] **Step 4: i18n keys** — `products.new_title`, field labels, provider/deal strings (en + fr).
- [ ] **Step 5: `bun run check` (or `bunx svelte-check`) in `apps/main`** — clean.
- [ ] **Step 6: Commit** — `git commit -m "main: /products/new create form, products CTA rewire to products.create"`

---

### Task 11: Verification

- [ ] `go build ./... && go vet ./...` — clean.
- [ ] `go test ./internal/wiki/` — green.
- [ ] `bun run check` in `apps/admin` and `apps/main` — clean.
- [ ] Manual smoke (if dev servers available): admin `/products` list renders; `/products/new` type→schema load + create lands on `/products/{slug}`; main `/products/new` create works; type edit provisions a new category group.
- [ ] `git status` — only plan files changed; each task committed.

## Self-review notes

- Spec coverage: all routes, RBAC nodes, migration items, both `/products/new` forms, admin pages, catalog/JEOS updates, tests — covered by Tasks 1–11.
- Legacy `/admin/wiki/products*` POST/PATCH/DELETE stay mounted (aliases); the main-app `/products` inline form that called `POST /admin/wiki/products` is removed in Task 10 anyway.
- `ProductInput.providers`/`deal_ids` use `nil` vs empty-slice semantics on PATCH: `nil` = untouched, `[]` = clear all — the `if in.Providers != nil` guards encode this.
