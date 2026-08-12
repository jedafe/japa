package notifications

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
)

// PushStore provides database access for push subscriptions.
// Ref: japaconnect-engineering/sections/35-notification-system.md
type PushStore struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewPushStore creates a PushStore backed by the given connection pools.
func NewPushStore(read, write *pgxpool.Pool) *PushStore {
	return &PushStore{
		read:  gen.New(read),
		write: gen.New(write),
	}
}

// SavePushSubscription upserts a push subscription (idempotent on user_id + endpoint).
func (s *PushStore) SavePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint, p256dh, auth string) error {
	if err := s.write.SavePushSubscription(ctx, gen.SavePushSubscriptionParams{
		UserID:   userID,
		Endpoint: endpoint,
		P256dh:   p256dh,
		AuthKey:  auth,
	}); err != nil {
		return fmt.Errorf("upserting push subscription: %w", err)
	}
	return nil
}

// DeletePushSubscriptionByEndpoint removes a push subscription by endpoint.
func (s *PushStore) DeletePushSubscriptionByEndpoint(ctx context.Context, userID pgtype.UUID, endpoint string) error {
	if err := s.write.DeletePushSubscriptionByEndpoint(ctx, gen.DeletePushSubscriptionByEndpointParams{
		UserID:   userID,
		Endpoint: endpoint,
	}); err != nil {
		return fmt.Errorf("deleting push subscription by endpoint: %w", err)
	}
	return nil
}

// DeletePushSubscriptionByID removes a push subscription by its primary key.
// BUG-005: catalog route is DELETE /me/push-tokens/{id} (path-param id).
func (s *PushStore) DeletePushSubscriptionByID(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeletePushSubscription(ctx, id); err != nil {
		return fmt.Errorf("deleting push subscription by id: %w", err)
	}
	return nil
}

// GetPushSubscriptionByID returns a push subscription by its primary key.
// Used for ownership checks before deletion (BUG-005).
func (s *PushStore) GetPushSubscriptionByID(ctx context.Context, id pgtype.UUID) (gen.GetPushSubscriptionByIDRow, error) {
	row, err := s.read.GetPushSubscriptionByID(ctx, id)
	if err != nil {
		return gen.GetPushSubscriptionByIDRow{}, fmt.Errorf("getting push subscription by id: %w", err)
	}
	return row, nil
}
