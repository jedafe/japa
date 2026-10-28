package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/rbac"
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
