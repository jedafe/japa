package publicapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/valkey-io/valkey-go/valkeycompat"
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
		INSERT INTO api_keys (user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, params.UserID, params.Name, params.KeyPrefix, params.KeyHash,
		params.Scopes, params.RateLimitTier, params.ExpiresAt).Scan(&id)

	return id, err
}

type createParams struct {
	UserID        pgtype.UUID
	Name          string
	KeyPrefix     string
	KeyHash       string
	Scopes        []string
	RateLimitTier string
	ExpiresAt     *pgtype.Timestamptz
}

// TierResolver resolves a user's effective unified tier (regular/pro/business).
// Implemented by the tiers package. Used by the public API rate limiter to cap
// an API key's tier against the key owner's actual user tier.
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// findByHash looks up an API key by its SHA-256 hash.
func (s *Store) findByHash(ctx context.Context, keyHash string) (*apiKeyRow, error) {
	var row apiKeyRow

	err := s.dbPool.QueryRow(ctx, `
		SELECT id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		WHERE key_hash = $1 AND status = 'active'
	`, keyHash).Scan(&row.ID, &row.UserID, &row.Name, &row.KeyPrefix,
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
		SELECT id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
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
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.KeyPrefix,
			&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
			&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}

		result = append(result, row)
	}

	return result, rows.Err()
}

