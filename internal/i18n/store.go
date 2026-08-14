// Package i18n provides the internationalization translation system for
// JapaConnect. The store layer wraps sqlc-generated queries with typed
// Go methods, keeping the service layer free of database concerns.
package i18n

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for translation operations.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// ListByLocale returns all translations for a given locale.
func (s *Store) ListByLocale(ctx context.Context, tenantID pgtype.UUID, locale string) ([]gen.ListTranslationsByLocaleRow, error) {
	rows, err := s.read.ListTranslationsByLocale(ctx, gen.ListTranslationsByLocaleParams{
		TenantID: tenantID,
		Locale:   locale,
	})
	if err != nil {
		return nil, fmt.Errorf("listing translations by locale: %w", err)
	}

	return rows, nil
}

// ListByKeyPrefix returns translations whose key starts with the given prefix.
func (s *Store) ListByKeyPrefix(ctx context.Context, tenantID pgtype.UUID, prefix string) ([]gen.ListTranslationsByKeyPrefixRow, error) {
	rows, err := s.read.ListTranslationsByKeyPrefix(ctx, gen.ListTranslationsByKeyPrefixParams{
		TenantID: tenantID,
		Prefix:   &prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("listing translations by key prefix: %w", err)
	}

	return rows, nil
}

// Get returns a single translation by locale and key.
func (s *Store) Get(ctx context.Context, tenantID pgtype.UUID, locale, key string) (gen.GetTranslationRow, error) {
	row, err := s.read.GetTranslation(ctx, gen.GetTranslationParams{
		TenantID: tenantID,
		Locale:   locale,
		Key:      key,
	})
	if err != nil {
		return gen.GetTranslationRow{}, fmt.Errorf("getting translation: %w", err)
	}

	return row, nil
}

// ListLocales returns all distinct locales for a tenant.
func (s *Store) ListLocales(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
	locales, err := s.read.ListTranslationLocales(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing locales: %w", err)
	}

	return locales, nil
}

// CountByLocale returns the count of translations per locale.
func (s *Store) CountByLocale(ctx context.Context, tenantID pgtype.UUID) ([]gen.CountTranslationsByLocaleRow, error) {
	rows, err := s.read.CountTranslationsByLocale(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("counting translations by locale: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Write operations
// ---------------------------------------------------------------------------

// Upsert inserts or updates a single translation.
func (s *Store) Upsert(ctx context.Context, p gen.UpsertTranslationParams) error {
	if err := s.write.UpsertTranslation(ctx, p); err != nil {
		return fmt.Errorf("upserting translation: %w", err)
	}

	return nil
}

// Delete removes a translation by ID.
func (s *Store) Delete(ctx context.Context, tenantID, id pgtype.UUID) error {
	if err := s.write.DeleteTranslation(ctx, gen.DeleteTranslationParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
		return fmt.Errorf("deleting translation: %w", err)
	}

	return nil
}

// DeleteByLocale removes all translations for a given locale.
func (s *Store) DeleteByLocale(ctx context.Context, tenantID pgtype.UUID, locale string) error {
	if err := s.write.DeleteTranslationsByLocale(ctx, gen.DeleteTranslationsByLocaleParams{
		TenantID: tenantID,
		Locale:   locale,
	}); err != nil {
		return fmt.Errorf("deleting translations by locale: %w", err)
	}

	return nil
}

// GetByID returns a single translation by its primary key.
func (s *Store) GetByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetTranslationByIDRow, error) {
	row, err := s.read.GetTranslationByID(ctx, gen.GetTranslationByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.GetTranslationByIDRow{}, fmt.Errorf("getting translation by id: %w", err)
	}

	return row, nil
}
