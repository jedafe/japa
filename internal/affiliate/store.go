package affiliate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database access for the affiliate program.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

func newStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ── Member Operations ───────────────────────────────────────────────

func (s *Store) createMember(ctx context.Context, m gen.CreateAffiliateMemberParams) (gen.AffiliateProgramMember, error) {
	return s.write.CreateAffiliateMember(ctx, m)
}

func (s *Store) getMember(ctx context.Context, userID, tenantID pgtype.UUID) (gen.AffiliateProgramMember, error) {
	return s.read.GetAffiliateMember(ctx, gen.GetAffiliateMemberParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) listMembers(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.AffiliateProgramMember, error) {
	return s.read.ListAffiliateMembers(ctx, gen.ListAffiliateMembersParams{
		TenantID:   tenantID,
		Status:     status,
		PageOffset: offset,
		PageLimit:  limit,
	})
}

func (s *Store) updateMemberStatus(ctx context.Context, userID, tenantID pgtype.UUID, status string, notes *string) (gen.AffiliateProgramMember, error) {
	return s.write.UpdateAffiliateMemberStatus(ctx, gen.UpdateAffiliateMemberStatusParams{
		UserID: userID, TenantID: tenantID, Status: status, Notes: notes,
	})
}

func (s *Store) incrementMemberClicks(ctx context.Context, userID pgtype.UUID) error {
	return s.write.IncrementAffiliateMemberClicks(ctx, userID)
}

func (s *Store) incrementMemberConversions(ctx context.Context, userID pgtype.UUID, commission pgtype.Numeric) error {
	return s.write.IncrementAffiliateMemberConversions(ctx, gen.IncrementAffiliateMemberConversionsParams{
		UserID: userID, Commission: commission,
	})
}

// ── Link Operations ─────────────────────────────────────────────────

func (s *Store) createLink(ctx context.Context, l gen.CreateAffiliateLinkParams) (gen.UserAffiliateLink, error) {
	return s.write.CreateAffiliateLink(ctx, l)
}

func (s *Store) getLinkByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.UserAffiliateLink, error) {
	return s.read.GetAffiliateLinkByID(ctx, gen.GetAffiliateLinkByIDParams{ID: id, TenantID: tenantID})
}

func (s *Store) getLinkByCode(ctx context.Context, code string) (gen.UserAffiliateLink, error) {
	return s.read.GetAffiliateLinkByCode(ctx, code)
}

func (s *Store) listLinksByUser(ctx context.Context, userID, tenantID pgtype.UUID, isActive bool, limit, offset int32) ([]gen.UserAffiliateLink, error) {
	return s.read.ListAffiliateLinksByUser(ctx, gen.ListAffiliateLinksByUserParams{
		UserID: userID, TenantID: tenantID, IsActive: isActive, PageLimit: limit, PageOffset: offset,
	})
}

func (s *Store) updateLink(ctx context.Context, id, userID, tenantID pgtype.UUID, description *string, isActive bool) (gen.UserAffiliateLink, error) {
	return s.write.UpdateAffiliateLink(ctx, gen.UpdateAffiliateLinkParams{
		ID: id, UserID: userID, TenantID: tenantID, Description: description, IsActive: isActive,
	})
}

func (s *Store) deactivateLink(ctx context.Context, id, userID, tenantID pgtype.UUID) error {
	return s.write.DeactivateAffiliateLink(ctx, gen.DeactivateAffiliateLinkParams{
		ID: id, UserID: userID, TenantID: tenantID,
	})
}

func (s *Store) countActiveLinksByUser(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.read.CountActiveLinksByUser(ctx, userID)
}

func (s *Store) deactivateLinksByMemberStatus(ctx context.Context, userID pgtype.UUID) error {
	return s.write.DeactivateLinksByMemberStatus(ctx, userID)
}

// ── Event Operations ────────────────────────────────────────────────

func (s *Store) createEvent(ctx context.Context, e gen.CreateAffiliateLinkEventParams) (gen.AffiliateLinkEvent, error) {
	return s.write.CreateAffiliateLinkEvent(ctx, e)
}

func (s *Store) incrementLinkClicks(ctx context.Context, linkID pgtype.UUID) error {
	return s.write.IncrementAffiliateLinkClicks(ctx, linkID)
}

func (s *Store) incrementLinkConversions(ctx context.Context, linkID pgtype.UUID, commission pgtype.Numeric) error {
	return s.write.IncrementAffiliateLinkConversions(ctx, gen.IncrementAffiliateLinkConversionsParams{
		LinkID: linkID, Commission: commission,
	})
}

// ── Earnings ────────────────────────────────────────────────────────

func (s *Store) getUserEarnings(ctx context.Context, userID, tenantID pgtype.UUID) (gen.GetUserAffiliateEarningsRow, error) {
	return s.read.GetUserAffiliateEarnings(ctx, gen.GetUserAffiliateEarningsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) getUserEarningsByProduct(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GetUserAffiliateEarningsByProductRow, error) {
	return s.read.GetUserAffiliateEarningsByProduct(ctx, gen.GetUserAffiliateEarningsByProductParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) getRecentEvents(ctx context.Context, userID, tenantID pgtype.UUID, limit int32) ([]gen.GetRecentAffiliateEventsRow, error) {
	return s.read.GetRecentAffiliateEvents(ctx, gen.GetRecentAffiliateEventsParams{UserID: userID, TenantID: tenantID, PageLimit: limit})
}

// ── Helpers ─────────────────────────────────────────────────────────

func isNoRows(err error) bool {
	return err != nil && err == pgx.ErrNoRows
}

func toNullableUUID(u *pgtype.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return *u
}

func toTextFromRef(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ = fmt.Sprintf
