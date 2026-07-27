package gdpr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for GDPR operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Consent Records
// ---------------------------------------------------------------------------

func (s *Store) recordConsent(ctx context.Context, p gen.RecordConsentParams) (gen.ConsentRecord, error) {
	row, err := s.write.RecordConsent(ctx, p)
	if err != nil {
		return gen.ConsentRecord{}, fmt.Errorf("recording consent: %w", err)
	}

	return row, nil
}

func (s *Store) getUserConsents(ctx context.Context, userID pgtype.UUID, consentType string) ([]gen.ConsentRecord, error) {
	rows, err := s.read.GetUserConsents(ctx, gen.GetUserConsentsParams{
		UserID:      userID,
		ConsentType: consentType,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user consents: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// GDPR Data Export Queries
// ---------------------------------------------------------------------------

func (s *Store) gdprExportUserContent(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GdprExportUserContentRow, error) {
	return s.read.GdprExportUserContent(ctx, gen.GdprExportUserContentParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserComments(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GdprExportUserCommentsRow, error) {
	return s.read.GdprExportUserComments(ctx, gen.GdprExportUserCommentsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserSpaces(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GdprExportUserSpacesRow, error) {
	return s.read.GdprExportUserSpaces(ctx, gen.GdprExportUserSpacesParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserPayments(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GdprExportUserPaymentsRow, error) {
	return s.read.GdprExportUserPayments(ctx, gen.GdprExportUserPaymentsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserNotifications(ctx context.Context, userID, tenantID pgtype.UUID) ([]gen.GdprExportUserNotificationsRow, error) {
	return s.read.GdprExportUserNotifications(ctx, gen.GdprExportUserNotificationsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

// ---------------------------------------------------------------------------
// GDPR Data Export — Additional User-Owned Tables (AUDIT-FRESH-GDPR-002)
// Each query returns a JSON array; decodeJSONSlice normalizes the pgx-decoded
// value (raw []byte or decoded []interface{}) into []map[string]interface{}.
// ---------------------------------------------------------------------------

// decodeJSONSlice converts the value returned by the json_agg export queries
// into a slice of maps. pgx may return raw []byte or a decoded Go value
// depending on codec wiring, so both shapes are handled deterministically.
func decodeJSONSlice(v interface{}) ([]map[string]interface{}, error) {
	if v == nil {
		return []map[string]interface{}{}, nil
	}

	var raw []byte
	switch b := v.(type) {
	case []byte:
		raw = b
	case string:
		raw = []byte(b)
	default:
		j, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("normalizing export json: %w", err)
		}
		raw = j
	}

	var out []map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding export json slice: %w", err)
	}

	return out, nil
}

// exportJSONSlice runs one of the json_agg export queries and decodes the
// result into a slice of maps, returning an empty slice when the user has no
// rows so the export payload is always well-formed. It is generic because each
// generated query has its own params struct type.
func exportJSONSlice[P any](
	ctx context.Context,
	query func(context.Context, P) (interface{}, error),
	params P,
) ([]map[string]interface{}, error) {
	data, err := query(ctx, params)
	if err != nil {
		return nil, err
	}
	return decodeJSONSlice(data)
}

func (s *Store) gdprExportUserProfiles(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfiles, gen.GdprExportUserProfilesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserReactions(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserReactions, gen.GdprExportUserReactionsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserBookmarks(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserBookmarks, gen.GdprExportUserBookmarksParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserContentShares(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserContentShares, gen.GdprExportUserContentSharesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserFollows(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserFollows, gen.GdprExportUserFollowsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserForumThreads(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserForumThreads, gen.GdprExportUserForumThreadsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserForumReplies(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserForumReplies, gen.GdprExportUserForumRepliesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserForumVotes(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserForumVotes, gen.GdprExportUserForumVotesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserForumBookmarks(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserForumBookmarks, gen.GdprExportUserForumBookmarksParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserMedia(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMedia, gen.GdprExportUserMediaParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserReviews(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserReviews, gen.GdprExportUserReviewsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserSpaceMembers(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserSpaceMembers, gen.GdprExportUserSpaceMembersParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserConsentRecords(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserConsentRecords, gen.GdprExportUserConsentRecordsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserWalletLedger(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserWalletLedger, gen.GdprExportUserWalletLedgerParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserLessonProgress(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserLessonProgress, gen.GdprExportUserLessonProgressParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserLearningCertificates(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserLearningCertificates, gen.GdprExportUserLearningCertificatesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserMentorshipMatches(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMentorshipMatches, gen.GdprExportUserMentorshipMatchesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserProfessionalProfiles(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfessionalProfiles, gen.GdprExportUserProfessionalProfilesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserOrganizations(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserOrganizations, gen.GdprExportUserOrganizationsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserApiKeys(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserApiKeys, gen.GdprExportUserApiKeysParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserPreferences(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserPreferences, gen.GdprExportUserPreferencesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserMutes(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMutes, gen.GdprExportUserMutesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserSpaceMutes(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserSpaceMutes, gen.GdprExportUserSpaceMutesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserTicketComments(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	// ticket_comments has no tenant_id column; the generated query takes only user_id.
	_ = tenantID
	data, err := s.read.GdprExportUserTicketComments(ctx, userID)
	if err != nil {
		return nil, err
	}
	return decodeJSONSlice(data)
}
func (s *Store) hasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error) {
	granted, err := s.read.HasConsent(ctx, gen.HasConsentParams{
		UserID:      userID,
		ConsentType: consentType,
	})
	if err != nil {
		return false, fmt.Errorf("checking consent: %w", err)
	}

	return granted, nil
}

func (s *Store) revokeConsent(ctx context.Context, p gen.RevokeConsentParams) error {
	if err := s.write.RevokeConsent(ctx, p); err != nil {
		return fmt.Errorf("revoking consent: %w", err)
	}

	return nil
}

func (s *Store) listConsentsByTenant(ctx context.Context, tenantID pgtype.UUID, consentType string, limit, offset int32) ([]gen.ConsentRecord, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListConsentsByTenant(ctx, gen.ListConsentsByTenantParams{
		TenantID:    tenantID,
		ConsentType: consentType,
		Q:           "",
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing consents by tenant: %w", err)
	}

	return rows, nil
}

func (s *Store) listConsentsByTenantFiltered(ctx context.Context, tenantID pgtype.UUID, consentType, q string, limit, offset int32) ([]gen.ConsentRecord, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListConsentsByTenant(ctx, gen.ListConsentsByTenantParams{
		TenantID:    tenantID,
		ConsentType: consentType,
		Q:           q,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing consents by tenant: %w", err)
	}

	return rows, nil
}

func (s *Store) deleteConsent(ctx context.Context, id, tenantID pgtype.UUID) (gen.ConsentRecord, error) {
	row, err := s.write.DeleteConsent(ctx, gen.DeleteConsentParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ConsentRecord{}, fmt.Errorf("deleting consent: %w", err)
	}

	return row, nil
}

// qPtr returns a pointer to s. An empty string yields a pointer to "" so the
// generated SQL filter `@q::text = ''` matches and the list returns all rows
// when no search term is supplied.
func qPtr(s string) *string {
	return &s
}

// ---------------------------------------------------------------------------
// Data Export Requests
// ---------------------------------------------------------------------------

func (s *Store) createDataExportRequest(ctx context.Context, p gen.CreateDataExportRequestParams) (gen.DataExportRequest, error) {
	row, err := s.write.CreateDataExportRequest(ctx, p)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("creating data export request: %w", err)
	}

	return row, nil
}

func (s *Store) getDataExportRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.DataExportRequest, error) {
	row, err := s.read.GetDataExportRequest(ctx, gen.GetDataExportRequestParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("getting data export request: %w", err)
	}

	return row, nil
}

// getLatestDataExportRequestByUser returns the caller's most recent export
// request, or (zero value, nil) if they have never requested one — "no
// pending export" is a valid state, not an error.
func (s *Store) getLatestDataExportRequestByUser(ctx context.Context, userID, tenantID pgtype.UUID) (gen.DataExportRequest, bool, error) {
	row, err := s.read.GetLatestDataExportRequestByUser(ctx, gen.GetLatestDataExportRequestByUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.DataExportRequest{}, false, nil
	}

	if err != nil {
		return gen.DataExportRequest{}, false, fmt.Errorf("getting latest data export request: %w", err)
	}

	return row, true, nil
}

func (s *Store) updateDataExportRequestStatus(ctx context.Context, p gen.UpdateDataExportRequestStatusParams) (gen.DataExportRequest, error) {
	row, err := s.write.UpdateDataExportRequestStatus(ctx, p)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("updating data export request status: %w", err)
	}

	return row, nil
}

func (s *Store) listPendingDataExportRequests(ctx context.Context, limit int32) ([]gen.DataExportRequest, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingDataExportRequests(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing pending data export requests: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Account Deletion Requests
// ---------------------------------------------------------------------------

func (s *Store) createAccountDeletionRequest(ctx context.Context, p gen.CreateAccountDeletionRequestParams) (gen.AccountDeletionRequest, error) {
	row, err := s.write.CreateAccountDeletionRequest(ctx, p)
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("creating account deletion request: %w", err)
	}

	return row, nil
}

func (s *Store) getAccountDeletionRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.AccountDeletionRequest, error) {
	row, err := s.read.GetAccountDeletionRequest(ctx, gen.GetAccountDeletionRequestParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("getting account deletion request: %w", err)
	}

	return row, nil
}

func (s *Store) updateAccountDeletionRequestStatus(ctx context.Context, p gen.UpdateAccountDeletionRequestStatusParams) (gen.AccountDeletionRequest, error) {
	row, err := s.write.UpdateAccountDeletionRequestStatus(ctx, p)
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("updating account deletion request status: %w", err)
	}

	return row, nil
}

func (s *Store) listPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.AccountDeletionRequest, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingDeletionRequests(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing pending deletion requests: %w", err)
	}

	return rows, nil
}

func (s *Store) anonymizeUser(ctx context.Context, p gen.AnonymizeUserParams) error {
	if err := s.write.AnonymizeUser(ctx, p); err != nil {
		return fmt.Errorf("anonymizing user: %w", err)
	}

	return nil
}

func (s *Store) deleteUserTokens(ctx context.Context, p gen.DeleteUserTokensParams) error {
	if err := s.write.DeleteUserTokens(ctx, p); err != nil {
		return fmt.Errorf("deleting user tokens: %w", err)
	}

	return nil
}

// The hard-delete mutations (ReassignContentAuthor, NullifyPaymentUserRefs,
// NullifyTicketSubmitter, HardDeleteUser) run via db.WithTx in the service so
// they share one transaction — see Service.hardDeleteUser.

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

func (s *Store) createAuditLog(ctx context.Context, p gen.CreateAuditLogParams) (gen.AuditLog, error) {
	row, err := s.write.CreateAuditLog(ctx, p)
	if err != nil {
		return gen.AuditLog{}, fmt.Errorf("creating audit log: %w", err)
	}

	return row, nil
}

func (s *Store) listAuditLogs(ctx context.Context, p gen.ListAuditLogsParams) ([]gen.AuditLog, error) {
	if p.PageLimit <= 0 || p.PageLimit > MaxPageSize {
		p.PageLimit = DefaultPageSize
	}

	rows, err := s.read.ListAuditLogs(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}

	return rows, nil
}