// listAll returns all API keys for the current platform (admin view).
// Platform-scoped to prevent cross-platform data bleed (PUB-005).
func (s *Store) listAll(ctx context.Context, limit, offset int32) ([]apiKeyRow, error) {
	rows, err := s.dbPool.Query(ctx, `
		SELECT id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
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
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.KeyPrefix,
			&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
			&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}

		result = append(result, row)
	}

	return result, rows.Err()
}

// setStatus updates the status of an API key (active/suspended/revoked).
// Platform-scoped to prevent cross-platform modification (PUB-003).
func (s *Store) setStatus(ctx context.Context, keyID pgtype.UUID, status string) error {
	_, err := s.dbPool.Exec(ctx, `
		UPDATE api_keys SET status = $1, updated_at = now()
		WHERE id = $2
	`, status, keyID)

	return err
}

// revokeKey marks an API key as revoked and records the revocation audit
// trail (revoked_at + revoked_by). Section 47 schema requires these columns
// for audit traceability (BUG-006 fix: setStatus only set status, not the
// audit columns).
func (s *Store) revokeKey(ctx context.Context, keyID, revokedBy pgtype.UUID) error {
	_, err := s.dbPool.Exec(ctx, `
		UPDATE api_keys
		SET status = 'revoked', revoked_at = now(), revoked_by = $1, updated_at = now()
		WHERE id = $2
	`, revokedBy, keyID)

	return err
}

// apiKeyRow is the internal row type for API key queries.
type apiKeyRow struct {
	ID            pgtype.UUID
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

// getAnalytics returns aggregate analytics for API keys.
func (s *Store) getAnalytics(ctx context.Context) (*APIKeyAnalytics, error) {
	analytics := &APIKeyAnalytics{}

	// Total counts by status.
	err := s.dbPool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'active'),
			count(*) FILTER (WHERE status = 'suspended'),
			count(*) FILTER (WHERE status = 'revoked'),
			coalesce(sum(request_count), 0)
		FROM api_keys
	`).Scan(&analytics.ActiveKeys, &analytics.SuspendedKeys, &analytics.RevokedKeys, &analytics.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("get analytics totals: %w", err)
	}

	analytics.TotalKeys = analytics.ActiveKeys + analytics.SuspendedKeys + analytics.RevokedKeys

	// Top keys by volume (top 10).
	rows, err := s.dbPool.Query(ctx, `
		SELECT id, name, key_prefix, request_count, status
		FROM api_keys
		WHERE request_count > 0
		ORDER BY request_count DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("get top keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v APIKeyVolume

		var id pgtype.UUID
		if err := rows.Scan(&id, &v.Name, &v.KeyPrefix, &v.RequestCount, &v.Status); err != nil {
			return nil, fmt.Errorf("scan top key: %w", err)
		}

		v.ID = id.String()
		analytics.TopKeys = append(analytics.TopKeys, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Zero usage keys.
	rows2, err := s.dbPool.Query(ctx, `
		SELECT id, name, key_prefix, request_count, status
		FROM api_keys
		WHERE request_count = 0 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("get zero usage keys: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var v APIKeyVolume

		var id pgtype.UUID
		if err := rows2.Scan(&id, &v.Name, &v.KeyPrefix, &v.RequestCount, &v.Status); err != nil {
			return nil, fmt.Errorf("scan zero usage key: %w", err)
		}

		v.ID = id.String()
		analytics.ZeroUsageKeys = append(analytics.ZeroUsageKeys, v)
	}

	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// Recent activity (last 7 days).
	rows3, err := s.dbPool.Query(ctx, `
		SELECT key_prefix, last_used_at, request_count
		FROM api_keys
		WHERE last_used_at IS NOT NULL
		  AND last_used_at > now() - interval '7 days'
		ORDER BY last_used_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, fmt.Errorf("get recent activity: %w", err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var a APIKeyActivity

		var lastUsed pgtype.Timestamptz
		if err := rows3.Scan(&a.KeyPrefix, &lastUsed, &a.RequestCount); err != nil {
			return nil, fmt.Errorf("scan recent activity: %w", err)
		}

		if lastUsed.Valid {
			a.LastUsedAt = lastUsed.Time.Format(time.RFC3339)
		}

		analytics.RecentActivity = append(analytics.RecentActivity, a)
	}

	if err := rows3.Err(); err != nil {
		return nil, err
	}

	return analytics, nil
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
	store          *Store
	keyCountReader keyCountSettingsReader
	tierResolver   TierResolver
	moat           *MoatReaders
	cache          valkeycompat.Cmdable
	settingsReader rateLimitSettingsReader
	// Phase 2 agent-data readers (feeds, scam alerts, semantic search) and
	// the sqlc queries used for tag-feed cursors and search hit resolution.
	feedReader       FeedReader
	safetyReader     SafetyReader
	semanticSearcher SemanticSearcher
	intelReader      IntelReader
	queries          *gen.Queries
	adminChecker     adminChecker
}

// NewService creates a Service with the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// SetKeyCountReader injects a settings reader for the api.keys.max_per_user
// system setting (Rule 3 — no hardcoded limits when a settings reader is
// available).
func (s *Service) SetKeyCountReader(r keyCountSettingsReader) {
	s.keyCountReader = r
}

// SetTierResolver injects the unified tier resolver so the rate limiter can
// cap an API key's rate limit tier by the key owner's user tier.
func (s *Service) SetTierResolver(r TierResolver) {
	s.tierResolver = r
}

// ResolveUserTier returns the key owner's effective unified tier, falling back
// to "regular" when no tier resolver is wired.
func (s *Service) ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error) {
	if s.tierResolver == nil {
		return "regular", nil
	}

	return s.tierResolver.ResolveUserTier(ctx, userID)
}

// APIKeyUpdateFields holds the optional fields for a partial API key update.
// Only non-zero/non-empty fields are written; the rest are left untouched.
// Scopes is a pointer so callers can distinguish "not provided" (nil) from an
// explicit scope list — the control-panel PATCH is the only path allowed to
// change scopes (PUB-001 keeps them out of self-service updates).
type APIKeyUpdateFields struct {
	Name          string
	Status        string
	RateLimitTier string
	Notes         string
	Scopes        *[]string
}

// ErrNoFieldsToUpdate is returned by UpdateAPIKeyFields when no updatable
// fields were provided. Callers should map this to a 400/422 response.
var ErrNoFieldsToUpdate = errors.New("no fields to update")

// apiKeyUpdateColumns is the strict allowlist mapping field name → column name.
// Only columns in this map can ever be interpolated into the UPDATE statement,
// eliminating any SQL-injection surface even if the caller is later refactored
// (SEC-002 / LAYER-003).
var apiKeyUpdateColumns = map[string]string{
	"name":            "name",
	"status":          "status",
	"rate_limit_tier": "rate_limit_tier",
	"notes":           "notes",
	"scopes":          "scopes",
}

// UpdateAPIKeyFields performs a partial update of an API key using a strict
// column allowlist. Returns (rowsAffected, error) so the caller can map a
// zero count to 404. The query is built from apiKeyUpdateColumns only — no
// user-supplied column names ever reach the SQL string.
// Platform-scoped to prevent cross-platform modification (PUB-004).
func (s *Service) UpdateAPIKeyFields(ctx context.Context, keyID pgtype.UUID, fields APIKeyUpdateFields) (int64, error) {
	provided := map[string]any{
		"name":            fields.Name,
		"status":          fields.Status,
		"rate_limit_tier": fields.RateLimitTier,
		"notes":           fields.Notes,
	}

	setClauses := []string{"updated_at = now()"}
	args := []any{keyID}
	argIdx := 2 // $1 is keyID

	for field, val := range provided {
		s, ok := val.(string)
		if !ok || s == "" {
			continue
		}

		col, ok := apiKeyUpdateColumns[field]
		if !ok {
			continue // unknown field — allowlist drops it
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, s)
		argIdx++
	}

	// Scopes is handled outside the string loop: it is a text[] column and
	// a non-nil pointer means "replace the scope list" even when empty is
	// rejected upstream.
	if fields.Scopes != nil {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", apiKeyUpdateColumns["scopes"], argIdx))
		args = append(args, *fields.Scopes)
		argIdx++
	}

	// Only updated_at — no user fields provided.
	if len(setClauses) == 1 {
		return 0, ErrNoFieldsToUpdate
	}

	query := fmt.Sprintf("UPDATE api_keys SET %s WHERE id = $1", strings.Join(setClauses, ", "))

	tag, err := s.store.dbPool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("update api key fields: %w", err)
	}

	return tag.RowsAffected(), nil
}

// keyCountSettingsReader reads the api.keys.max_per_user system setting.
// *config.Settings satisfies this structurally. When nil, the compiled-in
// default of 5 is used (Section 47 line 218).
type keyCountSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// ErrScopeRequiresAdmin is returned by CreateAPIKey when a non-admin caller
// requests an admin-only scope (api.write.*, *). Mapped to 403
// scope_requires_admin at the handler.
var ErrScopeRequiresAdmin = errors.New("scope requires admin grant")

// adminChecker reports whether a user holds an RBAC permission node.
// *rbac.RBAC satisfies this structurally.
type adminChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// SetAdminChecker injects the RBAC resolver used to decide whether the
// caller may mint admin-only scopes (Phase 3 scope gating). When nil, all
// admin-only scope requests are rejected — fail closed.
func (s *Service) SetAdminChecker(c adminChecker) {
	s.adminChecker = c
}

// CallerCanManageKeys reports whether the user may administer API keys —
// the RBAC node that unlocks admin-only scopes and minting keys for other
// users. Fails closed when no checker is wired.
func (s *Service) CallerCanManageKeys(ctx context.Context, userID pgtype.UUID) bool {
	return s.adminChecker != nil && s.adminChecker.HasPermission(ctx, userID, "api.keys.manage")
}

// GetIntSetting reads a system setting through the wired settings reader
// (api.webhooks.max_per_key etc.). Returns the fallback when unwired.
func (s *Service) GetIntSetting(ctx context.Context, key string, fallback int) int {
	if s.settingsReader == nil {
		return fallback
	}

	return s.settingsReader.GetIntOr(ctx, key, fallback)
}

// maxKeysPerUser is the compiled-in fallback when no settings reader is wired.
const maxKeysPerUser = 5

// CreateAPIKey generates a new API key owned by userID, stores its hash,
// and returns the full key exactly once. The caller must save it — it
// cannot be retrieved later. Enforces the api.keys.max_per_user system
// setting (Section 47 line 218, PUB-006). Thin wrapper over
// CreateAPIKeyForUser.
func (s *Service) CreateAPIKey(ctx context.Context, userID pgtype.UUID, input CreateAPIKeyInput) (CreateAPIKeyResponse, error) {
	return s.CreateAPIKeyForUser(ctx, userID, userID, input)
}

// CreateAPIKeyForUser generates a key owned by ownerID on behalf of
// callerID. The scope gate and the admin checks run against the CALLER —
// minting admin-only scopes or minting for another user both require the
// caller to hold api.keys.manage. Key cap and rate-limit tier are resolved
// against the OWNER (a granted key is bounded by its owner's tier and key
// budget, not the admin's).
func (s *Service) CreateAPIKeyForUser(ctx context.Context, callerID, ownerID pgtype.UUID, input CreateAPIKeyInput) (CreateAPIKeyResponse, error) {
	callerIsAdmin := s.CallerCanManageKeys(ctx, callerID)

	// Phase 3 scope gating: admin-only scopes (api.write.*, *) can never be
	// self-minted. Fail closed when no checker is wired.
	if AnyAdminOnly(input.Scopes) && !callerIsAdmin {
		return CreateAPIKeyResponse{}, ErrScopeRequiresAdmin
	}
	// Minting for another user is itself an admin action.
	if ownerID != callerID && !callerIsAdmin {
		return CreateAPIKeyResponse{}, ErrScopeRequiresAdmin
	}

	// Enforce max keys per user (Section 47: api.keys.max_per_user, default 5).
	limit := maxKeysPerUser
	if s.keyCountReader != nil {
		limit = s.keyCountReader.GetIntOr(ctx, "api.keys.max_per_user", maxKeysPerUser)
	}

	var currentCount int

	err := s.store.dbPool.QueryRow(ctx, `
		SELECT count(*) FROM api_keys WHERE user_id = $1 AND status = 'active'
	`, ownerID).Scan(&currentCount)
	if err != nil {
		return CreateAPIKeyResponse{}, fmt.Errorf("count api keys: %w", err)
	}

	if currentCount >= limit {
		return CreateAPIKeyResponse{}, fmt.Errorf("%w: maximum %d keys per user", errpkg.ErrForbidden, limit)
	}

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

	// Cap the requested rate limit tier by the key owner's unified user tier.
	// regular -> basic, pro -> standard, business -> premium.
	tier = CapRateLimitTier(ctx, s.tierResolver, ownerID, tier)

	id, err := s.store.create(ctx, createParams{
		UserID:        ownerID,
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid or revoked API key")
		}

		return nil, fmt.Errorf("validate api key: %w", err)
	}

	// Check expiry.
	if row.ExpiresAt != nil && row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("API key has expired")
	}

	// Increment usage (fire-and-forget — don't fail the request on counter errors).
	// Per Section 47 lines 116–119: usage update runs in a goroutine with a
	// detached request context so it never blocks the request path (PUB-008).
	go func(ctx context.Context, keyID pgtype.UUID) {
		_ = s.store.incrementUsage(ctx, keyID)
	}(context.WithoutCancel(ctx), row.ID)

	return &APIKeyContext{
		KeyID:  row.ID,
		UserID: row.UserID,

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

// ListAllAPIKeys returns all API keys for the admin view.
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

// RevokeAPIKey soft-deletes an API key by setting status to revoked and
// recording the revocation audit trail (revoked_at + revoked_by).
func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID pgtype.UUID) error {
	// Verify the key belongs to the user.
	var ownerID pgtype.UUID

	err := s.store.dbPool.QueryRow(ctx, `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("api key not found")
		}

		return fmt.Errorf("revoke api key: %w", err)
	}

	if ownerID != userID {
		return errors.New("api key does not belong to user")
	}

	return s.store.revokeKey(ctx, keyID, userID)
}

