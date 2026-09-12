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
	row, err := s.read.GetOnboardingState(ctx,

		userID,
	)
	if err != nil {
		return gen.OnboardingState{}, fmt.Errorf("getting onboarding state: %w", err)
	}

	return row, nil
}

// upsertState inserts or updates the onboarding state.
func (s *Store) upsertState(ctx context.Context, tenantID, userID pgtype.UUID, currentStep int32, completed bool, stepData []byte, tourDismissed bool) error {
	if err := s.write.UpsertOnboardingState(ctx, gen.UpsertOnboardingStateParams{

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
	if err := s.write.CompleteOnboarding(ctx,

		userID,
	); err != nil {
		return fmt.Errorf("completing onboarding: %w", err)
	}

	return nil
}

// dismissTour marks the product tour as dismissed.
func (s *Store) dismissTour(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.write.DismissTour(ctx,

		userID,
	); err != nil {
		return fmt.Errorf("dismissing tour: %w", err)
	}

	return nil
}

// listTemplates returns all onboarding templates for a tenant.
func (s *Store) listTemplates(ctx context.Context, tenantID pgtype.UUID) ([]gen.OnboardingTemplate, error) {
	rows, err := s.read.ListOnboardingTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing onboarding templates: %w", err)
	}

	return rows, nil
}

// getTemplate retrieves a single onboarding template by ID.
func (s *Store) getTemplate(ctx context.Context, tenantID, id pgtype.UUID) (gen.OnboardingTemplate, error) {
	row, err := s.read.GetOnboardingTemplate(ctx,
		id,
	)
	if err != nil {
		return gen.OnboardingTemplate{}, fmt.Errorf("getting onboarding template: %w", err)
	}

	return row, nil
}

// createTemplate inserts a new onboarding template.
func (s *Store) createTemplate(ctx context.Context, tenantID pgtype.UUID, name, description string, steps []byte, isActive, isSystem bool) (gen.OnboardingTemplate, error) {
	row, err := s.write.CreateOnboardingTemplate(ctx, gen.CreateOnboardingTemplateParams{

		Name:        name,
		Description: &description,
		Steps:       steps,
		IsActive:    isActive,
		IsSystem:    isSystem,
	})
	if err != nil {
		return gen.OnboardingTemplate{}, fmt.Errorf("creating onboarding template: %w", err)
	}

	return row, nil
}

// updateTemplate updates an existing onboarding template.
func (s *Store) updateTemplate(ctx context.Context, tenantID, id pgtype.UUID, name, description string, steps []byte, isActive bool) error {
	return s.write.UpdateOnboardingTemplate(ctx, gen.UpdateOnboardingTemplateParams{

		ID:          id,
		Name:        name,
		Description: &description,
		Steps:       steps,
		IsActive:    isActive,
	})
}

// deleteTemplate removes an onboarding template.
func (s *Store) deleteTemplate(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.write.DeleteOnboardingTemplate(ctx,

		id,
	)
}
