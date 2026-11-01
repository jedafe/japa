package verification

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database access for verification requests. Thin wrapper over
// generated sqlc queries — read/write split mirrors internal/organizations.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a Store with the given read/write queries.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// CreateVerificationRequest inserts a new request row.
func (s *Store) CreateVerificationRequest(ctx context.Context, params gen.CreateVerificationRequestParams) (gen.VerificationRequest, error) {
	return s.write.CreateVerificationRequest(ctx, params)
}

// GetVerificationRequestByID returns a request joined with requester/org display fields.
func (s *Store) GetVerificationRequestByID(ctx context.Context, id pgtype.UUID) (gen.GetVerificationRequestByIDRow, error) {
	return s.read.GetVerificationRequestByID(ctx, id)
}

// GetPendingForOrg returns the pending request for an org, if any.
func (s *Store) GetPendingForOrg(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetPendingVerificationRequestForOrg(ctx, orgID)
}

// GetPendingForUser returns the caller's pending profile request, if any.
func (s *Store) GetPendingForUser(ctx context.Context, userID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetPendingVerificationRequestForUser(ctx, userID)
}

// ListByRequester returns all requests filed by a user, newest first.
func (s *Store) ListByRequester(ctx context.Context, userID pgtype.UUID) ([]gen.ListVerificationRequestsByRequesterRow, error) {
	return s.read.ListVerificationRequestsByRequester(ctx, userID)
}

// List returns the admin review queue filtered by status/entity_type, with
// requester/org search (?q=) and whitelisted CASE sort (?sort=/&sort_dir=).
func (s *Store) List(ctx context.Context, status, entityType, search, sort, sortDir string, limit, offset int32) ([]gen.ListVerificationRequestsRow, int64, error) {
	rows, err := s.read.ListVerificationRequests(ctx, gen.ListVerificationRequestsParams{
		Column1: status, Column2: entityType, Column3: search,
		Sort: sort, SortDir: sortDir, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.read.CountVerificationRequests(ctx, gen.CountVerificationRequestsParams{
		Column1: status, Column2: entityType, Column3: search,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Review marks a pending request approved/rejected. Returns rows affected.
func (s *Store) Review(ctx context.Context, params gen.ReviewVerificationRequestParams) (int64, error) {
	return s.write.ReviewVerificationRequest(ctx, params)
}

// Withdraw marks a pending request withdrawn by its requester. Returns rows affected.
func (s *Store) Withdraw(ctx context.Context, id, requesterID pgtype.UUID) (int64, error) {
	return s.write.WithdrawVerificationRequest(ctx, gen.WithdrawVerificationRequestParams{
		ID: id, RequesterUserID: requesterID,
	})
}

// GetOrganizationMemberRole returns the caller's role on an org (or no-rows).
func (s *Store) GetOrganizationMemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error) {
	return s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
		OrganizationID: orgID, UserID: userID,
	})
}

// ActiveOrgExistsByID reports whether the org exists, is active, and not deleted.
func (s *Store) ActiveOrgExistsByID(ctx context.Context, orgID pgtype.UUID) (bool, error) {
	return s.read.ActiveOrgExistsByID(ctx, orgID)
}

// GetOrganizationByIDOnly returns the org row (for verified/owner checks).
func (s *Store) GetOrganizationByIDOnly(ctx context.Context, orgID pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationByIDOnly(ctx, orgID)
}

// GetFormSubmissionByID returns a form submission (ownership + status check).
func (s *Store) GetFormSubmissionByID(ctx context.Context, id pgtype.UUID) (gen.GetFormSubmissionByIDRow, error) {
	return s.read.GetFormSubmissionByID(ctx, id)
}

// GetUserVerified reports whether a user profile already carries the badge.
func (s *Store) GetUserVerified(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return s.read.GetUserVerified(ctx, userID)
}

// GetUserEmail resolves a user's email for review-outcome notifications.
func (s *Store) GetUserEmail(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserEmail(ctx, userID)
}

// Reopen flips a rejected request back to pending (§22 appeal overturn).
func (s *Store) Reopen(ctx context.Context, id pgtype.UUID) error {
	return s.write.ReopenVerificationRequest(ctx, id)
}

// GetFormBySlug resolves a Form Builder form by slug (verification.form_slug).
func (s *Store) GetFormBySlug(ctx context.Context, slug string) (gen.GetFormBySlugRow, error) {
	return s.read.GetFormBySlug(ctx, slug)
}

// ---- verification_types taxonomy (001013) ----

// ListTypes returns taxonomy rows; empty scope lists all scopes.
func (s *Store) ListTypes(ctx context.Context, scope string) ([]gen.VerificationType, error) {
	var sc *string
	if scope != "" {
		sc = &scope
	}
	return s.read.ListVerificationTypes(ctx, sc)
}

// ListActiveTypeValues returns the active value set for a scope.
func (s *Store) ListActiveTypeValues(ctx context.Context, scope string) ([]string, error) {
	return s.read.ListActiveVerificationTypeValues(ctx, scope)
}

// TypeExists reports whether value is an active type in scope.
func (s *Store) TypeExists(ctx context.Context, scope, value string) (bool, error) {
	return s.read.VerificationTypeExists(ctx, gen.VerificationTypeExistsParams{Scope: scope, Value: value})
}

// GetTypeByID returns one taxonomy row.
func (s *Store) GetTypeByID(ctx context.Context, id pgtype.UUID) (gen.VerificationType, error) {
	return s.read.GetVerificationTypeByID(ctx, id)
}

// CreateType inserts a taxonomy row.
func (s *Store) CreateType(ctx context.Context, params gen.CreateVerificationTypeParams) (gen.VerificationType, error) {
	return s.write.CreateVerificationType(ctx, params)
}

// UpdateType patches a taxonomy row.
func (s *Store) UpdateType(ctx context.Context, params gen.UpdateVerificationTypeParams) (gen.VerificationType, error) {
	return s.write.UpdateVerificationType(ctx, params)
}

// DeleteType removes a taxonomy row. Returns rows affected.
func (s *Store) DeleteType(ctx context.Context, id pgtype.UUID) (int64, error) {
	return s.write.DeleteVerificationType(ctx, id)
}
