package payments

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// DiscountCodeValidation holds the result of validating a discount code.
type DiscountCodeValidation struct {
	Valid              bool
	DiscountType       string
	DiscountValue      float64
	ApplicableTo       string
	CalculatedDiscount int64
}

// ValidateAndCalculateDiscount validates a discount code and calculates the discount amount.
func (s *Service) ValidateAndCalculateDiscount(ctx context.Context, userID pgtype.UUID, code, purposeType string, amount int64) (DiscountCodeValidation, error) {
	discountCode, err := s.ValidateDiscountCode(ctx, userID, code, purposeType)
	if err != nil {
		return DiscountCodeValidation{Valid: false}, err
	}

	discountAmount := s.calculateDiscount(discountCode, amount)

	discountValue, _ := discountCode.DiscountValue.Float64Value()

	return DiscountCodeValidation{
		Valid:              true,
		DiscountType:       discountCode.DiscountType,
		DiscountValue:      discountValue.Float64,
		ApplicableTo:       discountCode.ApplicableTo,
		CalculatedDiscount: discountAmount,
	}, nil
}

// DiscountCodeUsage represents usage statistics for a discount code.
type DiscountCodeUsage struct {
	TotalUses     int
	UserUses      int32
	IsValid       bool
	IsExpired     bool
	UsageLimited  bool
	RemainingUses *int32
}

// GetDiscountCodeUsageStats returns usage statistics for a discount code.
func (s *Service) GetDiscountCodeUsageStats(ctx context.Context, codeID pgtype.UUID) (DiscountCodeUsage, error) {
	discountCode, err := s.store.getDiscountCodeByID(ctx, codeID)
	if err != nil {
		return DiscountCodeUsage{}, errors.New("discount code not found")
	}

	// Get total uses
	uses, err := s.store.listDiscountCodeUses(ctx, discountCode.ID, 1000, 0)
	if err != nil {
		return DiscountCodeUsage{}, fmt.Errorf("getting discount code uses: %w", err)
	}

	now := time.Now()
	isExpired := discountCode.ValidUntil.Valid && now.After(discountCode.ValidUntil.Time)
	isValid := !discountCode.ValidFrom.Valid || now.After(discountCode.ValidFrom.Time)

	if isExpired {
		isValid = false
	}

	usageLimited := discountCode.MaxUses != nil

	var remainingUses *int32

	if usageLimited {
		usesLen := len(uses)
		if usesLen > math.MaxInt32 {
			usesLen = math.MaxInt32
		}

		remaining := *discountCode.MaxUses - int32(usesLen) //nolint:gosec // G115: bounded by math.MaxInt32 check above
		if remaining < 0 {
			remaining = 0
		}

		remainingUses = &remaining
	}

	return DiscountCodeUsage{
		TotalUses:     len(uses),
		IsValid:       isValid,
		IsExpired:     isExpired,
		UsageLimited:  usageLimited,
		RemainingUses: remainingUses,
	}, nil
}
