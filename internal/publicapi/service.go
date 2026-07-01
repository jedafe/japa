package publicapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides pgx-backed CRUD for API keys.
type Store struct {
	dbPool *pgxpool.Pool
}

// NewStore creates a Store with the given database pool.
func NewStore(dbPool *pgxpool.Pool) *Store {
	return &Store{dbPool: dbPool}
}

// create inserts a new API key row.
func (s *Store) create(ctx context.Context, params createParams) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.dbPool.QueryRow(ctx, `
		INSERT INTO api_keys (tenant_id, user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, params.TenantID, params.UserID, params.Name, params.KeyPrefix, params.KeyHash,
		params.Scopes, params.RateLimitTier, params.ExpiresAt).Scan(&id)
	return id, err
}

type createParams struct {
	TenantID      pgtype.UUID
	UserID        pgtype.UUID
	Name          string
	KeyPrefix     string
	KeyHash       string
	Scopes        []string
	RateLimitTier string
	ExpiresAt     *pgtype.Timestamptz
}

// findByHash looks up an API key by its SHA-256 hash.
func (s *Store) findByHash(ctx context.Context, keyHash string) (*apiKeyRow, error) {
	var row apiKeyRow
	err := s.dbPool.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		WHERE key_hash = $1 AND status = 'active'
	`, keyHash).Scan(&row.ID, &row.TenantID, &row.UserID, &row.Name, &row.KeyPrefix,
		&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
		&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// incrementUsage updates request_count and last_used_at atomically.
func (s *Store) incrementUsage(ctx context.Context, keyID pgtype.UUID) error {
	_, err := s.dbPool.Exec(ctx, `
		UPDATE api_keys
		SET request_count = request_count + 1, last_used_at = now(), updated_at = now()
		WHERE id = $1
	`, keyID)
	return err
}

// listByUser returns all API keys for a user (without the hash).
func (s *Store) listByUser(ctx context.Context, userID pgtype.UUID) ([]apiKeyRow, error) {
	rows, err := s.dbPool.Query(ctx, `
		SELECT id, tenant_id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []apiKeyRow
	for rows.Next() {
		var row apiKeyRow
		if err := rows.Scan(&row.ID, &row.TenantID, &row.UserID, &row.Name, &row.KeyPrefix,
			&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
			&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// listAll returns all API keys across all users (admin view).
func (s *Store) listAll(ctx context.Context, limit, offset int32) ([]apiKeyRow, error) {
	rows, err := s.dbPool.Query(ctx, `
		SELECT id, tenant_id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []apiKeyRow
	for rows.Next() {
		var row apiKeyRow
		if err := rows.Scan(&row.ID, &row.TenantID, &row.UserID, &row.Name, &row.KeyPrefix,
			&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
			&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// setStatus updates the status of an API key (active/suspended/revoked).
func (s *Store) setStatus(ctx context.Context, keyID pgtype.UUID, status string) error {
	_, err := s.dbPool.Exec(ctx, `
		UPDATE api_keys SET status = $1, updated_at = now() WHERE id = $2
	`, status, keyID)
	return err
}

// apiKeyRow is the internal row type for API key queries.
type apiKeyRow struct {
	ID            pgtype.UUID
	TenantID      pgtype.UUID
	UserID        pgtype.UUID
	Name          string
	KeyPrefix     string
	Scopes        []string
	RateLimitTier string
	Status        string
	LastUsedAt    *pgtype.Timestamptz
	RequestCount  int64
	ExpiresAt     *pgtype.Timestamptz
	CreatedAt     pgtype.Timestamptz
	UpdatedAt     pgtype.Timestamptz
}

func rowToAPIKey(row apiKeyRow) APIKey {
	return APIKey{
		ID:            row.ID.String(),
		Name:          row.Name,
		KeyPrefix:     row.KeyPrefix,
		Scopes:        row.Scopes,
		RateLimitTier: row.RateLimitTier,
		Status:        row.Status,
		LastUsedAt:    row.LastUsedAt,
		RequestCount:  row.RequestCount,
		ExpiresAt:     row.ExpiresAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

// Service provides business logic for API key management.
type Service struct {
	store *Store
}

// NewService creates a Service with the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// CreateAPIKey generates a new API key, stores its hash, and returns
// the full key exactly once. The caller must save it — it cannot be
// retrieved later.
func (s *Service) CreateAPIKey(ctx context.Context, userID, tenantID pgtype.UUID, input CreateAPIKeyInput) (CreateAPIKeyResponse, error) {
	// Generate a random key: jc_live_<32 hex chars>
	rawBytes := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(rawBytes); err != nil {
		return CreateAPIKeyResponse{}, fmt.Errorf("generate key: %w", err)
	}
	fullKey := "jc_live_" + hex.EncodeToString(rawBytes)

	// Hash for storage.
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Prefix for display (first 12 chars of the full key).
	keyPrefix := fullKey[:12] + "..."

	// Default scopes and tier.
	scopes := input.Scopes
	if len(scopes) == 0 {
		scopes = []string{"api.read.*"}
	}
	tier := input.RateLimitTier
	if tier == "" {
		tier = "basic"
	}

	id, err := s.store.create(ctx, createParams{
		TenantID:      tenantID,
		UserID:        userID,
		Name:          input.Name,
		KeyPrefix:     keyPrefix,
		KeyHash:       keyHash,
		Scopes:        scopes,
		RateLimitTier: tier,
		ExpiresAt:     input.ExpiresAt,
	})
	if err != nil {
		return CreateAPIKeyResponse{}, fmt.Errorf("create api key: %w", err)
	}

	return CreateAPIKeyResponse{
		APIKey: APIKey{
			ID:            id.String(),
			Name:          input.Name,
			KeyPrefix:     keyPrefix,
			Scopes:        scopes,
			RateLimitTier: tier,
			Status:        "active",
			CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UpdatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		Key: fullKey,
	}, nil
}

// ValidateAPIKey hashes the input key and looks it up. On success, it
// increments the request counter and returns the key metadata.
func (s *Service) ValidateAPIKey(ctx context.Context, key string) (*APIKeyContext, error) {
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	row, err := s.store.findByHash(ctx, keyHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("invalid or revoked API key")
		}
		return nil, fmt.Errorf("validate api key: %w", err)
	}

	// Check expiry.
	if row.ExpiresAt != nil && row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Increment usage (fire-and-forget — don't fail the request on counter errors).
	_ = s.store.incrementUsage(ctx, row.ID)

	return &APIKeyContext{
		KeyID:         row.ID,
		UserID:        row.UserID,
		TenantID:      row.TenantID,
		Scopes:        row.Scopes,
		KeyPrefix:     row.KeyPrefix,
		RateLimitTier: row.RateLimitTier,
	}, nil
}

// ListAPIKeys returns all API keys for a user.
func (s *Service) ListAPIKeys(ctx context.Context, userID pgtype.UUID) ([]APIKey, error) {
	rows, err := s.store.listByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	result := make([]APIKey, len(rows))
	for i, row := range rows {
		result[i] = rowToAPIKey(row)
	}
	return result, nil
}

// ListAllAPIKeys returns all API keys across all users (admin view).
func (s *Service) ListAllAPIKeys(ctx context.Context, limit, offset int32) ([]APIKey, error) {
	rows, err := s.store.listAll(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list all api keys: %w", err)
	}
	result := make([]APIKey, len(rows))
	for i, row := range rows {
		result[i] = rowToAPIKey(row)
	}
	return result, nil
}

// RevokeAPIKey soft-deletes an API key by setting status to revoked.
func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID pgtype.UUID) error {
	// Verify the key belongs to the user.
	var ownerID pgtype.UUID
	err := s.store.dbPool.QueryRow(ctx, `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("api key not found")
		}
		return fmt.Errorf("revoke api key: %w", err)
	}
	if ownerID != userID {
		return fmt.Errorf("api key does not belong to user")
	}
	return s.store.setStatus(ctx, keyID, "revoked")
}

// SuspendAPIKey sets an API key status to suspended (admin action).
func (s *Service) SuspendAPIKey(ctx context.Context, keyID pgtype.UUID) error {
	return s.store.setStatus(ctx, keyID, "suspended")
}

// ---------------------------------------------------------------------------
// IP Allowlist CRUD
// ---------------------------------------------------------------------------

// AddAllowedIP adds an IP or CIDR range to an API key's allowlist.
func (s *Service) AddAllowedIP(ctx context.Context, keyID pgtype.UUID, cidr, description string) error {
	if cidr == "" {
		return fmt.Errorf("cidr is required")
	}
	// Validate the CIDR/IP by attempting to parse it as inet.
	var id pgtype.UUID
	err := s.store.dbPool.QueryRow(ctx, `
		INSERT INTO api_key_ip_allowlist (api_key_id, cidr, description)
		VALUES ($1, $2::inet, $3)
		RETURNING id
	`, keyID, cidr, description).Scan(&id)
	if err != nil {
		return fmt.Errorf("add allowed IP: %w", err)
	}
	return nil
}

// RemoveAllowedIP removes an IP allowlist entry by its ID.
func (s *Service) RemoveAllowedIP(ctx context.Context, keyID, ipID pgtype.UUID) error {
	tag, err := s.store.dbPool.Exec(ctx, `
		DELETE FROM api_key_ip_allowlist
		WHERE id = $1 AND api_key_id = $2
	`, ipID, keyID)
	if err != nil {
		return fmt.Errorf("remove allowed IP: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("allowlist entry not found")
	}
	return nil
}

// ListAllowedIPs returns all IP allowlist entries for a given API key.
func (s *Service) ListAllowedIPs(ctx context.Context, keyID pgtype.UUID) ([]APIKeyIPAllowlist, error) {
	rows, err := s.store.dbPool.Query(ctx, `
		SELECT id, api_key_id, cidr::text, coalesce(description, ''), created_at
		FROM api_key_ip_allowlist
		WHERE api_key_id = $1
		ORDER BY created_at ASC
	`, keyID)
	if err != nil {
		return nil, fmt.Errorf("list allowed IPs: %w", err)
	}
	defer rows.Close()

	var result []APIKeyIPAllowlist
	for rows.Next() {
		var entry APIKeyIPAllowlist
		var id, apiKeyID pgtype.UUID
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &apiKeyID, &entry.CIDR, &entry.Description, &createdAt); err != nil {
			return nil, fmt.Errorf("scan allowed IP: %w", err)
		}
		entry.ID = id.String()
		entry.APIKeyID = apiKeyID.String()
		entry.CreatedAt = createdAt.Time.Format(time.RFC3339)
		result = append(result, entry)
	}
	return result, rows.Err()
}

// hasIPAccess checks if a given IP address is permitted by the key's allowlist.
// Returns true if the allowlist is empty (no restriction) or if the IP matches.
func (s *Service) hasIPAccess(ctx context.Context, keyID pgtype.UUID, clientIP string) (bool, error) {
	var count int
	err := s.store.dbPool.QueryRow(ctx, `
		SELECT count(*) FROM api_key_ip_allowlist WHERE api_key_id = $1
	`, keyID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check IP access: %w", err)
	}
	// No allowlist entries means unrestricted access.
	if count == 0 {
		return true, nil
	}
	// Check if the client IP falls within any of the CIDR ranges.
	var allowed int
	err = s.store.dbPool.QueryRow(ctx, `
		SELECT count(*) FROM api_key_ip_allowlist
		WHERE api_key_id = $1 AND $2::inet <<= cidr
	`, keyID, clientIP).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check IP against allowlist: %w", err)
	}
	return allowed > 0, nil
}

// RotateAPIKey generates a new API key with the same name, scopes, and
// rate_limit_tier as the old key, sets the old key's expires_at to now+24h
// (grace period), and returns the new key with the full secret (shown once).
func (s *Service) RotateAPIKey(ctx context.Context, userID, keyID pgtype.UUID) (*CreateAPIKeyResponse, error) {
	// Verify the key exists and belongs to the user.
	var row apiKeyRow
	err := s.store.dbPool.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		WHERE id = $1
	`, keyID).Scan(&row.ID, &row.TenantID, &row.UserID, &row.Name, &row.KeyPrefix,
		&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
		&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("rotate api key: %w", err)
	}
	if row.UserID != userID {
		return nil, fmt.Errorf("api key does not belong to user")
	}

	// Generate a new key with the same configuration.
	rawBytes := make([]byte, 16)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	fullKey := "jc_live_" + hex.EncodeToString(rawBytes)

	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	keyPrefix := fullKey[:12] + "..."

	// Grace period: old key remains valid for 24 hours.
	graceExpiry := time.Now().Add(24 * time.Hour)
	graceTstz := &pgtype.Timestamptz{Time: graceExpiry, Valid: true}

	// Create the new key and expire the old key in a single transaction.
	tx, err := s.store.dbPool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("rotate api key: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is a no-op after commit

	// Expire the old key.
	_, err = tx.Exec(ctx, `
		UPDATE api_keys
		SET expires_at = $1, updated_at = now()
		WHERE id = $2
	`, graceTstz, keyID)
	if err != nil {
		return nil, fmt.Errorf("rotate api key: expire old key: %w", err)
	}

	// Insert the new key.
	var newID pgtype.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO api_keys (tenant_id, user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, row.TenantID, userID, row.Name, keyPrefix, keyHash,
		row.Scopes, row.RateLimitTier, row.ExpiresAt).Scan(&newID)
	if err != nil {
		return nil, fmt.Errorf("rotate api key: create new key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("rotate api key: commit: %w", err)
	}

	return &CreateAPIKeyResponse{
		APIKey: APIKey{
			ID:            newID.String(),
			Name:          row.Name,
			KeyPrefix:     keyPrefix,
			Scopes:        row.Scopes,
			RateLimitTier: row.RateLimitTier,
			Status:        "active",
			CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UpdatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		Key: fullKey,
	}, nil
}
