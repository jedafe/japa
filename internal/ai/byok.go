// Package ai — byok.go implements BYOK (bring-your-own-key): an
// admin-curated allowlist of provider types (ai_byok_providers) plus
// encrypted per-user credentials (user_ai_keys). A BYOK turn resolves the
// caller's key into an ephemeral Provider and dispatches to it directly —
// never through the platform Dispatcher, so a rejected user key can never
// silently fall back to platform keys or burn platform quota.
//
// Ref: docs/superpowers/specs/2026-10-19-ai-byok-design.md
package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/crypto"
)

// ErrByokKeyInUse is returned when the submitted key is already attached to
// a different account — the same credential appearing on multiple Japa
// accounts is a strong stolen-key laundering signal.
var ErrByokKeyInUse = errors.New("ai: byok key already in use on another account")

// byokKeyHash fingerprints a submitted key for the cross-account dedup
// check. sha256 over the trimmed plaintext — high-entropy input, no salt.
func byokKeyHash(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// BYOK sentinels — mapped to HTTP codes by byokErrorStatus (byok_handlers.go)
// and writeAgentError (agent_handlers.go).
var (
	ErrByokProviderNotFound = errors.New("ai: byok provider not found")
	ErrByokProviderDisabled = errors.New("ai: byok provider is disabled")
	ErrAIKeyNotFound        = errors.New("ai: user ai key not found")
	ErrByokModelNotAllowed  = errors.New("ai: model is not in the byok provider catalog")
)

// SupportedByokProviderTypes returns the provider_type values users may
// attach keys for — the subset of SupportedProviderTypes whose auth and
// endpoint shapes work without an admin-configured base URL. Keep in sync
// with the ai_byok_providers CHECK constraint (migration 000890).
func SupportedByokProviderTypes() []string {
	return []string{"openai", "anthropic", "groq", "perplexity"}
}

// ByokProvider is one row of the admin-curated BYOK allowlist: the
// "storefront" entry describing which provider type a user may attach a
// key for, which models that key unlocks, and the affiliate signup link.
type ByokProvider struct {
	ID             pgtype.UUID `json:"id"`
	ProviderType   string      `json:"provider_type"`
	DisplayName    string      `json:"display_name"`
	Models         []ModelInfo `json:"models"`
	SignupURL      string      `json:"signup_url"`
	SignupLabel    string      `json:"signup_label"`
	Enabled        bool        `json:"enabled"`
	ReferralClicks int         `json:"referral_clicks"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// HasModel reports whether model is in the provider's curated catalog.
func (p ByokProvider) HasModel(model string) bool {
	if model == "" {
		return false
	}

	for _, m := range p.Models {
		if m.ID == model {
			return true
		}
	}

	return false
}

// ByokProviderStore provides DB access for the ai_byok_providers table.
type ByokProviderStore struct {
	read  *pgxpool.Pool
	write *pgxpool.Pool
}

// NewByokProviderStore creates a ByokProviderStore backed by the given pools.
func NewByokProviderStore(read, write *pgxpool.Pool) *ByokProviderStore {
	return &ByokProviderStore{read: read, write: write}
}

const byokProviderCols = `id, provider_type, display_name, signup_url, signup_label, models, enabled, referral_clicks, created_at, updated_at`

// scanByokProvider scans a row into a ByokProvider, decoding the models
// jsonb catalog.
func scanByokProvider(row interface {
	Scan(dest ...any) error
},
) (ByokProvider, error) {
	var bp ByokProvider

	var modelsJSON []byte

	if err := row.Scan(
		&bp.ID, &bp.ProviderType, &bp.DisplayName, &bp.SignupURL, &bp.SignupLabel,
		&modelsJSON, &bp.Enabled, &bp.ReferralClicks, &bp.CreatedAt, &bp.UpdatedAt,
	); err != nil {
		return ByokProvider{}, fmt.Errorf("scanning ai_byok_provider: %w", err)
	}

	if len(modelsJSON) > 0 {
		_ = json.Unmarshal(modelsJSON, &bp.Models)
	}

	if bp.Models == nil {
		bp.Models = []ModelInfo{}
	}

	return bp, nil
}

// List returns all BYOK provider rows ordered by display_name.
func (s *ByokProviderStore) List(ctx context.Context) ([]ByokProvider, error) {
	rows, err := s.read.Query(ctx, `
		SELECT `+byokProviderCols+`
		FROM ai_byok_providers
		ORDER BY display_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing ai_byok_providers: %w", err)
	}
	defer rows.Close()

	var out []ByokProvider

	for rows.Next() {
		bp, err := scanByokProvider(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, bp)
	}

	return out, rows.Err()
}

