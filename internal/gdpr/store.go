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

// exportJSONSliceNoTenant is like exportJSONSlice but for queries that
// take only a userID (no tenant_id column on the target table).
func exportJSONSliceNoTenant(
	ctx context.Context,
	query func(context.Context, pgtype.UUID) (interface{}, error),
	userID pgtype.UUID,
) ([]map[string]interface{}, error) {
	data, err := query(ctx, userID)
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

func (s *Store) gdprExportUserBookmarkFolders(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserBookmarkFolders, gen.GdprExportUserBookmarkFoldersParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserContentShares(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserContentShares, gen.GdprExportUserContentSharesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserContentViews(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSliceNoTenant(ctx, s.read.GdprExportUserContentViews, userID)
}

// GdprDeleteUserContentViews deletes all user_content_views rows for the
// given user. Called during GDPR erasure (ADR-014).
func (s *Store) GdprDeleteUserContentViews(ctx context.Context, userID pgtype.UUID) error {
	return s.write.GdprDeleteUserContentViews(ctx, userID)
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

// BUG-022: the following three export wrappers cover forum tables that were
// missing from the original GDPR export. Ref: Section 08 + GDPR erasure model.

func (s *Store) gdprExportUserForumPollVotes(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	// forum_poll_votes has no tenant_id column; the query takes only user_id.
	data, err := s.read.GdprExportUserForumPollVotes(ctx, userID)
	if err != nil {
		return nil, err
	}
	return decodeJSONSlice(data)
}

func (s *Store) gdprExportUserReplyViews(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	// reply_views has no tenant_id column; the query takes only user_id.
	data, err := s.read.GdprExportUserReplyViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	return decodeJSONSlice(data)
}

func (s *Store) gdprExportUserForumThreadTags(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserForumThreadTags, gen.GdprExportUserForumThreadTagsParams{UserID: userID, TenantID: tenantID})
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

func (s *Store) gdprExportUserSpaceModerators(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserSpaceModerators, gen.GdprExportUserSpaceModeratorsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserTagModerators(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserTagModerators, gen.GdprExportUserTagModeratorsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserConsentRecords(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserConsentRecords, gen.GdprExportUserConsentRecordsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserWalletLedger(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserWalletLedger, gen.GdprExportUserWalletLedgerParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserActivityEvents(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserActivityEvents, gen.GdprExportUserActivityEventsParams{UserID: userID, TenantID: tenantID})
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

// gdprExportUserMentorshipConnections exports §102 mentorship_connections
// rows where the user is mentor or mentee (ADR-110).
func (s *Store) gdprExportUserMentorshipConnections(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMentorshipConnections, gen.GdprExportUserMentorshipConnectionsParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserMentorProfiles exports §102 mentor_profiles rows for the
// user (ADR-110).
func (s *Store) gdprExportUserMentorProfiles(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMentorProfiles, gen.GdprExportUserMentorProfilesParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserMentorSessions exports §102 mentor_sessions rows where the
// user is the mentee (ADR-110).
func (s *Store) gdprExportUserMentorSessions(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserMentorSessions, gen.GdprExportUserMentorSessionsParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserMentorReviews exports §102 mentor_reviews rows written by
// the user (ADR-110). mentor_reviews has no tenant_id column, so this uses
// the no-tenant variant.
func (s *Store) gdprExportUserMentorReviews(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSliceNoTenant(ctx, s.read.GdprExportUserMentorReviews, userID)
}

func (s *Store) gdprExportUserProfessionalProfiles(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfessionalProfiles, gen.GdprExportUserProfessionalProfilesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserProfessionalRelocations(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfessionalRelocations, gen.GdprExportUserProfessionalRelocationsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserProfessionalDocuments(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfessionalDocuments, gen.GdprExportUserProfessionalDocumentsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserProfessionalVerifiers(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserProfessionalVerifiers, gen.GdprExportUserProfessionalVerifiersParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserOrganizations(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserOrganizations, gen.GdprExportUserOrganizationsParams{UserID: userID, TenantID: tenantID})
}

// gdprExportAttestationCommunityUpdates exports §97 attestation_community_updates
// rows authored by the user (BUG-015).
func (s *Store) gdprExportAttestationCommunityUpdates(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportAttestationCommunityUpdates, gen.GdprExportAttestationCommunityUpdatesParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserCalendarEntries exports §86 calendar_entries submitted by
// the user (BUG-025).
func (s *Store) gdprExportUserCalendarEntries(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserCalendarEntries, gen.GdprExportUserCalendarEntriesParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserCalendarSaves exports §86 calendar_saves for the user (BUG-025).
func (s *Store) gdprExportUserCalendarSaves(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserCalendarSaves, gen.GdprExportUserCalendarSavesParams{UserID: userID, TenantID: tenantID})
}

// gdprExportUserCalendarSubscriptions exports §86 calendar_subscriptions for
// the user (BUG-025).
func (s *Store) gdprExportUserCalendarSubscriptions(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserCalendarSubscriptions, gen.GdprExportUserCalendarSubscriptionsParams{UserID: userID, TenantID: tenantID})
}

// softDeleteAttestationCommunityUpdatesByUser soft-deletes §97
// attestation_community_updates authored by the user during GDPR erasure
// (BUG-015).
func (s *Store) softDeleteAttestationCommunityUpdatesByUser(ctx context.Context, userID pgtype.UUID) error {
	return s.write.SoftDeleteAttestationCommunityUpdatesByUser(ctx, userID)
}

// softDeleteCredentialCommunityNotesByUser soft-deletes §94
// credential_community_notes authored by the user during GDPR erasure.
// Sets deleted_at = now() to preserve the note text while removing the
// user's PII association.
func (s *Store) softDeleteCredentialCommunityNotesByUser(ctx context.Context, userID pgtype.UUID) error {
	return s.write.SoftDeleteCredentialCommunityNotesByUser(ctx, userID)
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

// Comparison GDPR export helpers (BUG-004). These export the linked
// comparison_items/dimensions/values for content authored by the user.
func (s *Store) gdprExportUserComparisonItems(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserComparisonItems, gen.GdprExportUserComparisonItemsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserComparisonDimensions(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserComparisonDimensions, gen.GdprExportUserComparisonDimensionsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportUserComparisonValues(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserComparisonValues, gen.GdprExportUserComparisonValuesParams{UserID: userID, TenantID: tenantID})
}

// Japa University GDPR export helpers (ADR-063 / audit C2).
func (s *Store) gdprExportInstructorProfiles(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportInstructorProfiles, gen.GdprExportInstructorProfilesParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportRevenueSplits(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportRevenueSplits, gen.GdprExportRevenueSplitsParams{UserID: userID, TenantID: tenantID})
}

func (s *Store) gdprExportInstructorPayouts(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportInstructorPayouts, gen.GdprExportInstructorPayoutsParams{UserID: userID, TenantID: tenantID})
}

// gdprExportCustomFieldValues exports custom_field_values for the user's
// profile entity (BUG-020). The query takes only userID (no tenantID)
// because custom_field_values is polymorphic on (entity_type, entity_id)
// and the user's ID is the entity_id.
func (s *Store) gdprExportCustomFieldValues(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportCustomFieldValues, userID)
}

// gdprExportPushTokens exports push_tokens for the user (BUG-014).
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
func (s *Store) gdprExportPushTokens(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportPushTokens, gen.GdprExportPushTokensParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

// BUG-022: GDPR export wrappers for DM tables. Each query returns a JSON
// array; decodeJSONSlice normalizes the pgx-decoded value.
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md

func (s *Store) gdprExportUserDMConversations(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserDMConversations, gen.GdprExportUserDMConversationsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserDMMessages(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserDMMessages, gen.GdprExportUserDMMessagesParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserDMRequests(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserDMRequests, gen.GdprExportUserDMRequestsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

func (s *Store) gdprExportUserDMSettings(ctx context.Context, userID, tenantID pgtype.UUID) ([]map[string]interface{}, error) {
	return exportJSONSlice(ctx, s.read.GdprExportUserDMSettings, gen.GdprExportUserDMSettingsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

// gdprExportUserBlocks exports user_blocks where the user is blocker or
// blocked. The user_blocks table has no tenant_id column; the query takes
// only user_id.
func (s *Store) gdprExportUserBlocks(ctx context.Context, userID pgtype.UUID) ([]map[string]interface{}, error) {
	data, err := s.read.GdprExportUserBlocks(ctx, userID)
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

func (s *Store) listConsentsByTenant(ctx context.Context, tenantID pgtype.UUID, consentType string, limit, offset int32) ([]gen.ListConsentsByTenantRow, error) {
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

func (s *Store) listConsentsByTenantFiltered(ctx context.Context, tenantID pgtype.UUID, consentType, q string, limit, offset int32) ([]gen.ListConsentsByTenantRow, error) {
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
// generated SQL filter `@q::text = ”` matches and the list returns all rows
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

// updateAccountDeletionRequestProgress checkpoints the erasure flow step
// (AUDIT-FRESH-GDPR-003) so a retried job resumes from the last success.
func (s *Store) updateAccountDeletionRequestProgress(ctx context.Context, id, tenantID pgtype.UUID, step int32) error {
	if err := s.write.UpdateAccountDeletionRequestProgress(ctx, gen.UpdateAccountDeletionRequestProgressParams{
		ID: id, TenantID: tenantID, ProgressStep: step,
	}); err != nil {
		return fmt.Errorf("checkpointing deletion progress to step %d: %w", step, err)
	}
	return nil
}

func (s *Store) listPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.ListPendingDeletionRequestsRow, error) {
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

// anonymizeUserProfile clears PII columns in user_profiles (display_name, bio,
// timezone, business info, content_country_preference, preferred_currency,
// avatar/cover URLs) during the GDPR retention window (ADR-014 / ADR-057).
// Called immediately after anonymizeUser so the profile is scrubbed before
// the hard-delete phase.
func (s *Store) anonymizeUserProfile(ctx context.Context, p gen.AnonymizeUserProfileParams) error {
	if err := s.write.AnonymizeUserProfile(ctx, p); err != nil {
		return fmt.Errorf("anonymizing user profile: %w", err)
	}

	return nil
}

// anonymizeAffiliateLinkEvents scrubs ip_hash/user_agent_hash PII from
// affiliate_link_events owned by the deleted user's affiliate links (Section
// 56, ADR-014). Best-effort: callers log a warning on failure but do not
// abort the erasure.
func (s *Store) anonymizeAffiliateLinkEvents(ctx context.Context, p gen.AnonymizeAffiliateLinkEventsParams) error {
	if err := s.write.AnonymizeAffiliateLinkEvents(ctx, p); err != nil {
		return fmt.Errorf("anonymizing affiliate link events: %w", err)
	}
	return nil
}

// anonymizeCouponEvents scrubs user_id PII from coupon_events recorded by the
// deleted user during the GDPR retention window (Section 55, ADR-014). The
// events are retained for aggregate click/copy analytics; only the user
// association is cleared. Best-effort: callers log a warning on failure but do
// not abort the erasure.
func (s *Store) anonymizeCouponEvents(ctx context.Context, userID pgtype.UUID) error {
	if err := s.write.AnonymizeCouponEvents(ctx, userID); err != nil {
		return fmt.Errorf("anonymizing coupon events: %w", err)
	}
	return nil
}

func (s *Store) deleteUserTokens(ctx context.Context, p gen.DeleteUserTokensParams) error {
	if err := s.write.DeleteUserTokens(ctx, p); err != nil {
		return fmt.Errorf("deleting user tokens: %w", err)
	}

	return nil
}

// Auth PII scrub methods (AUDIT-AUTH-008). These delete auth-related PII
// tables during the GDPR anonymization phase. Ref: sections/02-authentication-sessions.md.

func (s *Store) deleteUserTOTPCredentials(ctx context.Context, p gen.DeleteUserTOTPCredentialsParams) error {
	if err := s.write.DeleteUserTOTPCredentials(ctx, p); err != nil {
		return fmt.Errorf("deleting user TOTP credentials: %w", err)
	}
	return nil
}

func (s *Store) deleteUserOAuthAccounts(ctx context.Context, p gen.DeleteUserOAuthAccountsParams) error {
	if err := s.write.DeleteUserOAuthAccounts(ctx, p); err != nil {
		return fmt.Errorf("deleting user OAuth accounts: %w", err)
	}
	return nil
}

func (s *Store) deleteUserPasswordResetTokens(ctx context.Context, p gen.DeleteUserPasswordResetTokensParams) error {
	if err := s.write.DeleteUserPasswordResetTokens(ctx, p); err != nil {
		return fmt.Errorf("deleting user password reset tokens: %w", err)
	}
	return nil
}

func (s *Store) deleteUserEmailVerifications(ctx context.Context, p gen.DeleteUserEmailVerificationsParams) error {
	if err := s.write.DeleteUserEmailVerifications(ctx, p); err != nil {
		return fmt.Errorf("deleting user email verifications: %w", err)
	}
	return nil
}

func (s *Store) deleteUserPasswordHistory(ctx context.Context, p gen.DeleteUserPasswordHistoryParams) error {
	if err := s.write.DeleteUserPasswordHistory(ctx, p); err != nil {
		return fmt.Errorf("deleting user password history: %w", err)
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
