# Product Management — Design Spec

**Date:** 2026-11-06
**Scope:** Product management in the admin panel, product-type → custom-field-group
association, `/products/new` create forms in admin + main, providers and deals on products.
**JEOS refs:** `sections/80-destination-wiki.md` (Product Domain amendment),
`sections/30-generic-comparison-system.md`, `sections/06-*` (custom fields),
`sections/03-rbac.md`, `sections/55-coupons-external-affiliates.md`,
`docs/superpowers/specs/2026-11-06-wiki-dynamic-entity-kinds.md` (product domain origin).

## Background / current state

The product domain is partially implemented (§80, de-wikied in migration 001080):

- Tables: `product_types`, `products`, `product_providers` (N:M → `affiliate_providers`).
  Deals are `coupons` rows on providers; `has_deal` is derived, not stored.
- Public API: `GET /products`, `/products/types`, `/products/{slug}`,
  `/products/{slug}/providers`.
- Admin API (gated `wiki.manage`): `POST /admin/wiki/product-types`,
  `POST/PATCH/DELETE /admin/wiki/products[/{slug}]`,
  `POST/DELETE /admin/wiki/products/{slug}/providers/{provider_id}`.
- `CreateProductType` provisions per-category `custom_field_groups`
  (`wiki_{type}_{category}`) + `comparison_type_definitions` + pool rows in one tx —
  but the groups are **not** registered in `custom_field_group_assignments`, so a
  type's field schema cannot be resolved via `GET /custom-field-groups?entity_class=`.
- Main app: `/products` directory (inline mini create form gated `wiki.manage`),
  `/product/[slug]`, `/products/deals[/new]`. No `/products/new`.
- Admin app: deals/coupons/affiliate pages under `(app)/`. **No product pages.**

## Decisions (confirmed with user)

| Question | Decision |
|---|---|
| Main-app create access | RBAC node `products.create` — not a hardcoded staff check, not open submission |
| Permission model | New dedicated `products.*` / `product_types.*` nodes (seeded migration) |
| Product types in admin | Full management: create w/ categories, edit, enable/disable, manage bound field-group fields |
| Providers + deals on form | Providers repeater **plus** explicit deal linking → new `product_deals` table |
| `has_deal` semantics | Stays derived from provider coupons; `product_deals` is the explicit "featured deals" list |

## Data model

### New table: `product_deals`

```sql
CREATE TABLE product_deals (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    coupon_id  uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, coupon_id)
);
CREATE INDEX idx_product_deals_coupon ON product_deals (coupon_id);
```

A product's "featured deals" = explicit `product_deals` links. Deal provenance
(provider) resolves through `coupons.provider_id`. Provider-level `has_deal`
derivation is unchanged.

### Field-group association (§80 "entity_class = product type slug")

- Backfill `custom_field_group_assignments`: for every `product_types` row, every
  existing `wiki_{type}_{category}` group gets `entity_class = {type_slug},
  entity_id = NULL` (applies to all products of the type).
- `CreateProductType` writes the same assignments for new groups.
- `PATCH /admin/product-types/{slug}` can add categories → new group + assignment
  + pool row, same provisioning path.
- Seed `custom_field_entity_types` row `('product', 'Product')` so
  `custom_field_values.entity_type = 'product'` is registered.

### RBAC nodes (seed migration, granted to super_admin; BE-005 full seed)

```
products.list  products.view  products.create  products.update  products.delete
product_types.list  product_types.view  product_types.create
product_types.update  product_types.delete
```

## Backend

### Canonical routes

New canonical paths under `/admin/products` + `/admin/product-types`, gated by the
new nodes. Existing `/admin/wiki/products*` routes stay wired to the same handlers
(deprecated aliases — the main app already calls them; they keep working).