// ListEnabled returns enabled BYOK providers only — the user-facing
// storefront list.
func (s *ByokProviderStore) ListEnabled(ctx context.Context) ([]ByokProvider, error) {
	rows, err := s.read.Query(ctx, `
		SELECT `+byokProviderCols+`
		FROM ai_byok_providers
		WHERE enabled = true
		ORDER BY display_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing enabled ai_byok_providers: %w", err)
	}
	defer rows.Close()

	var out []ByokProvider

	for rows.Next() {
		bp, err := scanByokProvider(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, bp)
	}

	return out, rows.Err()
}

// Get returns a single BYOK provider by ID.
func (s *ByokProviderStore) Get(ctx context.Context, id pgtype.UUID) (ByokProvider, error) {
	row := s.read.QueryRow(ctx, `
		SELECT `+byokProviderCols+`
		FROM ai_byok_providers
		WHERE id = $1
	`, id)

	bp, err := scanByokProvider(row)
	if err != nil {
		return ByokProvider{}, ErrByokProviderNotFound
	}

	return bp, nil
}

// CreateByokProviderInput is the payload for creating a BYOK provider row.
type CreateByokProviderInput struct {
	ProviderType string
	DisplayName  string
	Models       []ModelInfo
	SignupURL    string
	SignupLabel  string
	Enabled      bool
}

// Create inserts a BYOK provider row and returns it.
func (s *ByokProviderStore) Create(ctx context.Context, input CreateByokProviderInput) (ByokProvider, error) {
	modelsJSON, _ := json.Marshal(input.Models)
	if input.Models == nil {
		modelsJSON = []byte("[]")
	}

	row := s.write.QueryRow(ctx, `
		INSERT INTO ai_byok_providers
			(provider_type, display_name, models, signup_url, signup_label, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+byokProviderCols+`
	`, input.ProviderType, input.DisplayName, modelsJSON, input.SignupURL, input.SignupLabel, input.Enabled)

	return scanByokProvider(row)
}

// UpdateByokProviderInput carries optional fields — nil means preserve.
type UpdateByokProviderInput struct {
	ID           pgtype.UUID
	ProviderType *string
	DisplayName  *string
	Models       *[]ModelInfo
	SignupURL    *string
	SignupLabel  *string
	Enabled      *bool
}

// Update updates a BYOK provider row and returns it.
func (s *ByokProviderStore) Update(ctx context.Context, input UpdateByokProviderInput) (ByokProvider, error) {
	setParts := []string{"updated_at = now()"}
	args := []any{input.ID}
	argIdx := 2

	if input.ProviderType != nil {
		setParts = append(setParts, fmt.Sprintf("provider_type = $%d", argIdx))
		args = append(args, *input.ProviderType)
		argIdx++
	}

	if input.DisplayName != nil {
		setParts = append(setParts, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, *input.DisplayName)
		argIdx++
	}

	if input.Models != nil {
		modelsJSON, _ := json.Marshal(*input.Models)

		setParts = append(setParts, fmt.Sprintf("models = $%d::jsonb", argIdx))
		args = append(args, modelsJSON)
		argIdx++
	}

	if input.SignupURL != nil {
		setParts = append(setParts, fmt.Sprintf("signup_url = $%d", argIdx))
		args = append(args, *input.SignupURL)
		argIdx++
	}

	if input.SignupLabel != nil {
		setParts = append(setParts, fmt.Sprintf("signup_label = $%d", argIdx))
		args = append(args, *input.SignupLabel)
		argIdx++
	}

	if input.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *input.Enabled)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE ai_byok_providers SET %s
		WHERE id = $1
		RETURNING `+byokProviderCols+`
	`, strings.Join(setParts, ", "))

	row := s.write.QueryRow(ctx, query, args...)

	bp, err := scanByokProvider(row)
	if err != nil {
		return ByokProvider{}, ErrByokProviderNotFound
	}

	return bp, nil
}

// Delete removes a BYOK provider row. user_ai_keys referencing it cascade.
func (s *ByokProviderStore) Delete(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `DELETE FROM ai_byok_providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting ai_byok_provider: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrByokProviderNotFound
	}

	return nil
}

