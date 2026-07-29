# Stock Photo Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Unsplash, Pexels, Pixabay, and Openverse stock photo APIs into JapaConnect media library — browse/search in a new Stock tab inside the media picker modal, import to SeaweedFS, display attribution.

**Architecture:** New `internal/stock/` package with a `StockProvider` interface implemented by four providers, orchestrated by `StockService` with in-memory LRU search caching. Import flows through `media.Service.StoreFromBytes` to reuse the existing confirm→scan→process pipeline. Provider config and API keys stored encrypted in DB, managed via admin panel.

**Tech Stack:** Go (chi router, pgx, sqlc), Svelte 5 (runes, Bits UI, Tailwind), PostgreSQL, SeaweedFS, AES-256-GCM (existing `JC_TOTP_ENCRYPTION_KEY`)

## Global Constraints

- Photos only — no video/audio from stock providers
- Import only — one-way from provider to media library, no user upload to providers
- Attribution stored on media record and displayed wherever thumbnails appear
- RBAC: `media.stock.search`, `media.stock.import`, `media.stock.providers`
- Openverse is always available with `media.stock.search` (no API key, no rate limit)
- Unsplash/Pexels/Pixabay gated behind `media.stock.providers` (admin-only)
- Admin provider management gated by existing `media.edit_any`
- Search results cached in-memory (LRU, 5min TTL, max 1000 entries, `{provider}:{query}:{page}` key)
- API keys encrypted at rest using same `v1:<nonce>:<ciphertext>` format as `totp_credentials.secret_encrypted`
- Encrypt/decrypt helpers extracted to `internal/crypto/` to avoid duplication

---

### Task 1: Extract shared encryption helpers to `internal/crypto/`

**Files:**
- Create: `internal/crypto/crypto.go`
- Modify: `internal/auth/totp.go:156-253`

**Interfaces:**
- Produces: `crypto.Encrypt(secret string) (string, error)` — same signature as current `encryptSecret`
- Produces: `crypto.Decrypt(encrypted string) (string, error)` — same signature as current `decryptSecret`
- Produces: `crypto.LoadEncryptionKey() ([]byte, error)` — same signature as current `loadEncryptionKey`, but reads `JC_TOTP_ENCRYPTION_KEY` / `JC_STOCK_ENCRYPTION_KEY` with fallback

- [ ] **Step 1: Create `internal/crypto/crypto.go`**

```go
// Package crypto provides shared AES-256-GCM encryption helpers.
// Used by auth (TOTP secrets) and stock (provider API keys).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Encrypt encrypts a plaintext string using AES-256-GCM.
// The key is derived from JC_STOCK_ENCRYPTION_KEY (falling back to
// JC_TOTP_ENCRYPTION_KEY) via SHA-256. Returns "v1:<hex nonce>:<hex ciphertext>".
// In production, returns an error if no encryption key is configured.
func Encrypt(plaintext string) (string, error) {
	key, err := LoadEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return fmt.Sprintf("v1:%s:%s", hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// Decrypt reverses Encrypt. Parses "v1:<nonce>:<ciphertext>" format and
// decrypts using AES-256-GCM. Returns plaintext or an error.
func Decrypt(encrypted string) (string, error) {
	key, err := LoadEncryptionKey()
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 || parts[0] != "v1" {
		// Legacy plaintext — return as-is for backward compatibility.
		return encrypted, nil
	}

	nonce, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// LoadEncryptionKey reads the encryption key from environment variables.
// Priority: JC_STOCK_ENCRYPTION_KEY → JC_TOTP_ENCRYPTION_KEY.
// In production, returns an error if no key is configured.
func LoadEncryptionKey() ([]byte, error) {
	raw := os.Getenv("JC_STOCK_ENCRYPTION_KEY")
	if raw == "" {
		raw = os.Getenv("JC_TOTP_ENCRYPTION_KEY")
	}
	if raw == "" {
		env := os.Getenv("JC_ENV")
		if strings.EqualFold(env, "production") {
			return nil, errors.New("encryption key not configured (set JC_STOCK_ENCRYPTION_KEY or JC_TOTP_ENCRYPTION_KEY)")
		}
		slog.Warn("encryption key not set — secrets stored without encryption (dev only)")
		devKey := sha256.Sum256([]byte("japaconnect-dev-encryption-key"))
		return devKey[:], nil
	}
	hash := sha256.Sum256([]byte(raw))
	return hash[:], nil
}
```

- [ ] **Step 2: Replace `internal/auth/totp.go` unexported helpers with `crypto` calls**

In `internal/auth/totp.go`, replace the bodies of `encryptSecret`, `decryptSecret`, and delete `loadEncryptionKey`:

```go
import "github.com/jedafe/japa/internal/crypto"

func encryptSecret(secret string) (string, error) {
	return crypto.Encrypt(secret)
}

func decryptSecret(encrypted string) (string, error) {
	return crypto.Decrypt(encrypted)
}
// DELETE: func loadEncryptionKey() ([]byte, error) { ... }  ← remove entirely
```

Remove the `loadEncryptionKey` function entirely. Remove unused imports (`crypto/aes`, `crypto/cipher`, `crypto/rand`, `crypto/sha256`, `encoding/hex`, `io`, `os` — but check which are still used by other functions in the file first; `os`, `log/slog`, `crypto/rand`, `encoding/base32`, `encoding/hex` may still be needed by `GenerateTOTPSecret` or `VerifyTOTP`).

- [ ] **Step 3: Verify `go build ./...` compiles**

```bash
cd /home/jmobile/websites/japa/japaconnect && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/crypto/crypto.go internal/auth/totp.go
git commit -m "refactor: extract AES-256-GCM encrypt/decrypt to internal/crypto

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 2: Database migration — stock_providers table + media attribution columns

**Files:**
- Create: `migrations/000186_stock_providers.up.sql`
- Create: `migrations/000186_stock_providers.down.sql`

- [ ] **Step 1: Write up migration**

`migrations/000186_stock_providers.up.sql`:

```sql
-- Migration 186: Stock photo provider configuration and media attribution columns.

