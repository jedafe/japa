package organizations

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// PublicOrgResponse is the lean public shape for the org profile header. It
// omits internal columns the raw gen.Organization row carries (tenant_id,
// owner_user_id, field_values, status, deleted_at, updated_at).
type PublicOrgResponse struct {
	ID               pgtype.UUID        `json:"id"`
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Tagline          *string            `json:"tagline"`
	Description      *string            `json:"description"`
	OrgType          string             `json:"org_type"`
	LogoMediaID      pgtype.UUID        `json:"logo_media_id"`
	HeaderMediaID    pgtype.UUID        `json:"header_media_id"`
	WebsiteURL       *string            `json:"website_url"`
	CountryCode      *string            `json:"country_code"`
	Verified         bool               `json:"verified"`
	VerificationType *string            `json:"verification_type"`
	FollowerCount    int32              `json:"follower_count"`
	MemberCount      int32              `json:"member_count"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
}

func toPublicOrg(o gen.Organization) PublicOrgResponse {
	return PublicOrgResponse{
		ID:               o.ID,
		Name:             o.Name,
		Slug:             o.Slug,
		Tagline:          o.Tagline,
		Description:      o.Description,
		OrgType:          o.OrgType,
		LogoMediaID:      o.LogoMediaID,
		HeaderMediaID:    o.HeaderMediaID,
		WebsiteURL:       o.WebsiteUrl,
		CountryCode:      o.CountryCode,
		Verified:         o.Verified,
		VerificationType: o.VerificationType,
		FollowerCount:    o.FollowerCount,
		MemberCount:      o.MemberCount,
		CreatedAt:        o.CreatedAt,
	}
}
