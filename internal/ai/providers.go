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
	ID               pgtype.UUID   `json:"id"`
	TenantID         pgtype.UUID   `json:"tenant_id"`
	Name             string        `json:"name"`
	ProviderType     string        `json:"provider_type"`
	APIKeyEncrypted  string        `json:"-"` // never serialized to JSON
	APIKeyMasked     string        `json:"api_key_masked"` // derived, read-only
	BaseURL          string        `json:"base_url"`
	Model            string        `json:"model"`
	Categories       []string      `json:"categories"`
	Priority         int           `json:"priority"`
	Enabled          bool          `json:"enabled"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
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
// return an empty string. Short keys (≤8 chars) return all-masked.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// scanProviderConfig scans a row into a ProviderConfig, deriving the masked
// API key from the encrypted value (decrypted first, then masked).
func (s *ProviderStore) scanProviderConfig(row interface {
	Scan(dest ...any) error
}) (ProviderConfig, error) {
	var pc ProviderConfig
	var categoriesJSON []byte
	var encryptedKey string

	if err := row.Scan(
		&pc.ID, &pc.TenantID, &pc.Name, &pc.ProviderType,
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

// ListProviders returns all AI provider configs for a tenant, ordered by
// priority ascending then name.
func (s *ProviderStore) ListProviders(ctx context.Context, tenantID pgtype.UUID) ([]ProviderConfig, error) {
	rows, err := s.read.Query(ctx, `
		SELECT id, tenant_id, name, provider_type, api_key_encrypted,
		       base_url, model, categories, priority, enabled, created_at, updated_at
		FROM ai_providers
		WHERE tenant_id = $1
		ORDER BY priority ASC, name ASC
	`, tenantID)
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

// GetProvider returns a single AI provider config by ID, scoped to the tenant.
func (s *ProviderStore) GetProvider(ctx context.Context, tenantID, id pgtype.UUID) (ProviderConfig, error) {
	row := s.read.QueryRow(ctx, `
		SELECT id, tenant_id, name, provider_type, api_key_encrypted,
		       base_url, model, categories, priority, enabled, created_at, updated_at
		FROM ai_providers
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	return s.scanProviderConfig(row)
}

// CreateProviderInput is the payload for creating a new provider config.
type CreateProviderInput struct {
	TenantID     pgtype.UUID
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
			(tenant_id, name, provider_type, api_key_encrypted, base_url, model, categories, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, tenant_id, name, provider_type, api_key_encrypted,
		          base_url, model, categories, priority, enabled, created_at, updated_at
	`, input.TenantID, input.Name, input.ProviderType, encryptedKey,
		input.BaseURL, input.Model, categoriesJSON, input.Priority, input.Enabled)

	return s.scanProviderConfig(row)
}

// UpdateProviderInput is the payload for updating a provider config.
// APIKey is optional — if empty, the existing encrypted key is preserved.
type UpdateProviderInput struct {
	TenantID     pgtype.UUID
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
	args := []any{input.ID, input.TenantID}
	argIdx := 3 // $1=id, $2=tenant_id, $3+

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
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, provider_type, api_key_encrypted,
		          base_url, model, categories, priority, enabled, created_at, updated_at
	`, strings.Join(setParts, ", "))

	row := s.write.QueryRow(ctx, query, args...)
	return s.scanProviderConfig(row)
}

// DeleteProvider removes a provider config by ID, scoped to the tenant.
func (s *ProviderStore) DeleteProvider(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `DELETE FROM ai_providers WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting ai_provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found: provider not found")
	}
	return nil
}

// GetDecryptedAPIKey returns the plaintext API key for a provider. Used
// internally by the dispatcher wiring to construct the provider at startup
// or on hot-reload. Returns empty string if no key is stored.
func (s *ProviderStore) GetDecryptedAPIKey(ctx context.Context, tenantID, id pgtype.UUID) (string, error) {
	var encrypted string
	err := s.read.QueryRow(ctx, `
		SELECT api_key_encrypted FROM ai_providers WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("getting api key: %w", err)
	}
	if encrypted == "" {
		return "", nil
	}
	return crypto.Decrypt(encrypted)
}

