// Package email — providers.go provides DB-backed email provider
// configuration management for the Control Panel (ADR-015). Provider
// configs (name, type, encrypted secret, base URL, non-secret config,
// categories, priority, enabled flag) are stored in the email_providers
// table and loaded at startup to register with the EmailDispatcher.
// Super-admins can CRUD providers at runtime via /control/email-providers;
// changes hot-reload the dispatcher. Secrets are encrypted at rest via
// internal/crypto — the payment-secret pattern required by ADR-015.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/crypto"
)

// ProviderConfig is the DB-backed representation of an email provider. The
// secret is stored encrypted (api_key_encrypted) and never serialized —
// the JSON response uses api_key_masked instead. Config holds non-secret
// per-type settings (from_address, region, smtp host/port/username,
// postmark message_stream, sendpulse client_id).
type ProviderConfig struct {
	ID pgtype.UUID `json:"id"`

	Name            string            `json:"name"`
	ProviderType    string            `json:"provider_type"`
	APIKeyEncrypted string            `json:"-"`              // never serialized to JSON
	APIKeyMasked    string            `json:"api_key_masked"` // derived, read-only
	BaseURL         string            `json:"base_url"`
	Config          map[string]string `json:"config"`
	Categories      []string          `json:"categories"`
	Priority        int               `json:"priority"`
	Enabled         bool              `json:"enabled"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// ProviderStore provides DB access for the email_providers table.
type ProviderStore struct {
	read  *pgxpool.Pool
	write *pgxpool.Pool
}

// NewProviderStore creates a ProviderStore backed by the given pools.
func NewProviderStore(read, write *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{read: read, write: write}
}

// providerColumns is the shared SELECT/RETURNING column list.
const providerColumns = `id, name, provider_type, api_key_encrypted,
		base_url, config, categories, priority, enabled, created_at, updated_at`

// maskSecret returns a masked representation of a secret for display.
// Secrets of ≤12 chars are fully masked — first4+last4 would reveal 8 of
// their ≤12 characters, i.e. nearly the entire secret.
func maskSecret(key string) string {
	if key == "" {
		return ""
	}

	if len(key) <= 12 {
		return "****"
	}

	return key[:4] + "****" + key[len(key)-4:]
}

// scanProviderConfig scans a row into a ProviderConfig, deriving the masked
// secret from the encrypted value (decrypted first, then masked).
func (s *ProviderStore) scanProviderConfig(row interface {
	Scan(dest ...any) error
},
) (ProviderConfig, error) {
	var pc ProviderConfig

	var configJSON, categoriesJSON []byte

	var encryptedKey string

	if err := row.Scan(
		&pc.ID, &pc.Name, &pc.ProviderType,
		&encryptedKey, &pc.BaseURL, &configJSON, &categoriesJSON,
		&pc.Priority, &pc.Enabled, &pc.CreatedAt, &pc.UpdatedAt,
	); err != nil {
		return ProviderConfig{}, fmt.Errorf("scanning email_provider: %w", err)
	}

	pc.APIKeyEncrypted = encryptedKey

	// Decrypt the secret for masking (best-effort — if decryption fails,
	// show a placeholder so the UI still works).
	if encryptedKey != "" {
		if plain, err := crypto.Decrypt(encryptedKey); err == nil {
			pc.APIKeyMasked = maskSecret(plain)
		} else {
			pc.APIKeyMasked = "****"
		}
	}

	if len(configJSON) > 0 {
		_ = json.Unmarshal(configJSON, &pc.Config)
	}

	if pc.Config == nil {
		pc.Config = map[string]string{}
	}

	if len(categoriesJSON) > 0 {
		_ = json.Unmarshal(categoriesJSON, &pc.Categories)
	}

	if pc.Categories == nil {
		pc.Categories = []string{}
	}

	return pc, nil
}

// ListProviders returns all email provider configs, ordered by priority
// ascending then name.
func (s *ProviderStore) ListProviders(ctx context.Context) ([]ProviderConfig, error) {
	return s.listProviders(ctx, s.read)
}

// ListProvidersForUpdate reads via the write pool so callers operating
// immediately after a mutation (hot-reload) observe their own writes even
// when the Reader pool is pointed at a lagging replica.
func (s *ProviderStore) ListProvidersForUpdate(ctx context.Context) ([]ProviderConfig, error) {
	return s.listProviders(ctx, s.write)
}

func (s *ProviderStore) listProviders(ctx context.Context, pool *pgxpool.Pool) ([]ProviderConfig, error) {
	rows, err := pool.Query(ctx, `
		SELECT `+providerColumns+`
		FROM email_providers
		ORDER BY priority ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing email_providers: %w", err)
	}
	defer rows.Close()

	var providers []ProviderConfig

	for rows.Next() {
		pc, err := s.scanProviderConfig(rows)
		if err != nil {
			return nil, err
		}

		providers = append(providers, pc)
	}

	return providers, rows.Err()
}

