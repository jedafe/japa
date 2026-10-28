package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/opencollective"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tips"
)

// tipsPermAdapter adapts *rbac.RBAC to tips.PermChecker.
type tipsPermAdapter struct {
	rbac *rbac.RBAC
}

// Can delegates to HasPermission.
func (a tipsPermAdapter) Can(ctx context.Context, userID pgtype.UUID, node string) bool {
	if a.rbac == nil {
		return false
	}
	return a.rbac.HasPermission(ctx, userID, node)
}

// tipsOrgAdapter adapts *organizations.Service to tips.OrgRoleChecker.
type tipsOrgAdapter struct {
	svc *organizations.Service
}

// MemberRole delegates to the organizations service (roles: admin|editor|member).
func (a tipsOrgAdapter) MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error) {
	return a.svc.MemberRole(ctx, orgID, userID)
}

// OrganizationOwner resolves organizations.owner_user_id.
func (a tipsOrgAdapter) OrganizationOwner(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error) {
	org, err := a.svc.GetOrganization(ctx, orgID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return org.OwnerUserID, nil
}

// tipsOCAdapter adapts *opencollective.Client to tips.OCClient.
type tipsOCAdapter struct {
	client *opencollective.Client
}

// VerifyToken delegates to the OC client.
func (a tipsOCAdapter) VerifyToken(ctx context.Context, token string) (string, error) {
	return a.client.VerifyToken(ctx, token)
}

// ListContributions delegates and maps onto the tips-side struct.
func (a tipsOCAdapter) ListContributions(ctx context.Context, slug, token string, limit, offset int) ([]tips.OCContribution, error) {
	rows, err := a.client.ListContributions(ctx, slug, token, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]tips.OCContribution, 0, len(rows))
	for _, c := range rows {
		out = append(out, tips.OCContribution{
			ID: c.ID, AmountCents: c.AmountCents, Currency: c.Currency,
			FromName: c.FromName, FromSlug: c.FromSlug, FromEmail: c.FromEmail,
		})
	}
	return out, nil
}
