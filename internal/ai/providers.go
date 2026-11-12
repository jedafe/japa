// Package ai — providers.go provides DB-backed AI provider configuration
// management for the Control Panel. Provider configs (name, type, encrypted
// API key, base URL, model, categories, priority, enabled flag) are stored in
// the ai_providers table and loaded at startup to register with the
// Dispatcher. Super-admins can CRUD providers at runtime via the
// /control/ai-providers endpoints; changes hot-reload the dispatcher.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/crypto"
)

// ProviderConfig is the DB-backed representation of an AI provider. The
// API key is stored encrypted (api_key_encrypted) and never sent to the
// frontend in plaintext — the JSON response uses api_key_masked instead.
type ProviderConfig struct {
	ID pgtype.UUID `json:"id"`

	Name            string    `json:"name"`
	ProviderType    string    `json:"provider_type"`
	APIKeyEncrypted string    `json:"-"`              // never serialized to JSON
	APIKeyMasked    string    `json:"api_key_masked"` // derived, read-only
	BaseURL         string    `json:"base_url"`
	Model           string    `json:"model"`
	Categories      []string  `json:"categories"`
	Priority        int       `json:"priority"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ProviderStore provides DB access for the ai_providers table.
type ProviderStore struct {
	read  *pgxpool.Pool
	write *pgxpool.Pool
}

// NewProviderStore creates a ProviderStore backed by the given pools.
func NewProviderStore(read, write *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{read: read, write: write}
}

// maskAPIKey returns a masked representation of an API key for display:
// shows the first 4 and last 4 characters with **** in between. Empty keys
// return an empty string. Keys of ≤12 chars are fully masked —
// first4+last4 would reveal 8 of their ≤12 characters, i.e. nearly the
// entire key.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	if len(key) <= 12 {
		return "****"
	}

	return key[:4] + "****" + key[len(key)-4:]
}

// scanProviderConfig scans a row into a ProviderConfig, deriving the masked
// API key from the encrypted value (decrypted first, then masked).
func (s *ProviderStore) scanProviderConfig(row interface {
	Scan(dest ...any) error
},
) (ProviderConfig, error) {
	var pc ProviderConfig

	var categoriesJSON []byte

	var encryptedKey string

	if err := row.Scan(
		&pc.ID, &pc.Name, &pc.ProviderType,
		&encryptedKey, &pc.BaseURL, &pc.Model, &categoriesJSON,
		&pc.Priority, &pc.Enabled, &pc.CreatedAt, &pc.UpdatedAt,
	); err != nil {
		return ProviderConfig{}, fmt.Errorf("scanning ai_provider: %w", err)
	}

	pc.APIKeyEncrypted = encryptedKey

	// Decrypt the API key for masking (best-effort — if decryption fails,
	// show a placeholder so the UI still works).
	if encryptedKey != "" {
		if plain, err := crypto.Decrypt(encryptedKey); err == nil {
			pc.APIKeyMasked = maskAPIKey(plain)
		} else {
			pc.APIKeyMasked = "****"
		}
	}

	if len(categoriesJSON) > 0 {
		_ = json.Unmarshal(categoriesJSON, &pc.Categories)
	}

	if pc.Categories == nil {
		pc.Categories = []string{}
	}

	return pc, nil
}

// ListProviders returns all AI provider configs for a platform, ordered by
// priority ascending then name.
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
		SELECT id, name, provider_type, api_key_encrypted,
		       base_url, model, categories, priority, enabled, created_at, updated_at
		FROM ai_providers
		
		ORDER BY priority ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing ai_providers: %w", err)
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

// GetProvider returns a single AI provider config by ID, scoped to the platform.
func (s *ProviderStore) GetProvider(ctx context.Context, id pgtype.UUID) (ProviderConfig, error) {
	return s.getProvider(ctx, s.read, id)
}

// GetProviderForUpdate reads via the write pool — see ListProvidersForUpdate.
// Used by the test-send endpoint so a test issued right after create/update
// sees the row it just wrote.
func (s *ProviderStore) GetProviderForUpdate(ctx context.Context, id pgtype.UUID) (ProviderConfig, error) {
	return s.getProvider(ctx, s.write, id)
}

func (s *ProviderStore) getProvider(ctx context.Context, pool *pgxpool.Pool, id pgtype.UUID) (ProviderConfig, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, name, provider_type, api_key_encrypted,
		       base_url, model, categories, priority, enabled, created_at, updated_at
		FROM ai_providers
		WHERE id = $1
	`, id)

	return s.scanProviderConfig(row)
}