// SuspendAPIKey sets an API key status to suspended (admin action).
func (s *Service) SuspendAPIKey(ctx context.Context, keyID pgtype.UUID) error {
	return s.store.setStatus(ctx, keyID, "suspended")
}

// UnsuspendAPIKey sets an API key status back to active.
func (s *Service) UnsuspendAPIKey(ctx context.Context, keyID pgtype.UUID) error {
	return s.store.setStatus(ctx, keyID, "active")
}

// DeleteAPIKey permanently deletes an API key (super_admin only).
// Platform-scoped to prevent cross-platform destruction (PUB-002).
func (s *Service) DeleteAPIKey(ctx context.Context, keyID pgtype.UUID) error {
	tag, err := s.store.dbPool.Exec(ctx, `
		DELETE FROM api_keys WHERE id = $1
	`, keyID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("api key not found")
	}

	return nil
}

// DeleteAPIKeys permanently deletes multiple API keys (super_admin only).
// Platform-scoped to prevent cross-platform destruction (PUB-002).
func (s *Service) DeleteAPIKeys(ctx context.Context, keyIDs []pgtype.UUID) error {
	if len(keyIDs) == 0 {
		return nil
	}

	// Build the query with ANY for efficient batch delete.
	_, err := s.store.dbPool.Exec(ctx, `
		DELETE FROM api_keys WHERE id = ANY($1)
	`, keyIDs)
	if err != nil {
		return fmt.Errorf("delete api keys: %w", err)
	}

	return nil
}

