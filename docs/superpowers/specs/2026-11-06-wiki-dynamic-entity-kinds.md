# Wiki Entity Directory + Product Kinds + Reviews — Design Spec

Date: 2026-11-06 (revised: Product model replaces generic entity store)
Status: Draft for review (JEOS amendment to follow in §80 / §30 / §14 before code)
Refs: §80-destination-wiki, §30-generic-comparison-system (Sub-spec 6), §14-content-type-system, migrations 000739, 000971, 001069, 001072

## Goal

Three scoped capabilities on `/wiki/e`:

1. `/wiki/e` — entity-kind index page ("browse the entity wiki").
2. `/wiki/e/{type_slug}` — per-kind entity listing ("All Programs", "All AI Agents").
3. A **`Product` domain model** — `products` + `product_types` (AI Agent, AI Model, Web App, …) — with a `Review` content type, all wired into wiki pages and JCE.

## Current architecture (verified)

- `/wiki/e/[type_slug]/[entity]/[page_slug]` is the only entity route — `/wiki/e` and `/wiki/e/{type}` 404 today.
- Every entity kind maps to a hand-picked backing table (`spaces`, `university_course_catalog`, `university_course_offerings`), a hand-written JCE resolver, seeded `comparison_type_definitions` + pool rows.
- `wiki_pages` binds pages to entities via `entity_type`/`entity_id` + `page_template_id` + `field_group_id`.
- JCE supports **subtype filtering already**: visa kinds share one resolver + comparison types filter on `config.subtype` → `visa_category`. Product types reuse this pattern exactly.
- The JCE pool machinery (Sub-spec 6) is kind-agnostic — consumes whatever cfv a resolver returns.

## Part A — Directory pages

### Backend

- `GET /wiki/entity-kinds` — `{ kinds: [{ slug, label, icon, entity_count, wiki_root }] }` from `wiki_entity_kinds` + kind source counts.
- `GET /wiki/entity-kinds/{slug}/entities?page&page_size&q` — paginated listing via each kind's source adapter (products query `products` filtered by `type`).

### Frontend

- `/wiki/e/+page.svelte` — kind cards → `/wiki/e/{slug}`.
- `/wiki/e/[type_slug]/+page.svelte` — entity listing (card grid, search, pagination). Compatible with the existing `[type_slug]/[entity]/[page_slug]` route.

## Part B — The Product model

### Schema

```
product_types(id, slug unique, label, icon, is_enabled, config jsonb)
-- AI Agent, AI Model, Web App, Website, SaaS, Mobile App, ... — new types are
-- INSERTs here, not new tables.

products(
  id uuid pk,
  product_type_id uuid fk → product_types,
  slug text unique,
  name text,
  vendor text,
  tagline text,
  website_url text,
  logo_id uuid fk → media,
  pricing_model text,          -- free | freemium | subscription | one_time | usage
  price_from numeric,
  rating_avg numeric,          -- denormalized aggregate over reviews
  review_count int,            -- denormalized aggregate over reviews
  geo_space_id uuid,           -- optional geo anchor
  created_by, deleted_at, timestamps
)
```

**Type-specific fields live in `custom_field_values`** (`entity_class` = product type slug, groups per `wiki_{type}_{category}` pattern). Core columns carry what all products share; cfv carries type-specific long tail (`context_window` on AI models, `framework` on web apps).

### Review content type

- New `reviews` content type in `content` (`/studio/create?type=review`) — inherits §14 machinery free: slugs, moderation, comments, feeds, `content_hash`.
- `contents.config` (or `content_entity_links`) binds `product_id` + `rating` (1–5) + optional `pros`/`cons` fields.
- On review publish/unpublish: update `products.rating_avg`/`review_count` in the same flow (or an Asynq rollup task — catalog-registered per §11 if async).

### Wiki wiring

- `product` gets a `wiki_entity_kinds` row (or kinds resolve via `product:{type}` — see open Q).
- Each product gets a `wiki_pages` row (`entity_type='product'`, `entity_id=product.id`, `field_group_id` = its type's bound group) — pages render like every other entity page.
- Product type's field groups seed via `wiki_type_categories` — Overview, Pricing, Features, Reviews, etc.

## Part C — JCE integration

- **One `product` resolver** — `Search/Get/List` reads `products` + batched cfv; honors `config.subtype` = product type slug for like-for-like filtering (the visa-kind pattern).
- **One comparison type per product type** — `comparison_type_definitions` seeded per `product_types` row with `config.subtype` filtering; pool rows seed its bound groups (Sub-spec 6 seed path).
- **Product CRUD + type CRUD** — `POST/PATCH/DELETE /admin/wiki/product-types` (admin RBAC node `wiki.entity_kind.create` — new) and `/wiki/products`; product create auto-creates its wiki page.
- Rating dimensions — `rating_avg`, `review_count` surface as resolver metadata → comparable dimensions (hydrated like cfv).

## Key decisions

| Decision | Choice | Why |
|---|---|---|
| Backing store | Real `products` table + cfv | Queryable core fields (price, rating, vendor); cfv only for type-specific long tail |
| Kind definition | `product_types` lookup row | New kind = config insert, not schema |
| JCE wiring | `config.subtype` filter | Reuses the proven visa-kind subtype pattern |
| Reviews | `review` content type + product link | Free §14 features (moderation, comments, feeds); aggregates denormalize onto product |
| Kind scope | Product-domain only | Doesn't try to be arbitrary entity kinds — programs/cities stay table-backed |

## Open questions

1. Entity kind for products: one `product` kind with subtype filter, vs `product:{type}` kinds — pending which gives cleaner wiki_page/resolver resolution.
2. Review aggregation sync vs Asynq rollup — if async, register `reviews:rollup_product_ratings` in §11 catalog.
3. `geo_space_id` on products — needed now or later?
4. Does `/wiki/e/{type}` for a product type list products, or does the generic entity-kind listing cover it — (same page either way; source adapter decides).

## Implementation order

1. Part A directory pages (independent, ships alone)
2. `product_types` + `products` migration + `review` content type + product link
3. Product resolver + `product` wiki kind + page templates
4. JCE comparison types per product type + pool seed
5. Product/type admin CRUD + `/wiki/e/{type}` create flow
6. JEOS §80/§30/§14 amendments land before each step's code