// CreateProviderInput is the payload for creating a new provider config.
type CreateProviderInput struct {
	Name         string
	ProviderType string
	APIKey       string // plaintext — encrypted before storage
	BaseURL      string
	Model        string
	Categories   []string
	Priority     int
	Enabled      bool
}

// CreateProvider inserts a new provider config and returns it.
func (s *ProviderStore) CreateProvider(ctx context.Context, input CreateProviderInput) (ProviderConfig, error) {
	encryptedKey, err := crypto.Encrypt(input.APIKey)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("encrypting api key: %w", err)
	}

	categoriesJSON, _ := json.Marshal(input.Categories)
	if input.Categories == nil {
		categoriesJSON = []byte("[]")
	}

	row := s.write.QueryRow(ctx, `
		INSERT INTO ai_providers
			(name, provider_type, api_key_encrypted, base_url, model, categories, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, provider_type, api_key_encrypted,
		          base_url, model, categories, priority, enabled, created_at, updated_at
	`, input.Name, input.ProviderType, encryptedKey, input.BaseURL, input.Model, categoriesJSON, input.Priority, input.Enabled)

	return s.scanProviderConfig(row)
}

// UpdateProviderInput is the payload for updating a provider config.
// APIKey is optional — if empty, the existing encrypted key is preserved.
type UpdateProviderInput struct {
	ID           pgtype.UUID
	Name         *string
	ProviderType *string
	APIKey       *string // if non-nil and non-empty, re-encrypt; if nil, preserve
	BaseURL      *string
	Model        *string
	Categories   *[]string
	Priority     *int
	Enabled      *bool
}

// UpdateProvider updates an existing provider config and returns it.
func (s *ProviderStore) UpdateProvider(ctx context.Context, input UpdateProviderInput) (ProviderConfig, error) {
	// Build the SET clause dynamically based on which fields are provided.
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

	if input.APIKey != nil && *input.APIKey != "" {
		encrypted, err := crypto.Encrypt(*input.APIKey)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("encrypting api key: %w", err)
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

	if input.Model != nil {
		setParts = append(setParts, fmt.Sprintf("model = $%d", argIdx))
		args = append(args, *input.Model)
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
		UPDATE ai_providers SET %s
		WHERE id = $1
		RETURNING id, name, provider_type, api_key_encrypted,
		          base_url, model, categories, priority, enabled, created_at, updated_at
	`, strings.Join(setParts, ", "))

	row := s.write.QueryRow(ctx, query, args...)

	return s.scanProviderConfig(row)
}

// DeleteProvider removes a provider config by ID, scoped to the platform.
func (s *ProviderStore) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `DELETE FROM ai_providers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting ai_provider: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: provider not found")
	}

	return nil
}

// GetDecryptedAPIKey returns the plaintext API key for a provider. Used
// internally by the dispatcher wiring to construct the provider at startup
// or on hot-reload. Returns empty string if no key is stored.
func (s *ProviderStore) GetDecryptedAPIKey(ctx context.Context, id pgtype.UUID) (string, error) {
	return s.getDecryptedAPIKey(ctx, s.read, id)
}

// GetDecryptedAPIKeyForUpdate reads via the write pool — see
// ListProvidersForUpdate for why reload paths read the primary.
func (s *ProviderStore) GetDecryptedAPIKeyForUpdate(ctx context.Context, id pgtype.UUID) (string, error) {
	return s.getDecryptedAPIKey(ctx, s.write, id)
}

func (s *ProviderStore) getDecryptedAPIKey(ctx context.Context, pool *pgxpool.Pool, id pgtype.UUID) (string, error) {
	var encrypted string

	err := pool.QueryRow(ctx, `
		SELECT api_key_encrypted FROM ai_providers WHERE id = $1
	`, id).Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("getting api key: %w", err)
	}

	if encrypted == "" {
		return "", nil
	}

	plaintext, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("ai: decrypt api key: %w", err)
	}

	return plaintext, nil
}