-- Provider configuration table — stores enabled/disabled state and encrypted API keys.
CREATE TABLE stock_providers (
    provider_name     TEXT PRIMARY KEY,          -- 'unsplash', 'pexels', 'pixabay', 'openverse'
    enabled           BOOLEAN NOT NULL DEFAULT true,
    api_key_encrypted TEXT,                     -- AES-256-GCM v1:<nonce>:<ciphertext>, null for Openverse
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the four known providers. API keys are NULL initially — they get populated
-- from env vars at startup via the application seed logic.
INSERT INTO stock_providers (provider_name) VALUES
    ('unsplash'),
    ('pexels'),
    ('pixabay'),
    ('openverse')
ON CONFLICT (provider_name) DO NOTHING;

-- Stock photo attribution columns on media table.
ALTER TABLE media ADD COLUMN source_platform         TEXT;
ALTER TABLE media ADD COLUMN source_photographer      TEXT;
ALTER TABLE media ADD COLUMN source_photographer_url  TEXT;
ALTER TABLE media ADD COLUMN source_license           TEXT;
ALTER TABLE media ADD COLUMN source_license_url       TEXT;
ALTER TABLE media ADD COLUMN source_photo_id          TEXT;

COMMENT ON COLUMN media.source_platform        IS 'Stock provider name: unsplash, pexels, pixabay, openverse';
COMMENT ON COLUMN media.source_photographer     IS 'Photographer display name for attribution';
COMMENT ON COLUMN media.source_photographer_url IS 'Link to photographer profile page';
COMMENT ON COLUMN media.source_license          IS 'License identifier: cc0, cc-by, cc-by-sa, pixabay';
COMMENT ON COLUMN media.source_license_url      IS 'Link to license deed';
COMMENT ON COLUMN media.source_photo_id         IS 'Provider-specific photo ID for dedup';
```

`migrations/000186_stock_providers.down.sql`:

```sql
ALTER TABLE media DROP COLUMN IF EXISTS source_photo_id;
ALTER TABLE media DROP COLUMN IF EXISTS source_license_url;
ALTER TABLE media DROP COLUMN IF EXISTS source_license;
ALTER TABLE media DROP COLUMN IF EXISTS source_photographer_url;
ALTER TABLE media DROP COLUMN IF EXISTS source_photographer;
ALTER TABLE media DROP COLUMN IF EXISTS source_platform;
DROP TABLE IF EXISTS stock_providers;
```

- [ ] **Step 2: Verify migration applies cleanly**

If you have a local DB running, apply: `go run ./cmd/migrate up` or equivalent migration runner. Otherwise verify file structure matches existing migration conventions.

- [ ] **Step 3: Commit**

```bash
git add migrations/000186_stock_providers.up.sql migrations/000186_stock_providers.down.sql
git commit -m "feat: add stock_providers table and media attribution columns

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 3: Seed RBAC permission nodes migration

**Files:**
- Create: `migrations/000187_seed_stock_rbac_nodes.up.sql`
- Create: `migrations/000187_seed_stock_rbac_nodes.down.sql`

- [ ] **Step 1: Write migration**

`migrations/000187_seed_stock_rbac_nodes.up.sql`:

```sql
-- Seed stock photo RBAC permission nodes.
-- Admin group gets all three; default member roles get search + import only.

INSERT INTO permission_nodes (node, description, category, created_at, updated_at)
VALUES
    ('media.stock.search',   'Browse and search stock photos (Openverse always; others require media.stock.providers)', 'media', now(), now()),
    ('media.stock.import',   'Import stock photos into the media library', 'media', now(), now()),
    ('media.stock.providers', 'Access rate-limited stock providers (Unsplash, Pexels, Pixabay)', 'media', now(), now())
ON CONFLICT (node) DO NOTHING;
```

`migrations/000187_seed_stock_rbac_nodes.down.sql`:

```sql
DELETE FROM permission_nodes WHERE node IN (
    'media.stock.search',
    'media.stock.import',
    'media.stock.providers'
);
```

- [ ] **Step 2: Commit**

```bash
git add migrations/000187_seed_stock_rbac_nodes.up.sql migrations/000187_seed_stock_rbac_nodes.down.sql
git commit -m "feat: seed stock photo RBAC permission nodes

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 4: Regenerate sqlc models and queries with attribution columns

**Files:**
- Modify: `internal/db/queries/media.sql`
- Regenerate: `internal/db/gen/media.sql.go`
- Regenerate: `internal/db/gen/models.go`

**Interfaces:**
- Produces: `Medium` struct gains six `*string` fields: `SourcePlatform`, `SourcePhotographer`, `SourcePhotographerUrl`, `SourceLicense`, `SourceLicenseUrl`, `SourcePhotoId`
- Produces: `InsertMediaParams` gains six `*string` fields (nullable)
- Produces: `InsertMedia` query updated for new columns

- [ ] **Step 1: Update InsertMedia query in `internal/db/queries/media.sql`**

Replace the existing InsertMedia query (lines 8-11):

```sql
-- name: InsertMedia :one
INSERT INTO media (
    tenant_id, uploader_user_id, original_filename, mime_type, file_size_bytes,
    storage_path, storage_bucket,
    source_platform, source_photographer, source_photographer_url,
    source_license, source_license_url, source_photo_id
)
VALUES (
    @tenant_id, @uploader_user_id, @original_filename, @mime_type, @file_size_bytes,
    @storage_path, @storage_bucket,
    @source_platform, @source_photographer, @source_photographer_url,
    @source_license, @source_license_url, @source_photo_id
)
RETURNING *;
```

- [ ] **Step 2: Regenerate sqlc**

```bash
cd /home/jmobile/websites/japa/japaconnect && sqlc generate
```

Expected: regenerates `gen.InsertMediaParams` struct with new fields, `gen.Medium` struct with new fields, updated `InsertMedia` method.

- [ ] **Step 3: Verify `go build ./...`**

```bash
go build ./...
```

Expected: may fail if `internal/media/store.go:InsertMedia` references the old params shape — that's expected, will be fixed in Task 6.

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/media.sql internal/db/gen/media.sql.go internal/db/gen/models.go
git commit -m "feat: add stock attribution columns to media queries and models

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 5: Add stock provider env vars to config

**Files:**
- Modify: `internal/config/config.go`

**Interfaces:**
- Produces: `config.Config` gains three new fields: `UnsplashAccessKey string`, `PexelsAPIKey string`, `PixabayAPIKey string`

- [ ] **Step 1: Add fields to Config struct**

In `internal/config/config.go`, add after the last secret-type field (line ~58, before `}`):

```go
UnsplashAccessKey string // JC_UNSPLASH_ACCESS_KEY — Unsplash API access key
PexelsAPIKey      string // JC_PEXELS_API_KEY — Pexels API key
PixabayAPIKey     string // JC_PIXABAY_API_KEY — Pixabay API key
```

- [ ] **Step 2: Add to Load() assignment block**

In the `Config{...}` block, add after the last assignment:

```go
UnsplashAccessKey: os.Getenv("JC_UNSPLASH_ACCESS_KEY"),
PexelsAPIKey:      os.Getenv("JC_PEXELS_API_KEY"),
PixabayAPIKey:     os.Getenv("JC_PIXABAY_API_KEY"),
```

- [ ] **Step 3: Verify `go build ./...`**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add stock provider env var config fields

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 6: Add `StoreFromBytes` to media.Service + update InsertMedia params

**Files:**
- Modify: `internal/media/media.go`
- Modify: `internal/media/service.go`
- Modify: `internal/media/store.go`

**Interfaces:**
- Produces: `MediaAttribution` struct (in media.go)
- Produces: `func (s *Service) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *MediaAttribution) (pgtype.UUID, error)`
- Produces: `func (s *Service) StoreFromURL(...)` refactored to call `StoreFromBytes` internally (optional, or duplicate the pipeline)

- [ ] **Step 1: Add MediaAttribution struct to `internal/media/media.go`**

```go
// MediaAttribution carries stock photo source metadata for attribution display.
type MediaAttribution struct {
	SourcePlatform        string
	SourcePhotographer    string
	SourcePhotographerURL string
	SourceLicense         string
	SourceLicenseURL      string
	SourcePhotoID         string
}
```

- [ ] **Step 2: Add `StoreFromBytes` to `internal/media/service.go`**

Before `StoreFromURL`, add:

```go
// StoreFromBytes stores raw bytes as a confirmed media item. When attribution
// is non-nil, the source_* columns are populated for stock photo attribution.
func (s *Service) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *MediaAttribution) (pgtype.UUID, error) {
	if len(data) == 0 {
		return pgtype.UUID{}, errors.New("data is empty")
	}
	if len(data) > MaxFileSizeBytes {
		return pgtype.UUID{}, fmt.Errorf("data too large: %d bytes exceeds limit of %d bytes", len(data), MaxFileSizeBytes)
	}

	// Sniff MIME type from content.
	mimeType := http.DetectContentType(data)

	// Generate unique storage path.
	ext := extensionFromMIME(mimeType)
	storagePath := fmt.Sprintf("%s/%s/%s%s",
		StoragePathMedia,
		tenantID.String(),
		uuid.New().String(),
		ext,
	)

	// Upload to S3/SeaweedFS with local fallback.
	stored := false
	if s.s3 != nil {
		if err := s.s3.PutObject(ctx, storagePath, mimeType, data); err != nil {
			slog.Warn("media: s3 put failed, trying local fallback",
				"storage_path", storagePath, "error", err)
		} else {
			stored = true
		}
	}
	if !stored {
		if err := storeLocalMediaFile(DefaultBucket, storagePath, data); err != nil {
			return pgtype.UUID{}, fmt.Errorf("storing to local fallback: %w", err)
		}
	}

	// Build insert params with optional attribution.
	params := gen.InsertMediaParams{
		TenantID:         tenantID,
		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    int64(len(data)),
		StoragePath:      storagePath,
		StorageBucket:    DefaultBucket,
	}
	if attribution != nil && attribution.SourcePlatform != "" {
		params.SourcePlatform = &attribution.SourcePlatform
		params.SourcePhotographer = &attribution.SourcePhotographer
		params.SourcePhotographerUrl = &attribution.SourcePhotographerURL
		params.SourceLicense = &attribution.SourceLicense
		params.SourceLicenseUrl = &attribution.SourceLicenseURL
		params.SourcePhotoId = &attribution.SourcePhotoID
	}

	m, err := s.store.InsertMedia(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("inserting media record: %w", err)
	}

	if err := s.store.ConfirmMedia(ctx, m.ID, tenantID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("confirming media: %w", err)
	}

	return m.ID, nil
}
```

- [ ] **Step 3: Refactor `StoreFromURL` to call `StoreFromBytes`**

Replace the body of `StoreFromURL` (keep the download + status check, then delegate):

```go
func (s *Service) StoreFromURL(ctx context.Context, tenantID, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0 (university:fetch_logo)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("downloading from %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return pgtype.UUID{}, fmt.Errorf("download returned %d for %s", resp.StatusCode, sourceURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFileSizeBytes))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("reading response body: %w", err)
	}

	return s.StoreFromBytes(ctx, tenantID, uploaderID, body, filename, nil)
}
```

Remove the now-duplicated MIME sniffing, S3 upload, and DB insert from `StoreFromURL`.

- [ ] **Step 4: Verify `go build ./...`**

```bash
go build ./...
```

May need to add `"github.com/google/uuid"` to imports if not already there.

- [ ] **Step 5: Commit**

```bash
git add internal/media/media.go internal/media/service.go
git commit -m "feat: add StoreFromBytes with optional attribution to media.Service

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 7: Backend — StockProvider interface and shared types

**Files:**
- Create: `internal/stock/provider.go`
- Create: `internal/stock/stock.go`
- Create: `internal/stock/config.go`

**Interfaces:**
- Produces: `StockProvider` interface (Name, Search, GetByID, Download)
- Produces: `SearchResult`, `StockPhoto`, `DownloadedPhoto` shared types
- Produces: `StockService` struct with cache map, provider map, media service reference
- Produces: `ProviderConfig` DB row struct

