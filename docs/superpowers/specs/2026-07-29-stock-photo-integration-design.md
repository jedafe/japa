# Stock Photo Integration Design

**Date:** 2026-07-29
**Status:** Draft
**Scope:** Integrate Unsplash, Pexels, Pixabay, and Openverse stock photo APIs into JapaConnect media library.

## Overview

Users browse and import stock photos from four providers directly inside the existing media library picker modal. Imported photos become full media library citizens — stored in SeaweedFS, virus-scanned by ClamAV, transformed by imgproxy, and subject to all existing RBAC and quarantine workflows. A new Stock tab in the media picker modal lets content creators search and import. An admin page manages which providers are enabled and stores API keys.

---

## Provider Rate Limits & Caching

| Provider   | Free Tier Limit    | API Key Required |
|------------|---------------------|-------------------|
| Unsplash   | 50 req/hr (demo) / 5,000/hr (production) | Yes |
| Pexels     | 200 req/hr         | Yes |
| Pixabay    | 100 req/min        | Yes |
| Openverse  | No rate limit      | No |

To handle multi-user load within these limits, **all search results are server-side cached** with a 5-minute TTL, keyed by `{provider}:{query}:{page}`. Repeated searches for the same terms serve from cache, not the external API. Cache is in-memory (LRU) scoped to the `StockService` instance, with a maximum of 1,000 entries. Evicted entries are simply dropped — the next identical search will re-fetch from the provider API.

---

## RBAC Model

Three new permission nodes:

| Node                  | Grants                                                              |
|-----------------------|---------------------------------------------------------------------|
| `media.stock.search`  | Access the Stock tab; browse/search Openverse results only          |
| `media.stock.import`  | Import a stock photo into the media library (from any allowed source) |
| `media.stock.providers`| Browse/search/import from Unsplash, Pexels, and Pixabay             |

**Scenarios:**

- Content creator with `media.stock.search` + `media.stock.import`: can browse Openverse, search freely, import images. No access to rate-limited providers.
- Admin with all three: full access to all four providers plus provider management UI.
- User with only `media.stock.search`: can browse Openverse but cannot import (Import button hidden).
- User with neither: Stock tab is not visible in the picker modal.

---

## Backend Architecture

### New package: `internal/stock/`

```
internal/stock/
├── stock.go          # StockService struct, LRU cache, orchestration
├── provider.go       # StockProvider interface definition
├── unsplash.go       # Unsplash provider
├── pexels.go         # Pexels provider
├── pixabay.go        # Pixabay provider
├── openverse.go      # Openverse provider
├── handlers.go       # HTTP handlers for stock endpoints
├── store.go          # DB store for provider config
└── config.go         # Provider settings model
```

### `StockProvider` Interface

```go
type StockProvider interface {
    Name() string
    Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error)
    GetByID(ctx context.Context, providerPhotoID string) (*StockPhoto, error)
    Download(ctx context.Context, providerPhotoID string) (*DownloadedPhoto, error)
}
```

Each provider implements this interface. The `Download` method returns image bytes + metadata (photographer name, profile URL, license type, license URL, download source URL). The service always fetches via `Download` rather than passing external URLs directly into `StoreFromURL`, so there's no coupling to provider URL formats.

### `StockService`

Holds a map of provider instances (constructed only if their API key is present + enabled in DB), an in-memory LRU search cache, and a reference to the existing `media.Service`.

- **`SearchProviders(ctx, userID, query, providerName, page, perPage)`**: checks RBAC via provided resolver, filters available providers, fans out to each, merges results, updates cache.
- **`GetProviderPhoto(ctx, userID, providerName, photoID)`**: RBAC check → `provider.GetByID()`.
- **`ImportPhoto(ctx, userID, providerName, photoID, tenantID)`**: RBAC check → `provider.Download()` → `media.Service.StoreFromURL()`-style pipeline (upload bytes to SeaweedFS, create media record with attribution columns, enqueue scan/process tasks).

### Provider Config Store (`stock_store`)

Database table `stock_providers`:

```sql
CREATE TABLE stock_providers (
    provider_name    TEXT PRIMARY KEY,          -- 'unsplash', 'pexels', 'pixabay', 'openverse'
    enabled          BOOLEAN NOT NULL DEFAULT true,
    api_key_encrypted TEXT,                     -- AES-256-GCM encrypted (same scheme as totp_credentials.secret_encrypted), null for Openverse
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

At startup, the service seeds any missing provider rows with `enabled = true` and `api_key_encrypted` from the environment variable (if set). After initial seed, the DB value takes precedence. Toggling enabled/disabled or updating an API key via the admin API takes effect on the next request — no server restart needed.

API keys are encrypted at rest using AES-256-GCM, following the same `v1:<nonce>:<ciphertext>` format used by `totp_credentials.secret_encrypted`. The encryption key is derived from `JC_TOTP_ENCRYPTION_KEY` (SHA-256). If a separate key is preferred for operational isolation, `JC_STOCK_ENCRYPTION_KEY` can be introduced — falling back to `JC_TOTP_ENCRYPTION_KEY` when absent. The encrypt/decrypt helpers should be extracted to `internal/crypto/` so both `auth` and `stock` packages can share them.

---

## API Endpoints

### User-Facing

| Method | Path | RBAC | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/stock/providers` | `media.stock.search` | List providers available to the calling user (name, display name, enabled status). Openverse always included if user has `media.stock.search`. Rate-limited providers only included if user has `media.stock.providers` AND the provider is enabled in DB. |
| `GET` | `/api/v1/stock/search` | `media.stock.search` | Search stock photos. Query params: `q` (required), `provider` (optional — omit to search all available), `page` (default 1), `per_page` (default 20, max 80). Provider filtering respects RBAC. |
| `GET` | `/api/v1/stock/photo/:provider/:id` | `media.stock.search` | Get single photo details (full metadata, attribution info). |
| `POST` | `/api/v1/stock/import` | `media.stock.import` | Import a photo into the media library. Body: `{ provider, id }`. Returns `{ media_id, url }`. Provider must be available to the user. |

### Admin

| Method | Path | RBAC | Description |
|--------|------|------|-------------|
| `GET` | `/admin/stock/providers` | `media.edit_any` | List all providers with enabled status, masked API key presence, last updated. Accessible to any admin with media edit rights. |
| `PATCH` | `/admin/stock/providers/:name` | `media.edit_any` | Toggle `enabled`, set/rotate `api_key`. Body: `{ enabled?, api_key? }`. |

Admin endpoints are registered on the `/admin/` prefix group alongside existing admin media routes. The `media.edit_any` permission (already seeded) gates admin media operations and is a natural fit for provider management — it means "this user can manage media configuration."

---

## Database: Media Attribution Columns

New columns on the `media` table (migration 000186):

```sql
ALTER TABLE media ADD COLUMN source_platform         TEXT;  -- 'unsplash', 'pexels', 'pixabay', 'openverse'
ALTER TABLE media ADD COLUMN source_photographer      TEXT;  -- photographer display name
ALTER TABLE media ADD COLUMN source_photographer_url  TEXT;  -- link to photographer profile
ALTER TABLE media ADD COLUMN source_license           TEXT;  -- 'cc0', 'cc-by', 'cc-by-sa', 'pixabay', null
ALTER TABLE media ADD COLUMN source_license_url       TEXT;  -- link to license deed
ALTER TABLE media ADD COLUMN source_photo_id          TEXT;  -- external provider ID (for dedup / re-import prevention)
```

All columns are nullable — null means the media was not sourced from a stock provider. Queries and serialization remain unchanged for non-stock media.

---

## Attribution Display Rules

| Field | When Visible | Where |
|-------|-------------|-------|
| Photographer name | `source_photographer` is non-null | All display contexts (picker modal, admin library, public profiles, embedded in content) — tiny font, below or overlaying the thumbnail |
| License badge | `source_license` is non-null (e.g., CC BY, CC BY-SA from Openverse) | Same contexts as photographer name — small badge adjacent, links to `source_license_url` |

Attribution is embedded in media record metadata. The frontend checks `source_photographer` and `source_license` columns to conditionally render the attribution line. No separate UI configuration needed.

---

## Frontend Design

### Admin Panel — Provider Management

Added to `apps/admin/src/routes/media/+page.svelte` as a new **"Stock Providers" tab**.

- Toggle table: provider name, enabled/disabled switch, API key field (password-masked, placeholder "Set from environment" if seeded from env), RBAC note showing which permission node gates it.
- Unsplash: shows current tier (demo/production) based on response header from a validation ping.
- Openverse row: enabled toggle only (no API key field).
- Save is per-row on toggle/key change — calls `PATCH /admin/stock/providers/:name`.

### Media Picker Modal — Stock Tab

Added to `apps/main/src/lib/components/media/MediaLibraryModal.svelte` as a new **"Stock" tab** alongside "Library" and "Upload".

- Search bar with 350ms debounce (same pattern as Library tab).
- Provider chips below search bar — only shows providers available to the user (RBAC-filtered). "All" is default when multiple are available, or pre-selects the single available provider.
- Responsive thumbnail grid (matches Library tab column layout).
- Each thumbnail: hover overlay with Import button (hidden if user lacks `media.stock.import`), photographer name (tiny font, bottom-left), license badge (bottom-right, if applicable).
- Import flow: click Import → loading spinner on thumbnail → `POST /stock/import` → success state (green check, "Imported") → photo now also appears in Library tab.
- Empty states: "No stock providers available. Contact your administrator." when no providers, or "No results for ..." on empty search.

### User Dashboard Media Page

No changes required. Stock-imported photos appear alongside regular uploads.

### Embedded Content (TipTap / Page Builder)

When a stock photo is used in content, the attribution travels with the media metadata. Existing image rendering components check `source_photographer` and conditionally render the tiny attribution line. No changes to the editor itself.

