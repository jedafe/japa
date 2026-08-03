// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074). Users register FCM/APNs device tokens
// for push notifications. The notification:send_push worker dispatches
// pushes to active tokens.
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
//      japaconnect-engineering/adr/ADR-074-screen-contract.md
package devices

import (
	"context"
	"fmt"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for device token management.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a devices store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

func (s *Store) UpsertToken(ctx context.Context, arg gen.UpsertDeviceTokenParams) (gen.DeviceToken, error) {
	row, err := s.queries.UpsertDeviceToken(ctx, arg)
	if err != nil {
		return gen.DeviceToken{}, fmt.Errorf("devices: upsert token: %w", err)
	}
	return row, nil
}

func (s *Store) DeregisterToken(ctx context.Context, arg gen.DeregisterDeviceTokenParams) error {
	if err := s.queries.DeregisterDeviceToken(ctx, arg); err != nil {
		return fmt.Errorf("devices: deregister token: %w", err)
	}
	return nil
}

func (s *Store) ListUserTokens(ctx context.Context, arg gen.ListUserDeviceTokensParams) ([]gen.DeviceToken, error) {
	rows, err := s.queries.ListUserDeviceTokens(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("devices: list user tokens: %w", err)
	}
	return rows, nil
}

func (s *Store) GetActiveTokensForUser(ctx context.Context, arg gen.GetActiveDeviceTokensForUserParams) ([]gen.DeviceToken, error) {
	rows, err := s.queries.GetActiveDeviceTokensForUser(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("devices: get active tokens: %w", err)
	}
	return rows, nil
}

func (s *Store) MarkTokenInactive(ctx context.Context, token string) error {
	if err := s.queries.MarkDeviceTokenInactive(ctx, token); err != nil {
		return fmt.Errorf("devices: mark token inactive: %w", err)
	}
	return nil
}

func (s *Store) TouchToken(ctx context.Context, arg gen.TouchDeviceTokenParams) error {
	if err := s.queries.TouchDeviceToken(ctx, arg); err != nil {
		return fmt.Errorf("devices: touch token: %w", err)
	}
	return nil
}

func (s *Store) GetTokenByDeviceID(ctx context.Context, arg gen.GetDeviceTokenByDeviceIDParams) (gen.DeviceToken, error) {
	row, err := s.queries.GetDeviceTokenByDeviceID(ctx, arg)
	if err != nil {
		return gen.DeviceToken{}, fmt.Errorf("devices: get token by device id: %w", err)
	}
	return row, nil
}