// GetProvider returns a single email provider config by ID.
func (s *ProviderStore) GetProvider(ctx context.Context, id pgtype.UUID) (ProviderConfig, error) {
	return s.getProvider(ctx, s.read, id)
}

func (s *ProviderStore) getProvider(ctx context.Context, pool *pgxpool.Pool, id pgtype.UUID) (ProviderConfig, error) {
	row := pool.QueryRow(ctx, `
		SELECT `+providerColumns+`
		FROM email_providers
		WHERE id = $1
	`, id)

	return s.scanProviderConfig(row)
}

// CreateProviderInput is the payload for creating a new provider config.
type CreateProviderInput struct {
	Name         string
	ProviderType string
	Secret       string // plaintext — encrypted before storage
	BaseURL      string
	Config       map[string]string
	Categories   []string
	Priority     int
	Enabled      bool
}

// CreateProvider inserts a new provider config and returns it.
func (s *ProviderStore) CreateProvider(ctx context.Context, input CreateProviderInput) (ProviderConfig, error) {
	var encryptedKey string

	if input.Secret != "" {
		var err error
		if encryptedKey, err = crypto.Encrypt(input.Secret); err != nil {
			return ProviderConfig{}, fmt.Errorf("encrypting secret: %w", err)
		}
	}

	configJSON, _ := json.Marshal(input.Config)
	if input.Config == nil {
		configJSON = []byte("{}")
	}

	categoriesJSON, _ := json.Marshal(input.Categories)
	if input.Categories == nil {
		categoriesJSON = []byte("[]")
	}

	row := s.write.QueryRow(ctx, `
		INSERT INTO email_providers
			(name, provider_type, api_key_encrypted, base_url, config, categories, priority, enabled)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)
		RETURNING `+providerColumns+`
	`, input.Name, input.ProviderType, encryptedKey, input.BaseURL, configJSON, categoriesJSON, input.Priority, input.Enabled)

	return s.scanProviderConfig(row)
}

// UpdateProviderInput is the payload for updating a provider config.
// Secret is optional — if non-nil and non-empty, re-encrypt; if nil,
// preserve the stored secret.
type UpdateProviderInput struct {
	ID           pgtype.UUID
	Name         *string
	ProviderType *string
	Secret       *string
	BaseURL      *string
	Config       *map[string]string
	Categories   *[]string
	Priority     *int
	Enabled      *bool
}