// NewProviderFromConfig constructs a Provider from a ProviderConfig. The
// apiKey must be the decrypted plaintext.
// // Supported types:
// - "xiaomi": Xiaomi MiMo (uses api-key header, auto-selects base URL from key prefix)
// - "openai": Any OpenAI-compatible API with Bearer auth (OpenAI, OpenRouter,
// OpenCode Zen, Together AI, LiteLLM, vLLM, Command Code /chat/completions, etc.)
// - "groq": Groq Cloud OpenAI-compatible API (https://api.groq.com/openai/v1).
// - "ollama": Ollama's OpenAI-compatible endpoint. Local: http://localhost:11434/v1
// with any non-empty key (ignored by Ollama). Cloud: https://ollama.com/v1 with
// a real Ollama API key from https://ollama.com/settings/keys.
// - "anthropic": Anthropic Claude native Messages API (x-api-key header,
// /v1/messages endpoint). Also works with Command Code's /messages endpoint.
// - "devin": Devin agent API (async session-based, Bearer cog_ token,
// polls until session completes)
// - "cursor": Cursor Cloud Agents API (async agent-based, Basic auth)
// - "perplexity": Perplexity sonar API (OpenAI-compatible, returns citations)
// - "custom": Any OpenAI-compatible endpoint
// // "openai", "ollama", and "custom" all use the OpenAIProvider implementation
// since they share the OpenAI Chat Completions API format. "anthropic" has a
// dedicated AnthropicProvider for the native Messages API. "devin" has a
// dedicated DevinProvider that wraps the async session flow.
func NewProviderFromConfig(cfg ProviderConfig, apiKey string) (Provider, error) {
	switch cfg.ProviderType {
	case "xiaomi":
		p := NewXiaomiProvider(apiKey, cfg.Model)
		if cfg.Name != "" {
			p.SetName(cfg.Name)
		}

		return p, nil
	case "openai", "ollama", "custom", "groq":
		// All OpenAI-compatible providers use the same implementation.
		// The base URL and model differentiate them:
		// - OpenAI: https://api.openai.com/v1
		// - OpenRouter: https://openrouter.ai/api/v1
		// - Groq: https://api.groq.com/openai/v1
		// - OpenCode Zen: (admin-configured base URL)
		// - Ollama: http://localhost:11434/v1 or https://ollama.com/v1 (Cloud)
		// - Command Code: https://api.commandcode.ai/provider/v1
		// // Validate the admin-supplied base URL before constructing the
		// provider. validateEndpointAllowHTTP permits HTTP for local
		// proxies (Ollama at http://localhost:11434) while still enforcing
		// a valid http(s) scheme, non-empty host, and rejecting HTTPS to
		// internal/loopback hosts (SSRF defense-in-depth, §103 Security).
		// An empty BaseURL is allowed — the constructor falls back to a
		// hardcoded default HTTPS endpoint.
		if cfg.BaseURL != "" {
			if err := validateEndpointAllowHTTP(cfg.BaseURL); err != nil {
				return nil, fmt.Errorf("ai: invalid base URL for %s provider: %w", cfg.ProviderType, err)
			}
		}

		var p Provider
		if cfg.ProviderType == "groq" {
			p = NewGroqProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		} else {
			p = NewOpenAIProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		}
		// OpenAI and Groq have adopted the current Chat Completions schema
		// (max_completion_tokens). Ollama and custom proxies may still require
		// the deprecated max_tokens field, so leave them on the legacy field.
		if cfg.ProviderType == "openai" || cfg.ProviderType == "groq" {
			if oa, ok := p.(*OpenAIProvider); ok {
				oa.UseMaxCompletionTokens()
			}
		}

		return p, nil
	case "anthropic":
		// Dedicated Anthropic Messages API provider.
		// Native: https://api.anthropic.com/v1 (x-api-key header)
		// Command Code: https://api.commandcode.ai/provider/v1 (Bearer auth, auto-detected)
		if cfg.BaseURL != "" {
			if err := validateEndpointAllowHTTP(cfg.BaseURL); err != nil {
				return nil, fmt.Errorf("ai: invalid base URL for anthropic provider: %w", err)
			}
		}

		p := NewAnthropicProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)

		return p, nil
	case "devin":
		// Devin agent API — async session-based, wraps create+poll into Send().
		// Base URL must include the org path:
		// https://api.devin.ai/v3/organizations/org-abc123
		if cfg.BaseURL != "" {
			if err := validateEndpointAllowHTTP(cfg.BaseURL); err != nil {
				return nil, fmt.Errorf("ai: invalid base URL for devin provider: %w", err)
			}
		}

		p := NewDevinProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)

		return p, nil
	case "cursor":
		// Cursor Cloud Agents API — async agent-based, wraps create+poll into Send().
		// Base URL: https://api.cursor.com/v1
		// Auth: Basic auth with API key as username.
		if cfg.BaseURL != "" {
			if err := validateEndpointAllowHTTP(cfg.BaseURL); err != nil {
				return nil, fmt.Errorf("ai: invalid base URL for cursor provider: %w", err)
			}
		}

		p := NewCursorProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)

		return p, nil
	case "perplexity":
		// Perplexity sonar API — OpenAI-compatible chat completions with citations.
		// Base URL: https://api.perplexity.ai (Perplexity does not use /v1 suffix)
		// Auth: Bearer token. Sonar models: sonar-pro, sonar-reasoning, etc.
		if cfg.BaseURL != "" {
			if err := validateEndpointAllowHTTP(cfg.BaseURL); err != nil {
				return nil, fmt.Errorf("ai: invalid base URL for perplexity provider: %w", err)
			}
		}

		p := NewPerplexityProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)

		return p, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.ProviderType)
	}
}