// IncrementReferralClicks bumps the affiliate click counter for a provider.
func (s *ByokProviderStore) IncrementReferralClicks(ctx context.Context, id pgtype.UUID) error {
	_, err := s.write.Exec(ctx, `
		UPDATE ai_byok_providers SET referral_clicks = referral_clicks + 1 WHERE id = $1
	`, id)

	return err
}

// ---------------------------------------------------------------------------
// User keys
// ---------------------------------------------------------------------------

// UserAIKey is one user-owned credential row joined with its BYOK provider
// allowlist entry. api_key_masked is derived — plaintext never leaves the
// backend.
type UserAIKey struct {
	ID             pgtype.UUID `json:"id"`
	UserID         pgtype.UUID `json:"-"`
	ByokProviderID pgtype.UUID `json:"byok_provider_id"`
	Label          string      `json:"label"`
	APIKeyMasked   string      `json:"api_key_masked"`
	Status         string      `json:"status"`
	LastError      string      `json:"last_error,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`

	Provider ByokProvider `json:"provider"`
}

// UserAIKeyStore provides DB access for the user_ai_keys table.
type UserAIKeyStore struct {
	read  *pgxpool.Pool
	write *pgxpool.Pool
}

// NewUserAIKeyStore creates a UserAIKeyStore backed by the given pools.
func NewUserAIKeyStore(read, write *pgxpool.Pool) *UserAIKeyStore {
	return &UserAIKeyStore{read: read, write: write}
}

const userAIKeyCols = `k.id, k.user_id, k.byok_provider_id, k.label, k.api_key_encrypted,
	k.status, k.last_error, k.created_at, k.updated_at,
	p.id, p.provider_type, p.display_name, p.signup_url, p.signup_label, p.models, p.enabled, p.referral_clicks, p.created_at, p.updated_at`

const userAIKeyJoin = `FROM user_ai_keys k JOIN ai_byok_providers p ON p.id = k.byok_provider_id`

// scanUserAIKey scans a joined user_ai_keys + ai_byok_providers row.
func scanUserAIKey(row interface {
	Scan(dest ...any) error
},
) (UserAIKey, error) {
	var k UserAIKey

	var encryptedKey string

	var providerID pgtype.UUID

	var modelsJSON []byte

	var pCreatedAt, pUpdatedAt time.Time

	if err := row.Scan(
		&k.ID, &k.UserID, &k.ByokProviderID, &k.Label, &encryptedKey,
		&k.Status, &k.LastError, &k.CreatedAt, &k.UpdatedAt,
		&providerID, &k.Provider.ProviderType, &k.Provider.DisplayName,
		&k.Provider.SignupURL, &k.Provider.SignupLabel, &modelsJSON, &k.Provider.Enabled,
		&k.Provider.ReferralClicks, &pCreatedAt, &pUpdatedAt,
	); err != nil {
		return UserAIKey{}, fmt.Errorf("scanning user_ai_key: %w", err)
	}

	k.Provider.ID = providerID
	k.Provider.CreatedAt = pCreatedAt
	k.Provider.UpdatedAt = pUpdatedAt
	k.Provider.Models = []ModelInfo{}

	if len(modelsJSON) > 0 {
		_ = json.Unmarshal(modelsJSON, &k.Provider.Models)
	}

	// Decrypt for masking (best-effort — a decrypt failure shows a
	// placeholder rather than breaking the list).
	if encryptedKey != "" {
		if plain, err := crypto.Decrypt(encryptedKey); err == nil {
			k.APIKeyMasked = maskAPIKey(plain)
		} else {
			k.APIKeyMasked = "****"
		}
	}

	return k, nil
}

