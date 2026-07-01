// Package referral — service.go provides business logic for referral program management.
package referral

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Program represents the referral program settings for a tenant.
type Program struct {
	ID                 pgtype.UUID      `json:"id"`
	TenantID           pgtype.UUID      `json:"tenant_id"`
	ReferrerRewardCents int32            `json:"referrer_reward_cents"`
	RefereeRewardCents  int32            `json:"referee_reward_cents"`
	MaxReferrals        int32            `json:"max_referrals"`
	CookieDays          int32            `json:"cookie_days"`
	IsActive            bool             `json:"is_active"`
	Description         string           `json:"description"`
	CreatedAt           pgtype.Timestamptz `json:"created_at"`
	UpdatedAt           pgtype.Timestamptz `json:"updated_at"`
}

// Referral represents a single referral record.
type Referral struct {
	ID            pgtype.UUID      `json:"id"`
	TenantID      pgtype.UUID      `json:"tenant_id"`
	ReferrerID    pgtype.UUID      `json:"referrer_id"`
	RefereeID     *pgtype.UUID     `json:"referee_id,omitempty"`
	ReferralCode  string           `json:"referral_code"`
	Status        string           `json:"status"`
	RewardPaidAt  *pgtype.Timestamptz `json:"reward_paid_at,omitempty"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
}

// Service coordinates referral program configuration and referral tracking.
type Service struct {
	db *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{db: database}
}

// GetProgram returns the referral program settings for the tenant.
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
	if err != nil {
		return Program{}, fmt.Errorf("fetching referral program: %w", err)
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