// rowQuerier is satisfied by *pgxpool.Pool and pgx.Tx so the update builder
// can run inside or outside a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// UpdateProviderWithLock atomically reads the row FOR UPDATE, hands it and
// its decrypted secret to fn for merge+validate, and writes the returned
// field updates — all inside one transaction. The row lock closes the
// read-merge-write window that previously let concurrent PATCHes each
// validate against a stale snapshot and clobber each other's changes.
//
// fn receives (existing, storedSecret) and returns the UpdateProviderInput
// to apply; its error aborts the transaction and is returned as-is.
func (s *ProviderStore) UpdateProviderWithLock(ctx context.Context, id pgtype.UUID, fn func(existing ProviderConfig, storedSecret string) (UpdateProviderInput, error)) (ProviderConfig, error) {
	tx, err := s.write.Begin(ctx)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("begin provider update tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := s.scanProviderConfig(tx.QueryRow(ctx, `
		SELECT `+providerColumns+`
		FROM email_providers
		WHERE id = $1
		FOR UPDATE
	`, id))
	if err != nil {
		return ProviderConfig{}, errors.New("not_found: provider not found")
	}

	var storedSecret string

	if existing.APIKeyEncrypted != "" {
		storedSecret, err = crypto.Decrypt(existing.APIKeyEncrypted)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("email: decrypt secret: %w", err)
		}
	}

	upd, err := fn(existing, storedSecret)
	if err != nil {
		return ProviderConfig{}, err
	}

	upd.ID = id

	provider, err := s.updateProvider(ctx, tx, upd)
	if err != nil {
		return ProviderConfig{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ProviderConfig{}, fmt.Errorf("commit provider update: %w", err)
	}

	return provider, nil
}

// updateProvider applies the field updates — shared by UpdateProviderWithLock
// (inside its transaction). There is deliberately no exported non-transactional
// variant: every update path must hold the row lock for read-merge-write
// correctness.
func (s *ProviderStore) updateProvider(ctx context.Context, q rowQuerier, input UpdateProviderInput) (ProviderConfig, error) {
	setParts := []string{"updated_at = now()"}
	args := []any{input.ID}
	argIdx := 2 // $1=id, $2+

	if input.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}

	if input.ProviderType != nil {
		setParts = append(setParts, fmt.Sprintf("provider_type = $%d", argIdx))
		args = append(args, *input.ProviderType)
		argIdx++
	}

	if input.Secret != nil && *input.Secret != "" {
		encrypted, err := crypto.Encrypt(*input.Secret)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("encrypting secret: %w", err)
		}

		setParts = append(setParts, fmt.Sprintf("api_key_encrypted = $%d", argIdx))
		args = append(args, encrypted)
		argIdx++
	}

	if input.BaseURL != nil {
		setParts = append(setParts, fmt.Sprintf("base_url = $%d", argIdx))
		args = append(args, *input.BaseURL)
		argIdx++
	}

	if input.Config != nil {
		configJSON, _ := json.Marshal(*input.Config)

		setParts = append(setParts, fmt.Sprintf("config = $%d::jsonb", argIdx))
		args = append(args, configJSON)
		argIdx++
	}

	if input.Categories != nil {
		categoriesJSON, _ := json.Marshal(*input.Categories)

		setParts = append(setParts, fmt.Sprintf("categories = $%d::jsonb", argIdx))
		args = append(args, categoriesJSON)
		argIdx++
	}

	if input.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *input.Priority)
		argIdx++
	}

	if input.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *input.Enabled)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE email_providers SET %s
		WHERE id = $1
		RETURNING `+providerColumns+`
	`, strings.Join(setParts, ", "))

	row := q.QueryRow(ctx, query, args...)

	return s.scanProviderConfig(row)
}