// ListForUser returns the caller's keys, newest first, each joined with its
// BYOK provider row.
func (s *UserAIKeyStore) ListForUser(ctx context.Context, userID pgtype.UUID) ([]UserAIKey, error) {
	rows, err := s.read.Query(ctx, `
		SELECT `+userAIKeyCols+`
		`+userAIKeyJoin+`
		WHERE k.user_id = $1
		ORDER BY k.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user_ai_keys: %w", err)
	}
	defer rows.Close()

	var out []UserAIKey

	for rows.Next() {
		k, err := scanUserAIKey(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, k)
	}

	return out, rows.Err()
}

// GetForUser returns one key scoped to the caller — a foreign id is a 404,
// never a permission leak about the row's existence.
func (s *UserAIKeyStore) GetForUser(ctx context.Context, userID, keyID pgtype.UUID) (UserAIKey, error) {
	row := s.read.QueryRow(ctx, `
		SELECT `+userAIKeyCols+`
		`+userAIKeyJoin+`
		WHERE k.id = $1 AND k.user_id = $2
	`, keyID, userID)

	k, err := scanUserAIKey(row)
	if err != nil {
		return UserAIKey{}, ErrAIKeyNotFound
	}

	return k, nil
}

// Create inserts a key for the caller, encrypting the plaintext API key.
// The BYOK provider row must exist and be enabled.
func (s *UserAIKeyStore) Create(ctx context.Context, userID, providerID pgtype.UUID, apiKey, label string) (UserAIKey, error) {
	var enabled bool

	err := s.read.QueryRow(ctx, `SELECT enabled FROM ai_byok_providers WHERE id = $1`, providerID).Scan(&enabled)
	if err != nil {
		return UserAIKey{}, ErrByokProviderNotFound
	}

	if !enabled {
		return UserAIKey{}, ErrByokProviderDisabled
	}

	encryptedKey, err := crypto.Encrypt(apiKey)
	if err != nil {
		return UserAIKey{}, fmt.Errorf("encrypting api key: %w", err)
	}

	// Cross-account dedup: the same credential on another user's row means
	// the key is shared or stolen — refuse to attach it.
	keyHash := byokKeyHash(apiKey)

	var other bool

	if err := s.read.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_ai_keys WHERE api_key_hash = $1 AND user_id <> $2)`,
		keyHash, userID).Scan(&other); err == nil && other {
		return UserAIKey{}, ErrByokKeyInUse
	}

	var id pgtype.UUID

	err = s.write.QueryRow(ctx, `
		INSERT INTO user_ai_keys
			(user_id, byok_provider_id, label, api_key_encrypted, api_key_hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, providerID, label, encryptedKey, keyHash).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "key_hash") {
			return UserAIKey{}, ErrByokKeyInUse
		}

		return UserAIKey{}, fmt.Errorf("inserting user_ai_key: %w", err)
	}

	s.auditByok(ctx, "ai.byok.key_created", userID, id,
		fmt.Sprintf(`{"byok_provider_id":%q}`, providerID.String()))

	return s.GetForUser(ctx, userID, id)
}

// Update renames the label and/or rotates the key. Rotating resets
// status/last_error so a fixed key is no longer flagged invalid.
func (s *UserAIKeyStore) Update(ctx context.Context, userID, keyID pgtype.UUID, label, apiKey *string) (UserAIKey, error) {
	setParts := []string{"updated_at = now()"}
	args := []any{keyID, userID}
	argIdx := 3

	if label != nil {
		setParts = append(setParts, fmt.Sprintf("label = $%d", argIdx))
		args = append(args, *label)
		argIdx++
	}

	if apiKey != nil && *apiKey != "" {
		keyHash := byokKeyHash(*apiKey)

		var other bool

		if err := s.read.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM user_ai_keys WHERE api_key_hash = $1 AND user_id <> $2 AND id <> $3)`,
			keyHash, userID, keyID).Scan(&other); err == nil && other {
			return UserAIKey{}, ErrByokKeyInUse
		}

		encrypted, err := crypto.Encrypt(*apiKey)
		if err != nil {
			return UserAIKey{}, fmt.Errorf("encrypting api key: %w", err)
		}

		setParts = append(setParts,
			fmt.Sprintf("api_key_encrypted = $%d", argIdx),
			fmt.Sprintf("api_key_hash = $%d", argIdx+1),
			"status = 'active'", "last_error = ''")
		args = append(args, encrypted, keyHash)
		argIdx += 2
	}

	query := fmt.Sprintf(`
		UPDATE user_ai_keys SET %s
		WHERE id = $1 AND user_id = $2
	`, strings.Join(setParts, ", "))

	tag, err := s.write.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "key_hash") {
			return UserAIKey{}, ErrByokKeyInUse
		}

		return UserAIKey{}, fmt.Errorf("updating user_ai_key: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return UserAIKey{}, ErrAIKeyNotFound
	}

	return s.GetForUser(ctx, userID, keyID)
}

