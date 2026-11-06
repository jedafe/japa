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

product_providers(
  product_id uuid fk → products,
  provider_id uuid fk → affiliate_providers,
  price_from numeric,            -- provider's price for this product
  offer_url text,                -- provider-specific landing/offer page
  position int,
  unique(product_id, provider_id)
)
```

**Type-specific fields live in `custom_field_values`** (`entity_class` = product type slug, groups per `wiki_{type}_{category}` pattern). Core columns carry what all products share; cfv carries type-specific long tail (`context_window` on AI models, `framework` on web apps).

### Provider + deals integration

- **N:M providers** — a product has many providers via `product_providers` (Kilo/Devin/OpenCode all offer Kimi K3 at different prices). `products.provider_id` is NOT a column — the join table is the truth.
- Providers are the **existing `affiliate_providers` registry** (`base_url`, `tracking_param`, `commission_rate`, `organization_id`).
- **"Deals" = the `coupons` table** — `/deals` is the frontend rename (`coupon_deal_slugs` migration); `coupon_events`, `coupon_redemptions`, `coupon_feed_sources` back it. A "deal" is a `coupons` row on a provider/service.
- `has_deal` is **derived, not a column**: product has ≥1 linked provider with an active coupon/deal or affiliate offer.
- **CTA contract** — the product page renders a provider/price row per `product_providers` entry (mini-comparison block); each row's CTA resolves that provider's affiliate link, falling back to `offer_url`/`website_url`. Same contract on every review — provider repeater carries per-provider CTAs.
- **Monetization loop** — routed clicks write `affiliate_link_events`; conversions pay via `affiliate_content_commissions` at `author_commission_pct`. The CTA carries the *content-author's* attribution on every provider row.
- `has_deal` + provider count surface as JCE dimensions via resolver metadata.

### Review content type

- New `reviews` content type in `content` (`/studio/create?type=review`) — inherits §14 machinery free: slugs, moderation, comments, feeds, `content_hash`.
- `contents.config` (or `content_entity_links`) binds `product_id` + `rating` (1–5) + optional `pros`/`cons` fields + a **`providers` repeater** (array of `{provider_id, price, offer_url}` entries).
- **Review form auto-populate** — selecting a product on `/studio/create?type=review` resolves its `product_type` (read-only display) and fills the provider repeater with the product's `product_providers` entries; the reviewer prunes/edits to the providers they evaluated.
- On review publish/unpublish: update `products.rating_avg`/`review_count` in the same flow (or an Asynq rollup task — catalog-registered per §11 if async).
- Every review renders the product CTA per provider entry — affiliate-linked when the provider has a deal, carrying the review author's attribution on each row.

### Wiki wiring

- `product` gets a `wiki_entity_kinds` row (or kinds resolve via `product:{type}` — see open Q).
- Each product gets a `wiki_pages` row (`entity_type='product'`, `entity_id=product.id`, `field_group_id` = its type's bound group) — pages render like every other entity page.
- Product type's field groups seed via `wiki_type_categories` — Overview, Pricing, Features, Reviews, etc.

## Part C — JCE integration

- **One `product` resolver** — `Search/Get/List` reads `products` + `product_providers` + batched cfv; honors `config.subtype` = product type slug for like-for-like filtering (the visa-kind pattern).
- **One `provider` resolver** — `affiliate_providers` rows become comparable entities; reads provider cfv + `coupon`/`affiliate` aggregates.
- **One comparison type per product type** — `comparison_type_definitions` seeded per `product_types` row with `config.subtype` filtering; pool rows seed its bound groups (Sub-spec 6 seed path).
- **Product CRUD + type CRUD** — `POST/PATCH/DELETE /admin/wiki/product-types` (admin RBAC node `wiki.entity_kind.create` — new) and `/wiki/products`; product create auto-creates its wiki page.
- Rating dimensions — `rating_avg`, `review_count` surface as resolver metadata → comparable dimensions (hydrated like cfv).

### JCE comparison candidates

The model enables four comparison shapes — all like-for-like (§30's one-kind-plus-nested-comparison rule holds):

1. **Product vs Product (per type)** — `ai-agents`, `ai-models`, `web-apps`, `websites`, `saas-tools`, `mobile-apps`. `item_ref_types=[product]` + `config.subtype={type_slug}` → like-for-like subtype filter. Dimension pool = the product type's bound cfv groups (Overview, Pricing, Features, …) + resolver-injected core columns as dims (`price_from`, `rating_avg`, `review_count`, `has_deal`, provider count).

2. **Provider vs Provider** — `providers` comparison type over `affiliate_providers`. Dims: commission_rate, provider_type, active deal count, product coverage, category. Answers "which reseller to use for a given class of product".

3. **Within-product provider matrix** — the product page's `product_providers` table IS a comparison (one product × its providers on price/offer/deal). Renders via the same matrix machinery as a scoped comparison — not a comparison type; it's the per-product detail surface. Reuses `hydrateMatrixCell` on provider cfv where the source is the provider entry.

4. **Deal vs Deal** — `deals` comparison type over `coupons` rows on providers (e.g. "Kilo's Kimi deal vs Devin's Kimi deal"). Dims: discount type/value, expiry, provider. Lower priority — most deals differ by product already covered in #3.

### Comparison-type seeds

| Comparison type slug | Item ref | Subtype filter | Pool (cfv groups) |
|---|---|---|---|
| `ai-agents` | `product` | `ai-agent` | `wiki_ai_agent_{overview,pricing,features,capabilities,reviews}` |
| `ai-models` | `product` | `ai-model` | `wiki_ai_model_{overview,pricing,capabilities,context,reviews}` |
| `web-apps` | `product` | `web-app` | `wiki_web_app_{overview,pricing,features,reviews}` |
| `websites` | `product` | `website` | `wiki_website_{overview,features,stack,reviews}` |
| `saas-tools` | `product` | `saas` | `wiki_saas_{overview,pricing,features,integrations,reviews}` |
| `mobile-apps` | `product` | `mobile-app` | `wiki_mobile_app_{overview,pricing,features,reviews}` |
| `providers` | `provider` | — | `wiki_provider_{overview,offers,commission,coverage}` |
| `deals` | `deal` | — | `wiki_deal_{overview,offer,terms}` |

Subtype filtering is `config.subtype` → `products.product_type_id` join — identical mechanics to `visa_category` on visa kinds.

## Key decisions

| Decision | Choice | Why |
|---|---|---|
| Backing store | Real `products` table + cfv | Queryable core fields (price, rating, vendor); cfv only for type-specific long tail |
| Kind definition | `product_types` lookup row | New kind = config insert, not schema |
| JCE wiring | `config.subtype` filter | Reuses the proven visa-kind subtype pattern |
| Reviews | `review` content type + product link | Free §14 features (moderation, comments, feeds); aggregates denormalize onto product |
| Kind scope | Product-domain + providers + deals | Product comparisons via subtype filter; provider/deal become new item kinds on the same machinery — other domains stay table-backed |
| Deal entity | `deal` item ref over `coupons` | Reuses the coupon backend; `/deals` surface already exists |

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