// DeleteProvider removes a provider config by ID.
func (s *ProviderStore) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `DELETE FROM email_providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting email_provider: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: provider not found")
	}

	return nil
}

// GetDecryptedSecret returns the plaintext secret for a provider. Used
// internally by the dispatcher wiring at startup or on hot-reload.
func (s *ProviderStore) GetDecryptedSecret(ctx context.Context, id pgtype.UUID) (string, error) {
	return s.getDecryptedSecret(ctx, s.read, id)
}

// GetDecryptedSecretForUpdate reads via the write pool — see
// ListProvidersForUpdate for why reload/merge paths read the primary.
func (s *ProviderStore) GetDecryptedSecretForUpdate(ctx context.Context, id pgtype.UUID) (string, error) {
	return s.getDecryptedSecret(ctx, s.write, id)
}

func (s *ProviderStore) getDecryptedSecret(ctx context.Context, pool *pgxpool.Pool, id pgtype.UUID) (string, error) {
	var encrypted string

	err := pool.QueryRow(ctx, `
		SELECT api_key_encrypted FROM email_providers WHERE id = $1
	`, id).Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("getting provider secret: %w", err)
	}

	if encrypted == "" {
		return "", nil
	}

	plaintext, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("email: decrypt secret: %w", err)
	}

	return plaintext, nil
}

// configStr reads a non-secret setting from the provider's config map.
func configStr(cfg map[string]string, key string) string {
	if cfg == nil {
		return ""
	}

	return strings.TrimSpace(cfg[key])
}

// NewProviderFromConfig constructs an EmailProvider from a ProviderConfig.
// The secret must be the decrypted plaintext.
//
// Supported types (ADR-015):
//   - "smtp":      generic SMTP — config{host,port,username,from_address};
//     secret = password
//   - "resend":    Resend API — config{from_address}; secret = API key
//   - "postmark":  Postmark API — config{from_address,message_stream};
//     secret = server token
//   - "ses":       AWS SES v2 — config{region,access_key_id,from_address};
//     secret = secret_access_key
//   - "sendgrid":  SendGrid v3 — config{from_address}; secret = API key
//   - "sendpulse": SendPulse SMTP API — config{client_id,from_address};
//     secret = client_secret
func NewProviderFromConfig(cfg ProviderConfig, secret string) (EmailProvider, error) {
	if cfg.BaseURL != "" {
		if err := validateEmailEndpoint(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("email: invalid base URL for %s provider: %w", cfg.ProviderType, err)
		}
	}

	switch cfg.ProviderType {
	case "smtp":
		return NewSMTPProviderFromConfig(SMTPConfig{
			Host: configStr(cfg.Config, "host"),
			Port: configStr(cfg.Config, "port"),
			User: configStr(cfg.Config, "username"),
			Pass: secret,
			From: configStr(cfg.Config, "from_address"),
			Name: cfg.Name,
		})
	case "resend":
		return NewResendProvider(cfg.Name, secret, cfg.BaseURL,
			configStr(cfg.Config, "from_address")), nil
	case "postmark":
		return NewPostmarkProvider(cfg.Name, secret, cfg.BaseURL,
			configStr(cfg.Config, "from_address"),
			configStr(cfg.Config, "message_stream")), nil
	case "ses":
		return NewSESProvider(cfg.Name,
			configStr(cfg.Config, "region"),
			configStr(cfg.Config, "access_key_id"),
			secret, cfg.BaseURL,
			configStr(cfg.Config, "from_address")), nil
	case "sendgrid":
		return NewSendGridProvider(cfg.Name, secret, cfg.BaseURL,
			configStr(cfg.Config, "from_address")), nil
	case "sendpulse":
		return NewSendPulseProvider(cfg.Name,
			configStr(cfg.Config, "client_id"),
			secret, cfg.BaseURL,
			configStr(cfg.Config, "from_address")), nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.ProviderType)
	}
}

// LoadAndRegisterProviders rebuilds every supported category in the
// dispatcher from the database: enabled provider configs are registered in
// priority order (lower = tried first), and categories left without a
// DB-backed provider get fallback re-registered (the env-var SMTP provider,
// nil-safe). A full rebuild — rather than patching categories present in
// the DB set — is required so delete/disable of a category's last DB
// provider actually unregisters it instead of leaving a stale provider
// serving mail until restart.
//
// Called at startup (after env-var registration) and on hot-reload when a
// provider config is created/updated/deleted.
func LoadAndRegisterProviders(ctx context.Context, store *ProviderStore, dispatcher *EmailDispatcher, fallback EmailProvider) error {
	// Read via the write pool: reloads run immediately after a CRUD
	// mutation, and a lagging Reader replica could rebuild stale chains —
	// re-registering a provider that was just deleted/disabled (the same
	// stale-provider defect the full rebuild exists to prevent).
	providers, err := store.ListProvidersForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("loading email providers from db: %w", err)
	}

	type providerEntry struct {
		config ProviderConfig
		secret string
	}

	byCategory := make(map[string][]providerEntry)

	for _, pc := range providers {
		if !pc.Enabled {
			continue
		}

		secret, err := store.GetDecryptedSecretForUpdate(ctx, pc.ID)
		if err != nil {
			slog.Warn("email: skipping provider, failed to decrypt secret", "provider", pc.Name, "error", err)
			continue
		}

		for _, cat := range pc.Categories {
			byCategory[cat] = append(byCategory[cat], providerEntry{config: pc, secret: secret})
		}
	}

	// Build every supported category's chain first, then swap atomically —
	// a clear-then-register sequence would let an in-flight Dispatch
	// observe an empty category and fail with ErrNoProviders (SkipRetry →
	// dropped email). Constructors are pure, so building before the swap
	// costs nothing.
	chains := make(map[string][]EmailProvider, len(SupportedEmailCategories()))

	for _, cat := range SupportedEmailCategories() {
		var chain []EmailProvider

		for _, entry := range byCategory[cat] {
			p, err := NewProviderFromConfig(entry.config, entry.secret)
			if err != nil {
				slog.Warn("email: skipping provider, unsupported or invalid config", "provider", entry.config.Name, "error", err)
				continue
			}

			chain = append(chain, p)
		}

		if len(chain) == 0 && fallback != nil {
			chain = append(chain, fallback)
		}

		// Skip empty chains — a present-but-empty category key is
		// indistinguishable noise for any future map iteration.
		if len(chain) > 0 {
			chains[cat] = chain
		}
	}

	dispatcher.ReplaceProviders(chains)

	return nil
}

// ValidateProviderConfig checks a provider config before it is persisted —
// catches configs that would persist fine but silently degrade sends at
// runtime (missing host/from/credentials). secret is the plaintext secret
// being stored (for create) or the decrypted stored value (for update).
func ValidateProviderConfig(cfg ProviderConfig, secret string) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("name is required")
	}

	validType := false

	for _, t := range SupportedProviderTypes() {
		if cfg.ProviderType == t {
			validType = true
			break
		}
	}

	if !validType {
		return fmt.Errorf("invalid provider_type %q", cfg.ProviderType)
	}

	if len(cfg.Categories) == 0 {
		return fmt.Errorf("at least one category is required")
	}

	validCat := make(map[string]bool, len(SupportedEmailCategories()))
	for _, c := range SupportedEmailCategories() {
		validCat[c] = true
	}

	for _, cat := range cfg.Categories {
		if !validCat[cat] {
			return fmt.Errorf("invalid category %q", cat)
		}
	}

	if cfg.BaseURL != "" {
		if err := validateEmailEndpoint(cfg.BaseURL); err != nil {
			return fmt.Errorf("invalid base_url: %w", err)
		}
	}

	for _, field := range RequiredConfigFields(cfg.ProviderType) {
		if configStr(cfg.Config, field) == "" {
			return fmt.Errorf("config.%s is required for provider_type %q", field, cfg.ProviderType)
		}
	}

	// SMTP's From falls back to the username (same as NewSMTPProviderFromConfig),
	// so at least one of from_address/username must be set. A non-numeric
	// port would otherwise persist and only fail at dial time.
	if cfg.ProviderType == "smtp" {
		if configStr(cfg.Config, "from_address") == "" && configStr(cfg.Config, "username") == "" {
			return fmt.Errorf("config.from_address or config.username is required for provider_type %q", cfg.ProviderType)
		}

		if port := configStr(cfg.Config, "port"); port != "" {
			if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("config.port must be a numeric TCP port for provider_type %q", cfg.ProviderType)
			}
		}
	}

	// Every vendor type needs a secret; only generic SMTP may go passwordless
	// (open relays / local dev servers).
	if cfg.ProviderType != "smtp" && secret == "" {
		return fmt.Errorf("secret is required for provider_type %q", cfg.ProviderType)
	}

	return nil
}

// RequiredConfigFields returns the non-secret config keys a provider type
// must have set — consumed by ValidateProviderConfig and exposed to the
// control panel so its form marks required inputs (Rule 16).
func RequiredConfigFields(providerType string) []string {
	switch providerType {
	case "smtp":
		return []string{"host"}
	case "ses":
		return []string{"access_key_id", "from_address"}
	case "sendpulse":
		return []string{"client_id", "from_address"}
	case "resend", "postmark", "sendgrid":
		return []string{"from_address"}
	default:
		return nil
	}
}

// SupportedProviderTypes returns the provider_type values that can be
// instantiated by NewProviderFromConfig — the single source of truth the
// control-plane validator and the factory stay in sync through.
func SupportedProviderTypes() []string {
	return []string{
		"smtp",
		"resend",
		"postmark",
		"ses",
		"sendgrid",
		"sendpulse",
	}
}

// SupportedEmailCategories returns the canonical dispatcher categories
// used by EmailRequest.Category and provider registration.
func SupportedEmailCategories() []string {
	return []string{
		"transactional",
		"marketing",
	}
}