// NewProviderFromConfig constructs a Provider from a ProviderConfig. The
// apiKey must be the decrypted plaintext.
//
// Supported types:
//   - "xiaomi": Xiaomi MiMo (uses api-key header, auto-selects base URL from key prefix)
//   - "openai": Any OpenAI-compatible API with Bearer auth (OpenAI, OpenRouter,
//     OpenCode Zen, Groq, Together AI, LiteLLM, vLLM, Command Code /chat/completions, etc.)
//   - "ollama": Ollama's OpenAI-compatible endpoint (uses Bearer auth, no key needed)
//   - "anthropic": Anthropic Claude native Messages API (x-api-key header,
//     /v1/messages endpoint). Also works with Command Code's /messages endpoint.
//   - "devin": Devin agent API (async session-based, Bearer cog_ token,
//     polls until session completes)
//   - "custom": Any OpenAI-compatible endpoint
//
// "openai", "ollama", and "custom" all use the OpenAIProvider implementation
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
	case "openai", "ollama", "custom":
		// All OpenAI-compatible providers use the same implementation.
		// The base URL and model differentiate them:
		//   - OpenAI:       https://api.openai.com/v1
		//   - OpenRouter:   https://openrouter.ai/api/v1
		//   - OpenCode Zen: (admin-configured base URL)
		//   - Ollama:       http://localhost:11434/v1
		//   - Command Code: https://api.commandcode.ai/provider/v1
		p := NewOpenAIProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		return p, nil
	case "anthropic":
		// Dedicated Anthropic Messages API provider.
		// Native: https://api.anthropic.com/v1 (x-api-key header)
		// Command Code: https://api.commandcode.ai/provider/v1 (Bearer auth, auto-detected)
		p := NewAnthropicProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		return p, nil
	case "devin":
		// Devin agent API — async session-based, wraps create+poll into Send().
		// Base URL must include the org path:
		//   https://api.devin.ai/v3/organizations/org-abc123
		p := NewDevinProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		return p, nil
	case "cursor":
		// Cursor Cloud Agents API — async agent-based, wraps create+poll into Send().
		// Base URL: https://api.cursor.com/v1
		// Auth: Basic auth with API key as username.
		p := NewCursorProvider(cfg.Name, apiKey, cfg.BaseURL, cfg.Model)
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.ProviderType)
	}
}

// LoadAndRegisterProviders loads all enabled provider configs from the
// database and registers them with the dispatcher, grouped by category.
// Providers are sorted by priority within each category (lower priority
// value = tried first). Categories that have no DB-backed providers are
// left untouched (so env-var-registered providers from startup remain).
//
// This is called at startup (after env-var providers are registered) and
// on hot-reload when a provider config is created/updated/deleted.
func LoadAndRegisterProviders(ctx context.Context, store *ProviderStore, dispatcher *Dispatcher, tenantID pgtype.UUID) error {
	providers, err := store.ListProviders(ctx, tenantID)
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
		apiKey, err := store.GetDecryptedAPIKey(ctx, tenantID, pc.ID)
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

	// Register DB-backed providers. We do NOT clear categories that already
	// have env-var providers — instead DB providers are appended (they take
	// priority since they're registered after env-var ones... actually no,
	// the dispatcher tries providers in registration order, first success
	// wins. To make DB providers take priority, we clear and re-register
	// only categories that have DB-backed providers.
	for cat, entries := range byCategory {
		dispatcher.ClearCategory(cat)
		for _, entry := range entries {
			p, err := NewProviderFromConfig(entry.config, entry.apiKey)
			if err != nil {
				slog.Warn("ai: skipping provider, unsupported type", "provider", entry.config.Name, "error", err)
				continue
			}
			dispatcher.RegisterProvider(cat, p)
		}
	}

	return nil
}
