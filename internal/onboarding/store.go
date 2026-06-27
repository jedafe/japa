package onboarding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for onboarding state.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// getState returns the onboarding state for a user, or false if not found.
func (s *Store) getState(ctx context.Context, tenantID, userID pgtype.UUID) (gen.OnboardingState, error) {
	row, err := s.read.GetOnboardingState(ctx, gen.GetOnboardingStateParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.OnboardingState{}, fmt.Errorf("getting onboarding state: %w", err)
	}
	return row, nil
}

// upsertState inserts or updates the onboarding state.
func (s *Store) upsertState(ctx context.Context, tenantID, userID pgtype.UUID, currentStep int32, completed bool, stepData []byte, tourDismissed bool) error {
	if err := s.write.UpsertOnboardingState(ctx, gen.UpsertOnboardingStateParams{
		TenantID:      tenantID,
		UserID:        userID,
		CurrentStep:   currentStep,
		Completed:     completed,
		StepData:      stepData,
		TourDismissed: tourDismissed,
	}); err != nil {
		return fmt.Errorf("upserting onboarding state: %w", err)
	}
	return nil
}

// completeOnboarding marks the onboarding as completed.
func (s *Store) completeOnboarding(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.write.CompleteOnboarding(ctx, gen.CompleteOnboardingParams{
		TenantID: tenantID,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("completing onboarding: %w", err)
	}
	return nil
}

// dismissTour marks the product tour as dismissed.
func (s *Store) dismissTour(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.write.DismissTour(ctx, gen.DismissTourParams{
		TenantID: tenantID,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("dismissing tour: %w", err)
	}
	return nil
}
