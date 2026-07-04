package notifications

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PushSubscription represents a stored browser push subscription.
type PushSubscription struct {
	ID       pgtype.UUID
	UserID   pgtype.UUID
	Endpoint string
	P256DH   string
	Auth     string
}

// PushStore provides database access for push subscriptions.
type PushStore struct {
	write *pgxpool.Pool
	read  *pgxpool.Pool
}

// NewPushStore creates a PushStore backed by the given connection pools.
func NewPushStore(read, write *pgxpool.Pool) *PushStore {
	return &PushStore{read: read, write: write}
}

// GetPushSubscriptions returns all push subscriptions for a user.
func (s *PushStore) GetPushSubscriptions(ctx context.Context, userID pgtype.UUID) ([]PushSubscription, error) {
	rows, err := s.read.Query(ctx,
		`SELECT id, user_id, endpoint, p256dh, auth_key
		 FROM push_subscriptions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying push subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []PushSubscription

	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256DH, &sub.Auth); err != nil {
			return nil, fmt.Errorf("scanning push subscription: %w", err)
		}

		subs = append(subs, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating push subscriptions: %w", err)
	}

	return subs, nil
}

// SavePushSubscription upserts a push subscription (idempotent on user_id + endpoint).
func (s *PushStore) SavePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint, p256dh, auth string) error {
	_, err := s.write.Exec(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth_key)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, endpoint) DO UPDATE SET p256dh = EXCLUDED.p256dh, auth_key = EXCLUDED.auth_key`,
		userID, endpoint, p256dh, auth)
	if err != nil {
		return fmt.Errorf("upserting push subscription: %w", err)
	}

	return nil
}

// DeletePushSubscription removes a push subscription by endpoint.
func (s *PushStore) DeletePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint string) error {
	_, err := s.write.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`,
		userID, endpoint)
	if err != nil {
		return fmt.Errorf("deleting push subscription: %w", err)
	}

	return nil
}