- [ ] **Step 1: Create `internal/stock/stock.go`**

```go
// Package stock provides stock photo search and import from Unsplash, Pexels,
// Pixabay, and Openverse.
package stock

import (
	"sync"
	"time"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/media"
)

// SearchResult holds a page of search results from a stock provider.
type SearchResult struct {
	Photos      []StockPhoto `json:"photos"`
	Total       int          `json:"total"`
	Page        int          `json:"page"`
	PerPage     int          `json:"per_page"`
	HasNextPage bool         `json:"has_next_page"`
}

// StockPhoto represents a single photo from a stock provider.
type StockPhoto struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	ThumbnailURL   string `json:"thumbnail_url"`
	FullURL        string `json:"full_url"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Photographer    string `json:"photographer"`
	PhotographerURL string `json:"photographer_url"`
	License        string `json:"license,omitempty"`
	LicenseURL     string `json:"license_url,omitempty"`
	Description    string `json:"description,omitempty"`
}

// DownloadedPhoto is the result of downloading a full-resolution photo.
type DownloadedPhoto struct {
	Data             []byte
	Filename         string
	Photographer      string
	PhotographerURL   string
	License          string
	LicenseURL       string
	ProviderPhotoID  string
}

// cacheEntry holds cached search results with expiry.
type cacheEntry struct {
	Result   *SearchResult
	ExpiresAt time.Time
}

// Service orchestrates stock photo search and import across providers.
type Service struct {
	providers map[string]StockProvider
	store     *Store
	media     *media.Service
	cache     map[string]*cacheEntry
	cacheMu   sync.RWMutex
	cfg       *config.Config
	db        *db.DB
}

// NewService creates a Service, constructing providers that have API keys available.
func NewService(database *db.DB, mediaSvc *media.Service, cfg *config.Config) *Service {
	s := &Service{
		providers: make(map[string]StockProvider),
		store:     NewStore(database),
		media:     mediaSvc,
		cache:     make(map[string]*cacheEntry),
		mu:        sync.RWMutex{},
		cfg:       cfg,
		db:        database,
	}
	// Providers are loaded lazily on first request or on startup via s.ReloadProviders().
	return s
}

// cacheKey builds a cache key for search results.
func cacheKey(provider, query string, page, perPage int) string {
	return provider + ":" + query + ":" + itoa(page) + ":" + itoa(perPage)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
```

- [ ] **Step 2: Create `internal/stock/provider.go`**

```go
package stock

import "context"

// StockProvider implements search and download for a stock photo service.
type StockProvider interface {
	// Name returns the provider identifier (e.g., "unsplash", "pexels").
	Name() string

	// Search queries the provider for photos matching the given terms.
	Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error)

	// GetByID fetches metadata for a single photo.
	GetByID(ctx context.Context, photoID string) (*StockPhoto, error)

	// Download fetches full-resolution photo bytes plus attribution metadata.
	Download(ctx context.Context, photoID string) (*DownloadedPhoto, error)
}
```

- [ ] **Step 3: Create `internal/stock/config.go`**

```go
package stock

import "time"

// ProviderConfigRow mirrors the stock_providers database row.
type ProviderConfigRow struct {
	ProviderName    string
	Enabled         bool
	APIKeyEncrypted string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
```

- [ ] **Step 4: Verify `go build ./internal/stock/...` compiles**

```bash
go build ./internal/stock/...
```

Expected: compiles (no external dependencies yet).

- [ ] **Step 5: Commit**

```bash
git add internal/stock/stock.go internal/stock/provider.go internal/stock/config.go
git commit -m "feat: add StockProvider interface, Service, and shared types

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 8: Backend — Stock provider config store (CRUD on stock_providers)

**Files:**
- Create: `internal/stock/store.go`

**Interfaces:**
- Produces: `func (s *Store) ListProviders(ctx context.Context) ([]ProviderConfigRow, error)`
- Produces: `func (s *Store) GetProvider(ctx context.Context, name string) (*ProviderConfigRow, error)`
- Produces: `func (s *Store) UpsertProvider(ctx context.Context, row ProviderConfigRow) error`
- Produces: `func (s *Store) ToggleProvider(ctx context.Context, name string, enabled bool) error`
- Produces: `func (s *Store) SetAPIKey(ctx context.Context, name string, encryptedKey string) error`

- [ ] **Step 1: Create `internal/stock/store.go`**

```go
package stock

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
)

// Store manages stock_providers database operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the database pool.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database.Writer}
}

// ListProviders returns all configured stock providers.
func (s *Store) ListProviders(ctx context.Context) ([]ProviderConfigRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT provider_name, enabled, COALESCE(api_key_encrypted, ''),
		        created_at, updated_at
		 FROM stock_providers
		 ORDER BY provider_name`)
	if err != nil {
		return nil, fmt.Errorf("listing stock providers: %w", err)
	}
	defer rows.Close()

	var providers []ProviderConfigRow
	for rows.Next() {
		var r ProviderConfigRow
		if err := rows.Scan(&r.ProviderName, &r.Enabled, &r.APIKeyEncrypted,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning provider row: %w", err)
		}
		providers = append(providers, r)
	}
	return providers, rows.Err()
}

// GetProvider returns a single provider by name.
func (s *Store) GetProvider(ctx context.Context, name string) (*ProviderConfigRow, error) {
	var r ProviderConfigRow
	err := s.pool.QueryRow(ctx,
		`SELECT provider_name, enabled, COALESCE(api_key_encrypted, ''),
		        created_at, updated_at
		 FROM stock_providers
		 WHERE provider_name = $1`, name,
	).Scan(&r.ProviderName, &r.Enabled, &r.APIKeyEncrypted, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting provider %s: %w", name, err)
	}
	return &r, nil
}

// ToggleProvider enables or disables a provider.
func (s *Store) ToggleProvider(ctx context.Context, name string, enabled bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE stock_providers SET enabled = $2, updated_at = now()
		 WHERE provider_name = $1`, name, enabled)
	if err != nil {
		return fmt.Errorf("toggling provider %s: %w", name, err)
	}
	return nil
}

// SetAPIKey updates the encrypted API key for a provider.
func (s *Store) SetAPIKey(ctx context.Context, name, encryptedKey string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE stock_providers SET api_key_encrypted = $2, updated_at = now()
		 WHERE provider_name = $1`, name, encryptedKey)
	if err != nil {
		return fmt.Errorf("setting API key for %s: %w", name, err)
	}
	return nil
}