// GetAnalytics returns aggregate analytics for all API keys.
func (s *Service) GetAnalytics(ctx context.Context) (*APIKeyAnalytics, error) {
	return s.store.getAnalytics(ctx)
}

// ---------------------------------------------------------------------------
// IP Allowlist CRUD
// ---------------------------------------------------------------------------

// AddAllowedIP adds an IP or CIDR range to an API key's allowlist.
func (s *Service) AddAllowedIP(ctx context.Context, keyID pgtype.UUID, cidr, description string) error {
	if cidr == "" {
		return errors.New("cidr is required")
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
		return errors.New("allowlist entry not found")
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

// HasIPAccess checks if a given IP address is permitted by the key's allowlist.
// Returns true if the allowlist is empty (no restriction) or if the IP matches.
func (s *Service) HasIPAccess(ctx context.Context, keyID pgtype.UUID, clientIP string) (bool, error) {
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

// HasIPRestrictions reports whether the key has any IP allowlist rows.
// Used by the middleware to fail closed on unidentifiable client IPs:
// unrestricted keys pass, restricted keys deny.
func (s *Service) HasIPRestrictions(ctx context.Context, keyID pgtype.UUID) (bool, error) {
	var count int

	err := s.store.dbPool.QueryRow(ctx, `
		SELECT count(*) FROM api_key_ip_allowlist WHERE api_key_id = $1
	`, keyID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check IP restrictions: %w", err)
	}

	return count > 0, nil
}

// ---------------------------------------------------------------------------
// Self-service API key methods (P5.6 — /me/api-keys/{id})
// ---------------------------------------------------------------------------

// GetOwnAPIKey returns a single API key owned by the given user.
// Returns pgx.ErrNoRows if the key does not exist or does not belong to the
// user (PUB-013 — caller maps via HandleServiceError).
func (s *Service) GetOwnAPIKey(ctx context.Context, userID, keyID pgtype.UUID) (APIKey, error) {
	var key APIKey

	err := s.store.dbPool.QueryRow(ctx,
		`SELECT id, name, key_prefix, scopes, status, created_at, last_used_at
		 FROM api_keys WHERE id = $1 AND user_id = $2`,
		keyID, userID).
		Scan(&key.ID, &key.Name, &key.KeyPrefix, &key.Scopes, &key.Status, &key.CreatedAt, &key.LastUsedAt)
	if err != nil {
		return APIKey{}, err
	}

	return key, nil
}

// UpdateOwnAPIKeyName updates the display name of a user's own API key.
// Returns (rowsAffected, error) so the caller can map zero to 404.
// Scope changes are NOT allowed via self-service (PUB-001 — admin approval
// required per Section 47 line 182).
func (s *Service) UpdateOwnAPIKeyName(ctx context.Context, userID, keyID pgtype.UUID, name string) (int64, error) {
	tag, err := s.store.dbPool.Exec(ctx,
		`UPDATE api_keys SET name = $3, updated_at = now()
		 WHERE id = $1 AND user_id = $2`, keyID, userID, name)
	if err != nil {
		return 0, fmt.Errorf("update own api key: %w", err)
	}

	return tag.RowsAffected(), nil
}

// ---------------------------------------------------------------------------
// IP Allowlist — user-scoped wrappers (ownership check in service layer)
// ---------------------------------------------------------------------------

// ListAllowedIPsForUser lists IP allowlist entries after verifying the key
// belongs to the user (PUB-011 — ownership check moved out of handler).
func (s *Service) ListAllowedIPsForUser(ctx context.Context, userID, keyID pgtype.UUID) ([]APIKeyIPAllowlist, error) {
	if err := s.verifyKeyOwnership(ctx, userID, keyID); err != nil {
		return nil, err
	}

	return s.ListAllowedIPs(ctx, keyID)
}

// AddAllowedIPForUser adds an IP allowlist entry after verifying ownership.
func (s *Service) AddAllowedIPForUser(ctx context.Context, userID, keyID pgtype.UUID, cidr, description string) error {
	if err := s.verifyKeyOwnership(ctx, userID, keyID); err != nil {
		return err
	}

	return s.AddAllowedIP(ctx, keyID, cidr, description)
}

// RemoveAllowedIPForUser removes an IP allowlist entry after verifying ownership.
func (s *Service) RemoveAllowedIPForUser(ctx context.Context, userID, keyID, ipID pgtype.UUID) error {
	if err := s.verifyKeyOwnership(ctx, userID, keyID); err != nil {
		return err
	}

	return s.RemoveAllowedIP(ctx, keyID, ipID)
}

// verifyKeyOwnership returns nil if the key belongs to the user, an error
// otherwise. Centralises the ownership check that was previously duplicated
// as raw SQL in every IP-allowlist handler (PUB-011).
func (s *Service) verifyKeyOwnership(ctx context.Context, userID, keyID pgtype.UUID) error {
	var ownerID pgtype.UUID

	err := s.store.dbPool.QueryRow(ctx, `
		SELECT user_id FROM api_keys WHERE id = $1
	`, keyID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("api key not found")
		}

		return fmt.Errorf("verify key ownership: %w", err)
	}

	if ownerID != userID {
		return fmt.Errorf("%w: api key does not belong to user", errpkg.ErrForbidden)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Search (PUB-012 — ILIKE wildcard escaping, moved from handler to service)
// ---------------------------------------------------------------------------

// SearchContent searches published content by title.
// ILIKE metacharacters (% and _) in the query are escaped to prevent
// wildcard injection (PUB-012).
func (s *Service) SearchContent(ctx context.Context, query string, limit, offset int32) ([]map[string]any, error) {
	// Escape ILIKE metacharacters so user input is treated as a literal
	// substring, not a wildcard pattern (PUB-012).
	escaped := strings.ReplaceAll(query, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "%", "\\%")
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	pattern := "%" + escaped + "%"

	rows, err := s.store.dbPool.Query(ctx, `
		SELECT id, title, slug, content_type, status, created_at
		FROM contents
		WHERE status = 'published' AND visibility = 'public' AND deleted_at IS NULL AND title ILIKE $1 ESCAPE '\'
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search content: %w", err)
	}
	defer rows.Close()

	var results []map[string]any

	for rows.Next() {
		var id pgtype.UUID

		var title, slug, contentType, status string

		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &title, &slug, &contentType, &status, &createdAt); err != nil {
			return nil, fmt.Errorf("search content scan: %w", err)
		}

		results = append(results, map[string]any{
			"id":           id.String(),
			"title":        title,
			"slug":         slug,
			"content_type": contentType,
			"status":       status,
			"created_at":   createdAt,
		})
	}

	if results == nil {
		results = []map[string]any{}
	}

	return results, rows.Err()
}

// ListPublishedContent returns a paginated list of published, public content.
// Only status='published' AND visibility='public' rows are
// returned — the public API must never expose drafts, scheduled, or
// private/member-only content (Section 47, api.read.content scope).
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (s *Service) ListPublishedContent(ctx context.Context, limit, offset int32) ([]map[string]any, error) {
	rows, err := s.store.dbPool.Query(ctx, `
		SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
		       c.view_count, c.comment_count, c.read_time_minutes,
		       c.published_at, c.created_at,
		       u.username AS author_username,
		       s.name AS space_name
		FROM contents c
		LEFT JOIN users u ON u.id = c.author_id
		LEFT JOIN spaces s ON s.id = c.space_id
		WHERE c.status = 'published'
		  AND c.visibility = 'public'
		  AND c.deleted_at IS NULL
		  AND c.auto_hidden = false
		ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list published content: %w", err)
	}
	defer rows.Close()

	var results []map[string]any

	for rows.Next() {
		var id, authorID, spaceID pgtype.UUID

		var title, slug, contentType string

		var viewCount, commentCount, readTime int32

		var publishedAt, createdAt pgtype.Timestamptz

		var authorUsername, spaceName *string
		if err := rows.Scan(&id, &title, &slug, &contentType, &authorID, &spaceID,
			&viewCount, &commentCount, &readTime,
			&publishedAt, &createdAt,
			&authorUsername, &spaceName); err != nil {
			return nil, fmt.Errorf("list published content scan: %w", err)
		}

		item := map[string]any{
			"id":            id.String(),
			"title":         title,
			"slug":          slug,
			"content_type":  contentType,
			"author_id":     authorID.String(),
			"space_id":      spaceID.String(),
			"view_count":    viewCount,
			"comment_count": commentCount,
			"read_time":     readTime,
			"published_at":  publishedAt,
			"created_at":    createdAt,
		}
		if authorUsername != nil {
			item["author_username"] = *authorUsername
		}

		if spaceName != nil {
			item["space_name"] = *spaceName
		}

		results = append(results, item)
	}

	if results == nil {
		results = []map[string]any{}
	}

	return results, rows.Err()
}

// RotateAPIKey generates a new API key with the same name, scopes, and
// rate_limit_tier as the old key, sets the old key's expires_at to now+24h
// (grace period), and returns the new key with the full secret (shown once).
// Enforces the api.keys.max_per_user system setting (BUG-007 fix: rotation
// previously bypassed the limit check, allowing N+1 active keys).
func (s *Service) RotateAPIKey(ctx context.Context, userID, keyID pgtype.UUID) (*CreateAPIKeyResponse, error) {
	// Verify the key exists and belongs to the user.
	var row apiKeyRow

	err := s.store.dbPool.QueryRow(ctx, `
		SELECT id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
		FROM api_keys
		WHERE id = $1
	`, keyID).Scan(&row.ID, &row.UserID, &row.Name, &row.KeyPrefix,
		&row.Scopes, &row.RateLimitTier, &row.Status, &row.LastUsedAt,
		&row.RequestCount, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("api key not found")
		}

		return nil, fmt.Errorf("rotate api key: %w", err)
	}

	if row.UserID != userID {
		return nil, errors.New("api key does not belong to user")
	}

	// Enforce max keys per user (Section 47: api.keys.max_per_user, default 5).
	// Count active keys excluding the one being rotated (it will be expired,
	// not revoked, during the 24h grace period — so it still counts as active
	// until the grace expiry). The new key adds 1 to the active count.
	limit := maxKeysPerUser
	if s.keyCountReader != nil {
		limit = s.keyCountReader.GetIntOr(ctx, "api.keys.max_per_user", maxKeysPerUser)
	}

	var currentCount int

	err = s.store.dbPool.QueryRow(ctx, `
		SELECT count(*) FROM api_keys
		WHERE user_id = $1 AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > now())
	`, userID).Scan(&currentCount)
	if err != nil {
		return nil, fmt.Errorf("rotate api key: count active keys: %w", err)
	}
	// The old key is still active (will be expired in the tx below), and the
	// new key adds 1. So the post-rotation active count is currentCount + 1.
	// We allow rotation if currentCount + 1 <= limit, i.e., currentCount < limit.
	if currentCount >= limit {
		return nil, fmt.Errorf("%w: maximum %d keys per user", errpkg.ErrForbidden, limit)
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

	defer func() { _ = tx.Rollback(ctx) }()

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
		INSERT INTO api_keys (user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, userID, row.Name, keyPrefix, keyHash,
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
