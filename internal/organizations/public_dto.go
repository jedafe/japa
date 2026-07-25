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

// OrgMemberUser is the lean public user shape embedded in a member row.
type OrgMemberUser struct {
	ID          pgtype.UUID `json:"id"`
	Username    string      `json:"username"`
	DisplayName *string     `json:"display_name"`
	AvatarURL   *string     `json:"avatar_url"`
}

// OrgMemberResponse is the lean public shape for the Members tab: the member's
// public user identity plus their org role. Omits invited_by and organization_id.
type OrgMemberResponse struct {
	User     OrgMemberUser      `json:"user"`
	Role     string             `json:"role"`
	JoinedAt pgtype.Timestamptz `json:"joined_at"`
}

func toOrgMembers(rows []gen.ListPublicOrganizationMembersRow) []OrgMemberResponse {
	out := make([]OrgMemberResponse, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = OrgMemberResponse{
			User: OrgMemberUser{
				ID:          r.UserID,
				Username:    r.Username,
				DisplayName: r.DisplayName,
				AvatarURL:   r.AvatarUrl,
			},
			Role:     r.Role,
			JoinedAt: r.JoinedAt,
		}
	}
	return out
}

// PublicOrgListItem is the lean shape for the public Discover org list.
type PublicOrgListItem struct {
	ID            pgtype.UUID        `json:"id"`
	Name          string             `json:"name"`
	Slug          string             `json:"slug"`
	Tagline       *string            `json:"tagline"`
	OrgType       string             `json:"org_type"`
	LogoMediaID   pgtype.UUID        `json:"logo_media_id"`
	HeaderMediaID pgtype.UUID        `json:"header_media_id"`
	CountryCode   *string            `json:"country_code"`
	Verified      bool               `json:"verified"`
	FollowerCount int32              `json:"follower_count"`
	MemberCount   int32              `json:"member_count"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
}

func toPublicOrgList(rows []gen.ListPublicOrganizationsRow) []PublicOrgListItem {
	out := make([]PublicOrgListItem, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = PublicOrgListItem{
			ID:            r.ID,
			Name:          r.Name,
			Slug:          r.Slug,
			Tagline:       r.Tagline,
			OrgType:       r.OrgType,
			LogoMediaID:   r.LogoMediaID,
			HeaderMediaID: r.HeaderMediaID,
			CountryCode:   r.CountryCode,
			Verified:      r.Verified,
			FollowerCount: r.FollowerCount,
			MemberCount:   r.MemberCount,
			CreatedAt:     r.CreatedAt,
		}
	}
	return out
}