```
GET    /admin/products                 products.list
GET    /admin/products/{slug}          products.view
POST   /admin/products                 products.create
PATCH  /admin/products/{slug}          products.update
DELETE /admin/products/{slug}          products.delete (soft; ?force=true → admin.hard_delete)
POST   /admin/products/{slug}/restore  products.update
POST   /admin/products/bulk            products.delete {action: delete|restore, ids[]}
PUT    /admin/products/{slug}/providers/{provider_id}    products.update
DELETE /admin/products/{slug}/providers/{provider_id}    products.update
PUT    /admin/products/{slug}/deals/{coupon_id}          products.update
DELETE /admin/products/{slug}/deals/{coupon_id}          products.update

GET    /admin/product-types            product_types.list
GET    /admin/product-types/{slug}     product_types.view (groups + fields)
POST   /admin/product-types            product_types.create
PATCH  /admin/product-types/{slug}     product_types.update
DELETE /admin/product-types/{slug}     product_types.delete (only when no products)
```

Plus one auth-only (non-admin) read for the forms:

```
GET /products/types/{slug}/field-schema   → groups + fields for a type (one shot)
```

### List contract (`GET /admin/products`)

`?page=&limit=&sort=&q=&type=&status=active|deleted|all` → DataTable-style
`{data: {items, meta:{total,page,limit}}}` per the admin list convention. Each
item carries: id, slug, name, type_slug/label, pricing_model, price_from,
rating_avg, review_count, provider_count, deal_count, logo, deleted_at,
created_at, updated_at.

### Create/update body (extended `ProductInput`)

```json
{
  "type_slug": "ai-agent",
  "slug": "kimi-k3", "name": "Kimi K3",
  "tagline": "...", "website_url": "...", "pricing_model": "...", "price_from": 0,
  "providers": [{"provider_id": "...", "price_from": "12.00", "offer_url": "...", "position": 0}],
  "deal_ids": ["<coupon uuid>", "..."],
  "custom_fields": [{"field_id": "...", "value_text": "...", "value_number": 0,
                     "value_bool": true, "value_date": "...", "value_json": {...}}]
}
```

- `providers` on PATCH = full replace semantics (diff → upsert/remove) inside the tx.
- `deal_ids` on PATCH = full replace semantics.
- `custom_fields` written as `custom_field_values` rows (`entity_type='product'`,
  `entity_id=product.id`) via the existing upsert query — PATCH upserts only the
  fields sent; explicit clearing goes through a `clear_field_ids: [...]` array on
  the same body.
- Create runs in one tx: product row → provider links → deal links → cfv rows.

### Product type update (`PATCH /admin/product-types/{slug}`)

Mutable: `label`, `icon`, `is_enabled`, `config`, `categories` (append-only —
new categories provision group + assignment + pool row; existing categories are
never removed here since their groups may carry field data). `slug` is immutable.

Type delete: only allowed when zero live products reference it
(`ON DELETE RESTRICT` already enforces; the handler pre-checks and returns 409).

### cfv read/write reuse

- Read: `GET /custom-field-values/product/{id}` (existing).
- Field-schema read: new `GET /products/types/{slug}/field-schema` returns
  `[{group: {id, slug, name, category?}, fields: [field defs]}]` resolved from
  `custom_field_group_assignments(entity_class = type_slug)` +
  `custom_field_groups` + `custom_fields`. Auth-only (`RequireAuth`), like the
  sibling `/custom-field-groups` routes.