// Delete removes the caller's key.
func (s *UserAIKeyStore) Delete(ctx context.Context, userID, keyID pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `
		DELETE FROM user_ai_keys WHERE id = $1 AND user_id = $2
	`, keyID, userID)
	if err != nil {
		return fmt.Errorf("deleting user_ai_key: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrAIKeyNotFound
	}

	s.auditByok(ctx, "ai.byok.key_deleted", userID, keyID, "{}")

	return nil
}

// MarkStatus sets the key's status flag and last_error. Called by the
// verify endpoint and by the agent loop when the provider rejects the key
// with a 401 (credential rejection).
func (s *UserAIKeyStore) MarkStatus(ctx context.Context, keyID pgtype.UUID, status, lastError string) error {
	// Provider error bodies can be large or echo request details — cap what
	// we persist so last_error stays a diagnostic, not a data sink.
	const maxErrLen = 512
	if len(lastError) > maxErrLen {
		lastError = lastError[:maxErrLen]
	}

	_, err := s.write.Exec(ctx, `
		UPDATE user_ai_keys SET status = $2, last_error = $3, updated_at = now()
		WHERE id = $1
	`, keyID, status, lastError)

	return err
}

// auditByok writes an audit_logs row for BYOK credential lifecycle events.
// Failures are logged and swallowed — audit must not break the operation.
func (s *UserAIKeyStore) auditByok(ctx context.Context, action string, userID, keyID pgtype.UUID, meta string) {
	if _, err := s.write.Exec(ctx, `
		INSERT INTO audit_logs (action_type, actor_user_id, target_type, target_id, metadata)
		VALUES ($1, $2, 'user_ai_key', $3, $4::jsonb)
	`, action, userID, keyID, meta); err != nil {
		slog.Warn("ai: byok audit write failed", "action", action, "error", err)
	}
}

// decryptedKey returns the plaintext API key for internal provider
// construction — never crosses the HTTP boundary.
func (s *UserAIKeyStore) decryptedKey(ctx context.Context, keyID pgtype.UUID) (string, error) {
	var encrypted, hash string

	err := s.read.QueryRow(ctx, `
		SELECT api_key_encrypted, api_key_hash FROM user_ai_keys WHERE id = $1
	`, keyID).Scan(&encrypted, &hash)
	if err != nil {
		return "", fmt.Errorf("getting api key: %w", err)
	}

	plaintext, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("ai: decrypt user api key: %w", err)
	}

	// Lazy backfill: rows predating api_key_hash get hashed on first use so
	// the dedup index covers them without a migration-time decrypt (the
	// encryption key lives in app env, not in SQL).
	if hash == "" {
		if _, uerr := s.write.Exec(ctx,
			`UPDATE user_ai_keys SET api_key_hash = $1 WHERE id = $2 AND api_key_hash = ''`,
			byokKeyHash(plaintext), keyID); uerr != nil {
			slog.Warn("ai: byok key-hash backfill failed", "key_id", keyID, "error", uerr)
		}
	}

	return plaintext, nil
}

// ResolveProvider loads the caller's key, checks the BYOK provider is still
// enabled and the model is in its catalog, decrypts, and constructs an
// ephemeral Provider. It never touches the platform Dispatcher — a BYOK
// turn can never fall back to platform keys. A key flagged 'invalid' still
// resolves: status is advisory (the user may have rotated the underlying
// provider-side key out-of-band); dispatch failures re-mark it.
func (s *UserAIKeyStore) ResolveProvider(ctx context.Context, userID, keyID pgtype.UUID, model string) (Provider, error) {
	k, err := s.GetForUser(ctx, userID, keyID)
	if err != nil {
		return nil, ErrAIKeyNotFound
	}

	if !k.Provider.Enabled {
		return nil, ErrByokProviderDisabled
	}

	if model != "" && !k.Provider.HasModel(model) {
		return nil, ErrByokModelNotAllowed
	}

	plain, err := s.decryptedKey(ctx, keyID)
	if err != nil {
		return nil, err
	}

	return NewProviderFromConfig(ProviderConfig{
		Name:         k.Provider.DisplayName,
		ProviderType: k.Provider.ProviderType,
		Model:        model,
	}, plain)
}
