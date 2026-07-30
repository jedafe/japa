// Package referral — service.go provides business logic for referral program management.
package referral

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Program represents the referral program settings for a tenant.
type Program struct {
	ID                  pgtype.UUID        `json:"id"`
	TenantID            pgtype.UUID        `json:"tenant_id"`
	ReferrerRewardCents int32              `json:"referrer_reward_cents"`
	RefereeRewardCents  int32              `json:"referee_reward_cents"`
	MaxReferrals        int32              `json:"max_referrals"`
	CookieDays          int32              `json:"cookie_days"`
	IsActive            bool               `json:"is_active"`
	Description         string             `json:"description"`
	CreatedAt           pgtype.Timestamptz `json:"created_at"`
	UpdatedAt           pgtype.Timestamptz `json:"updated_at"`
}

// Referral represents a single referral record.
type Referral struct {
	ID           pgtype.UUID         `json:"id"`
	TenantID     pgtype.UUID         `json:"tenant_id"`
	ReferrerID   pgtype.UUID         `json:"referrer_id"`
	RefereeID    *pgtype.UUID        `json:"referee_id,omitempty"`
	ReferralCode string              `json:"referral_code"`
	Status       string              `json:"status"`
	RewardPaidAt *pgtype.Timestamptz `json:"reward_paid_at,omitempty"`
	CreatedAt    pgtype.Timestamptz  `json:"created_at"`
}

// Service coordinates referral program configuration and referral tracking.
type Service struct {
	db *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{db: database}
}

// GetProgram returns the referral program settings for the tenant. If no row
// exists yet (first admin visit), it seeds a default row using the column
// defaults from migration 000056 and returns it — so the admin UI always has
// a program to render instead of a 500.
func (s *Service) GetProgram(ctx context.Context, tenantID pgtype.UUID) (Program, error) {
	var p Program

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_reward_cents, referee_reward_cents,
		       max_referrals, cookie_days, is_active, description, created_at, updated_at
		FROM referral_programs
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Program{}, fmt.Errorf("fetching referral program: %w", err)
	}

	// Seed a default row (UNIQUE(tenant_id) makes this safe against races;
	// a concurrent insert surfaces as a unique violation we re-read past).
	return s.seedDefaultProgram(ctx, tenantID)
}

// seedDefaultProgram inserts a default referral_programs row for the tenant
// and returns it. On a unique-violation race it re-reads the existing row.
func (s *Service) seedDefaultProgram(ctx context.Context, tenantID pgtype.UUID) (Program, error) {
	var p Program

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO referral_programs (tenant_id)
		VALUES ($1)
		ON CONFLICT (tenant_id) DO NOTHING
		RETURNING id, tenant_id, referrer_reward_cents, referee_reward_cents,
		          max_referrals, cookie_days, is_active, description, created_at, updated_at
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Program{}, fmt.Errorf("seeding referral program: %w", err)
	}

	// ON CONFLICT DO NOTHING returned no rows — another request won the race.
	err = s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_reward_cents, referee_reward_cents,
		       max_referrals, cookie_days, is_active, description, created_at, updated_at
		FROM referral_programs
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&p.ID, &p.TenantID, &p.ReferrerRewardCents, &p.RefereeRewardCents,
		&p.MaxReferrals, &p.CookieDays, &p.IsActive, &p.Description,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Program{}, fmt.Errorf("re-reading seeded referral program: %w", err)
	}
	return p, nil
}

// UpdateProgram updates the referral program settings for the tenant.
func (s *Service) UpdateProgram(ctx context.Context, tenantID pgtype.UUID, referrerRewardCents, refereeRewardCents, maxReferrals, cookieDays *int32, isActive *bool, description *string) (Program, error) {
	// Fetch current values for merge.
	current, err := s.GetProgram(ctx, tenantID)
	if err != nil {
		return Program{}, err
	}

	rr := current.ReferrerRewardCents
	if referrerRewardCents != nil {
		rr = *referrerRewardCents
	}

	rf := current.RefereeRewardCents
	if refereeRewardCents != nil {
		rf = *refereeRewardCents
	}

	mr := current.MaxReferrals
	if maxReferrals != nil {
		mr = *maxReferrals
	}

	cd := current.CookieDays
	if cookieDays != nil {
		cd = *cookieDays
	}

	ia := current.IsActive
	if isActive != nil {
		ia = *isActive
	}

	desc := current.Description
	if description != nil {
		desc = *description
	}

	var updated Program

	err = s.db.Writer.QueryRow(ctx, `
		UPDATE referral_programs
		SET referrer_reward_cents = $2, referee_reward_cents = $3,
		    max_referrals = $4, cookie_days = $5, is_active = $6,
		    description = $7, updated_at = now()
		WHERE tenant_id = $1
		RETURNING id, tenant_id, referrer_reward_cents, referee_reward_cents,
		          max_referrals, cookie_days, is_active, description, created_at, updated_at
	`, tenantID, rr, rf, mr, cd, ia, desc).Scan(
		&updated.ID, &updated.TenantID, &updated.ReferrerRewardCents, &updated.RefereeRewardCents,
		&updated.MaxReferrals, &updated.CookieDays, &updated.IsActive, &updated.Description,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return Program{}, fmt.Errorf("updating referral program: %w", err)
	}

	return updated, nil
}

// ListReferrals returns all referrals for the tenant.
func (s *Service) ListReferrals(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Referral, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, referrer_id, referee_id, referral_code, status, reward_paid_at, created_at
		FROM referrals
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing referrals: %w", err)
	}
	defer rows.Close()

	var referrals []Referral

	for rows.Next() {
		var ref Referral
		if err := rows.Scan(&ref.ID, &ref.TenantID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status, &ref.RewardPaidAt, &ref.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning referral: %w", err)
		}

		referrals = append(referrals, ref)
	}

	return referrals, rows.Err()
}

// GetReferral returns a single referral by ID.
func (s *Service) GetReferral(ctx context.Context, tenantID, id pgtype.UUID) (Referral, error) {
	var ref Referral

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, referrer_id, referee_id, referral_code, status, reward_paid_at, created_at
		FROM referrals
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&ref.ID, &ref.TenantID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status, &ref.RewardPaidAt, &ref.CreatedAt)
	if err != nil {
		return Referral{}, fmt.Errorf("fetching referral %s: %w", id, err)
	}

	return ref, nil
}

// DeleteReferral soft-deletes a referral by setting status to 'expired'
// (Rule 11 — no hard-delete of user content in V1). The referrals table has
// no deleted_at column, so the existing 'expired' status value (valid per the
// CHECK constraint from migration 000056) is used to mark it inactive.
func (s *Service) DeleteReferral(ctx context.Context, tenantID, id pgtype.UUID) error {
	cmd, err := s.db.Writer.Exec(ctx, `
		UPDATE referrals
		SET status = 'expired'
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("deleting referral %s: %w", id, err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
