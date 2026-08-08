package stock

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
)

// ErrProviderNotFound is returned when a provider update affects no rows.
var ErrProviderNotFound = errors.New("stock provider not found")

// Store manages stock_providers database operations.
type Store struct {
	pool *pgxpool.Pool // writer — used for INSERT/UPDATE
	read *pgxpool.Pool // reader — used for SELECT (read-replica aware)
}

// NewStore creates a Store backed by the database pool.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database.Writer, read: database.Reader}
}

// ListProviders returns all configured stock providers.
func (s *Store) ListProviders(ctx context.Context) ([]ProviderConfigRow, error) {
	rows, err := s.read.Query(ctx,
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
	err := s.read.QueryRow(ctx,
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

// UpsertProvider inserts or updates a provider row.
func (s *Store) UpsertProvider(ctx context.Context, row ProviderConfigRow) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stock_providers (provider_name, enabled, api_key_encrypted)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (provider_name) DO UPDATE
		 SET enabled = $2, api_key_encrypted = $3, updated_at = now()`,
		row.ProviderName, row.Enabled, row.APIKeyEncrypted)
	if err != nil {
		return fmt.Errorf("upserting provider %s: %w", row.ProviderName, err)
	}
	return nil
}

// ToggleProvider enables or disables a provider.
func (s *Store) ToggleProvider(ctx context.Context, name string, enabled bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stock_providers SET enabled = $2, updated_at = now()
		 WHERE provider_name = $1`, name, enabled)
	if err != nil {
		return fmt.Errorf("toggling provider %s: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("provider %s: %w", name, ErrProviderNotFound)
	}
	return nil
}

// SetAPIKey updates the encrypted API key for a provider.
func (s *Store) SetAPIKey(ctx context.Context, name, encryptedKey string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stock_providers SET api_key_encrypted = $2, updated_at = now()
		 WHERE provider_name = $1`, name, encryptedKey)
	if err != nil {
		return fmt.Errorf("setting API key for %s: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("provider %s: %w", name, ErrProviderNotFound)
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