- Field definition management (create/edit fields inside a type's group): reuse
  existing `/custom-field-groups/{id}/fields` CRUD — no new endpoints.

## Admin app (`apps/admin`)

New pages under `src/routes/(app)/` next to deals/coupons/affiliate:

| Route | Content |
|---|---|
| `/products` | `EntityPage` table — columns: name, type, price_from, rating_avg, provider_count, deal_count, status (deleted badge), created_at. Server-side sort/search/`type` filter/paginate, bulk delete + restore, export. Row → `/products/{slug}`. Header "New product" → `/products/new` (`hideCreate` + custom action). |
| `/products/new` | Dedicated create page (per request #3). Sections: core fields → type picker (loads field-schema on change → dynamic custom-fields renderer) → providers repeater (provider autocomplete via `GET /admin/affiliate-providers`, price_from, offer_url, position, has_deal hint) → deals multi-select (options = deals on selected providers + free deal search via `GET /deals` admin/public search). |
| `/products/[slug]` | Edit page — same `ProductForm` component in edit mode (pre-loaded via `GET /admin/products/{slug}` + cfv), providers/deals replace-on-save, delete/restore actions. |
| `/product-types` | `EntityPage` — slug, label, icon, enabled, product count, bound group names. Create modal w/ categories repeater. Edit → `/product-types/{slug}` detail page: core fields + enable toggle + "add category" + per-group field management (reuses `/admin/field-groups` + `/custom-field-groups/{id}/fields` endpoints; link out to `/custom-fields` for the full editor). |

Shared `ProductForm.svelte` component used by `new` and `[slug]` (mode prop).

Sidebar → **Monetization** section (sits with Deals/Affiliate):

```ts
{ path: "/products",      label: t("nav.products", "Products"),           icon: Package,    permission: "products.list" },
{ path: "/product-types", label: t("nav.product_types", "Product Types"), icon: ListOrdered, permission: "product_types.list" },
```

## Main app (`apps/main`)

- **`/products/new/+page.server.ts`** — `RequireAuth`; `hasPerm(perms, 'products.create')`
  else 403. Loads: product types (`/products/types`), providers
  (`/deals/partners?page_size=100`), field schema is fetched client-side on type
  change (or first type preloaded server-side).
- **`/products/new/+page.svelte`** — same structure as admin's form with main-app
  components (`AutocompleteSelect`, existing input styles, i18n `t()`): type
  picker → dynamic custom fields → providers repeater → deal picker → submit to
  `POST /admin/products` (same pattern as deals/new staff mode calling
  `/admin/coupons`). Success → `/product/{slug}`.
- **`/products`** — replace the inline mini-create card with the `New product`
  `PageAction` navigating to `/products/new`; `canCreate` changes from
  `wiki.manage` to `products.create`.

## JEOS / catalog updates (same PR — Gate 2)

- `.jeos/sections/80-destination-wiki.md` — amendment: `product_deals`,
  `/admin/products*` + `/admin/product-types*` contract, `products.*` /
  `product_types.*` nodes, entity_class assignments, `/products/new` surfaces.
- `.jeos/conventions/api-routes-catalog.md` — every new route.
- `.jeos/sections/03-rbac.md` — new permission nodes.
- `docs/superpowers/specs/2026-11-06-wiki-dynamic-entity-kinds.md` — short pointer
  amendment to this spec (keeps the origin doc current).

## Implementation order

1. Migration: `product_deals` + cfga backfill + `custom_field_entity_types` row +
   RBAC seed (paired up/down, next migration number).
2. sqlc queries (`products.sql`): admin list/count/detail, product-deals,
   product-types admin CRUD, cfga assignment insert, field-schema read → `make sqlc-gen`.
3. Service + handlers: extended create/update tx, link/unlink, type mgmt,
   field-schema endpoint; route registration + alias wiring.
4. JEOS section + catalog updates.
5. Admin app: sidebar nav, `/products`, `/products/new`, `/products/[slug]`,
   `/product-types` (+ detail).
6. Main app: `/products/new`, `/products` CTA rewire.
7. Tests + `go build ./... && go vet ./...` + `bun run check` on touched apps.

## Out of scope

- Review content type wiring (rating_avg/review_count rollups already spec'd
  separately in §80).
- Product create by non-staff organizations (no submission/moderation flow —
  RBAC node only).
- Meilisearch indexing changes.
- Deal creation itself (existing `/products/deals/new` + admin deals surfaces).