---

## Import Flow (End-to-End)

```
1. User searches in Stock tab → GET /api/v1/stock/search?q=cats&provider=openverse
   → StockService checks RBAC, hits cache, calls provider.Search()
   → Returns thumbnail URLs, photographer, license metadata from provider API

2. User clicks Import on a photo → POST /api/v1/stock/import { provider: "openverse", id: "abc123" }
   → StockService checks RBAC (media.stock.import)
   → Calls provider.Download("abc123") → returns photo bytes + metadata
   → Calls media.Service.StoreFromURL-equivalent pipeline:
     a. Sniff MIME from bytes (http.DetectContentType)
     b. Generate storage path
     c. Upload bytes to SeaweedFS (or local fallback)
     d. Create media record with source_* attribution columns populated
     e. Confirm media record
     f. Enqueue virus scan + image processing tasks (same as any upload)
   → Returns { media_id, url }

3. Photo now appears in Library tab as a normal media item.
```

The import reuses the existing upload→confirm→scan→process pipeline. No new scanning or processing code. The only new step is populating the `source_*` columns during media record creation.

A new method `StoreFromBytes` is added to `media.Service` that accepts `[]byte` and optional `MediaAttribution` struct instead of a URL:

```go
type MediaAttribution struct {
    SourcePlatform        string
    SourcePhotographer    string
    SourcePhotographerURL string
    SourceLicense         string
    SourceLicenseURL      string
    SourcePhotoID         string
}

func (s *Service) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID,
    data []byte, filename string, attribution *MediaAttribution) (pgtype.UUID, error)
```

This mirrors the existing `StoreFromURL` logic (MIME sniffing, SeaweedFS upload, DB insert, confirm, enqueue tasks) but takes raw bytes instead of a download URL. The existing `StoreFromURL` is refactored internally to call `StoreFromBytes` after its own download step, avoiding duplication.

---

## Environment Variable → DB Seeding

On startup, `StockService` reads these env vars and seeds the `stock_providers` table for any row that doesn't yet have an `api_key_encrypted` value:

| Env Var | Provider |
|---------|----------|
| `JC_UNSPLASH_ACCESS_KEY` | unsplash |
| `JC_PEXELS_API_KEY` | pexels |
| `JC_PIXABAY_API_KEY` | pixabay |

Openverse has no env var (no API key needed). After seeding, the DB value is authoritative. Admins can override keys or disable providers through the admin UI without touching environment variables.

---

## Files to Create / Modify

### New Files
- `internal/crypto/crypto.go` — shared AES-256-GCM encrypt/decrypt (extracted from `internal/auth/totp.go`)
- `internal/stock/stock.go`
- `internal/stock/provider.go`
- `internal/stock/unsplash.go`
- `internal/stock/pexels.go`
- `internal/stock/pixabay.go`
- `internal/stock/openverse.go`
- `internal/stock/handlers.go`
- `internal/stock/store.go`
- `internal/stock/config.go`
- `migrations/000186_stock_providers.down.sql`
- `migrations/000186_stock_providers.up.sql` (stock_providers table + media attribution columns)
- `migrations/000187_seed_stock_rbac_nodes.up.sql`
- `migrations/000187_seed_stock_rbac_nodes.down.sql`

### Modified Files
- `internal/auth/totp.go` — migrate encrypt/decrypt helpers to `internal/crypto/`, reimport
- `internal/server/server.go` — register stock routes (user + admin)
- `internal/media/service.go` — add `StoreFromBytes` + `MediaAttribution`, refactor `StoreFromURL` to use it
- `internal/media/store.go` — add `InsertMediaWithAttribution` (or update `InsertMedia` to accept attribution fields)
- `internal/db/queries/media.sql` — add attribution columns to insert queries
- `internal/db/gen/media.sql.go` — regenerate from updated queries
- `internal/db/gen/models.go` — regenerated `Medium` struct with new fields
- `internal/config/config.go` — add `JC_UNSPLASH_ACCESS_KEY`, `JC_PEXELS_API_KEY`, `JC_PIXABAY_API_KEY`
- `apps/main/src/lib/components/media/MediaLibraryModal.svelte` — add Stock tab
- `apps/admin/src/routes/media/+page.svelte` — add Stock Providers tab
- `apps/main/src/lib/api/create.ts` — add stock API client functions
- `apps/admin/src/lib/api.ts` — add admin stock API client functions
- `apps/main/src/lib/types/index.ts` — add `StockPhoto`, `StockProvider`, `StockSearchResult` types, add source fields to `MediaAsset`
- `apps/main/src/lib/components/ui/` — any component rendering media thumbnails, for conditional attribution display

---

## Out of Scope

- Stock video or audio — photos only
- User-uploaded photos to stock providers (one-way: import only)
- Provider API key rotation automation
- Usage analytics per provider
- Watermark handling (all four providers serve clean, unwatermarked photos at their free tier)