// LoadAndRegisterProviders rebuilds the dispatcher's category→provider
// chains from the database: enabled provider configs are registered in
// priority order (lower = tried first), and categories left without a
// DB-backed provider get their env-registered chain restored from
// fallbackChains (a Dispatcher.SnapshotProviders copy taken at startup
// after env registration, nil-safe).
//
// A full rebuild — rather than patching categories present in the DB set —
// is required so delete/disable of a category's last DB provider actually
// unregisters it instead of leaving a stale provider serving traffic until
// restart. The rebuild is swapped in atomically so an in-flight Dispatch
// never observes a partially built category.
//
// Called at startup (after env-var providers are registered and snapshotted)
// and on hot-reload when a provider config is created/updated/deleted.
func LoadAndRegisterProviders(ctx context.Context, store *ProviderStore, dispatcher *Dispatcher, fallbackChains map[string][]Provider) error {
	// Read via the write pool: reloads run immediately after a CRUD
	// mutation, and a lagging Reader replica could rebuild stale chains —
	// re-registering a provider that was just deleted/disabled (the same
	// stale-provider defect the full rebuild exists to prevent).
	providers, err := store.ListProvidersForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("loading providers from db: %w", err)
	}

	// Group enabled providers by category, preserving priority order.
	type providerEntry struct {
		config ProviderConfig
		apiKey string
	}

	byCategory := make(map[string][]providerEntry)

	for _, pc := range providers {
		if !pc.Enabled {
			continue
		}

		apiKey, err := store.GetDecryptedAPIKeyForUpdate(ctx, pc.ID)
		if err != nil {
			slog.Warn("ai: skipping provider, failed to decrypt api key", "provider", pc.Name, "error", err)
			continue
		}
		// If no DB-stored key, skip — the env-var provider is already
		// registered from startup and will serve as fallback.
		if apiKey == "" {
			continue
		}

		for _, cat := range pc.Categories {
			byCategory[cat] = append(byCategory[cat], providerEntry{config: pc, apiKey: apiKey})
		}
	}

	// Rebuild every category that could be affected: the canonical set, any
	// category present in the DB rows, and any category with env-registered
	// providers. Anything outside the union can't exist in the dispatcher.
	cats := make(map[string]struct{})
	for _, c := range SupportedAICategories() {
		cats[c] = struct{}{}
	}
	for c := range byCategory {
		cats[c] = struct{}{}
	}
	for c := range fallbackChains {
		cats[c] = struct{}{}
	}

	chains := make(map[string][]Provider, len(cats))

	for cat := range cats {
		var chain []Provider

		for _, entry := range byCategory[cat] {
			p, err := NewProviderFromConfig(entry.config, entry.apiKey)
			if err != nil {
				slog.Warn("ai: skipping provider, unsupported type", "provider", entry.config.Name, "error", err)
				continue
			}

			chain = append(chain, p)
		}

		// No usable DB provider → restore the env-registered chain so the
		// model picker still returns models for the category.
		if len(chain) == 0 {
			chain = append(chain, fallbackChains[cat]...)
		}

		// Skip empty chains — a present-but-empty category key would leak
		// into RegisteredCategories even though it has no providers.
		if len(chain) > 0 {
			chains[cat] = chain
		}
	}

	dispatcher.ReplaceProviders(chains)

	return nil
}

// SupportedProviderTypes returns the provider_type values that can be
// instantiated by NewProviderFromConfig. This should be the single source of
// truth so the control-panel validator and the gateway stay in sync.
func SupportedProviderTypes() []string {
	return []string{
		"openai",
		"anthropic",
		"groq",
		"ollama",
		"xiaomi",
		"devin",
		"cursor",
		"perplexity",
		"custom",
	}
}

// SupportedAICategories returns the canonical dispatcher categories used by
// Request.Category and the provider registry. Keep this in sync with any new
// category introduced by generation, chat, embedding, moderation,
// extraction, or classification surfaces.
func SupportedAICategories() []string {
	return []string{
		"chat",
		"classification",
		"content_generation",
		"embedding",
		"extraction",
		"moderation",
	}
}
