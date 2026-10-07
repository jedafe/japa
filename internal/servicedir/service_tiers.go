// Package servicedir — service_tiers.go implements the Services Marketplace
// package pricing-tier surface: the tiers[] input on package create/edit
// (replace-on-save, same semantics as package items), the tiers[] payload on
// the public package detail, and the min_price list row. Gated on the
// service_packages.tiers_enabled feature flag — when off, input.Tiers is
// silently ignored and the payload fields stay empty.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§2.4)
package servicedir

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ErrInvalidTiers — malformed tiers[] payload on package create/edit
// (400 invalid_tiers): missing name, negative price, more than one featured
// tier, or the set exceeding service_packages.tiers.max_count. Distinct
// from ErrInvalidTier — the booking-time tier reference sentinel (400
// invalid_tier) raised when a booking names an unknown/inactive/foreign
// tier row.
var ErrInvalidTiers = errors.New("invalid_tiers")

// TierInput is one tier in the create/edit tiers[] payload. The whole slice
// is replace-on-save (same semantics as items[]): the incoming set deletes
// and reinserts all rows, position = array index. is_active lets the editor
// hide a tier without deleting it; absent fields take column defaults.
type TierInput struct {
	Name          string   `json:"name"`
	Badge         string   `json:"badge"`
	Description   string   `json:"description"`
	PriceAmount   *int64   `json:"price_amount"` // minor units; 0 is a valid (free) tier
	PriceCurrency string   `json:"price_currency"`
	Features      []string `json:"features"`
	IsFeatured    *bool    `json:"is_featured"`
	IsActive      *bool    `json:"is_active"`
}

// PackageTierDTO is the public detail shape for one pricing tier. Only
// active rows are exposed (ListServicePackageTiers); is_active is an
// input-only field and never serializes.
type PackageTierDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Badge         string   `json:"badge,omitempty"`
	Description   string   `json:"description,omitempty"`
	PriceAmount   int64    `json:"price_amount"`
	PriceCurrency string   `json:"price_currency,omitempty"`
	Features      []string `json:"features"`
	Position      int32    `json:"position"`
	IsFeatured    bool     `json:"is_featured"`
}

// TiersEnabled reads the service_packages.tiers_enabled flag — gates the
// tiers[] write field, the detail tiers[] payload, and the booking tier_id
// reference. Independent of the packages flag so a marketplace listing can
// ship flat pricing while tiers stay dark-launched.
func (s *Service) TiersEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "service_packages.tiers_enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "service_packages.tiers_enabled", false)
}

// validatePackageTiers validates the tiers[] payload and converts it to
// insert params (position = array index). Every rejection wraps
// ErrInvalidTiers so the error envelope maps to 400 invalid_tiers — a plain
// fmt.Errorf would surface as a 500.
func (s *Service) validatePackageTiers(ctx context.Context, tiers []TierInput) ([]gen.CreateServicePackageTierParams, error) {
	if len(tiers) == 0 {
		return nil, nil
	}

	maxTiers := int32(s.settings.GetIntOr(ctx, "service_packages.tiers.max_count", 5))
	if int32(len(tiers)) > maxTiers { //nolint:gosec // slice length is bounded by request body
		return nil, fmt.Errorf("tiers: exceeds service_packages.tiers.max_count (%d): %w", maxTiers, ErrInvalidTiers)
	}

	featured := 0
	params := make([]gen.CreateServicePackageTierParams, 0, len(tiers))

	for i, t := range tiers {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("tiers[%d]: name is required: %w", i, ErrInvalidTiers)
		}

		if t.PriceAmount == nil || *t.PriceAmount < 0 {
			return nil, fmt.Errorf("tiers[%d]: price_amount must be >= 0: %w", i, ErrInvalidTiers)
		}

		isFeatured := t.IsFeatured != nil && *t.IsFeatured
		if isFeatured {
			featured++
			if featured > 1 {
				return nil, fmt.Errorf("tiers[%d]: at most one tier may be featured: %w", i, ErrInvalidTiers)
			}
		}

		var isActive interface{}
		if t.IsActive != nil {
			isActive = *t.IsActive
		}

		features := t.Features
		if features == nil {
			features = []string{}
		}

		params = append(params, gen.CreateServicePackageTierParams{
			Name:          name,
			Badge:         nilIfEmpty(&t.Badge),
			Description:   nilIfEmpty(&t.Description),
			PriceAmount:   *t.PriceAmount,
			PriceCurrency: nilIfEmpty(&t.PriceCurrency),
			Features:      features,
			Position:      int32(i), //nolint:gosec // bounded by max_count check
			IsFeatured:    isFeatured,
			IsActive:      isActive,
		})
	}

	return params, nil
}

// tierToDTO maps one active tier row to the public detail shape.
func tierToDTO(t gen.ServicePackageTier) PackageTierDTO {
	return PackageTierDTO{
		ID:            t.ID.String(),
		Name:          t.Name,
		Badge:         strOrEmpty(t.Badge),
		Description:   strOrEmpty(t.Description),
		PriceAmount:   t.PriceAmount,
		PriceCurrency: strOrEmpty(t.PriceCurrency),
		Features:      t.Features,
		Position:      t.Position,
		IsFeatured:    t.IsFeatured,
	}
}

// ListPackageTiers returns the listing's active pricing tiers in position
// order for the detail payload.
func (s *Service) listPackageTiers(ctx context.Context, metaID pgtype.UUID) ([]PackageTierDTO, error) {
	rows, err := s.store.listPackageTiers(ctx, metaID)
	if err != nil {
		return nil, err
	}

	out := make([]PackageTierDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, tierToDTO(r))
	}

	return out, nil
}