// SeedProvider inserts a provider row if it doesn't exist, and seeds the API key
// from an environment variable if the row has no key yet.
func (s *Store) SeedProvider(ctx context.Context, name, apiKey string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stock_providers (provider_name)
		 VALUES ($1)
		 ON CONFLICT (provider_name) DO NOTHING`, name)
	if err != nil {
		return fmt.Errorf("seeding provider %s: %w", name, err)
	}

	if apiKey == "" {
		return nil
	}

	// Only set the key if the row currently has no encrypted value.
	_, err = s.pool.Exec(ctx,
		`UPDATE stock_providers
		 SET api_key_encrypted = $2, updated_at = now()
		 WHERE provider_name = $1
		   AND (api_key_encrypted IS NULL OR api_key_encrypted = '')`,
		name, apiKey)
	if err != nil {
		return fmt.Errorf("seeding API key for %s: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 2: Verify `go build ./internal/stock/...`**

```bash
go build ./internal/stock/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/stock/store.go
git commit -m "feat: add stock provider config store

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 9: Backend — Implement four stock providers

**Files:**
- Create: `internal/stock/unsplash.go`
- Create: `internal/stock/pexels.go`
- Create: `internal/stock/pixabay.go`
- Create: `internal/stock/openverse.go`

**Interfaces:**
- Each implements: `StockProvider` interface
- Each constructor: `func NewUnsplashProvider(apiKey string) *UnsplashProvider` (etc.)
- Consumes: `StockProvider`, `SearchResult`, `StockPhoto`, `DownloadedPhoto` from Task 7

- [ ] **Step 1: Create `internal/stock/unsplash.go`**

```go
package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const unsplashAPI = "https://api.unsplash.com"

// UnsplashProvider implements StockProvider for Unsplash (unsplash.com/developers).
type UnsplashProvider struct {
	accessKey string
	client    *http.Client
}

// NewUnsplashProvider creates an Unsplash provider with the given access key.
func NewUnsplashProvider(accessKey string) *UnsplashProvider {
	return &UnsplashProvider{
		accessKey: accessKey,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *UnsplashProvider) Name() string { return "unsplash" }

func (p *UnsplashProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		unsplashAPI+"/search/photos?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building unsplash request: %w", err)
	}
	req.Header.Set("Authorization", "Client-ID "+p.accessKey)
	req.Header.Set("Accept-Version", "v1")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("unsplash returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
		Results    []struct {
			ID     string `json:"id"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Urls   struct {
				Thumb string `json:"thumb"`
				Full  string `json:"full"`
				Raw   string `json:"raw"`
			} `json:"urls"`
			Description *string `json:"description"`
			User       struct {
				Name     string `json:"name"`
				Username string `json:"username"`
				Links    struct {
					HTML string `json:"html"`
				} `json:"links"`
			} `json:"user"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding unsplash response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Results {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		photos = append(photos, StockPhoto{
			ID:              r.ID,
			Provider:        "unsplash",
			ThumbnailURL:    r.Urls.Thumb,
			FullURL:         r.Urls.Full,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:     r.User.Name,
			PhotographerURL:  r.User.Links.HTML + "?utm_source=japaconnect&utm_medium=referral",
			Description:     desc,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.Total,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page < raw.TotalPages,
	}, nil
}

func (p *UnsplashProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		unsplashAPI+"/photos/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building unsplash request: %w", err)
	}
	req.Header.Set("Authorization", "Client-ID "+p.accessKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsplash returned %d", resp.StatusCode)
	}

	var raw struct {
		ID     string `json:"id"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Urls   struct {
			Thumb string `json:"thumb"`
			Full  string `json:"full"`
			Raw   string `json:"raw"`
		} `json:"urls"`
		Description *string `json:"description"`
		User       struct {
			Name     string `json:"name"`
			Username string `json:"username"`
			Links    struct {
				HTML string `json:"html"`
			} `json:"links"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding unsplash photo: %w", err)
	}

	desc := ""
	if raw.Description != nil {
		desc = *raw.Description
	}

	return &StockPhoto{
		ID:              raw.ID,
		Provider:        "unsplash",
		ThumbnailURL:    raw.Urls.Thumb,
		FullURL:         raw.Urls.Full,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:     raw.User.Name,
		PhotographerURL:  raw.User.Links.HTML + "?utm_source=japaconnect&utm_medium=referral",
		Description:     desc,
	}, nil
}

func (p *UnsplashProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	// Unsplash requires a download tracking call per API guidelines.
	go func() {
		req, _ := http.NewRequest(http.MethodGet,
			unsplashAPI+"/photos/"+photoID+"/download?client_id="+p.accessKey, nil)
		p.client.Do(req) //nolint:errcheck // fire-and-forget tracking
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:            data,
		Filename:        photoID + ".jpg",
		Photographer:     photo.Photographer,
		PhotographerURL:  photo.PhotographerURL,
		ProviderPhotoID:  photoID,
	}, nil
}
```

- [ ] **Step 2: Create `internal/stock/pexels.go`**

```go
package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const pexelsAPI = "https://api.pexels.com/v1"

// PexelsProvider implements StockProvider for Pexels (pexels.com/api).
type PexelsProvider struct {
	apiKey  string
	client  *http.Client
}

func NewPexelsProvider(apiKey string) *PexelsProvider {
	return &PexelsProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PexelsProvider) Name() string { return "pexels" }

func (p *PexelsProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pexelsAPI+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pexels request: %w", err)
	}
	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("pexels returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		TotalResults int `json:"total_results"`
		Page         int `json:"page"`
		PerPage      int `json:"per_page"`
		Photos       []struct {
			ID     int    `json:"id"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Src    struct {
				Medium    string `json:"medium"`
				Large     string `json:"large"`
				Original  string `json:"original"`
			} `json:"src"`
			Photographer      string `json:"photographer"`
			PhotographerURL   string `json:"photographer_url"`
			Alt               string `json:"alt"`
		} `json:"photos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pexels response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Photos {
		photos = append(photos, StockPhoto{
			ID:              strconv.Itoa(r.ID),
			Provider:        "pexels",
			ThumbnailURL:    r.Src.Medium,
			FullURL:         r.Src.Original,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:     r.Photographer,
			PhotographerURL:  r.PhotographerURL,
			Description:     r.Alt,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.TotalResults,
		Page:        raw.Page,
		PerPage:     raw.PerPage,
		HasNextPage: len(raw.Photos) == raw.PerPage,
	}, nil
}

func (p *PexelsProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pexelsAPI+"/photos/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building pexels request: %w", err)
	}
	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels returned %d", resp.StatusCode)
	}

	var raw struct {
		ID     int    `json:"id"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Src    struct {
			Medium   string `json:"medium"`
			Large    string `json:"large"`
			Original string `json:"original"`
		} `json:"src"`
		Photographer      string `json:"photographer"`
		PhotographerURL   string `json:"photographer_url"`
		Alt               string `json:"alt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pexels photo: %w", err)
	}

	return &StockPhoto{
		ID:              strconv.Itoa(raw.ID),
		Provider:        "pexels",
		ThumbnailURL:    raw.Src.Medium,
		FullURL:         raw.Src.Original,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:     raw.Photographer,
		PhotographerURL:  raw.PhotographerURL,
		Description:     raw.Alt,
	}, nil
}

func (p *PexelsProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:            data,
		Filename:        photoID + ".jpg",
		Photographer:     photo.Photographer,
		PhotographerURL:  photo.PhotographerURL,
		ProviderPhotoID:  photoID,
	}, nil
}
```

- [ ] **Step 3: Create `internal/stock/pixabay.go`**

```go
package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const pixabayAPI = "https://pixabay.com/api"

// PixabayProvider implements StockProvider for Pixabay (pixabay.com/api/docs).
type PixabayProvider struct {
	apiKey  string
	client  *http.Client
}

func NewPixabayProvider(apiKey string) *PixabayProvider {
	return &PixabayProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PixabayProvider) Name() string { return "pixabay" }

func (p *PixabayProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("key", p.apiKey)
	params.Set("q", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))
	params.Set("image_type", "photo")
	params.Set("safesearch", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pixabayAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pixabay request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("pixabay returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Total     int `json:"total"`
		TotalHits int `json:"totalHits"`
		Hits      []struct {
			ID            int    `json:"id"`
			WebformatURL  string `json:"webformatURL"`
			LargeImageURL string `json:"largeImageURL"`
			ImageWidth    int    `json:"imageWidth"`
			ImageHeight   int    `json:"imageHeight"`
			User          string `json:"user"`
			UserImageURL  string `json:"userImageURL"`
			Tags          string `json:"tags"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pixabay response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Hits {
		photos = append(photos, StockPhoto{
			ID:              strconv.Itoa(r.ID),
			Provider:        "pixabay",
			ThumbnailURL:    r.WebformatURL,
			FullURL:         r.LargeImageURL,
			Width:           r.ImageWidth,
			Height:          r.ImageHeight,
			Photographer:     r.User,
			Description:     r.Tags,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.Total,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page*perPage < raw.TotalHits,
	}, nil
}

func (p *PixabayProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	params := url.Values{}
	params.Set("key", p.apiKey)
	params.Set("id", photoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pixabayAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pixabay request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pixabay returned %d", resp.StatusCode)
	}

	var raw struct {
		Hits []struct {
			ID            int    `json:"id"`
			WebformatURL  string `json:"webformatURL"`
			LargeImageURL string `json:"largeImageURL"`
			ImageWidth    int    `json:"imageWidth"`
			ImageHeight   int    `json:"imageHeight"`
			User          string `json:"user"`
			UserImageURL  string `json:"userImageURL"`
			Tags          string `json:"tags"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pixabay photo: %w", err)
	}

	if len(raw.Hits) == 0 {
		return nil, fmt.Errorf("pixabay photo not found: %s", photoID)
	}
	h := raw.Hits[0]

	return &StockPhoto{
		ID:              strconv.Itoa(h.ID),
		Provider:        "pixabay",
		ThumbnailURL:    h.WebformatURL,
		FullURL:         h.LargeImageURL,
		Width:           h.ImageWidth,
		Height:          h.ImageHeight,
		Photographer:     h.User,
		Description:     h.Tags,
	}, nil
}

func (p *PixabayProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:            data,
		Filename:        photoID + ".jpg",
		Photographer:     photo.Photographer,
		ProviderPhotoID:  photoID,
	}, nil
}
```

- [ ] **Step 4: Create `internal/stock/openverse.go`**

```go
package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openverseAPI = "https://api.openverse.org/v1"

// OpenverseProvider implements StockProvider for Openverse (api.openverse.org).
// No API key required — this is the zero-friction default provider.
type OpenverseProvider struct {
	client *http.Client
}

func NewOpenverseProvider() *OpenverseProvider {
	return &OpenverseProvider{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *OpenverseProvider) Name() string { return "openverse" }

func (p *OpenverseProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(perPage))
	params.Set("mature", "false")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		openverseAPI+"/images?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openverse search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("openverse returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		ResultCount int `json:"result_count"`
		PageCount   int `json:"page_count"`
		Results      []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Thumbnail     string `json:"thumbnail"`
			URL           string `json:"url"`
			Width         int    `json:"width"`
			Height        int    `json:"height"`
			License       string `json:"license"`
			LicenseURL    string `json:"license_url"`
			LicenseVersion string `json:"license_version"`
			Creator       string `json:"creator"`
			CreatorURL    string `json:"creator_url"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding openverse response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Results {
		photos = append(photos, StockPhoto{
			ID:              r.ID,
			Provider:        "openverse",
			ThumbnailURL:    r.Thumbnail,
			FullURL:         r.URL,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:     r.Creator,
			PhotographerURL:  r.CreatorURL,
			License:         normalizeLicense(r.License),
			LicenseURL:      r.LicenseURL,
			Description:     r.Title,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.ResultCount,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page < raw.PageCount,
	}, nil
}

func (p *OpenverseProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		openverseAPI+"/images/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openverse get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openverse returned %d", resp.StatusCode)
	}

	var raw struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Thumbnail      string `json:"thumbnail"`
		URL            string `json:"url"`
		Width          int    `json:"width"`
		Height         int    `json:"height"`
		License        string `json:"license"`
		LicenseURL     string `json:"license_url"`
		Creator        string `json:"creator"`
		CreatorURL     string `json:"creator_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding openverse photo: %w", err)
	}

	return &StockPhoto{
		ID:              raw.ID,
		Provider:        "openverse",
		ThumbnailURL:    raw.Thumbnail,
		FullURL:         raw.URL,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:     raw.Creator,
		PhotographerURL:  raw.CreatorURL,
		License:         normalizeLicense(raw.License),
		LicenseURL:      raw.LicenseURL,
		Description:     raw.Title,
	}, nil
}

func (p *OpenverseProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:            data,
		Filename:        strings.ReplaceAll(photo.Description, " ", "_") + ".jpg",
		Photographer:     photo.Photographer,
		PhotographerURL:  photo.PhotographerURL,
		License:         photo.License,
		LicenseURL:      photo.LicenseURL,
		ProviderPhotoID:  photoID,
	}, nil
}

// normalizeLicense maps Openverse license names to canonical short codes.
func normalizeLicense(raw string) string {
	switch {
	case strings.Contains(strings.ToLower(raw), "cc0"):
		return "cc0"
	case strings.Contains(strings.ToLower(raw), "by-sa"):
		return "cc-by-sa"
	case strings.Contains(strings.ToLower(raw), "by-nd"):
		return "cc-by-nd"
	case strings.Contains(strings.ToLower(raw), "by-nc"):
		return "cc-by-nc"
	case strings.Contains(strings.ToLower(raw), "by"):
		return "cc-by"
	default:
		return raw
	}
}
```

- [ ] **Step 5: Verify `go build ./internal/stock/...`**

```bash
go build ./internal/stock/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/stock/unsplash.go internal/stock/pexels.go internal/stock/pixabay.go internal/stock/openverse.go
git commit -m "feat: implement four stock photo providers

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 10: Backend — StockService orchestration (search, import, reload providers)

**Files:**
- Modify: `internal/stock/stock.go` — expand Service with methods

**Interfaces:**
- Produces: `func (s *Service) ReloadProviders(ctx context.Context) error` — reads DB config, constructs/destroys provider instances
- Produces: `func (s *Service) AvailableProviders(ctx context.Context, userPerms ...) []string` — RBAC-filtered provider list
- Produces: `func (s *Service) Search(ctx context.Context, query, provider string, page, perPage int) (*SearchResult, error)` — cached search
- Produces: `func (s *Service) GetPhoto(ctx context.Context, provider, photoID string) (*StockPhoto, error)` — single photo detail
- Produces: `func (s *Service) ImportPhoto(ctx context.Context, tenantID, uploaderID pgtype.UUID, provider, photoID string) (pgtype.UUID, error)` — download + StoreFromBytes

- [ ] **Step 1: Expand `internal/stock/stock.go` with full implementation**

Replace the placeholder Service struct and NewService with:

```go
package stock

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/media"
)

// cacheEntry holds cached search results with expiry.
type cacheEntry struct {
	Result   *SearchResult
	ExpiresAt time.Time
}

// DefaultCacheTTL is how long search results stay in the LRU cache.
const DefaultCacheTTL = 5 * time.Minute

// MaxCacheEntries is the maximum number of cached search result pages.
const MaxCacheEntries = 1000

// Service orchestrates stock photo search and import across providers.
type Service struct {
	providers map[string]StockProvider
	store     *Store
	media     *media.Service
	cache     map[string]*cacheEntry
	mu        sync.RWMutex
	cfg       *config.Config
	db        *db.DB
}

// NewService creates a Service. Call ReloadProviders after construction
// to populate providers from DB config.
func NewService(database *db.DB, mediaSvc *media.Service, cfg *config.Config) *Service {
	return &Service{
		providers: make(map[string]StockProvider),
		store:     NewStore(database),
		media:     mediaSvc,
		cache:     make(map[string]*cacheEntry),
		mu:        sync.RWMutex{},
		cfg:       cfg,
		db:        database,
	}
}

// ReloadProviders reads stock_providers from the DB, decrypts API keys,
// and constructs/destroys provider instances to match the current config.
func (s *Service) ReloadProviders(ctx context.Context) error {
	rows, err := s.store.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("listing providers: %w", err)
	}

	// Seed any providers not yet in DB from env vars.
	s.seedFromEnv(ctx)

	for _, row := range rows {
		s.mu.Lock()
		if !row.Enabled {
			delete(s.providers, row.ProviderName)
			s.mu.Unlock()
			continue
		}

		// Skip if provider doesn't need an API key (Openverse) or already loaded.
		if _, exists := s.providers[row.ProviderName]; exists {
			s.mu.Unlock()
			continue
		}

		apiKey, err := crypto.Decrypt(row.APIKeyEncrypted)
		if err != nil {
			slog.Warn("stock: failed to decrypt API key", "provider", row.ProviderName, "error", err)
			s.mu.Unlock()
			continue
		}

		switch row.ProviderName {
		case "unsplash":
			s.providers["unsplash"] = NewUnsplashProvider(apiKey)
		case "pexels":
			s.providers["pexels"] = NewPexelsProvider(apiKey)
		case "pixabay":
			s.providers["pixabay"] = NewPixabayProvider(apiKey)
		case "openverse":
			s.providers["openverse"] = NewOpenverseProvider()
		}
		s.mu.Unlock()
	}

	// Evict cache since providers changed.
	s.cacheMu.Lock()
	s.cache = make(map[string]*cacheEntry)
	s.cacheMu.Unlock()

	return nil
}

// seedFromEnv ensures provider rows exist and seeds API keys from env vars
// the first time a provider has no encrypted key.
func (s *Service) seedFromEnv(ctx context.Context) {
	envKeys := map[string]string{
		"unsplash": s.cfg.UnsplashAccessKey,
		"pexels":   s.cfg.PexelsAPIKey,
		"pixabay":  s.cfg.PixabayAPIKey,
	}
	for name, rawKey := range envKeys {
		encrypted := ""
		if rawKey != "" {
			var err error
			encrypted, err = crypto.Encrypt(rawKey)
			if err != nil {
				slog.Warn("stock: failed to encrypt env API key", "provider", name, "error", err)
				continue
			}
		}
		if err := s.store.SeedProvider(ctx, name, encrypted); err != nil {
			slog.Warn("stock: failed to seed provider", "provider", name, "error", err)
		}
	}
	// Openverse always gets seeded (no API key needed).
	_ = s.store.SeedProvider(ctx, "openverse", "")
}

// AvailableProviders returns provider names available to a user with the given
// permission nodes. Openverse is available with media.stock.search; the other
// three require media.stock.providers.
func (s *Service) AvailableProviders(ctx context.Context, userPerms []string) []string {
	hasSearch := false
	hasProviders := false
	for _, p := range userPerms {
		if p == "media.stock.search" {
			hasSearch = true
		}
		if p == "media.stock.providers" {
			hasProviders = true
		}
	}

	if !hasSearch {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var names []string
	for name, prov := range s.providers {
		if name == "openverse" {
			names = append(names, name)
			continue
		}
		if hasProviders {
			_ = prov // provider exists, include it
			names = append(names, name)
		}
	}
	return names
}

// Search queries stock providers with caching.
func (s *Service) Search(ctx context.Context, query, providerFilter string, page, perPage int) (*SearchResult, error) {
	if perPage <= 0 || perPage > 80 {
		perPage = 20
	}
	if page <= 0 {
		page = 1
	}

	// Determine which providers to search.
	s.mu.RLock()
	var searchProviders []StockProvider
	if providerFilter != "" {
		if p, ok := s.providers[providerFilter]; ok {
			searchProviders = append(searchProviders, p)
		}
	} else {
		for _, p := range s.providers {
			searchProviders = append(searchProviders, p)
		}
	}
	s.mu.RUnlock()

	if len(searchProviders) == 0 {
		return &SearchResult{Photos: []StockPhoto{}, Page: page, PerPage: perPage}, nil
	}

	// Search single provider — check cache first.
	if providerFilter != "" {
		return s.searchSingle(ctx, searchProviders[0], query, page, perPage)
	}

	// Fan out to all providers and merge.
	merged := &SearchResult{Photos: []StockPhoto{}, Page: page, PerPage: perPage}
	for _, p := range searchProviders {
		result, err := s.searchSingle(ctx, p, query, page, perPage)
		if err != nil {
			slog.Warn("stock: provider search failed", "provider", p.Name(), "query", query, "error", err)
			continue
		}
		merged.Photos = append(merged.Photos, result.Photos...)
		merged.Total += result.Total
		merged.HasNextPage = merged.HasNextPage || result.HasNextPage
	}
	return merged, nil
}

func (s *Service) searchSingle(ctx context.Context, p StockProvider, query string, page, perPage int) (*SearchResult, error) {
	key := cacheKey(p.Name(), query, page, perPage)

	s.cacheMu.RLock()
	if entry, ok := s.cache[key]; ok && time.Now().Before(entry.ExpiresAt) {
		s.cacheMu.RUnlock()
		return entry.Result, nil
	}
	s.cacheMu.RUnlock()

	result, err := p.Search(ctx, query, page, perPage)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	// Simple LRU eviction: if over max, clear the oldest half.
	if len(s.cache) >= MaxCacheEntries {
		for k := range s.cache {
			delete(s.cache, k)
			if len(s.cache) < MaxCacheEntries/2 {
				break
			}
		}
	}
	s.cache[key] = &cacheEntry{Result: result, ExpiresAt: time.Now().Add(DefaultCacheTTL)}
	s.cacheMu.Unlock()

	return result, nil
}

// GetPhoto fetches metadata for a single stock photo.
func (s *Service) GetPhoto(ctx context.Context, providerName, photoID string) (*StockPhoto, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider %s is not available", providerName)
	}

	val, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, fmt.Errorf("getting photo %s from %s: %w", photoID, providerName, err)
	}
	return val, nil
}

// ImportPhoto downloads a stock photo and stores it in the media library.
func (s *Service) ImportPhoto(ctx context.Context, tenantID, uploaderID pgtype.UUID, providerName, photoID string) (pgtype.UUID, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()
	if !ok {
		return pgtype.UUID{}, fmt.Errorf("provider %s is not available", providerName)
	}

	downloaded, err := p.Download(ctx, photoID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("downloading from %s: %w", providerName, err)
	}

	attr := &media.MediaAttribution{
		SourcePlatform:        providerName,
		SourcePhotographer:    downloaded.Photographer,
		SourcePhotographerURL: downloaded.PhotographerURL,
		SourceLicense:         downloaded.License,
		SourceLicenseURL:      downloaded.LicenseURL,
		SourcePhotoID:         downloaded.ProviderPhotoID,
	}

	mediaID, err := s.media.StoreFromBytes(ctx, tenantID, uploaderID, downloaded.Data, downloaded.Filename, attr)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("storing imported photo: %w", err)
	}

	return mediaID, nil
}

func cacheKey(provider, query string, page, perPage int) string {
	return fmt.Sprintf("%s:%s:%d:%d", provider, query, page, perPage)
}
```

- [ ] **Step 2: Fix compile errors**

```bash
go build ./internal/stock/...
```

Likely issues: `cacheMu` vs `mu` naming, unused imports. Fix each compile error.

- [ ] **Step 3: Commit**

```bash
git add internal/stock/stock.go
git commit -m "feat: implement StockService orchestration with LRU cache

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 11: Backend — HTTP handlers for stock endpoints

**Files:**
- Create: `internal/stock/handlers.go`

**Interfaces:**
- Produces: `StockHandlers` struct with HTTP handler methods
- Produces: `func NewStockHandlers(svc *Service) *StockHandlers`
- Handlers: `ListProviders`, `Search`, `GetPhoto`, `ImportPhoto`, `AdminListProviders`, `AdminPatchProvider`

- [ ] **Step 1: Create `internal/stock/handlers.go`**

```go
package stock

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/middleware"
)

// StockHandlers holds the stock service for HTTP handler methods.
type StockHandlers struct {
	svc *Service
}

// NewStockHandlers creates a StockHandlers with the given service.
func NewStockHandlers(svc *Service) *StockHandlers {
	return &StockHandlers{svc: svc}
}

// --- User-facing handlers ---

// ListProviders returns providers available to the authenticated user.
func (h *StockHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	userPerms := getUserPermissions(r) // from middleware context
	names := h.svc.AvailableProviders(r.Context(), userPerms)

	var result []map[string]string
	for _, name := range names {
		result = append(result, map[string]string{
			"name":         name,
			"display_name": displayName(name),
		})
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{"providers": result})
}

// Search searches stock photos across available providers.
func (h *StockHandlers) Search(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, `{"error":"query parameter 'q' is required"}`, http.StatusBadRequest)
		return
	}

	provider := r.URL.Query().Get("provider")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 || perPage > 80 {
		perPage = 20
	}

	// RBAC filter: if user doesn't have media.stock.providers, only allow openverse.
	userPerms := getUserPermissions(r)
	hasProviders := false
	for _, p := range userPerms {
		if p == "media.stock.providers" {
			hasProviders = true
			break
		}
	}
	if !hasProviders && provider != "" && provider != "openverse" {
		http.Error(w, `{"error":"provider not available"}`, http.StatusForbidden)
		return
	}
	if !hasProviders && provider == "" {
		provider = "openverse"
	}

	result, err := h.svc.Search(r.Context(), q, provider, page, perPage)
	if err != nil {
		slog.Error("stock: search failed", "error", err)
		middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetPhoto returns metadata for a single stock photo.
func (h *StockHandlers) GetPhoto(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	photoID := chi.URLParam(r, "id")

	photo, err := h.svc.GetPhoto(r.Context(), providerName, photoID)
	if err != nil {
		middleware.WriteJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{"data": photo})
}

// ImportPhoto downloads a stock photo and stores it in the media library.
func (h *StockHandlers) ImportPhoto(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if body.Provider == "" || body.ID == "" {
		http.Error(w, `{"error":"provider and id are required"}`, http.StatusBadRequest)
		return
	}

	// RBAC: user must have media.stock.import, and if provider != openverse, media.stock.providers.
	userPerms := getUserPermissions(r)
	hasProviders := false
	hasImport := false
	for _, p := range userPerms {
		if p == "media.stock.providers" {
			hasProviders = true
		}
		if p == "media.stock.import" {
			hasImport = true
		}
	}
	if !hasImport {
		http.Error(w, `{"error":"missing media.stock.import permission"}`, http.StatusForbidden)
		return
	}
	if body.Provider != "openverse" && !hasProviders {
		http.Error(w, `{"error":"provider requires media.stock.providers permission"}`, http.StatusForbidden)
		return
	}

	tenantID := middleware.GetTenantID(r.Context())
	var uploaderID pgtype.UUID
	uploaderID.Scan(user.ID)

	mediaID, err := h.svc.ImportPhoto(r.Context(), tenantID, uploaderID, body.Provider, body.ID)
	if err != nil {
		slog.Error("stock: import failed", "error", err, "provider", body.Provider, "photo_id", body.ID)
		middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"media_id": mediaID.String(),
		},
	})
}

// --- Admin handlers ---

// AdminListProviders returns all providers with enabled/disabled status.
func (h *StockHandlers) AdminListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.store.ListProviders(r.Context())
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var result []map[string]any
	for _, row := range rows {
		hasKey := row.APIKeyEncrypted != ""
		result = append(result, map[string]any{
			"provider_name":  row.ProviderName,
			"display_name":   displayName(row.ProviderName),
			"enabled":        row.Enabled,
			"has_api_key":    hasKey,
			"updated_at":     row.UpdatedAt,
		})
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"providers": result}})
}

// AdminPatchProvider toggles enabled status or updates the API key.
func (h *StockHandlers) AdminPatchProvider(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "name")

	var body struct {
		Enabled *bool   `json:"enabled,omitempty"`
		APIKey  *string `json:"api_key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if body.Enabled != nil {
		if err := h.svc.store.ToggleProvider(r.Context(), providerName, *body.Enabled); err != nil {
			middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	if body.APIKey != nil {
		encrypted, err := encrypt(*body.APIKey)
		if err != nil {
			middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to encrypt API key"})
			return
		}
		if err := h.svc.store.SetAPIKey(r.Context(), providerName, encrypted); err != nil {
			middleware.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	// Reload providers to pick up the change.
	if err := h.svc.ReloadProviders(r.Context()); err != nil {
		slog.Warn("stock: failed to reload providers after admin update", "error", err)
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

func displayName(name string) string {
	switch name {
	case "unsplash":
		return "Unsplash"
	case "pexels":
		return "Pexels"
	case "pixabay":
		return "Pixabay"
	case "openverse":
		return "Openverse"
	default:
		return name
	}
}

// getUserPermissions extracts permission nodes from the request context.
// The actual implementation depends on how RBAC middleware stores permissions.
func getUserPermissions(r *http.Request) []string {
	// This should read from the RBAC context set by rbac middleware.
	// Placeholder — actual implementation depends on project's RBAC resolver.
	return middleware.GetPermissions(r.Context())
}
```

**Note:** The `getUserPermissions` function needs to match the project's actual RBAC middleware pattern. Check how `internal/server/server.go` passes RBAC checks — look for how `rbac.RequirePermission` works and how the resolver stores permissions in the context. Adjust accordingly.

- [ ] **Step 2: Verify `go build ./internal/stock/...`**

```bash
go build ./internal/stock/...
```

Expected: may have import errors for `internal/auth`, `internal/middleware`. These will resolve when registering routes in Task 12. For now, fix any obvious issues.

- [ ] **Step 3: Commit**

```bash
git add internal/stock/handlers.go
git commit -m "feat: add stock photo HTTP handlers

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 12: Backend — Register stock routes in server.go and wire up StockService

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Create StockService and register routes**

In `internal/server/server.go`, find where `mediaService` is created (around line 1374) and add stock service initialization after it:

```go
// Stock photo service — provider search and import.
stockService := stock.NewService(database, mediaService, cfg)
if err := stockService.ReloadProviders(context.Background()); err != nil {
    slog.Warn("server: failed to load stock providers on startup", "error", err)
}
stockHandlers := stock.NewStockHandlers(stockService)
```

Then add routes in the same `r.Group(func(r chi.Router) { ... })` block where media routes are registered (around line 1372):

```go
// Stock photo routes (user-facing)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
    Get("/stock/providers", stockHandlers.ListProviders)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
    Get("/stock/search", stockHandlers.Search)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
    Get("/stock/photo/{provider}/{id}", stockHandlers.GetPhoto)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.import")).
    Post("/stock/import", stockHandlers.ImportPhoto)

// Stock photo admin routes
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.edit_any")).
    Get("/admin/stock/providers", stockHandlers.AdminListProviders)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.edit_any")).
    Patch("/admin/stock/providers/{name}", stockHandlers.AdminPatchProvider)
```

- [ ] **Step 2: Add stock import**

```go
import "github.com/jedafe/japa/internal/stock"
```

- [ ] **Step 3: Verify `go build ./...`**

```bash
go build ./...
```

Expected: compiles. If `middleware.GetPermissions` or `middleware.GetTenantID` or `auth.GetUser` or `middleware.WriteJSON` don't match the project's actual middleware, adjust to match. Look at how existing handlers (e.g., media handlers) extract user/tenant data.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: register stock photo routes and wire up StockService

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 13: Frontend — TypeScript types for stock photos

**Files:**
- Modify: `apps/main/src/lib/types/index.ts`

- [ ] **Step 1: Add stock photo types**

Add after the `MediaAsset` interface (around line 240):

```typescript
export interface StockProvider {
    name: string;
    display_name: string;
}

export interface StockPhoto {
    id: string;
    provider: string;
    thumbnail_url: string;
    full_url: string;
    width: number;
    height: number;
    photographer: string;
    photographer_url: string;
    license?: string;
    license_url?: string;
    description?: string;
}

export interface StockSearchResult {
    photos: StockPhoto[];
    total: number;
    page: number;
    per_page: number;
    has_next_page: boolean;
}

export interface StockImportResult {
    media_id: string;
}

export interface StockProvidersResponse {
    providers: StockProvider[];
}

// Update MediaAsset to include stock attribution fields
export interface MediaAsset {
    id: string;
    url: string;
    filename?: string;
    mime_type?: string;
    file_size?: number;
    alt_text?: string;
    status?: string;
    created_at?: string;
    user_group_id?: string | null;
    // Stock attribution (present when media was imported from a stock provider)
    source_platform?: string;
    source_photographer?: string;
    source_photographer_url?: string;
    source_license?: string;
    source_license_url?: string;
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/main/src/lib/types/index.ts
git commit -m "feat: add stock photo TypeScript types

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 14: Frontend — API client functions for stock endpoints

**Files:**
- Create or modify: `apps/main/src/lib/api/stock.ts` (new file)

- [ ] **Step 1: Create `apps/main/src/lib/api/stock.ts`**

```typescript
import { api } from '$lib/api/client';
import type { StockPhoto, StockSearchResult, StockImportResult, StockProvidersResponse, StockProvider } from '$lib/types';

export const listStockProviders = () =>
    api.get<StockProvidersResponse>('/stock/providers');

export const searchStock = (params: { q: string; provider?: string; page?: number; per_page?: number }) => {
    const searchParams = new URLSearchParams();
    searchParams.set('q', params.q);
    if (params.provider) searchParams.set('provider', params.provider);
    if (params.page) searchParams.set('page', String(params.page));
    if (params.per_page) searchParams.set('per_page', String(params.per_page));
    return api.get<StockSearchResult>(`/stock/search?${searchParams.toString()}`);
};

export const getStockPhoto = (provider: string, id: string) =>
    api.get<StockPhoto>(`/stock/photo/${provider}/${id}`);

export const importStockPhoto = (provider: string, id: string) =>
    api.post<StockImportResult>('/stock/import', { provider, id });
```

- [ ] **Step 2: Add admin stock API functions to `apps/admin/src/lib/api.ts`**

After the `mediaAdminApi` export (~line 622):

```typescript
export const stockAdminApi = {
    listProviders: () => api<unknown>('/admin/stock/providers'),
    patchProvider: (name: string, body: { enabled?: boolean; api_key?: string }) =>
        api<unknown>(`/admin/stock/providers/${name}`, { method: 'PATCH', body }),
};
```

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/api/stock.ts apps/admin/src/lib/api.ts
git commit -m "feat: add stock photo API client functions

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 15: Frontend — Stock tab in media picker modal (Search + Browse)

**Files:**
- Modify: `apps/main/src/lib/components/media/MediaLibraryModal.svelte`

- [ ] **Step 1: Add Stock tab type and state**

Change the `Tab` type definition (line 24):

```typescript
type Tab = 'library' | 'upload' | 'stock';
```

Add stock-specific state:

```typescript
let stockProviders = $state<StockProvider[]>([]);
let stockResults = $state<StockPhoto[]>([]);
let stockLoading = $state(false);
let stockQuery = $state('');
let stockTimer: ReturnType<typeof setTimeout> | null = null;
let stockPage = $state(1);
let stockHasMore = $state(false);
let stockProviderFilter = $state<string>(''); // '' = all
let importingIds = $state<Set<string>>(new Set());
let importedIds = $state<Set<string>>(new Set());
```

- [ ] **Step 2: Load providers on Stock tab activation**

```typescript
function loadStockProviders() {
    listStockProviders().then((res) => {
        if (res.data?.providers) {
            stockProviders = res.data.providers;
        } else {
            stockProviders = [];
        }
    }).catch(() => {
        stockProviders = [];
    });
}

// Load providers when switching to stock tab.
$effect(() => {
    if (tab === 'stock') {
        loadStockProviders();
    }
});
```

- [ ] **Step 3: Add stock search with debounce**

```typescript
function searchStock(q: string) {
    stockQuery = q;
    stockPage = 1;
    if (stockTimer) clearTimeout(stockTimer);
    if (!q.trim()) {
        stockResults = [];
        stockHasMore = false;
        return;
    }
    stockTimer = setTimeout(() => {
        stockLoading = true;
        searchStock({
            q: q.trim(),
            provider: stockProviderFilter || undefined,
            page: stockPage,
            per_page: 30
        }).then((res) => {
            if (res.data) {
                stockResults = res.data.photos || [];
                stockHasMore = res.data.has_next_page;
            }
        }).catch(() => {
            stockResults = [];
        }).finally(() => {
            stockLoading = false;
        });
    }, 350);
}

function loadMoreStock() {
    stockPage++;
    stockLoading = true;
    searchStock({
        q: stockQuery.trim(),
        provider: stockProviderFilter || undefined,
        page: stockPage,
        per_page: 30
    }).then((res) => {
        if (res.data) {
            stockResults = [...stockResults, ...(res.data.photos || [])];
            stockHasMore = res.data.has_next_page;
        }
    }).finally(() => {
        stockLoading = false;
    });
}
```

- [ ] **Step 4: Add import function**

```typescript
function importPhoto(photo: StockPhoto) {
    importingIds.add(photo.id);
    importingIds = new Set(importingIds);
    importStockPhoto(photo.provider, photo.id).then((res) => {
        if (res.data?.media_id) {
            importedIds.add(photo.id);
            importedIds = new Set(importedIds);
            // Also refresh the Library tab.
            loadLibrary();
        }
    }).finally(() => {
        importingIds.delete(photo.id);
        importingIds = new Set(importingIds);
    });
}
```

- [ ] **Step 5: Add Stock tab UI in the template**

After the `'library' | 'upload'` tab buttons, add the Stock tab button:

```svelte
<button
    class="tab-btn"
    class:active={tab === 'stock'}
    onclick={() => tab = 'stock'}
>
    {t('media.stock_tab', 'Stock')}
</button>
```

Add the Stock tab content panel — search bar, provider chips, photo grid:

```svelte
{#if tab === 'stock'}
    <div class="stock-panel">
        {#if stockProviders.length === 0}
            <div class="empty-state">
                <p>{t('media.stock_no_providers', 'No stock providers available. Contact your administrator.')}</p>
            </div>
        {:else}
            <!-- Search bar -->
            <div class="stock-search">
                <input
                    type="text"
                    placeholder={t('media.stock_search', 'Search stock photos...')}
                    oninput={(e) => searchStock(e.currentTarget.value)}
                />
            </div>

            <!-- Provider chips -->
            <div class="stock-providers">
                <button
                    class="chip"
                    class:active={stockProviderFilter === ''}
                    onclick={() => { stockProviderFilter = ''; if (stockQuery) searchStock(stockQuery); }}
                >All</button>
                {#each stockProviders as p}
                    <button
                        class="chip"
                        class:active={stockProviderFilter === p.name}
                        onclick={() => { stockProviderFilter = p.name; if (stockQuery) searchStock(stockQuery); }}
                    >{p.display_name}</button>
                {/each}
            </div>

            <!-- Results grid -->
            {#if stockLoading && stockResults.length === 0}
                <div class="loading"><Loader2 class="animate-spin" /></div>
            {:else if stockResults.length === 0 && stockQuery}
                <div class="empty-state">
                    <p>{t('media.stock_no_results', 'No results for "{query}"', { query: stockQuery })}</p>
                </div>
            {:else}
                <div class="stock-grid">
                    {#each stockResults as photo (photo.provider + ':' + photo.id)}
                        <div class="stock-card">
                            <img src={photo.thumbnail_url} alt={photo.description || ''} loading="lazy" />
                            <div class="stock-card-overlay">
                                <span class="stock-photographer">{photo.photographer}</span>
                                {#if photo.license && (photo.license === 'cc-by' || photo.license === 'cc-by-sa')}
                                    <a href={photo.license_url || '#'} target="_blank" class="stock-license-badge" rel="noreferrer">
                                        {photo.license.toUpperCase()}
                                    </a>
                                {/if}
                                {#if importedIds.has(photo.id)}
                                    <span class="imported-check">✓ Imported</span>
                                {:else if importingIds.has(photo.id)}
                                    <Loader2 class="animate-spin" />
                                {:else}
                                    <!-- Import button (only if user has media.stock.import — determined by API response or context) -->
                                    <button class="import-btn" onclick={() => importPhoto(photo)}>
                                        Import
                                    </button>
                                {/if}
                            </div>
                        </div>
                    {/each}
                </div>
                {#if stockHasMore}
                    <button class="load-more" onclick={loadMoreStock}>
                        {t('media.load_more', 'Load more')}
                    </button>
                {/if}
            {/if}
        {/if}
    </div>
{/if}
```

- [ ] **Step 4: Add import for stock API functions**

At the top of the script, add:

```typescript
import { listStockProviders, searchStock, importStockPhoto } from '$lib/api/stock';
import type { StockPhoto, StockProvider } from '$lib/types';
```

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/media/MediaLibraryModal.svelte
git commit -m "feat: add Stock tab with search and import to media picker modal

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 16: Frontend — Admin stock providers management tab

**Files:**
- Modify: `apps/admin/src/routes/media/+page.svelte`

- [ ] **Step 1: Add providers tab state and data loading**

After the `activeTab` state (line 50), add:

```typescript
let stockProviders = $state<any[]>([]);
let stockProvidersLoading = $state(false);
let stockProvidersError = $state('');
```

Add a `loadStockProviders` function:

```typescript
function loadStockProviders() {
    stockProvidersLoading = true;
    stockProvidersError = '';
    stockAdminApi.listProviders().then((res) => {
        stockProviders = res.data?.providers || [];
    }).catch(() => {
        stockProvidersError = 'Failed to load providers';
    }).finally(() => {
        stockProvidersLoading = false;
    });
}
```

- [ ] **Step 2: Add toggle and API key update handlers**

```typescript
async function toggleStockProvider(name: string, enabled: boolean) {
    try {
        await stockAdminApi.patchProvider(name, { enabled });
        loadStockProviders();
    } catch {
        stockProvidersError = 'Failed to update provider';
    }
}

async function updateProviderKey(name: string, apiKey: string) {
    if (!apiKey.trim()) return;
    try {
        await stockAdminApi.patchProvider(name, { api_key: apiKey.trim() });
        loadStockProviders();
    } catch {
        stockProvidersError = 'Failed to update API key';
    }
}
```

- [ ] **Step 3: Add "Stock Providers" tab in Template**

After the existing tab buttons, add:

```svelte
<button class="tab-btn" class:active={activeTab === 'stock-providers'} onclick={() => { activeTab = 'stock-providers'; loadStockProviders(); }}>
    Stock Providers
</button>
```

Add the tab content panel:

```svelte
{#if activeTab === 'stock-providers'}
    <Card>
        <CardContent>
            {#if stockProvidersLoading}
                <p>Loading...</p>
            {:else if stockProvidersError}
                <p class="error">{stockProvidersError}</p>
            {:else}
                <table class="stock-providers-table">
                    <thead>
                        <tr>
                            <th>Provider</th>
                            <th>Enabled</th>
                            <th>API Key</th>
                            <th>RBAC</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each stockProviders as p}
                            <tr>
                                <td>{p.display_name}</td>
                                <td>
                                    <label class="toggle">
                                        <input
                                            type="checkbox"
                                            checked={p.enabled}
                                            onchange={(e) => toggleStockProvider(p.provider_name, e.currentTarget.checked)}
                                        />
                                    </label>
                                </td>
                                <td>
                                    {#if p.provider_name === 'openverse'}
                                        <span class="text-muted">No key required</span>
                                    {:else}
                                        <input
                                            type="password"
                                            placeholder={p.has_api_key ? '••••••••' : 'Paste API key...'}
                                            onkeydown={(e) => {
                                                if (e.key === 'Enter') {
                                                    updateProviderKey(p.provider_name, e.currentTarget.value);
                                                    e.currentTarget.value = '';
                                                }
                                            }}
                                        />
                                    {/if}
                                </td>
                                <td>
                                    {#if p.provider_name === 'openverse'}
                                        <Badge>media.stock.search</Badge>
                                    {:else}
                                        <Badge>media.stock.providers</Badge>
                                    {/if}
                                </td>
                            </tr>
                        {/each}
                    </tbody>
                </table>
            {/if}
        </CardContent>
    </Card>
{/if}
```

- [ ] **Step 4: Add stockAdminApi import**

Already imported `api, apiFetch, mediaAdminApi` — add `stockAdminApi`:

```typescript
import { api, apiFetch, mediaAdminApi, stockAdminApi } from "$lib/api";
```

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/routes/media/+page.svelte
git commit -m "feat: add Stock Providers admin management tab

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 17: Rendering attribution on existing media thumbnails

**Files:**
- Modify: any components rendering media thumbnails in `apps/main/src/lib/components/`

Find files that render media thumbnails and add conditional stock attribution display.

- [ ] **Step 1: Find thumbnail rendering components**

```bash
grep -r "alt_text\|MediaAsset\|thumbnail" apps/main/src/lib/components/ --include="*.svelte" -l
```

- [ ] **Step 2: Add attribution line pattern to each**

For each component rendering a media item that has `source_photographer`, add below the image/thumbnail:

```svelte
{#if media.source_photographer}
    <span class="stock-attribution">
        <a href={media.source_photographer_url || '#'} target="_blank" rel="noreferrer">
            {media.source_photographer}
        </a>
        {#if media.source_license && (media.source_license === 'cc-by' || media.source_license === 'cc-by-sa')}
            <a href={media.source_license_url || '#'} target="_blank" rel="noreferrer" class="license-badge">
                {media.source_license.toUpperCase()}
            </a>
        {/if}
    </span>
{/if}
```

- [ ] **Step 3: Add CSS for tiny attribution**

```css
.stock-attribution {
    display: block;
    font-size: 0.625rem; /* 10px */
    color: var(--color-text-muted, #888);
    margin-top: 2px;
    line-height: 1.2;
}
.stock-attribution a {
    color: inherit;
    text-decoration: none;
}
.stock-attribution a:hover {
    text-decoration: underline;
}
.license-badge {
    display: inline-block;
    margin-left: 4px;
    padding: 0 3px;
    border: 1px solid currentColor;
    border-radius: 2px;
    font-size: 0.5625rem; /* 9px */
}
```

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/
git commit -m "feat: render stock photo attribution on media thumbnails

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```

---

### Task 18: Integration testing and final verification

**Files:**
- No new files; verify all changes work together

- [ ] **Step 1: Run `go build ./...`**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 2: Run `go vet ./...`**

```bash
go vet ./...
```

Expected: no issues.

- [ ] **Step 3: Run existing media tests**

```bash
go test ./internal/media/... -v -count=1
```

Expected: all pass.

- [ ] **Step 4: Verify frontend builds**

```bash
cd apps/main && bun run build 2>&1 | tail -20
cd apps/admin && bun run build 2>&1 | tail -20
```

Expected: both build without errors.

- [ ] **Step 5: Smoke test**

Manual checklist:
- [ ] Open admin media page → Stock Providers tab visible
- [ ] Toggle Openverse off → Stock tab disappears from picker modal
- [ ] Toggle Openverse on → Stock tab appears in picker modal
- [ ] Search stock photos → results load
- [ ] Import a photo → appears in Library tab
- [ ] Attribution displayed on imported photo thumbnail
- [ ] Admin sets Unsplash API key → Unsplash appears in stock tab (for users with `media.stock.providers`)

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "chore: final integration verification for stock photo feature

Co-authored-by: CommandCodeBot <noreply@commandcode.ai>"
```
