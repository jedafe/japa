package gdpr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// HonchoEraser deletes a user's AI context from the Honcho agent-memory
// service. The honcho.Client implements this interface. The erasure step in
// ProcessAccountDeletion calls DeleteUser and propagates the error so the
// Asynq task retries — Honcho deletion is mandatory, not best-effort.
type HonchoEraser interface {
	DeleteUser(ctx context.Context, userID string) error
}

// HonchoFetcher retrieves a user's AI context from the Honcho agent-memory
// service for data export (BUG-012). The honcho.Client satisfies this.
type HonchoFetcher interface {
	GetContext(ctx context.Context, userID string) ([]map[string]interface{}, error)
}

// SessionDeleter deletes user sessions from Valkey/Redis.
type SessionDeleter interface {
	DeleteSessions(ctx context.Context, userID string) error
}

// Service provides GDPR operations with reader/writer pool separation.
type Service struct {
	store          *Store
	db             *db.DB
	enqueuer       Enqueuer
	honchoEraser   HonchoEraser
	honchoFetcher  HonchoFetcher
	sessionDeleter SessionDeleter
	siteName       string
	settings       SettingsProvider
	storage        Storage
}

// SettingsProvider abstracts system settings access for hard-delete configuration.
type SettingsProvider interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingBool(ctx context.Context, key string) (bool, error)
	GetSettingInt(ctx context.Context, key string) (int, error)
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetStringOr(ctx context.Context, key, fallback string) string
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, siteName string) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:       database,
		enqueuer: &noopEnqueuer{},
		siteName: siteName,
	}
}

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer, siteName string) *Service {
	svc := NewService(database, siteName)
	svc.enqueuer = enqueuer

	return svc
}

// SetHonchoEraser injects the Honcho client used for AI context erasure
// during account deletion. When set, the erasure step must succeed or the
// deletion job retries. When nil (development), the step is skipped.
func (s *Service) SetHonchoEraser(h HonchoEraser) {
	s.honchoEraser = h
}

// SetHonchoFetcher injects the Honcho client used to read a user's AI
// context for GDPR data export (BUG-012). When nil, the export omits
// Honcho context.
func (s *Service) SetHonchoFetcher(h HonchoFetcher) {
	s.honchoFetcher = h
}

// SetSessionDeleter injects the session deleter for clearing Valkey sessions
// during account deletion. When nil (development), the step is skipped.
func (s *Service) SetSessionDeleter(d SessionDeleter) {
	s.sessionDeleter = d
}

// SetSettings injects the system settings provider for hard-delete configuration.
func (s *Service) SetSettings(sp SettingsProvider) {
	s.settings = sp
}

// SetStorage injects the storage backend for GDPR data export files
// (ADR-048). When unset, exports use the default LocalStorage rooted at
// data/gdpr-exports.
func (s *Service) SetStorage(st Storage) {
	s.storage = st
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
	return nil
}

// ---------------------------------------------------------------------------
// Consent Management
// ---------------------------------------------------------------------------

// RecordConsentInput holds parameters for recording a consent decision.
type RecordConsentInput struct {
	ConsentType   string
	Granted       bool
	PolicyVersion string
	IPAddress     *string
	UserAgent     *string
}

// RecordConsent records a user's consent decision.
func (s *Service) RecordConsent(ctx context.Context, userID pgtype.UUID, input RecordConsentInput) (gen.ConsentRecord, error) {
	if input.ConsentType == "" {
		return gen.ConsentRecord{}, errors.New("consent_type is required")
	}

	if !ValidConsentType(input.ConsentType) {
		return gen.ConsentRecord{}, fmt.Errorf("invalid consent_type: %s", input.ConsentType)
	}

	if input.PolicyVersion == "" {
		return gen.ConsentRecord{}, errors.New("policy_version is required")
	}

	return s.store.recordConsent(ctx, gen.RecordConsentParams{

		UserID:        userID,
		ConsentType:   input.ConsentType,
		Granted:       input.Granted,
		PolicyVersion: input.PolicyVersion,
		IpAddress:     parseIPAddress(input.IPAddress),
		UserAgent:     input.UserAgent,
	})
}

// GetUserConsents returns consent records for a user, optionally filtered by type.
func (s *Service) GetUserConsents(ctx context.Context, userID pgtype.UUID, consentType string) ([]gen.ConsentRecord, error) {
	return s.store.getUserConsents(ctx, userID, consentType)
}

// ConsentConfigResponse is the API payload returned by GET /consent/config.
// It surfaces the platform's valid consent types and the current privacy
// policy version so the frontend does not need to hardcode them
// (BUG-015, BUG-016).
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md.
type ConsentConfigResponse struct {
	ConsentTypes  []string `json:"consent_types"`
	PolicyVersion string   `json:"policy_version"`
}

// validConsentTypes is the canonical list of consent types recognized by the
// platform. Mirrors the values documented in Section 19 and enforced by the
// consent_records.consent_type column. These are platform constants — they
// are NOT admin-configurable, but surfacing them via the API keeps the
// frontend in sync if the set changes (BUG-015).
var validConsentTypes = []string{
	"marketing_email",
	"analytics_tracking",
	"third_party_sharing",
	"ai_processing",
}

// GetConsentConfig returns the valid consent types and the current privacy
// policy version for the platform. The policy version is sourced from
// system_settings key "gdpr.policy_version" (default "1.0") so admins can
// bump it without a code change (BUG-016).
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md.
func (s *Service) GetConsentConfig(ctx context.Context) (*ConsentConfigResponse, error) {
	policyVersion := "1.0"
	if s.settings != nil {
		// Use GetStringOr — system_settings values are stored as JSON-encoded
		// strings (e.g. '"1.0"'), so GetSetting returns the raw quoted value.
		// GetStringOr unquotes it to '1.0'. Ref: §6.2 nil/quote handling.
		policyVersion = s.settings.GetStringOr(ctx, "gdpr.policy_version", "1.0")
	}

	return &ConsentConfigResponse{
		ConsentTypes:  validConsentTypes,
		PolicyVersion: policyVersion,
	}, nil
}

// HasConsent checks whether a user has granted a specific consent.
func (s *Service) HasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error) {
	return s.store.hasConsent(ctx, userID, consentType)
}

// RevokeConsent records a consent revocation.
func (s *Service) RevokeConsent(ctx context.Context, userID pgtype.UUID, consentType, policyVersion string) error {
	if consentType == "" {
		return errors.New("consent_type is required")
	}

	if !ValidConsentType(consentType) {
		return fmt.Errorf("invalid consent_type: %s", consentType)
	}

	return s.store.revokeConsent(ctx, gen.RevokeConsentParams{

		UserID:        userID,
		ConsentType:   consentType,
		PolicyVersion: policyVersion,
	})
}

// ListConsents returns consent records for admin compliance reports.
func (s *Service) ListConsents(ctx context.Context, consentType string, limit, offset int32) ([]gen.ListConsentsRow, error) {
	return s.store.listConsentsByPlatform(ctx, consentType, limit, offset)
}

// ---------------------------------------------------------------------------
// Data Export
// ---------------------------------------------------------------------------

// RequestDataExport creates a data export request and enqueues a background job.
func (s *Service) RequestDataExport(ctx context.Context, userID pgtype.UUID) (gen.DataExportRequest, error) {
	request, err := s.store.createDataExportRequest(ctx, userID)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("creating data export request: %w", err)
	}

	// Enqueue background job.
	payload, marshalErr := json.Marshal(map[string]any{
		"request_id": request.ID.String(),

		"user_id": userID.String(),
	})
	if marshalErr == nil {
		if err := s.enqueuer.Enqueue(worker.TypeDataExport, payload); err != nil {
			slog.Error("enqueue data export", "error", err)
		}
	}

	return request, nil
}

// GetDataExportRequest retrieves a data export request by ID.
func (s *Service) GetDataExportRequest(ctx context.Context, id pgtype.UUID) (gen.DataExportRequest, error) {
	return s.store.getDataExportRequest(ctx, id)
}

// GetLatestDataExportRequest returns the caller's most recent export request.
// The bool is false if they have never requested one (catalog: GET
// /me/data-export/status).
func (s *Service) GetLatestDataExportRequest(ctx context.Context, userID pgtype.UUID) (gen.DataExportRequest, bool, error) {
	return s.store.getLatestDataExportRequestByUser(ctx, userID)
}

// CancelDataExportRequest cancels a pending data export request. Only
// requests with status "pending" or "processing" can be cancelled; once
// completed the data has already been generated and the download link
// should be allowed to expire naturally.
func (s *Service) CancelDataExportRequest(ctx context.Context, userID pgtype.UUID) (gen.DataExportRequest, error) {
	request, found, err := s.store.getLatestDataExportRequestByUser(ctx, userID)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("getting data export request: %w", err)
	}

	if !found {
		return gen.DataExportRequest{}, errors.New("no data export request to cancel")
	}

	if request.Status != string(ExportPending) && request.Status != string(ExportProcessing) {
		return gen.DataExportRequest{}, fmt.Errorf("cannot cancel export with status: %s", request.Status)
	}

	updated, err := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		Status:      string(ExportCancelled),
		DownloadUrl: nil,
		ExpiresAt:   pgtype.Timestamptz{},
		CompletedAt: pgtype.Timestamptz{},
		ID:          request.ID,
	})
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("cancelling data export request: %w", err)
	}

	return updated, nil
}

// ProcessDataExport collects user data, generates JSON, and stores a signed URL.
// This is called by the Asynq worker.
func (s *Service) ProcessDataExport(ctx context.Context, requestID pgtype.UUID) error {
	// Mark as processing.
	request, err := s.store.getDataExportRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("fetching export request: %w", err)
	}

	_, err = s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		ID:     requestID,
		Status: string(ExportProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking export as processing: %w", err)
	}

	// Collect user data from all relevant tables.
	userData, err := s.collectUserData(ctx, request.UserID)
	if err != nil {
		if _, statusErr := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
			ID:     requestID,
			Status: string(ExportFailed),
		}); statusErr != nil {
			slog.Error("update data export status to failed", "error", statusErr)
		}

		return fmt.Errorf("collecting user data: %w", err)
	}

	// Generate JSON export.
	exportJSON, err := json.MarshalIndent(userData, "", "  ")
	if err != nil {
		if _, statusErr := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
			ID:     requestID,
			Status: string(ExportFailed),
		}); statusErr != nil {
			slog.Error("update data export status to failed", "error", statusErr)
		}

		return fmt.Errorf("marshaling export data: %w", err)
	}

	// Store the export file via the injected Storage backend (ADR-048).
	// When no storage is configured, fall back to a marker URL.
	// AUDIT-FRESH-SETTING-008: export expiry is configurable via system_settings.
	exportExpiryDays := 1
	if s.settings != nil {
		exportExpiryDays = s.settings.GetIntOr(ctx, "gdpr.export_expiry_days", exportExpiryDays)
	}

	expiresAt := time.Now().Add(time.Duration(exportExpiryDays) * 24 * time.Hour)

	var exportURL string

	if s.storage != nil {
		url, _, storeErr := s.storage.Store(ctx, requestID, "application/json", exportJSON)
		if storeErr != nil {
			if _, statusErr := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
				ID:     requestID,
				Status: string(ExportFailed),
			}); statusErr != nil {
				slog.Error("update data export status to failed", "error", statusErr)
			}

			return fmt.Errorf("storing export file: %w", storeErr)
		}

		exportURL = url
	} else {
		exportURL = fmt.Sprintf("/exports/%s/data.json", request.UserID.String())
	}

	slog.Info("data export generated", "user_id", request.UserID.String(), "export_size", len(exportJSON), "url", exportURL)

	_, err = s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		ID:          requestID,
		Status:      string(ExportCompleted),
		DownloadUrl: &exportURL,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("marking export as completed: %w", err)
	}

	return nil
}

// ListPendingDataExportRequests returns pending export requests for the
// caller's platform (BUG-GDPR-002: platform-scoped so an admin in platform A
// cannot see platform B's pending export requests, §5.3 cross-platform bleed).
func (s *Service) ListPendingDataExportRequests(ctx context.Context, limit int32) ([]gen.DataExportRequest, error) {
	return s.store.listPendingDataExportRequests(ctx, limit)
}

// CleanupExpiredExports deletes files and nulls download URLs for completed
// data export requests whose ExpiresAt has passed (BUG-GDPR-005, Section 19
// time-limited download links). Called by the gdpr:retention_sweep cron.
// Best-effort: file deletion failures log a warning but the DB row is still
// expired so the download handler returns 404.
func (s *Service) CleanupExpiredExports(ctx context.Context) error {
	rq := s.store.read
	if rq == nil {
		return nil
	}

	expired, err := rq.ListExpiredDataExports(ctx)
	if err != nil {
		return fmt.Errorf("listing expired data exports: %w", err)
	}

	for _, e := range expired {
		// Delete the file from storage (best-effort). The LocalStorage
		// GetPath resolves the on-disk file; we remove it via os.Remove.
		if s.storage != nil {
			if path, pathErr := s.storage.GetPath(e); pathErr == nil {
				if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
					slog.Warn("gdpr: failed to delete expired export file", "request_id", e.String(), "error", rmErr)
				}
			}
		}

		// Null out the download URL and set status to 'expired' so the
		// download handler returns 404 and the row is not re-cleaned.
		if err := s.store.write.ExpireDataExport(ctx, e); err != nil {
			slog.Warn("gdpr: failed to expire data export row", "request_id", e.String(), "error", err)
		}
	}

	if len(expired) > 0 {
		slog.Info("gdpr: cleaned up expired data exports", "count", len(expired))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Account Deletion
// ---------------------------------------------------------------------------

// RequestAccountDeletion creates a deletion request and enqueues a background job.
func (s *Service) RequestAccountDeletion(ctx context.Context, userID pgtype.UUID, reason string) (gen.AccountDeletionRequest, error) {
	// ADR-068 compliance: persona backing accounts (is_bot = true) are system
	// accounts, not data subjects — user-erasure/export flows do not apply to
	// them. Erasing a persona backing account would cascade-delete the
	// ai_personas row and destroy the persona. Reject the request up front.
	isBot, err := s.store.read.GetUserIsBot(ctx,
		userID,
	)
	if err == nil && isBot {
		return gen.AccountDeletionRequest{}, fmt.Errorf("%w: cannot erase AI persona backing account (system account, not a data subject)", errpkg.ErrForbidden)
	}
	// If the user doesn't exist (pgx.ErrNoRows), let the create call below
	// surface the appropriate error — don't mask it here.

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	request, err := s.store.createAccountDeletionRequest(ctx, gen.CreateAccountDeletionRequestParams{

		UserID: userID,
		Reason: reasonPtr,
	})
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("creating account deletion request: %w", err)
	}

	// Enqueue background job.
	payload, marshalErr := json.Marshal(map[string]any{
		"request_id": request.ID.String(),

		"user_id": userID.String(),
	})
	if marshalErr == nil {
		if err := s.enqueuer.Enqueue(worker.TypeAccountDeletionCleanup, payload); err != nil {
			slog.Error("enqueue account deletion", "error", err)
		}
	}

	return request, nil
}

// GetAccountDeletionRequest retrieves a deletion request by ID.
func (s *Service) GetAccountDeletionRequest(ctx context.Context, id pgtype.UUID) (gen.AccountDeletionRequest, error) {
	return s.store.getAccountDeletionRequest(ctx, id)
}

// ProcessAccountDeletion performs the full account deletion workflow.
// This is called by the Asynq worker.
// // Resumability (AUDIT-FRESH-GDPR-003): each phase checkpoints a progress_step
// on the request row. If the job crashes or is retried by Asynq, the next run
// reads progress_step and skips phases already completed, so partial work is
// not re-executed from the top. The step map is mirrored in migration
// 000173_account_deletion_progress_step.up.sql.
// //	retention_wait flow (AUDIT-FRESH-GDPR-006): when gdpr.hard_delete_enabled
// is true but the gdpr.retention_days window has not elapsed since the
// request was created, the request is marked retention_wait instead of
// completed. The anonymization + token/session/Honcho erasure has already
// happened (steps 1-4), so the user is functionally erased from the
// platform; only the physical row deletion is deferred. A periodic sweep
// (ListRetentionWaitElapsed, run by the maintenance scheduler) re-invokes
// this handler once the window elapses, at which point step 5 performs the
// hard delete and step 6 marks the request completed.
func (s *Service) ProcessAccountDeletion(ctx context.Context, requestID pgtype.UUID) error {
	request, err := s.store.getAccountDeletionRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("fetching deletion request: %w", err)
	}

	// ADR-068 compliance (defense-in-depth): if the target user is a persona
	// backing account (is_bot = true), abort processing. The guard in
	// RequestAccountDeletion prevents creating such requests, but a stale
	// request created before the guard was added must not be executed.
	isBot, botErr := s.store.read.GetUserIsBot(ctx,
		request.UserID,
	)
	if botErr == nil && isBot {
		// Mark the request as failed so it is not retried.
		s.markDeletionFailed(ctx, requestID)
		return fmt.Errorf("%w: cannot erase AI persona backing account (system account, not a data subject)", errpkg.ErrForbidden)
	}

	// Mark as processing. A retried job re-enters here; progress_step is
	// preserved across retries because UpdateAccountDeletionRequestStatus
	// does not touch progress_step.
	_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:     requestID,
		Status: string(DeletionProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking deletion as processing: %w", err)
	}

	step := request.ProgressStep

	// 1. Anonymize user row + user_profile PII.
	if step < deletionStepAnonymized {
		if err := s.store.anonymizeUser(ctx, request.UserID); err != nil {
			s.markDeletionFailed(ctx, requestID)
			return fmt.Errorf("anonymizing user: %w", err)
		}
		// Clear PII in user_profiles (display_name, bio, timezone, business
		// info, content_country_preference, preferred_currency, avatar/cover)
		// so it is scrubbed during the retention window, not just at hard
		// delete (ADR-014 / ADR-057). Best-effort: a failure logs a warning
		// but does not abort — the user row is already anonymized.
		if err := s.store.anonymizeUserProfile(ctx, request.UserID); err != nil {
			slog.Warn("failed to anonymize user profile during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub PII (goals) from mentorship_connections where the user is
		// mentor or mentee (§102, ADR-014). The connection rows are retained
		// for audit until HardDeleteUser cascades; only the goals text is
		// cleared so it is not recoverable during the retention window.
		// Best-effort: a failure logs a warning but does not abort.
		if err := s.store.anonymizeMentorshipConnections(ctx, request.UserID); err != nil {
			slog.Warn("failed to anonymize mentorship connections during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub PII (ip_hash, user_agent_hash) from affiliate_link_events
		// owned by the user's affiliate links (Section 56, ADR-014). The
		// events are retained for commission audit; only tracking hashes are
		// cleared. Best-effort: a failure logs a warning but does not abort.
		if err := s.store.anonymizeAffiliateLinkEvents(ctx, request.UserID); err != nil {
			slog.Warn("failed to anonymize affiliate link events during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub user_id from coupon_events recorded by the deleted user
		// (Section 55, ADR-014). The events are retained for aggregate
		// click/copy analytics; only the user association is cleared.
		// Best-effort: a failure logs a warning but does not abort.
		if err := s.store.anonymizeCouponEvents(ctx, request.UserID); err != nil {
			slog.Warn("failed to anonymize coupon events during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub traveler PII from service_bookings (§111, ADR-014):
		// note + preferred_dates are wiped, booker_user_id nulled, and
		// anonymized_at stamped. The financial columns survive as the
		// org's audit record — the booking is a contract artifact, not
		// erasable PII. Best-effort like the other scrub steps.
		if err := s.store.write.AnonymizeServiceBookings(ctx,
			request.UserID,
		); err != nil {
			slog.Warn("failed to anonymize service bookings during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub PII from onboarding_state.step_data (name, current_location,
		// destination) during the anonymization phase (Section 09, ADR-014).
		// The row has ON DELETE CASCADE from users(id) so it is removed at
		// hard-delete, but step_data PII must be cleared now so it is not
		// recoverable during the retention window. Best-effort.
		if err := s.store.write.AnonymizeOnboardingState(ctx,
			request.UserID,
		); err != nil {
			slog.Warn("failed to anonymize onboarding state during deletion", "user_id", request.UserID.String(), "error", err)
		}
		// Scrub auth-related PII tables during the anonymization phase so
		// sensitive data (TOTP secrets, OAuth provider IDs, email addresses,
		// password hashes, reset/verification tokens) is not recoverable
		// during the retention window. These tables have ON DELETE CASCADE
		// from users(id), so they would be cleaned at hard-delete, but the
		// anonymization phase runs first and the data must be scrubbed now.
		// Ref: sections/02-authentication-sessions.md, ADR-014.
		// Best-effort: failures log warnings but do not abort.
		authPIITables := []struct {
			name string
			fn   func() error
		}{
			{"totp_credentials", func() error {
				return s.store.deleteUserTOTPCredentials(ctx, request.UserID)
			}},
			{"oauth_accounts", func() error {
				return s.store.deleteUserOAuthAccounts(ctx, request.UserID)
			}},
			{"password_reset_tokens", func() error {
				return s.store.deleteUserPasswordResetTokens(ctx, request.UserID)
			}},
			{"email_verifications", func() error {
				return s.store.deleteUserEmailVerifications(ctx, request.UserID)
			}},
			{"password_history", func() error {
				return s.store.deleteUserPasswordHistory(ctx, request.UserID)
			}},
		}
		for _, t := range authPIITables {
			if err := t.fn(); err != nil {
				slog.Warn("failed to scrub auth PII table during deletion", "table", t.name, "user_id", request.UserID.String(), "error", err)
			}
		}
		// Revoke all of the user's API keys during the anonymization phase
		// so they cannot authenticate during the retention_wait window
		// (Section 19 GDPR/NDPR, BUG-008 fix: API keys remained active
		// until the hard-delete cascade removed them). Best-effort: a
		// failure logs a warning but does not abort — the user row is
		// already anonymized.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE api_keys
			SET status = 'revoked', revoked_at = now(), revoked_by = user_id, updated_at = now()
			WHERE user_id = $1 AND status = 'active'
		`, request.UserID); err != nil {
			slog.Warn("failed to revoke api keys during deletion", "user_id", request.UserID.String(), "error", err)
		}

		if err := s.checkpoint(ctx, requestID, deletionStepAnonymized); err != nil {
			return err
		}

		step = deletionStepAnonymized
	}

	// 1b. Scrub PII from saved_scholarships.notes during the anonymization
	// phase (Section 73, ADR-073, ADR-014). The table has ON DELETE CASCADE
	// from users(id) so rows are removed at hard-delete, but the user's
	// free-text application notes must be cleared now so they are not
	// recoverable during the retention window. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE saved_scholarships
			SET notes = NULL
			WHERE user_id = $1 AND notes IS NOT NULL
		`, request.UserID); err != nil {
			slog.Warn("failed to scrub saved_scholarships notes during deletion", "user_id", request.UserID.String(), "error", err)
		}
	}

	// 1c. Scrub PII from content_imports.source_url during the anonymization
	// phase (Section 65, ADR-083, ADR-014). The table has ON DELETE CASCADE
	// from users(id) so rows are removed at hard-delete, but source_url may
	// contain the user's external identity (e.g. medium.com/@username) and
	// must be cleared now so it is not recoverable during the retention
	// window. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE content_imports
			SET source_url = NULL
			WHERE user_id = $1 AND source_url IS NOT NULL
		`, request.UserID); err != nil {
			slog.Warn("failed to scrub content_imports source_url during deletion", "user_id", request.UserID.String(), "error", err)
		}
	}

	// 1d. Scrub PII from checklist_progress.notes during the anonymization
	// phase (Section 70/§92, ADR-014). The table has ON DELETE CASCADE from
	// users(id) so rows are removed at hard-delete, but the user's free-text
	// checklist notes (visa + arrival) must be cleared now so they are not
	// recoverable during the retention window. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE checklist_progress
			SET notes = NULL
			WHERE user_id = $1 AND notes IS NOT NULL
		`, request.UserID); err != nil {
			slog.Warn("failed to scrub checklist_progress notes during deletion", "user_id", request.UserID.String(), "error", err)
		}
	}

	// 1e. Delete the user's remittance_rate_alerts during the anonymization
	// phase (Section 74, ADR-075, ADR-014). The table has ON DELETE CASCADE
	// from users(id) so rows are removed at hard-delete, but the user's rate
	// alert subscriptions reveal financial interests (corridor + threshold)
	// and could fire notifications to a deleted account during the retention
	// window. Deletion (not anonymization) is correct because alerts have no
	// free-text PII and are useless to a deleted user. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM remittance_rate_alerts
			WHERE user_id = $1
		`, request.UserID); err != nil {
			slog.Warn("failed to delete remittance_rate_alerts during deletion", "user_id", request.UserID.String(), "error", err)
		}
	}

	// 1f. Soft-delete the user's appointment_alerts during the anonymization
	// phase (Section 91, ADR-099, ADR-014). The table has ON DELETE CASCADE
	// from users(id) so rows are removed at hard-delete, but the user's
	// appointment alert subscriptions reveal immigration intent (embassy +
	// date range + visa category) and could fire notifications to a deleted
	// account during the retention window. Soft-delete (not hard-delete) is
	// correct because appointment_alerts are user content (Rule 11) and the
	// nightly embassy:expire_checks sweep already transitions soft-deleted
	// rows. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE appointment_alerts
			SET deleted_at = now(), updated_at = now(), status = 'expired'
			WHERE user_id = $1 AND deleted_at IS NULL
		`, request.UserID); err != nil {
			slog.Warn("failed to soft-delete appointment_alerts during deletion", "user_id", request.UserID.String(), "error", err)
		}
	}

	// 2. Revoke all refresh tokens (best-effort).
	if step < deletionStepTokensRevoked {
		if err := s.store.deleteUserTokens(ctx, request.UserID); err != nil {
			slog.Warn("failed to revoke user tokens during deletion", "user_id", request.UserID.String(), "error", err)
		}

		if err := s.checkpoint(ctx, requestID, deletionStepTokensRevoked); err != nil {
			return err
		}

		step = deletionStepTokensRevoked
	}

	// 3. Delete Valkey session keys (best-effort).
	if step < deletionStepSessionsDeleted {
		if s.sessionDeleter != nil {
			if err := s.sessionDeleter.DeleteSessions(ctx, request.UserID.String()); err != nil {
				slog.Warn("failed to delete valkey sessions", "user_id", request.UserID.String(), "error", err)
			}
		}

		if err := s.checkpoint(ctx, requestID, deletionStepSessionsDeleted); err != nil {
			return err
		}

		step = deletionStepSessionsDeleted
	}

	// 4. Delete Honcho AI context — mandatory, not best-effort. If the Honcho
	// client is configured and the call fails, mark the deletion as failed
	// and return the error so Asynq retries from this step.
	if step < deletionStepHonchoDeleted {
		if s.honchoEraser != nil {
			if err := s.honchoEraser.DeleteUser(ctx, request.UserID.String()); err != nil {
				s.markDeletionFailed(ctx, requestID)
				return fmt.Errorf("deleting honcho context for user %s: %w", request.UserID, err)
			}
		} else if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
			// AUDIT-BE-019: In production, Honcho erasure is mandatory.
			// A nil eraser means the service was misconfigured — fail the
			// entire deletion job so it retries after correct wiring.
			s.markDeletionFailed(ctx, requestID)
			return fmt.Errorf("honcho eraser not configured — required in production for user %s", request.UserID)
		} else {
			slog.Warn("honcho eraser not configured, skipping AI context deletion", "request_id", requestID, "user_id", request.UserID)
		}

		if err := s.checkpoint(ctx, requestID, deletionStepHonchoDeleted); err != nil {
			return err
		}

		step = deletionStepHonchoDeleted
	}

	// 5. Hard delete — optional physical erasure phase (ADR-057).
	// See the retention_wait flow documented in the function comment above.
	retentionWait := false

	if step < deletionStepHardDeleted {
		if s.settings != nil {
			hardDeleteEnabled, settingsErr := s.settings.GetSettingBool(ctx, "gdpr.hard_delete_enabled")
			if settingsErr == nil && hardDeleteEnabled {
				retentionDays, _ := s.settings.GetSettingInt(ctx, "gdpr.retention_days")

				retentionDeadline := request.CreatedAt.Time.Add(time.Duration(retentionDays) * 24 * time.Hour)
				if time.Now().After(retentionDeadline) {
					slog.Info("gdpr: performing physical erasure", "user_id", request.UserID.String(), "request_id", requestID.String())

					if hardErr := s.hardDeleteUser(ctx, request.UserID); hardErr != nil {
						s.markDeletionFailed(ctx, requestID)
						return fmt.Errorf("hard deleting user %s: %w", request.UserID, hardErr)
					}
				} else {
					slog.Info("gdpr: retention period not elapsed, entering retention_wait", "user_id", request.UserID.String(), "retention_days", retentionDays, "retention_deadline", retentionDeadline.Format(time.RFC3339))

					retentionWait = true
				}
			}
		}

		if err := s.checkpoint(ctx, requestID, deletionStepHardDeleted); err != nil {
			return err
		}
	}

	// 6. Mark deletion as completed or retention_wait.
	if retentionWait {
		_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:     requestID,
			Status: string(DeletionRetentionWait),
		})
		if err != nil {
			return fmt.Errorf("marking deletion as retention_wait: %w", err)
		}
	} else {
		_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:          requestID,
			Status:      string(DeletionCompleted),
			CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("marking deletion as completed: %w", err)
		}
	}

	// AUDIT-FRESH-GDPR-004: record an audit trail entry for the erasure.
	// GDPR requires an audit log of deletions; the actor is the system when
	// the worker drives the flow, or the user when self-service triggered it.
	s.logDeletionAudit(ctx, request, retentionWait)

	return nil
}

// deletion progress_step constants (AUDIT-FRESH-GDPR-003). Mirrored in
// migration 000173_account_deletion_progress_step.up.sql.
const (
	deletionStepNotStarted      int32 = 0
	deletionStepAnonymized      int32 = 1
	deletionStepTokensRevoked   int32 = 2
	deletionStepSessionsDeleted int32 = 3
	deletionStepHonchoDeleted   int32 = 4
	deletionStepHardDeleted     int32 = 5
)

// checkpoint persists the completed step so a retried job resumes after it.
func (s *Service) checkpoint(ctx context.Context, requestID pgtype.UUID, step int32) error {
	if err := s.store.updateAccountDeletionRequestProgress(ctx, requestID, step); err != nil {
		return fmt.Errorf("checkpointing deletion step %d: %w", step, err)
	}

	return nil
}

// markDeletionFailed flips the request status to failed and logs the secondary error.
func (s *Service) markDeletionFailed(ctx context.Context, requestID pgtype.UUID) {
	if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:     requestID,
		Status: string(DeletionFailed),
	}); statusErr != nil {
		slog.Error("update deletion status to failed", "error", statusErr)
	}
}

// logDeletionAudit writes the GDPR-required audit trail entry for an erasure.
// Best-effort: a logging failure is logged but does not fail the deletion.
func (s *Service) logDeletionAudit(ctx context.Context, request gen.AccountDeletionRequest, retentionWait bool) {
	action := "gdpr.account_deleted"
	if retentionWait {
		action = "gdpr.account_retention_wait"
	}

	if err := logAuditEvent(ctx, s.store, LogAuditParams{
		ActorUserID: request.UserID,
		ActionType:  action,
		TargetType:  "user",
		TargetID:    request.UserID,
		Metadata: map[string]any{
			"request_id":     request.ID.String(),
			"retention_wait": retentionWait,
		},
	}); err != nil {
		slog.Warn("gdpr: failed to write deletion audit log", "request_id", request.ID.String(), "error", err)
	}
}

// hardDeleteUser performs physical deletion of the user row after handling
// non-CASCADE foreign key references. Called only when gdpr.hard_delete_enabled
// is true and the retention period has elapsed.
func (s *Service) hardDeleteUser(ctx context.Context, userID pgtype.UUID) error {
	// Resolve the deleted-user placeholder UUID from settings, or use zero UUID.
	deletedUserPlaceholder := pgtype.UUID{}
	_ = deletedUserPlaceholder.Scan("00000000-0000-0000-0000-000000000000")

	if s.settings != nil {
		if placeholderStr, err := s.settings.GetSetting(ctx, "gdpr.deleted_user_placeholder_uuid"); err == nil && placeholderStr != "" {
			var p pgtype.UUID
			if scanErr := p.Scan(placeholderStr); scanErr == nil {
				deletedUserPlaceholder = p
			}
		}
	}

	// Defensive Honcho erasure (ADR-057: Honcho deletion is a mandatory
	// prerequisite to physical row deletion). The automated pipeline
	// (ProcessAccountDeletion) calls Honcho DeleteUser at step 4 before
	// reaching this method at step 5. This admin-triggered path may be
	// called on a user whose step 4 was skipped or failed, so we repeat
	// the call here — it is idempotent (404 = already deleted = success).
	// In production, a failure blocks the hard delete so AI context is
	// never left behind after physical erasure. The call is outside the
	// DB transaction below to avoid holding locks during an external API
	// call (Bug Hunter's Guide §6.2).
	if s.honchoEraser != nil {
		if err := s.honchoEraser.DeleteUser(ctx, userID.String()); err != nil {
			if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
				return fmt.Errorf("hard delete blocked: honcho erasure failed for user %s: %w", userID, err)
			}

			slog.Warn("hard delete: honcho erasure failed (non-production, proceeding)", "user_id", userID.String(), "error", err)
		}
	} else if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
		return fmt.Errorf("hard delete blocked: honcho eraser not configured — required in production for user %s", userID)
	}

	// All four steps run in one transaction so a failure cannot leave the user
	// half-erased (e.g. authorship reassigned but the user row still present).
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// 1. Reassign content authorship to preserve thread continuity.
		if err := q.ReassignContentAuthor(ctx, gen.ReassignContentAuthorParams{
			UserID: userID,

			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning content authorship: %w", err)
		}

		// C7: reassign blog_series authorship to preserve curated multi-part
		// series. Without this, ON DELETE CASCADE on blog_series.author_user_id
		// destroys all user-authored series and cascades to blog_series_posts.
		if err := q.ReassignBlogSeriesAuthor(ctx, gen.ReassignBlogSeriesAuthorParams{
			UserID: userID,

			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning blog series authorship: %w", err)
		}

		// C7: reassign content_reviews reviewer to preserve editorial audit
		// history. Without this, ON DELETE CASCADE on
		// content_reviews.reviewer_user_id destroys all review records.
		if err := q.ReassignContentReviewReviewer(ctx, gen.ReassignContentReviewReviewerParams{
			UserID: userID,

			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning content review reviewer: %w", err)
		}

		// BUG-009: reassign media uploader to preserve user-uploaded media
		// (Rule 11 — soft-delete only for user content; only
		// account:deletion_cleanup may hard-delete). Without this, ON DELETE
		// CASCADE on media.uploader_user_id physically deletes the user's
		// media rows, breaking featured/header images on posts whose
		// authorship was reassigned to the placeholder. Ref: §34, ADR-014.
		if err := q.ReassignMediaUploader(ctx, gen.ReassignMediaUploaderParams{
			UserID: userID,

			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning media uploader: %w", err)
		}

		// 2. Nullify payment user references (financial audit trail survives).
		// Errors are fatal inside the tx — a leftover reference would block
		// the final DELETE on its foreign key anyway.
		if err := q.NullifyPaymentUserRefs(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying payment user refs: %w", err)
		}

		// 2a. Nullify subscription user references (BUG-020).
		if err := q.NullifySubscriptionUserRefs(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying subscription user refs: %w", err)
		}

		// 2b. Nullify discount code creator references (BUG-020).
		if err := q.NullifyDiscountCodeCreatorRefs(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying discount code creator refs: %w", err)
		}

		// 2c. Nullify discount code use user references (BUG-020).
		// Uses a sentinel UUID because the PRIMARY KEY (code_id, user_id) does
		// not allow NULL user_id.
		if err := q.NullifyDiscountCodeUseUserRefs(ctx, userID); err != nil {
			return fmt.Errorf("nullifying discount code use user refs: %w", err)
		}

		// 3. Nullify ticket submitter references.
		if err := q.NullifyTicketSubmitter(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying ticket submitter: %w", err)
		}

		// 3b. Anonymize comparison_items authored by the user (BUG-029).
		// comparison_items carry title/description/image_id that can re-identify
		// a user even after their content is soft-deleted.
		if _, err := q.AnonymizeComparisonItemsByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing comparison items: %w", err)
		}

		// BUG-003: anonymize comparison_dimensions and comparison_values
		// authored by the user. The label and value columns are user-authored
		// free text that can re-identify a deleted user via their comparison
		// content, just like comparison_items.title.
		if _, err := q.AnonymizeComparisonDimensionsByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing comparison dimensions: %w", err)
		}
		if _, err := q.AnonymizeComparisonValuesByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing comparison values: %w", err)
		}

		// JCE Sub-spec 4: delete ingestion jobs tied to user-authored comparison
		// content and any user-owned data sources (created_by_user_id is populated
		// for future non-admin source creation; admin sources remain NULL).
		if err := q.DeleteComparisonDataIngestionJobsByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting comparison data ingestion jobs: %w", err)
		}
		if err := q.DeleteComparisonDataSourcesByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting comparison data sources: %w", err)
		}

		// BUG-018: delete custom_field_values for the user's profile entity
		// so PII stored in custom fields (Japa Status, phone, address, etc.)
		// does not survive account deletion. The custom_field_values table
		// is polymorphic on (entity_type, entity_id); we delete rows where
		// entity_type is 'user_profile' or 'user' and entity_id is the user.
		if _, err := q.DeleteCustomFieldValuesByEntity(ctx, userID); err != nil {
			return fmt.Errorf("deleting custom field values for user entity: %w", err)
		}

		// BUG-019: anonymize form_submissions authored by the deleted user.
		// The submission row is preserved for analytics/audit (form
		// submission counts, trigger audit trail), but the user_id reference
		// is severed and the PII fields (ip_address, field_values jsonb) are
		// scrubbed to '{}' so submitted answers cannot re-identify the user.
		if _, err := q.AnonymizeFormSubmissionsByUser(ctx, userID); err != nil {
			return fmt.Errorf("anonymizing form submissions: %w", err)
		}

		// GDPR: delete assessment_results for the user. The FK has ON DELETE
		// CASCADE but we delete explicitly to document the erasure path and
		// ensure PII-adjacent data (dimension scores, roadmap content) is
		// removed before the final HardDeleteUser. Ref: Section 71, ADR-071.
		if err := q.GdprDeleteAssessmentResultsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting assessment results: %w", err)
		}

		// 4. AUDIT-FRESH-GDPR-001: explicit cleanup for tables whose FK to
		// users(id) has no ON DELETE CASCADE/SET NULL. Without these the
		// final HardDeleteUser would block on a residual foreign key.

		// content_revisions.edited_by has no CASCADE; drop the user's edit history.
		if err := q.DeleteContentRevisionsByEditor(ctx, userID); err != nil {
			return fmt.Errorf("deleting content revisions: %w", err)
		}

		// user_content_views.user_id has ON DELETE CASCADE, but we delete
		// explicitly to document the erasure path and ensure PII is
		// removed before the final HardDeleteUser (Section 14, ADR-014).
		if err := q.GdprDeleteUserContentViews(ctx, userID); err != nil {
			return fmt.Errorf("deleting user content views: %w", err)
		}

		// GDPR: delete ad_events (impression/click tracking) for the user.
		// ad_events.user_id has ON DELETE CASCADE but we delete explicitly to
		// document the erasure path and remove the user's ad-interaction PII
		// before the final HardDeleteUser. Ref: Section 25/61, GDPR erasure.
		if err := q.GdprDeleteAdEventsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user ad events: %w", err)
		}

		// BUG-011: explicitly delete reactions, content_shares, bookmarks, and
		// bookmark folders authored by the user. These tables have ON DELETE
		// CASCADE on user_id, but explicit deletion makes the erasure path
		// self-documenting and survives future schema changes.
		if err := q.GdprDeleteReactionsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user reactions: %w", err)
		}
		if err := q.GdprDeleteContentSharesByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user content shares: %w", err)
		}
		if err := q.GdprDeleteBookmarksByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user bookmarks: %w", err)
		}
		if err := q.GdprDeleteBookmarkFoldersByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user bookmark folders: %w", err)
		}
		if err := q.GdprDeleteForumBookmarksByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user forum bookmarks: %w", err)
		}
		if err := q.GdprDeleteUnifiedForumBookmarksByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user unified forum bookmarks: %w", err)
		}

		// GDPR: nullify ad_campaigns.created_by for campaigns created by the
		// user. The FK has no CASCADE/SET NULL, so without this step the final
		// HardDeleteUser would block on the residual foreign key. The campaign
		// row is preserved (it may belong to an organization). Ref: Section 25/61.
		if err := q.GdprNullifyAdCampaignCreator(ctx, userID); err != nil {
			return fmt.Errorf("nullifying ad campaign creator: %w", err)
		}

		// GDPR: nullify studio_jobs/studio_batches created_by for jobs/batches
		// created by the user. The FKs have no CASCADE/SET NULL and were NOT
		// NULL until migration 000421 dropped that constraint. Without this
		// step the final HardDeleteUser would block on the residual foreign
		// key. The job/batch rows are preserved (they may have drafts in the
		// review queue). Ref: Section 75, ADR-076, GDPR erasure model.
		if err := q.GdprNullifyStudioJobCreator(ctx, userID); err != nil {
			return fmt.Errorf("nullifying studio job creator: %w", err)
		}
		if err := q.GdprNullifyStudioBatchCreator(ctx, userID); err != nil {
			return fmt.Errorf("nullifying studio batch creator: %w", err)
		}

		// space_pinned_posts.pinned_by has no CASCADE; remove pins made by the user.
		if err := q.DeleteSpacePinnedPostsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting space pinned posts: %w", err)
		}

		// forum_tags.moderator_user_id is nullable; clear it so the tag survives.
		if err := q.NullifyForumTagModerator(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying forum tag moderator: %w", err)
		}

		// tag_moderators.user_id links the user to tags they moderate (Section 08/49).
		// The FK has ON DELETE CASCADE, but we delete explicitly to document the
		// erasure path and clear the moderation history before the final HardDeleteUser.
		if err := q.DeleteTagModeratorsByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting tag moderators: %w", err)
		}

		// BUG-022: forum_thread_tags for threads authored by the user are
		// junction rows that reference the user's threads. Delete them so the
		// final HardDeleteUser (which cascades to forum_threads →
		// forum_thread_tags) doesn't leave orphaned junction rows if the
		// thread is retained for soft-delete. Ref: Section 08 + GDPR.
		if err := q.DeleteForumThreadTagsByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting forum thread tags by author: %w", err)
		}

		// Universal tag junction rows on the user's content (content_tags —
		// the post-000719 successor to forum_thread_tags; Section 08/14).
		if err := q.DeleteContentTagsByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting content tags by author: %w", err)
		}

		// ticket_comments.user_id has no CASCADE; delete the user's ticket comments.
		if err := q.DeleteTicketCommentsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting ticket comments: %w", err)
		}

		// GDPR-001: anonymize comments authored by the erased user. The
		// comments.author_user_id FK has ON DELETE SET NULL so HardDeleteUser
		// will null it, but the comment body (TipTap JSON, potentially
		// containing PII and @mentions) must be cleared before the user is
		// hard-deleted. Ref: Section 15, ADR-014.
		if err := q.AnonymizeCommentsByAuthor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing comments: %w", err)
		}

		// BUG-024: DM erasure — anonymize DM content and reassign participant
		// references to the placeholder so the conversation survives for the
		// other participant. Without these steps, the CASCADE on users(id)
		// would hard-delete the entire conversation (including the other
		// participant's copy) during HardDeleteUser, violating ADR-014's
		// "never hard delete; soft-delete-and-anonymize" principle.
		// Ref: Section 19 + ADR-014.

		// Anonymize dm_messages authored by the user (clear body, reassign sender).
		if err := q.AnonymizeDMMessagesBySender(ctx, gen.AnonymizeDMMessagesBySenderParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing dm messages: %w", err)
		}

		// Reassign dm_conversations participant references to placeholder.
		if err := q.ReassignDMConversationParticipant1(ctx, gen.ReassignDMConversationParticipant1Params{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning dm conversation participant 1: %w", err)
		}

		if err := q.ReassignDMConversationParticipant2(ctx, gen.ReassignDMConversationParticipant2Params{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning dm conversation participant 2: %w", err)
		}

		// Anonymize dm_requests (clear preview, reassign requester/recipient).
		if err := q.AnonymizeDMRequestsByUser(ctx, gen.AnonymizeDMRequestsByUserParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing dm requests: %w", err)
		}

		// Delete user_dm_settings (user-owned config, no PII in the row).
		if err := q.DeleteUserDMSettingsByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting user dm settings: %w", err)
		}

		// Delete user_blocks (user-owned config).
		if err := q.DeleteUserBlocksByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user blocks: %w", err)
		}

		// BE-2: delete user_mutes where the user is the muter or muted party.
		// Mute relationships are user-owned feed-filter config and must not
		// survive anonymization. Ref: Section 52, Section 15.
		if err := q.DeleteUserMutesByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user mutes: %w", err)
		}

		// BE-2: delete space_mutes for the user. Ref: Section 52, Section 15.
		if err := q.DeleteSpaceMutesByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting space mutes: %w", err)
		}

		// BUG-M006: delete notifications addressed to the erased user.
		// Notification rows are user-owned data (the recipient's inbox) and
		// must be removed during GDPR erasure (Section 19).
		if err := q.DeleteNotificationsByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting user notifications: %w", err)
		}

		// BUG-011: delete notification_preferences for the erased user.
		// Preferences are user-owned config (in-app/email/push toggles) with no
		// audit requirement; safe to hard-delete. Ref: §35, §15.
		if err := q.DeleteNotificationPreferencesByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting notification preferences: %w", err)
		}

		// BUG-012: delete notification_batch_windows for the erased user.
		// Orphaned batch windows would dispatch notifications to a deleted user.
		// Ref: §35, §15.
		if err := q.DeleteNotificationBatchWindowsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting notification batch windows: %w", err)
		}

		// BUG-010: delete push_subscriptions for the erased user. The endpoint URL
		// and p256dh/auth keys are device-identifying PII. The table has ON DELETE
		// CASCADE on user_id, but the anonymize phase runs before hard-delete, so
		// we remove them explicitly here to clear PII immediately. Ref: §35, §15.
		if err := q.DeletePushSubscriptionsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting push subscriptions: %w", err)
		}

		// BUG-015: soft-delete attestation_community_updates authored by the
		// user so their PII is removed while the pathway audit trail survives.
		// Ref: §97, ADR-105.
		if err := q.SoftDeleteAttestationCommunityUpdatesByUser(ctx, userID); err != nil {
			return fmt.Errorf("soft-deleting attestation community updates: %w", err)
		}

		// §94: soft-delete credential_community_notes authored by the user
		// so their PII is removed while the note text survives for other
		// users. Ref: §94, ADR-102.
		if err := q.SoftDeleteCredentialCommunityNotesByUser(ctx, userID); err != nil {
			return fmt.Errorf("soft-deleting credential community notes: %w", err)
		}

		// ticket_events.user_id is nullable audit trail; nullify the actor, keep the event.
		if err := q.NullifyTicketEventsUser(ctx, userID); err != nil {
			return fmt.Errorf("nullifying ticket events user: %w", err)
		}

		// user_preferences is user-owned config; remove it explicitly.
		if err := q.DeleteUserPreferences(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting user preferences: %w", err)
		}

		// organizations.owner_user_id is NOT NULL; reassign to placeholder to preserve the org for remaining members.
		if err := q.ReassignOrganizationOwner(ctx, gen.ReassignOrganizationOwnerParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning organization owner: %w", err)
		}

		// organization_invitations.invited_by is nullable; clear inviter reference.
		if err := q.NullifyOrganizationInvitationInviter(ctx, userID); err != nil {
			return fmt.Errorf("nullifying organization invitation inviter: %w", err)
		}

		// ownership_claims.claimant_user_id is NOT NULL; delete the user's submitted claims.
		if err := q.DeleteOwnershipClaimsByClaimant(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting ownership claims: %w", err)
		}

		// ownership_claims.reviewed_by is nullable; clear reviewer reference when an admin is erased.
		if err := q.NullifyOwnershipClaimReviewer(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying ownership claim reviewer: %w", err)
		}

		// organization_members.user_id is NOT NULL; remove the erased user's memberships
		// to prevent re-identification via the member roster.
		if err := q.DeleteOrganizationMembersByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting organization memberships: %w", err)
		}

		// consent_records.user_id is nullable; preserve the compliance record, detach the user.
		if err := q.NullifyConsentRecordUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("nullifying consent record user: %w", err)
		}

		// data_export_requests.user_id is NOT NULL; reassign to placeholder to preserve export audit trail.
		if err := q.ReassignDataExportRequestUser(ctx, gen.ReassignDataExportRequestUserParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning data export request user: %w", err)
		}

		// account_deletion_requests.user_id is NOT NULL; reassign to placeholder so the in-flight request row survives.
		if err := q.ReassignAccountDeletionRequestUser(ctx, gen.ReassignAccountDeletionRequestUserParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning account deletion request user: %w", err)
		}

		// ai_personas.created_by is NOT NULL; reassign to placeholder so persona
		// config rows survive when the admin who created them is erased (BUG-016,
		// ADR-068 compliance).
		if err := q.ReassignAIPersonaCreatedBy(ctx, gen.ReassignAIPersonaCreatedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning ai_personas created_by: %w", err)
		}

		// audit_logs must be preserved for compliance; repoint actor/impersonated refs to the placeholder "Deleted User".
		if err := q.AnonymizeAuditLogActor(ctx, gen.AnonymizeAuditLogActorParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing audit log actor: %w", err)
		}

		// platform_data_imports.created_by is NOT NULL; reassign to placeholder so
		// the import audit trail survives when the admin who created the import is
		// erased (BUG-013, Section 67 / Section 15 compliance).
		if err := q.ReassignPlatformDataImportCreatedBy(ctx, gen.ReassignPlatformDataImportCreatedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning platform_data_imports created_by: %w", err)
		}

		// Japa Academy GDPR cascade (ADR-063 / audit C2):
		// - Clear sensitive payout_method jsonb on the user's instructor
		// profile(s) before the profile is cascade-deleted.
		// - Anonymize revenue_splits and academy_instructor_payouts: sever the
		// instructor_id reference (SET NULL) so the financial audit
		// records survive for regulatory retention. The FKs were changed
		// from ON DELETE CASCADE to ON DELETE SET NULL in migration
		// 000181 to enforce this at the schema level.
		if _, err := q.ClearInstructorPayoutMethod(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("clearing instructor payout method: %w", err)
		}
		if _, err := q.AnonymizeRevenueSplitsForInstructor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing revenue splits: %w", err)
		}
		if _, err := q.AnonymizeInstructorPayoutsForInstructor(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("anonymizing instructor payouts: %w", err)
		}

		// C7: follows where the deleted user is the TARGET (entity_id with
		// entity_type='user') have no FK (polymorphic entity_id) and are not
		// CASCADE-deleted. Remove them explicitly so other users' "following"
		// lists don't contain dead entries pointing to the deleted user.
		// Follows where the deleted user is the FOLLOWER are CASCADE-deleted
		// by the follower_id FK and need no explicit cleanup here.
		// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if err := q.DeleteFollowsByEntity(ctx, gen.DeleteFollowsByEntityParams{
			EntityType: "user",
			EntityID:   userID,
		}); err != nil {
			return fmt.Errorf("deleting follows targeting deleted user: %w", err)
		}

		// BUG-010: search_analytics.user_id references users(id) but has no
		// ON DELETE clause. Nullify the user_id on the user's search history
		// rows so the search analytics (zero-result detection, trending
		// queries) survive for platform insights while severing the PII link.
		// Ref: Section 53; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE search_analytics SET user_id = NULL
			WHERE user_id = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying search_analytics user_id: %w", err)
		}

		// Nullify feature_flags.updated_by references. The feature_flags table
		// has updated_by uuid REFERENCES users(id) with no ON DELETE clause
		// (defaults to NO ACTION). Without this nullification, the final
		// HardDeleteUser DELETE fails with a FK violation for any admin who
		// has toggled a feature flag. Ref: ADR-057; Section 42.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE feature_flags SET updated_by = NULL
			WHERE updated_by = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying feature_flags updated_by: %w", err)
		}

		// Nullify doc_pages.last_reviewed_by references. The doc_pages table
		// has last_reviewed_by uuid REFERENCES users(id) with no ON DELETE clause.
		// Without this, the HardDeleteUser DELETE fails with a FK violation for
		// any admin who has approved/published doc pages. The doc pages themselves
		// are retained (they are platform content, not user PII). Ref: §44; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE doc_pages SET last_reviewed_by = NULL
			WHERE last_reviewed_by = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying doc_pages last_reviewed_by: %w", err)
		}

		// Nullify pages.created_by references. The pages table (Page Builder,
		// Section 16) has created_by uuid REFERENCES users(id) with no ON DELETE
		// clause (migration 000055). Without this, the HardDeleteUser DELETE
		// fails with a FK violation for any admin who has created page-builder
		// pages. The pages themselves are retained (they are platform CMS content,
		// not user PII). Ref: §16; §15 GDPR erasure.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE pages SET created_by = NULL
			WHERE created_by = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying pages created_by: %w", err)
		}

		// BUG-011: trending_snapshots with entity_type='user' reference the
		// user via entity_id (no FK — polymorphic). Delete them so the user's
		// trending presence does not survive erasure.
		// Ref: Section 53; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM trending_snapshots
			WHERE entity_type = 'user' AND entity_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting trending_snapshots for user: %w", err)
		}

		// BUG-018: webhook_endpoints.created_by references users(id) with ON
		// DELETE SET NULL (migration 000361), so the FK is handled automatically.
		// However, webhook_deliveries.payload may contain user PII (e.g. event
		// payloads referencing the user). Delete deliveries whose payload
		// contains the user's UUID so PII does not survive erasure. The
		// webhook_endpoints themselves are platform config and survive with
		// created_by nullified. Ref: §12; §15 GDPR erasure.
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM webhook_deliveries
			WHERE payload::text LIKE '%' || $1::text || '%'
		`, userID); err != nil {
			return fmt.Errorf("deleting webhook_deliveries with user PII: %w", err)
		}

		// BUG-025: Community Calendar GDPR erasure (§86, ADR-094).
		// calendar_entries.submitted_by and calendar_entries.reviewed_by
		// reference users(id) with no ON DELETE clause. Reassign to the
		// placeholder so the entries survive (they are community content, not
		// user PII). calendar_saves and calendar_subscriptions are user-owned
		// config — delete them to prevent re-identification.
		// Ref: japaconnect-engineering/sections/86-community-calendar.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if err := q.GdprReassignCalendarEntriesSubmittedBy(ctx, gen.GdprReassignCalendarEntriesSubmittedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning calendar_entries submitted_by: %w", err)
		}
		if err := q.GdprReassignCalendarEntriesReviewedBy(ctx, gen.GdprReassignCalendarEntriesReviewedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning calendar_entries reviewed_by: %w", err)
		}
		if err := q.GdprDeleteCalendarSavesByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting calendar_saves: %w", err)
		}
		if err := q.GdprDeleteCalendarSubscriptionsByUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("deleting calendar_subscriptions: %w", err)
		}
		// BUG-005: Mutual Aid Board GDPR erasure (§85, ADR-093).
		// mutual_aid_posts.user_id references users(id). Reassign to the
		// placeholder so posts survive (they are community content) but the
		// user is no longer attributable.
		// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `UPDATE mutual_aid_posts SET user_id = $2, updated_at = now() WHERE user_id = $1 AND deleted_at IS NULL`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning mutual_aid_posts user_id: %w", err)
		}

		// §99 Language Test Hub GDPR erasure (ADR-107).
		// language_score_stories.user_id references users(id) with ON
		// DELETE CASCADE. Score stories are community content (peer
		// preparation experiences) and should survive erasure with a
		// stable "deleted user" attribution, matching the mutual_aid_posts
		// pattern. Soft-deleted stories (deleted_at IS NOT NULL) are left
		// alone — they are already anonymised by the soft-delete pathway.
		// Ref: japaconnect-engineering/sections/99-language-test-hub.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `UPDATE language_score_stories SET user_id = $2 WHERE user_id = $1 AND deleted_at IS NULL`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning language_score_stories user_id: %w", err)
		}

		// §81 Scam Alert Board GDPR erasure (ADR-089).
		// scam_reports.reporter_id references users(id) with ON DELETE SET
		// NULL, so the final HardDeleteUser would nullify it. Reassign to the
		// placeholder so confirmed reports (community content) survive with a
		// stable "deleted user" attribution rather than an anonymous NULL,
		// matching the mutual_aid_posts pattern. scam_alert_subscriptions is
		// pure user PII (subscription preferences) and is deleted outright.
		// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE scam_reports
			SET reporter_id = $2, updated_at = now()
			WHERE reporter_id = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning scam_reports reporter_id: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM scam_alert_subscriptions
			WHERE user_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting scam_alert_subscriptions for user: %w", err)
		}

		// §83 Visa Timeline Tracker GDPR erasure (ADR-091).
		// visa_timelines.user_id references users(id) with ON DELETE SET
		// NULL. Shared timelines feed the anonymised
		// visa_processing_stats aggregate (which has no user_id) and must
		// survive erasure — the contribution is intentionally anonymised.
		// Private (is_shared=false) timelines are user PII and are
		// soft-deleted. For shared timelines, nullify user_id + stamp
		// anonymised_at so the audit trail records the erasure-triggered
		// anonymisation. visa_timeline_milestones cascade with their
		// parent timeline (ON DELETE CASCADE).
		// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE visa_timelines
			SET user_id = NULL, anonymised_at = now(), updated_at = now()
			WHERE user_id = $1 AND is_shared = TRUE AND deleted_at IS NULL
		`, userID); err != nil {
			return fmt.Errorf("anonymising shared visa_timelines: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE visa_timelines
			SET deleted_at = now(), updated_at = now()
			WHERE user_id = $1 AND is_shared = FALSE AND deleted_at IS NULL
		`, userID); err != nil {
			return fmt.Errorf("soft-deleting private visa_timelines: %w", err)
		}

		// BUG-016: Community Locator GDPR erasure (§101, ADR-109).
		// community_organisations.submitted_by and reviewed_by reference
		// users(id) with ON DELETE SET NULL. Reassign to the placeholder so
		// the entries survive (they are community content, not user PII) and
		// attribution is preserved rather than silently lost.
		// Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE community_organisations
			SET submitted_by = $2
			WHERE submitted_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning community_organisations submitted_by: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE community_organisations
			SET reviewed_by = $2
			WHERE reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning community_organisations reviewed_by: %w", err)
		}
		// Also reassign hitl_reviewed_by (ADR-088 AI generation column).
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE community_organisations
			SET hitl_reviewed_by = $2
			WHERE hitl_reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning community_organisations hitl_reviewed_by: %w", err)
		}

		// housing_guides.hitl_reviewed_by references users(id) with ON
		// DELETE SET NULL (migration 000335). Reassign to the placeholder
		// so the HITL audit trail preserves a stable "deleted user"
		// attribution rather than an anonymous NULL, matching the
		// platform-wide erasure convention (ADR-014) and the
		// community_organisations pattern above.
		// Ref: japaconnect-engineering/sections/96-housing-guide.md
		// japaconnect-engineering/adr/ADR-104-housing-guide.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE housing_guides
			SET hitl_reviewed_by = $2
			WHERE hitl_reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning housing_guides hitl_reviewed_by: %w", err)
		}

		// tax_obligation_guides.hitl_reviewed_by references users(id) with
		// ON DELETE SET NULL (migration 000341). Reassign to the placeholder
		// so the HITL audit trail preserves a stable "deleted user"
		// attribution rather than an anonymous NULL, matching the
		// platform-wide erasure convention (ADR-014) and the
		// housing_guides / community_organisations pattern above.
		// Ref: japaconnect-engineering/sections/100-tax-calculator.md
		// japaconnect-engineering/adr/ADR-108-tax-calculator.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE tax_obligation_guides
			SET hitl_reviewed_by = $2
			WHERE hitl_reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning tax_obligation_guides hitl_reviewed_by: %w", err)
		}

		// visa_routes.hitl_reviewed_by references users(id) with ON
		// DELETE SET NULL (migration 000325). Reassign to the placeholder
		// so the HITL audit trail preserves a stable "deleted user"
		// attribution rather than an anonymous NULL, matching the
		// platform-wide erasure convention (ADR-014) and the
		// housing_guides / community_organisations pattern above.
		// Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
		// japaconnect-engineering/adr/ADR-098-visa-route-comparison.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE visa_routes
			SET hitl_reviewed_by = $2
			WHERE hitl_reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning visa_routes hitl_reviewed_by: %w", err)
		}

		// BUG-017: Scrub PII from cv_format_sessions during the
		// anonymization phase (§95, ADR-103, ADR-014). The table has
		// ON DELETE CASCADE from users(id) so rows are removed at
		// hard-delete, but input_text (the user's full CV) and
		// output_text (the formatted CV) must be cleared now so they
		// are not recoverable during the retention window. Best-effort:
		// a failure logs a warning but does not abort.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE cv_format_sessions
			SET input_text = '[scrubbed]',
			    output_text = NULL
			WHERE user_id = $1
		`, userID); err != nil {
			slog.Warn("failed to scrub cv_format_sessions PII during deletion", "user_id", userID.String(), "error", err)
		}

		// §§107–110 SEO growth tools GDPR erasure (ADR-118..121, §19).
		// sop_drafts and crs_score_history are user-owned PII (profile
		// inputs + generated SOP text) with ON DELETE CASCADE — they are
		// removed at hard-delete, but inputs/generated_text are scrubbed
		// now so they are not recoverable during the retention window
		// (BUG-017 pattern). eca_community_updates are community content:
		// user_id is reassigned to the placeholder so approved updates
		// survive anonymised, matching mutual_aid_posts. The
		// hitl_reviewed_by audit columns reassign to the placeholder.
		// Ref: japaconnect-engineering/sections/107-crs-calculator.md
		// japaconnect-engineering/sections/108-pof-calculator.md
		// japaconnect-engineering/sections/109-eca-guide.md
		// japaconnect-engineering/sections/110-sop-generator.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE sop_drafts
			SET inputs = '{}'::jsonb, generated_text = NULL, updated_at = now()
			WHERE user_id = $1
		`, userID); err != nil {
			slog.Warn("failed to scrub sop_drafts PII during deletion", "user_id", userID.String(), "error", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE crs_score_history
			SET inputs = '{}'::jsonb, breakdown = '[]'::jsonb
			WHERE user_id = $1
		`, userID); err != nil {
			slog.Warn("failed to scrub crs_score_history PII during deletion", "user_id", userID.String(), "error", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE eca_community_updates
			SET user_id = $2
			WHERE user_id = $1 AND deleted_at IS NULL
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning eca_community_updates user_id: %w", err)
		}
		for _, tbl := range []string{
			"crs_rulesets", "pof_requirements", "eca_guides", "sop_samples",
		} {
			if _, err := s.db.Writer.Exec(ctx, fmt.Sprintf(
				`UPDATE %s SET hitl_reviewed_by = $2 WHERE hitl_reviewed_by = $1`, tbl),
				userID, deletedUserPlaceholder); err != nil {
				return fmt.Errorf("reassigning %s hitl_reviewed_by: %w", tbl, err)
			}
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE eca_community_updates
			SET reviewed_by = $2
			WHERE reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning eca_community_updates reviewed_by: %w", err)
		}

		// BUG-010: Platform data exports/imports (Section 67) created_by
		// references users(id). The migration 000403 made the column
		// nullable and switched the FK to ON DELETE SET NULL, so the
		// final HardDeleteUser would already nullify the column. We also
		// reassign to the placeholder here so the audit trail preserves a
		// stable "deleted user" attribution rather than an anonymous NULL,
		// matching the platform-wide erasure convention (ADR-014).
		// Ref: japaconnect-engineering/sections/67-platform-data-portability.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if err := q.ReassignPlatformDataExportCreatedBy(ctx, gen.ReassignPlatformDataExportCreatedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning platform_data_exports created_by: %w", err)
		}
		if err := q.ReassignPlatformDataImportCreatedBy(ctx, gen.ReassignPlatformDataImportCreatedByParams{
			UserID: userID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning platform_data_imports created_by: %w", err)
		}

		// BUG-015: Newsletter subscriber PII (email + subscription state) must
		// not survive erasure. newsletter_subscribers.user_id references
		// users(id) with ON DELETE SET NULL (migration 000051), so the final
		// HardDeleteUser would nullify user_id but leave the email row behind.
		// Delete the user's subscriber row explicitly so the PII email is
		// removed. newsletter_unsubscribe_tokens cascade via subscriber_id.
		// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		// japaconnect-engineering/adr/ADR-080-newsletter-digest.md
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM newsletter_subscribers
			WHERE user_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting newsletter_subscribers for user: %w", err)
		}

		// BUG-JOBS-013: job_applications.applicant_id references users(id) ON
		// DELETE CASCADE, so rows are removed at hard-delete, but employer_notes
		// may contain applicant PII and the table must be explicitly listed for
		// GDPR traceability (§15, Rule 11). Delete the user's applications in
		// the anonymization phase so the PII is gone before the retention window
		// closes. submission_id → form_submissions PII is handled separately
		// (BUG-019). Ref: japaconnect-engineering/sections/63-jobs-board.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM job_applications
			WHERE applicant_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting job_applications for user: %w", err)
		}

		// BUG-006/BUG-007: Scrub PII from ai_agent_sessions (and cascading
		// ai_agent_tool_calls) during the anonymization phase (§103, ADR-111,
		// ADR-014). The table has ON DELETE CASCADE from users(id) so rows are
		// removed at hard-delete, but the prompt field (the user's personal
		// questions about visa, finance, health) and tool-call arguments/results
		// (which may contain profile data) must be cleared now so they are not
		// recoverable during the retention window. Delete the rows explicitly
		// rather than scrubbing in-place — the session history has no value
		// once the user is erased. ai_agent_tool_calls cascades from
		// ai_agent_sessions via ON DELETE CASCADE on session_id.
		// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM ai_agent_sessions
			WHERE user_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting ai_agent_sessions for user: %w", err)
		}

		// BUG-008: ai_hitl_reviews.reviewer_id references users(id) WITHOUT
		// an ON DELETE clause, so the final HardDeleteUser would fail with a
		// foreign key violation if the user reviewed any HITL items. Nullify
		// reviewer_id (and clear human_feedback PII) before the hard delete.
		// The review records themselves are retained for audit traceability —
		// only the reviewer attribution is anonymized.
		// Ref: japaconnect-engineering/sections/20-ai-core.md (HITL)
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE ai_hitl_reviews
			SET reviewer_id = NULL,
			    human_feedback = NULL
			WHERE reviewer_id = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying ai_hitl_reviews reviewer_id: %w", err)
		}

		// BUG-003: Purge RAG content_embeddings for the user's SOFT-DELETED
		// content. Non-deleted content is reassigned to the placeholder user
		// (step 1 above) and its embeddings are preserved — the content still
		// exists with a new author. But soft-deleted content (deleted_at IS NOT
		// NULL) is not retrievable in RAG (BUG-001 fix filters by deleted_at IS
		// NULL), so its embeddings are orphaned PII that must be removed.
		// content_embeddings has no FK to contents (polymorphic entity_type/
		// entity_id), so ON DELETE CASCADE does not fire.
		// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM content_embeddings
			WHERE entity_type = 'content'
			  AND entity_id IN (
			      SELECT id FROM contents
			      WHERE author_id = $1 AND deleted_at IS NOT NULL
			  )
		`, userID); err != nil {
			return fmt.Errorf("purging content_embeddings for soft-deleted content: %w", err)
		}

		// BUG-003 (rag-audit): Purge RAG content_embeddings for the user's
		// forum threads. Forum threads are user-authored content (title +
		// body) embedded into content_embeddings with entity_type =
		// 'forum_thread'. content_embeddings has no FK to forum_threads
		// (polymorphic entity_type/entity_id), so ON DELETE CASCADE does not
		// fire. The thread body may contain PII (the user's words), so it
		// must be removed on erasure. Spaces and academy_tracks are
		// platform-owned content (no author_user_id column) and are not
		// purged here — they are not user PII.
		// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM content_embeddings
			WHERE entity_type = 'forum_thread'
			  AND entity_id IN (
			      SELECT id FROM forum_threads
			      WHERE author_user_id = $1
			  )
		`, userID); err != nil {
			return fmt.Errorf("purging content_embeddings for user forum threads: %w", err)
		}

		// 5. Physical deletion — ON DELETE CASCADE handles all child tables.
		// The following feed-related tables are cascade-purged here (no
		// explicit step needed, documented for GDPR traceability — BUG-014):
		// - feeds (ON DELETE CASCADE from feeds.user_id → users(id) and
		// feeds.content_id → contents(id); the user's feed rows and any
		// feed rows pointing to their reassigned content are removed)
		// - content_scores (ON DELETE CASCADE from
		// content_scores.content_id → contents(id); scores for the
		// user's content are removed when the content is cascade-deleted
		// or retained when content is reassigned to the placeholder)
		// - story_meta (ON DELETE CASCADE from story_meta.content_id →
		// contents(id); the user's story metadata is removed when the
		// story content is cascade-deleted or retained when reassigned
		// to the placeholder. story_meta.hitl_reviewed_by → users(id)
		// ON DELETE SET NULL, so the admin reviewer reference is
		// nullified. Ref: §79 Community Story Wall, ADR-087.)
		// - achievement_meta (ON DELETE CASCADE from
		// achievement_meta.content_id → contents(id); the user's
		// achievement metadata is removed when the achievement content
		// is cascade-deleted or retained when reassigned to the
		// placeholder. achievement_meta has no user_id FK — the author
		// is on the contents row — so no additional nullification is
		// needed. Ref: §89 Community Achievement Feed, ADR-097.)
		// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md

		// Intelligence System (§INTEL, ADR-112) — delete the user's review
		// audit trail. intel_review_log.actor_user_id is the only PII-bearing
		// reference in the intelligence tables; the spec §8 requires it be
		// included in the erasure job. BUG-015 fix.
		// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md §8
		if err := q.DeleteIntelReviewLogByActor(ctx,

			userID,
		); err != nil {
			return fmt.Errorf("deleting intel review log by actor: %w", err)
		}

		// BUG-006/BUG-022: Destination Wiki GDPR erasure (§80, ADR-088).
		// wiki_pages.created_by and wiki_revisions.{submitted_by, reviewed_by,
		// hitl_reviewed_by} reference users(id) with ON DELETE SET NULL, so the
		// final HardDeleteUser would nullify them. Reassign to the placeholder
		// so wiki pages and revisions (community content) survive with a stable
		// "deleted user" attribution rather than an anonymous NULL, matching the
		// calendar_entries / mutual_aid_posts / scam_reports pattern. Wiki
		// content is collaborative and long-lived; nullifying authorship would
		// break the revision history audit trail.
		// Ref: japaconnect-engineering/sections/80-destination-wiki.md
		// japaconnect-engineering/adr/ADR-088-destination-wiki.md
		// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE wiki_pages
			SET created_by = $2, updated_at = now()
			WHERE created_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning wiki_pages created_by: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE wiki_revisions
			SET submitted_by = $2
			WHERE submitted_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning wiki_revisions submitted_by: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE wiki_revisions
			SET reviewed_by = $2
			WHERE reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning wiki_revisions reviewed_by: %w", err)
		}
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE wiki_revisions
			SET hitl_reviewed_by = $2
			WHERE hitl_reviewed_by = $1
		`, userID, deletedUserPlaceholder); err != nil {
			return fmt.Errorf("reassigning wiki_revisions hitl_reviewed_by: %w", err)
		}

		if err := q.HardDeleteUser(ctx,
			userID,
		); err != nil {
			return fmt.Errorf("physically deleting user row: %w", err)
		}

		return nil
	}); err != nil {
		slog.Warn("gdpr: hard delete transaction failed", "user_id", userID.String(), "error", err)
		return err
	}

	slog.Info("gdpr: user physically deleted", "user_id", userID.String())

	// §15/§19: remove the user's document from the Meilisearch users index
	// so they no longer appear in search results. Also delete any content
	// they authored that was reassigned to the placeholder — those
	// documents should be removed from the posts index too. Best-effort:
	// the enqueuer fires search:index_entity with operation "delete" so the
	// worker handles the Meilisearch cleanup asynchronously.
	s.enqueueSearchIndexDelete(userID, "user")

	return nil
}

// enqueueSearchIndexDelete enqueues a search:index_entity task with operation
// "delete" to remove the entity's document from the Meilisearch index. Used by
// GDPR erasure to ensure erased users' documents are purged from search.
// Best-effort — logs errors but never fails the caller.
func (s *Service) enqueueSearchIndexDelete(entityID pgtype.UUID, entityType string) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"entity_id":   entityID.String(),
		"entity_type": entityType,

		"operation": "delete",
	})
	if err := s.enqueuer.Enqueue("search:index_entity", payload); err != nil {
		slog.Warn("gdpr: failed to enqueue search:index_entity delete", "entity_id", entityID.String(), "entity_type", entityType, "error", err)
	}
}

// HardDeleteUser is the public entry point for admin-triggered hard deletes.
// It performs the full GDPR cascade (reassign content, nullify refs, physical delete)
// in a single transaction. This is the same logic as the internal hardDeleteUser
// but exposed for admin use on already-soft-deleted users.
func (s *Service) HardDeleteUser(ctx context.Context, userID pgtype.UUID) error {
	return s.hardDeleteUser(ctx, userID)
}

// ListPendingDeletionRequests returns pending deletion requests for the
// caller's platform (BUG-GDPR-001: platform-scoped so an admin in platform A
// cannot see platform B's pending deletion requests, §5.3 cross-platform bleed).
func (s *Service) ListPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.ListPendingDeletionRequestsRow, error) {
	return s.store.listPendingDeletionRequests(ctx, limit)
}

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

// LogAuditParams holds parameters for creating an audit log entry.
type LogAuditParams struct {
	ActorUserID        pgtype.UUID
	ImpersonatedUserID pgtype.UUID
	ActionType         string
	TargetType         string
	TargetID           pgtype.UUID
	Metadata           map[string]any
	IPAddress          *string
}

// LogAuditEvent creates an audit log entry.
func (s *Service) LogAuditEvent(ctx context.Context, params LogAuditParams) error {
	if params.ActionType == "" {
		return errors.New("action_type is required")
	}

	metadataJSON := []byte("{}")

	if len(params.Metadata) > 0 {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return fmt.Errorf("marshaling audit metadata: %w", err)
		}

		metadataJSON = b
	}

	var targetType *string
	if params.TargetType != "" {
		targetType = &params.TargetType
	}

	ipAddress := parseIPAddress(params.IPAddress)

	_, err := s.store.createAuditLog(ctx, gen.CreateAuditLogParams{

		ActorUserID:        params.ActorUserID,
		ImpersonatedUserID: params.ImpersonatedUserID,
		ActionType:         params.ActionType,
		TargetType:         targetType,
		TargetID:           params.TargetID,
		Metadata:           metadataJSON,
		IpAddress:          ipAddress,
	})

	return err
}

// ListAuditLogs returns audit logs with filters for admin access.
func (s *Service) ListAuditLogs(ctx context.Context, actorUserID pgtype.UUID, actionType, targetType string, startDate, endDate *time.Time, limit, offset int32) ([]gen.AuditLog, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	var startDateTZ, endDateTZ pgtype.Timestamptz
	if startDate != nil {
		startDateTZ = pgtype.Timestamptz{Time: *startDate, Valid: true}
	}

	if endDate != nil {
		endDateTZ = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}

	var targetTypePtr *string
	if targetType != "" {
		targetTypePtr = &targetType
	}

	// Use zero UUID as sentinel for "no filter" on actor_user_id.
	zeroUUID := pgtype.UUID{}
	_ = zeroUUID.Scan("00000000-0000-0000-0000-000000000000")

	actorFilter := zeroUUID
	if actorUserID.Valid {
		actorFilter = actorUserID
	}

	return s.store.listAuditLogs(ctx, gen.ListAuditLogsParams{

		ActorUserID: actorFilter,
		ActionType:  actionType,
		TargetType:  targetTypePtr,
		StartDate:   startDateTZ,
		EndDate:     endDateTZ,
		PageLimit:   limit,
		PageOffset:  offset,
	})
}

// parseIPAddress converts a string IP address to a netip.Prefix pointer.
// Plain IPs are expanded to their host-prefix so they scan into PG inet.
func parseIPAddress(s *string) *netip.Prefix {
	if s == nil {
		return nil
	}

	if p, err := netip.ParsePrefix(*s); err == nil {
		return &p
	}

	addr, err := netip.ParseAddr(*s)
	if err != nil {
		return nil
	}

	bits := 128
	if addr.Is4() {
		bits = 32
	}
	p, _ := addr.Prefix(bits)
	return &p
}

// UserDataExport represents the complete user data export.
type UserDataExport struct {
	UserProfile                 map[string]interface{}   `json:"user_profile"`
	Consents                    []map[string]interface{} `json:"consents"`
	Content                     []map[string]interface{} `json:"content"`
	Comments                    []map[string]interface{} `json:"comments"`
	Spaces                      []map[string]interface{} `json:"spaces"`
	Payments                    []map[string]interface{} `json:"payments"`
	Notifications               []map[string]interface{} `json:"notifications"`
	UserProfiles                []map[string]interface{} `json:"user_profiles"`
	Reactions                   []map[string]interface{} `json:"reactions"`
	Bookmarks                   []map[string]interface{} `json:"bookmarks"`
	BookmarkFolders             []map[string]interface{} `json:"bookmark_folders"` // BL-005
	ContentShares               []map[string]interface{} `json:"content_shares"`
	ContentViews                []map[string]interface{} `json:"content_views"`
	Follows                     []map[string]interface{} `json:"follows"`
	ForumThreads                []map[string]interface{} `json:"forum_threads"`
	ForumReplies                []map[string]interface{} `json:"forum_replies"`
	ForumVotes                  []map[string]interface{} `json:"forum_votes"`
	ForumBookmarks              []map[string]interface{} `json:"forum_bookmarks"`
	ForumPollVotes              []map[string]interface{} `json:"forum_poll_votes"`
	ForumThreadTags             []map[string]interface{} `json:"forum_thread_tags"`
	ContentTags                 []map[string]interface{} `json:"content_tags"`
	ReplyViews                  []map[string]interface{} `json:"reply_views"`
	Media                       []map[string]interface{} `json:"media"`
	Reviews                     []map[string]interface{} `json:"reviews"`
	SopDrafts                   []map[string]interface{} `json:"sop_drafts"`        // §110
	CrsScoreHistory             []map[string]interface{} `json:"crs_score_history"` // §107
	SpaceMembers                []map[string]interface{} `json:"space_members"`
	SpaceModerators             []map[string]interface{} `json:"space_moderators"`
	SpaceParentSpaces           []map[string]interface{} `json:"space_parent_spaces"` // BUG-004
	SpaceRules                  []map[string]interface{} `json:"space_rules"`         // BUG-004
	TagModerators               []map[string]interface{} `json:"tag_moderators"`
	ConsentRecords              []map[string]interface{} `json:"consent_records"`
	WalletLedger                []map[string]interface{} `json:"wallet_ledger"`
	ActivityEvents              []map[string]interface{} `json:"activity_events"`
	AcademyLessonProgress              []map[string]interface{} `json:"academy_lesson_progress"`
	AcademyCertificates        []map[string]interface{} `json:"academy_certificates"`
	MentorshipMatches           []map[string]interface{} `json:"mentorship_matches"`
	MentorshipConnections       []map[string]interface{} `json:"mentorship_connections"` // §102 ADR-110
	MentorProfiles              []map[string]interface{} `json:"mentor_profiles"`        // §102 ADR-110
	MentorSessions              []map[string]interface{} `json:"mentor_sessions"`        // §102 ADR-110
	MentorReviews               []map[string]interface{} `json:"mentor_reviews"`         // §102 ADR-110
	Organizations               []map[string]interface{} `json:"organizations"`
	ApiKeys                     []map[string]interface{} `json:"api_keys"`
	UserPreferences             []map[string]interface{} `json:"user_preferences"`
	UserMutes                   []map[string]interface{} `json:"user_mutes"`
	SpaceMutes                  []map[string]interface{} `json:"space_mutes"`
	TicketComments              []map[string]interface{} `json:"ticket_comments"`
	AcademyInstructorProfiles          []map[string]interface{} `json:"academy_instructor_profiles"`
	RevenueSplits               []map[string]interface{} `json:"revenue_splits"`
	AcademyInstructorPayouts           []map[string]interface{} `json:"academy_instructor_payouts"`
	CustomFieldValues           []map[string]interface{} `json:"custom_field_values"`            // BUG-020
	PushTokens                  []map[string]interface{} `json:"push_tokens"`                    // BUG-014
	DMConversations             []map[string]interface{} `json:"dm_conversations"`               // BUG-022
	DMMessages                  []map[string]interface{} `json:"dm_messages"`                    // BUG-022
	DMRequests                  []map[string]interface{} `json:"dm_requests"`                    // BUG-022
	DMSettings                  []map[string]interface{} `json:"dm_settings"`                    // BUG-022
	UserBlocks                  []map[string]interface{} `json:"user_blocks"`                    // BUG-022
	AttestationCommunityUpdates []map[string]interface{} `json:"attestation_community_updates"`  // §97 BUG-015
	CalendarEntries             []map[string]interface{} `json:"calendar_entries"`               // §86 BUG-025
	CalendarSaves               []map[string]interface{} `json:"calendar_saves"`                 // §86 BUG-025
	CalendarSubscriptions       []map[string]interface{} `json:"calendar_subscriptions"`         // §86 BUG-025
	MutualAidPosts              []map[string]interface{} `json:"mutual_aid_posts"`               // §85 BUG-005
	ScamReports                 []map[string]interface{} `json:"scam_reports"`                   // §81
	ScamAlertSubscriptions      []map[string]interface{} `json:"scam_alert_subscriptions"`       // §81
	CommunityOrganisations      []map[string]interface{} `json:"community_organisations"`        // §101 BUG-016
	ComparisonItems             []map[string]interface{} `json:"comparison_items"`               // BUG-004
	ComparisonDimensions        []map[string]interface{} `json:"comparison_dimensions"`          // BUG-004
	ComparisonValues            []map[string]interface{} `json:"comparison_values"`              // BUG-004
	ComparisonDataIngestionJobs []map[string]interface{} `json:"comparison_data_ingestion_jobs"` // JCE Sub-spec 4
	ComparisonDataSources       []map[string]interface{} `json:"comparison_data_sources"`        // JCE Sub-spec 4
	CvFormatSessions            []map[string]interface{} `json:"cv_format_sessions"`             // §95 BUG-014
	RemittanceRateAlerts        []map[string]interface{} `json:"remittance_rate_alerts"`         // §74 BUG-013
	ReferralCodes               []map[string]interface{} `json:"referral_codes"`                 // §60 BUG-013
	ReferralsAsReferrer         []map[string]interface{} `json:"referrals_as_referrer"`          // §60 BUG-013
	ReferralsAsReferee          []map[string]interface{} `json:"referrals_as_referee"`           // §60 BUG-013
	ServiceListings             []map[string]interface{} `json:"service_listings"`               // §98 BUG-008
	LanguageScoreStories        []map[string]interface{} `json:"language_score_stories"`         // §99 BUG-004
	WikiPages                   []map[string]interface{} `json:"wiki_pages"`                     // §80 BUG-022
	WikiRevisions               []map[string]interface{} `json:"wiki_revisions"`                 // §80 BUG-022
	AIUsageLog                  []map[string]interface{} `json:"ai_usage_log"`                   // BUG-012
	AIAgentSessions             []map[string]interface{} `json:"ai_agent_sessions"`              // BUG-012
	AIAgentToolCalls            []map[string]interface{} `json:"ai_agent_tool_calls"`            // BUG-012
	AIHITLReviews               []map[string]interface{} `json:"ai_hitl_reviews"`                // BUG-012
	HonchoContext               []map[string]interface{} `json:"honcho_context"`                 // BUG-012
	ExportMetadata              map[string]interface{}   `json:"export_metadata"`
}

// collectUserData gathers all user data from relevant tables.
func (s *Service) collectUserData(ctx context.Context, userID pgtype.UUID) (*UserDataExport, error) {
	export := &UserDataExport{
		ExportMetadata: map[string]interface{}{
			"user_id": userID.String(),

			"generated": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0",
			"platform":  s.siteName,
		},
	}

	// Collect user profile (anonymized in export since user is being deleted).
	export.UserProfile = map[string]interface{}{
		"user_id":    userID.String(),
		"exported":   true,
		"anonymized": true,
	}

	// Collect consents
	consents, err := s.store.getUserConsents(ctx, userID, "")
	if err != nil {
		slog.Warn("failed to collect consents for export", "user_id", userID.String(), "error", err)
	} else {
		for _, c := range consents {
			export.Consents = append(export.Consents, map[string]interface{}{
				"type":        c.ConsentType,
				"granted":     c.Granted,
				"policy":      c.PolicyVersion,
				"recorded_at": c.CreatedAt.Time,
			})
		}
	}

	// Collect user's published content.
	if contentRows, err := s.store.gdprExportUserContent(ctx, userID); err == nil {
		for _, c := range contentRows {
			export.Content = append(export.Content, map[string]interface{}{
				"id":           c.ID.String(),
				"title":        c.Title,
				"slug":         c.Slug,
				"content_type": c.ContentType,
				"status":       c.Status,
				"created_at":   c.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect content for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's service listings (§98 BUG-008). The base content row is
	// already in export.Content above; this adds the service_listing_meta
	// structured fields (category, destination, origin communities, etc.).
	if serviceRows, err := s.store.gdprExportUserServiceListings(ctx, userID); err == nil {
		for _, r := range serviceRows {
			entry := map[string]interface{}{
				"content_id":         r.ContentID.String(),
				"title":              r.Title,
				"slug":               r.Slug,
				"service_category":   r.ServiceCategory,
				"is_verified":        r.IsVerified,
				"claim_status":       r.ClaimStatus,
				"origin_communities": r.OriginCommunities,
				"is_remote":          r.IsRemote,
				"created_at":         r.CreatedAt.Time,
			}
			if r.VerificationType != nil {
				entry["verification_type"] = *r.VerificationType
			}

			if r.WebsiteUrl != nil {
				entry["website_url"] = *r.WebsiteUrl
			}

			if r.BookingUrl != nil {
				entry["booking_url"] = *r.BookingUrl
			}

			if r.PriceRange != nil {
				entry["price_range"] = *r.PriceRange
			}

			export.ServiceListings = append(export.ServiceListings, entry)
		}
	} else {
		slog.Warn("failed to collect service listings for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's comments.
	if commentRows, err := s.store.gdprExportUserComments(ctx, userID); err == nil {
		for _, c := range commentRows {
			export.Comments = append(export.Comments, map[string]interface{}{
				"id":         c.ID.String(),
				"content_id": c.ContentID.String(),
				"body":       c.Body,
				"created_at": c.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect comments for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's space memberships.
	if spaceRows, err := s.store.gdprExportUserSpaces(ctx, userID); err == nil {
		for _, sp := range spaceRows {
			export.Spaces = append(export.Spaces, map[string]interface{}{
				"id":        sp.ID.String(),
				"name":      sp.Name,
				"slug":      sp.Slug,
				"status":    sp.Status,
				"role":      sp.Role,
				"joined_at": sp.JoinedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect spaces for export", "user_id", userID.String(), "error", err)
	}

	// BUG-004: Collect parent space links and rules for spaces owned by the
	// user (space_parent_spaces, space_rules).
	if parentRows, err := s.store.gdprExportOwnedSpaceParents(ctx, userID); err == nil {
		for _, p := range parentRows {
			export.SpaceParentSpaces = append(export.SpaceParentSpaces, map[string]interface{}{
				"space_id":        p.SpaceID.String(),
				"parent_space_id": p.ParentSpaceID.String(),
			})
		}
	} else {
		slog.Warn("failed to collect space_parent_spaces for export", "user_id", userID.String(), "error", err)
	}

	if ruleRows, err := s.store.gdprExportOwnedSpaceRules(ctx, userID); err == nil {
		for _, r := range ruleRows {
			export.SpaceRules = append(export.SpaceRules, map[string]interface{}{
				"id":       r.ID.String(),
				"space_id": r.SpaceID.String(),
				"title":    r.Title,
				"body":     r.Body,
				"position": r.Position,
			})
		}
	} else {
		slog.Warn("failed to collect space_rules for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's payment history.
	if paymentRows, err := s.store.gdprExportUserPayments(ctx, userID); err == nil {
		for _, p := range paymentRows {
			export.Payments = append(export.Payments, map[string]interface{}{
				"id":         p.ID.String(),
				"provider":   p.Provider,
				"amount":     p.Amount,
				"currency":   p.Currency,
				"status":     p.Status,
				"created_at": p.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect payments for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's notifications.
	if notifRows, err := s.store.gdprExportUserNotifications(ctx, userID); err == nil {
		for _, n := range notifRows {
			export.Notifications = append(export.Notifications, map[string]interface{}{
				"id":                n.ID.String(),
				"notification_type": n.NotificationType,
				"title":             n.Title,
				"body":              n.Body,
				"read_at":           n.ReadAt,
				"created_at":        n.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect notifications for export", "user_id", userID.String(), "error", err)
	}

	// AUDIT-FRESH-GDPR-002: collect the remaining user-owned tables. Each block
	// is best-effort — a failure logs a warning and the export still succeeds
	// with the tables that could be collected, matching the existing pattern.

	// User profile detail row.
	if rows, err := s.store.gdprExportUserProfiles(ctx, userID); err == nil {
		export.UserProfiles = rows
	} else {
		slog.Warn("failed to collect user_profiles for export", "user_id", userID.String(), "error", err)
	}

	// Reactions the user placed on content.
	if rows, err := s.store.gdprExportUserReactions(ctx, userID); err == nil {
		export.Reactions = rows
	} else {
		slog.Warn("failed to collect reactions for export", "user_id", userID.String(), "error", err)
	}

	// Bookmarks the user saved.
	if rows, err := s.store.gdprExportUserBookmarks(ctx, userID); err == nil {
		export.Bookmarks = rows
	} else {
		slog.Warn("failed to collect bookmarks for export", "user_id", userID.String(), "error", err)
	}

	// Bookmark folders the user created (BL-005).
	if rows, err := s.store.gdprExportUserBookmarkFolders(ctx, userID); err == nil {
		export.BookmarkFolders = rows
	} else {
		slog.Warn("failed to collect bookmark_folders for export", "user_id", userID.String(), "error", err)
	}

	// Content shares the user created.
	if rows, err := s.store.gdprExportUserContentShares(ctx, userID); err == nil {
		export.ContentShares = rows
	} else {
		slog.Warn("failed to collect content_shares for export", "user_id", userID.String(), "error", err)
	}

	// Content views the user recorded (Section 14 — user_content_views PII).
	if rows, err := s.store.gdprExportUserContentViews(ctx, userID); err == nil {
		export.ContentViews = rows
	} else {
		slog.Warn("failed to collect content_views for export", "user_id", userID.String(), "error", err)
	}

	// Follow relationships initiated by the user.
	if rows, err := s.store.gdprExportUserFollows(ctx, userID); err == nil {
		export.Follows = rows
	} else {
		slog.Warn("failed to collect follows for export", "user_id", userID.String(), "error", err)
	}

	// Forum threads authored by the user.
	if rows, err := s.store.gdprExportUserForumThreads(ctx, userID); err == nil {
		export.ForumThreads = rows
	} else {
		slog.Warn("failed to collect forum_threads for export", "user_id", userID.String(), "error", err)
	}

	// Forum replies authored by the user.
	if rows, err := s.store.gdprExportUserForumReplies(ctx, userID); err == nil {
		export.ForumReplies = rows
	} else {
		slog.Warn("failed to collect forum_replies for export", "user_id", userID.String(), "error", err)
	}

	// Forum votes cast by the user.
	if rows, err := s.store.gdprExportUserForumVotes(ctx, userID); err == nil {
		export.ForumVotes = rows
	} else {
		slog.Warn("failed to collect forum_votes for export", "user_id", userID.String(), "error", err)
	}

	// Forum bookmarks the user saved.
	if rows, err := s.store.gdprExportUserForumBookmarks(ctx, userID); err == nil {
		export.ForumBookmarks = rows
	} else {
		slog.Warn("failed to collect forum_bookmarks for export", "user_id", userID.String(), "error", err)
	}

	// BUG-022: forum_poll_votes, forum_thread_tags, and reply_views were
	// missing from the original GDPR export. These are user-owned rows that
	// must be included in a data export. Ref: Section 08 + GDPR erasure model.
	if rows, err := s.store.gdprExportUserForumPollVotes(ctx, userID); err == nil {
		export.ForumPollVotes = rows
	} else {
		slog.Warn("failed to collect forum_poll_votes for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserForumThreadTags(ctx, userID); err == nil {
		export.ForumThreadTags = rows
	} else {
		slog.Warn("failed to collect forum_thread_tags for export", "user_id", userID.String(), "error", err)
	}

	// Universal tag attachments on the user's content (all content_types share
	// content_tags — Section 08/14).
	if rows, err := s.store.gdprExportUserContentTags(ctx, userID); err == nil {
		export.ContentTags = rows
	} else {
		slog.Warn("failed to collect content_tags for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserReplyViews(ctx, userID); err == nil {
		export.ReplyViews = rows
	} else {
		slog.Warn("failed to collect reply_views for export", "user_id", userID.String(), "error", err)
	}

	// Media uploaded by the user.
	if rows, err := s.store.gdprExportUserMedia(ctx, userID); err == nil {
		export.Media = rows
	} else {
		slog.Warn("failed to collect media for export", "user_id", userID.String(), "error", err)
	}

	// Reviews the user wrote.
	if rows, err := s.store.gdprExportUserReviews(ctx, userID); err == nil {
		export.Reviews = rows
	} else {
		slog.Warn("failed to collect reviews for export", "user_id", userID.String(), "error", err)
	}

	// Space membership records.
	if rows, err := s.store.gdprExportUserSpaceMembers(ctx, userID); err == nil {
		export.SpaceMembers = rows
	} else {
		slog.Warn("failed to collect space_members for export", "user_id", userID.String(), "error", err)
	}

	// Space moderator assignments (space_moderators table).
	if rows, err := s.store.gdprExportUserSpaceModerators(ctx, userID); err == nil {
		export.SpaceModerators = rows
	} else {
		slog.Warn("failed to collect space_moderators for export", "user_id", userID.String(), "error", err)
	}

	// Tag moderator assignments (tag_moderators table — Section 08/49).
	if rows, err := s.store.gdprExportUserTagModerators(ctx, userID); err == nil {
		export.TagModerators = rows
	} else {
		slog.Warn("failed to collect tag_moderators for export", "user_id", userID.String(), "error", err)
	}

	// Consent records (full rows, complementing the summarized Consents block).
	if rows, err := s.store.gdprExportUserConsentRecords(ctx, userID); err == nil {
		export.ConsentRecords = rows
	} else {
		slog.Warn("failed to collect consent_records for export", "user_id", userID.String(), "error", err)
	}

	// Wallet ledger entries for the user.
	if rows, err := s.store.gdprExportUserWalletLedger(ctx, userID); err == nil {
		export.WalletLedger = rows
	} else {
		slog.Warn("failed to collect wallet_ledger for export", "user_id", userID.String(), "error", err)
	}

	// Activity events for the user (BUG-008 — previously missing from export).
	if rows, err := s.store.gdprExportUserActivityEvents(ctx, userID); err == nil {
		export.ActivityEvents = rows
	} else {
		slog.Warn("failed to collect activity_events for export", "user_id", userID.String(), "error", err)
	}

	// Learning lesson progress.
	if rows, err := s.store.gdprExportUserLessonProgress(ctx, userID); err == nil {
		export.AcademyLessonProgress = rows
	} else {
		slog.Warn("failed to collect academy_lesson_progress for export", "user_id", userID.String(), "error", err)
	}

	// Issued learning certificates.
	if rows, err := s.store.gdprExportUserLearningCertificates(ctx, userID); err == nil {
		export.AcademyCertificates = rows
	} else {
		slog.Warn("failed to collect academy_certificates for export", "user_id", userID.String(), "error", err)
	}

	// Mentorship matches where the user is mentor or mentee.
	if rows, err := s.store.gdprExportUserMentorshipMatches(ctx, userID); err == nil {
		export.MentorshipMatches = rows
	} else {
		slog.Warn("failed to collect mentorship_matches for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentorship connections where the user is mentor or mentee (ADR-110).
	if rows, err := s.store.gdprExportUserMentorshipConnections(ctx, userID); err == nil {
		export.MentorshipConnections = rows
	} else {
		slog.Warn("failed to collect mentorship_connections for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor profiles owned by the user (ADR-110).
	if rows, err := s.store.gdprExportUserMentorProfiles(ctx, userID); err == nil {
		export.MentorProfiles = rows
	} else {
		slog.Warn("failed to collect mentor_profiles for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor sessions where the user is the mentee (ADR-110).
	if rows, err := s.store.gdprExportUserMentorSessions(ctx, userID); err == nil {
		export.MentorSessions = rows
	} else {
		slog.Warn("failed to collect mentor_sessions for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor reviews written by the user (ADR-110).
	if rows, err := s.store.gdprExportUserMentorReviews(ctx, userID); err == nil {
		export.MentorReviews = rows
	} else {
		slog.Warn("failed to collect mentor_reviews for export", "user_id", userID.String(), "error", err)
	}

	// Organizations owned by the user.
	if rows, err := s.store.gdprExportUserOrganizations(ctx, userID); err == nil {
		export.Organizations = rows
	} else {
		slog.Warn("failed to collect organizations for export", "user_id", userID.String(), "error", err)
	}

	// API keys issued by the user.
	if rows, err := s.store.gdprExportUserApiKeys(ctx, userID); err == nil {
		export.ApiKeys = rows
	} else {
		slog.Warn("failed to collect api_keys for export", "user_id", userID.String(), "error", err)
	}

	// User preference settings.
	if rows, err := s.store.gdprExportUserPreferences(ctx, userID); err == nil {
		export.UserPreferences = rows
	} else {
		slog.Warn("failed to collect user_preferences for export", "user_id", userID.String(), "error", err)
	}

	// User-initiated mutes of other users.
	if rows, err := s.store.gdprExportUserMutes(ctx, userID); err == nil {
		export.UserMutes = rows
	} else {
		slog.Warn("failed to collect user_mutes for export", "user_id", userID.String(), "error", err)
	}

	// Spaces the user muted.
	if rows, err := s.store.gdprExportUserSpaceMutes(ctx, userID); err == nil {
		export.SpaceMutes = rows
	} else {
		slog.Warn("failed to collect space_mutes for export", "user_id", userID.String(), "error", err)
	}

	// Comments the user posted on support tickets.
	if rows, err := s.store.gdprExportUserTicketComments(ctx, userID); err == nil {
		export.TicketComments = rows
	} else {
		slog.Warn("failed to collect ticket_comments for export", "user_id", userID.String(), "error", err)
	}

	// Comparison data authored by the user (BUG-004). The parent content rows
	// are exported above via GdprExportUserContent; these export the linked
	// comparison_items/dimensions/values so the user receives the full set.
	if rows, err := s.store.gdprExportUserComparisonItems(ctx, userID); err == nil {
		export.ComparisonItems = rows
	} else {
		slog.Warn("failed to collect comparison_items for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserComparisonDimensions(ctx, userID); err == nil {
		export.ComparisonDimensions = rows
	} else {
		slog.Warn("failed to collect comparison_dimensions for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserComparisonValues(ctx, userID); err == nil {
		export.ComparisonValues = rows
	} else {
		slog.Warn("failed to collect comparison_values for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserComparisonDataIngestionJobs(ctx, userID); err == nil {
		export.ComparisonDataIngestionJobs = rows
	} else {
		slog.Warn("failed to collect comparison_data_ingestion_jobs for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserComparisonDataSources(ctx, userID); err == nil {
		export.ComparisonDataSources = rows
	} else {
		slog.Warn("failed to collect comparison_data_sources for export", "user_id", userID.String(), "error", err)
	}

	// Japa Academy creator-economy data (ADR-063 / audit C2).
	if rows, err := s.store.gdprExportInstructorProfiles(ctx, userID); err == nil {
		export.AcademyInstructorProfiles = rows
	} else {
		slog.Warn("failed to collect academy_instructor_profiles for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportRevenueSplits(ctx, userID); err == nil {
		export.RevenueSplits = rows
	} else {
		slog.Warn("failed to collect revenue_splits for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportInstructorPayouts(ctx, userID); err == nil {
		export.AcademyInstructorPayouts = rows
	} else {
		slog.Warn("failed to collect academy_instructor_payouts for export", "user_id", userID.String(), "error", err)
	}

	// BUG-020: export custom_field_values for the user's profile entity so
	// the GDPR data export includes PII stored in custom fields.
	if rows, err := s.store.gdprExportCustomFieldValues(ctx, userID); err == nil {
		export.CustomFieldValues = rows
	} else {
		slog.Warn("failed to collect custom_field_values for export", "user_id", userID.String(), "error", err)
	}

	// BUG-014: export push_tokens (device push notification tokens) so the
	// GDPR data export includes PII-adjacent device identifiers.
	// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	if rows, err := s.store.gdprExportPushTokens(ctx, userID); err == nil {
		export.PushTokens = rows
	} else {
		slog.Warn("failed to collect push_tokens for export", "user_id", userID.String(), "error", err)
	}

	// BUG-022: export DM tables so the GDPR data export includes DM
	// conversations, messages, requests, privacy settings, and block
	// relationships. Ref: Section 19 + ADR-014.
	if rows, err := s.store.gdprExportUserDMConversations(ctx, userID); err == nil {
		export.DMConversations = rows
	} else {
		slog.Warn("failed to collect dm_conversations for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMMessages(ctx, userID); err == nil {
		export.DMMessages = rows
	} else {
		slog.Warn("failed to collect dm_messages for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMRequests(ctx, userID); err == nil {
		export.DMRequests = rows
	} else {
		slog.Warn("failed to collect dm_requests for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMSettings(ctx, userID); err == nil {
		export.DMSettings = rows
	} else {
		slog.Warn("failed to collect user_dm_settings for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserBlocks(ctx, userID); err == nil {
		export.UserBlocks = rows
	} else {
		slog.Warn("failed to collect user_blocks for export", "user_id", userID.String(), "error", err)
	}

	// BUG-015: export attestation_community_updates authored by the user
	// so the GDPR data export includes community-submitted attestation
	// pathway updates. Ref: §97, ADR-105.
	if rows, err := s.store.gdprExportAttestationCommunityUpdates(ctx, userID); err == nil {
		export.AttestationCommunityUpdates = rows
	} else {
		slog.Warn("failed to collect attestation_community_updates for export", "user_id", userID.String(), "error", err)
	}

	// BUG-025: export calendar_entries submitted by the user, calendar_saves,
	// and calendar_subscriptions. Ref: §86, ADR-094.
	if rows, err := s.store.gdprExportUserCalendarEntries(ctx, userID); err == nil {
		export.CalendarEntries = rows
	} else {
		slog.Warn("failed to collect calendar_entries for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserCalendarSaves(ctx, userID); err == nil {
		export.CalendarSaves = rows
	} else {
		slog.Warn("failed to collect calendar_saves for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserCalendarSubscriptions(ctx, userID); err == nil {
		export.CalendarSubscriptions = rows
	} else {
		slog.Warn("failed to collect calendar_subscriptions for export", "user_id", userID.String(), "error", err)
	}

	// BUG-005: export mutual_aid_posts authored by the user. Ref: §85.
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, post_type, category, title, description,
		       destination_space_id, city_space_id, status, is_fulfilled,
		       expires_at, created_at, updated_at
		FROM mutual_aid_posts
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.MutualAidPosts = append(export.MutualAidPosts, row)
		}
	} else {
		slog.Warn("failed to collect mutual_aid_posts for export", "user_id", userID.String(), "error", err)
	}

	// BUG-004: export language_score_stories authored by the user so the
	// GDPR data export includes their community score-story contributions.
	// Ref: japaconnect-engineering/sections/99-language-test-hub.md
	// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, test_id, starting_level, final_score,
		       destination_space_id, prep_duration_weeks, resources_used,
		       advice, status, created_at
		FROM language_score_stories
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.LanguageScoreStories = append(export.LanguageScoreStories, row)
		}
	} else {
		slog.Warn("failed to collect language_score_stories for export", "user_id", userID.String(), "error", err)
	}

	// §81: export scam_reports filed by the user + their scam-alert
	// subscriptions so the GDPR data export includes their scam-alert
	// activity. Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, scam_type, destination_space_id, origin_space_id,
		       origin_community, platform_used, approximate_date, amount_range,
		       description, reported_to_authorities, status, moderator_note,
		       is_pinned, pinned_space_id, reviewed_by, reviewed_at,
		       created_at, updated_at
		FROM scam_reports
		WHERE reporter_id = $1
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.ScamReports = append(export.ScamReports, row)
		}
	} else {
		slog.Warn("failed to collect scam_reports for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.db.Reader.Query(ctx, `
		SELECT user_id, destination_space_id, created_at
		FROM scam_alert_subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.ScamAlertSubscriptions = append(export.ScamAlertSubscriptions, row)
		}
	} else {
		slog.Warn("failed to collect scam_alert_subscriptions for export", "user_id", userID.String(), "error", err)
	}

	// BUG-016: export community_organisations submitted by the user so the
	// GDPR data export includes community-submitted org entries. Ref: §101.
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, name, slug, org_type, origin_community,
		       destination_space_id, status, claim_status, is_verified,
		       is_community_submitted, submitted_by, created_at, updated_at
		FROM community_organisations
		WHERE submitted_by = $1 AND deleted_at IS NULL
	`, userID); err == nil {
		collected := []map[string]interface{}{}

		for rows.Next() {
			m := map[string]interface{}{}

			var id, destSpaceID, submittedBy pgtype.UUID

			var name, slug, orgType, originCommunity, status, claimStatus string

			var isVerified, isCommunitySubmitted bool

			var createdAt, updatedAt pgtype.Timestamptz
			if scanErr := rows.Scan(&id, &name, &slug, &orgType, &originCommunity,
				&destSpaceID, &status, &claimStatus, &isVerified, &isCommunitySubmitted,
				&submittedBy, &createdAt, &updatedAt); scanErr == nil {
				m["id"] = id
				m["name"] = name
				m["slug"] = slug
				m["org_type"] = orgType
				m["origin_community"] = originCommunity
				m["destination_space_id"] = destSpaceID
				m["status"] = status
				m["claim_status"] = claimStatus
				m["is_verified"] = isVerified
				m["is_community_submitted"] = isCommunitySubmitted
				m["submitted_by"] = submittedBy
				m["created_at"] = createdAt
				m["updated_at"] = updatedAt
				collected = append(collected, m)
			}
		}

		rows.Close()

		export.CommunityOrganisations = collected
	} else {
		slog.Warn("failed to collect community_organisations for export", "user_id", userID.String(), "error", err)
	}

	// BUG-014: export cv_format_sessions owned by the user so the GDPR
	// data export includes their CV formatting session history. The
	// input_text and output_text fields contain the user's CV content.
	// Ref: §95, ADR-103.
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, user_id, origin_space_id, destination_space_id,
		       input_text, output_text, output_format, status, ai_model,
		       tokens_used, created_at, expires_at
		FROM cv_format_sessions
		WHERE user_id = $1
	`, userID); err == nil {
		collected := []map[string]interface{}{}

		for rows.Next() {
			var (
				id, userID2, originSpaceID, destSpaceID pgtype.UUID
				inputText, outputFormat, status         string
				outputText, aiModel                     *string
				tokensUsed                              *int32
				createdAt, expiresAt                    pgtype.Timestamptz
			)

			if err := rows.Scan(
				&id, &userID2, &originSpaceID, &destSpaceID,
				&inputText, &outputText, &outputFormat, &status, &aiModel,
				&tokensUsed, &createdAt, &expiresAt,
			); err != nil {
				slog.Warn("failed to scan cv_format_sessions row for export", "user_id", userID.String(), "error", err)
				continue
			}

			m := map[string]interface{}{
				"id": id.String(),

				"user_id":              userID2.String(),
				"destination_space_id": destSpaceID.String(),
				"input_text":           inputText,
				"output_format":        outputFormat,
				"status":               status,
				"created_at":           createdAt.Time,
				"expires_at":           expiresAt.Time,
			}
			if originSpaceID.Valid {
				m["origin_space_id"] = originSpaceID.String()
			}

			if outputText != nil {
				m["output_text"] = *outputText
			}

			if aiModel != nil {
				m["ai_model"] = *aiModel
			}

			if tokensUsed != nil {
				m["tokens_used"] = *tokensUsed
			}

			collected = append(collected, m)
		}

		rows.Close()

		export.CvFormatSessions = collected
	} else {
		slog.Warn("failed to collect cv_format_sessions for export", "user_id", userID.String(), "error", err)
	}

	// §§107–110: export sop_drafts (generated text + inputs are user data)
	// and crs_score_history so the GDPR export covers the new tool tables.
	// Ref: §110 (ADR-121), §107 (ADR-118), §19.
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, purpose, program, institution, title, inputs,
		       generated_text, status, ai_model, version, created_at
		FROM sop_drafts
		WHERE user_id = $1
	`, userID); err == nil {
		collected := []map[string]interface{}{}
		for rows.Next() {
			var (
				id            pgtype.UUID
				purpose       string
				program       *string
				institution   *string
				title         *string
				inputs        []byte
				generatedText *string
				status        string
				aiModel       *string
				version       int32
				createdAt     pgtype.Timestamptz
			)
			if err := rows.Scan(&id, &purpose, &program, &institution, &title,
				&inputs, &generatedText, &status, &aiModel, &version, &createdAt); err != nil {
				slog.Warn("failed to scan sop_drafts row for export", "user_id", userID.String(), "error", err)
				continue
			}
			m := map[string]interface{}{
				"id":         id.String(),
				"purpose":    purpose,
				"status":     status,
				"version":    version,
				"inputs":     json.RawMessage(inputs),
				"created_at": createdAt.Time,
			}
			if program != nil {
				m["program"] = *program
			}
			if institution != nil {
				m["institution"] = *institution
			}
			if title != nil {
				m["title"] = *title
			}
			if generatedText != nil {
				m["generated_text"] = *generatedText
			}
			if aiModel != nil {
				m["ai_model"] = *aiModel
			}
			collected = append(collected, m)
		}
		rows.Close()
		export.SopDrafts = collected
	} else {
		slog.Warn("failed to collect sop_drafts for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, score, inputs, breakdown, created_at
		FROM crs_score_history
		WHERE user_id = $1
	`, userID); err == nil {
		collected := []map[string]interface{}{}
		for rows.Next() {
			var (
				id        pgtype.UUID
				score     int32
				inputs    []byte
				breakdown []byte
				createdAt pgtype.Timestamptz
			)
			if err := rows.Scan(&id, &score, &inputs, &breakdown, &createdAt); err != nil {
				slog.Warn("failed to scan crs_score_history row for export", "user_id", userID.String(), "error", err)
				continue
			}
			collected = append(collected, map[string]interface{}{
				"id":         id.String(),
				"score":      score,
				"inputs":     json.RawMessage(inputs),
				"breakdown":  json.RawMessage(breakdown),
				"created_at": createdAt.Time,
			})
		}
		rows.Close()
		export.CrsScoreHistory = collected
	} else {
		slog.Warn("failed to collect crs_score_history for export", "user_id", userID.String(), "error", err)
	}

	// Collect remittance_rate_alerts (§74, ADR-075 — user's rate alert
	// subscriptions reveal financial interests and are user data per §15).
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, send_currency, receive_currency, target_rate, direction,
		       is_active, last_triggered_at, created_at
		FROM remittance_rate_alerts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID); err == nil {
		collected := make([]map[string]interface{}, 0)

		for rows.Next() {
			var (
				id              pgtype.UUID
				sendCurrency    string
				receiveCurrency string
				targetRate      pgtype.Numeric
				direction       string
				isActive        bool
				lastTriggered   pgtype.Timestamptz
				createdAt       pgtype.Timestamptz
			)

			if err := rows.Scan(&id, &sendCurrency, &receiveCurrency, &targetRate,
				&direction, &isActive, &lastTriggered, &createdAt); err != nil {
				slog.Warn("failed to scan remittance_rate_alerts row for export", "user_id", userID.String(), "error", err)
				continue
			}

			m := map[string]interface{}{
				"id":               id.String(),
				"send_currency":    sendCurrency,
				"receive_currency": receiveCurrency,
				"target_rate":      targetRate,
				"direction":        direction,
				"is_active":        isActive,
				"created_at":       createdAt.Time,
			}
			if lastTriggered.Valid {
				m["last_triggered_at"] = lastTriggered.Time
			}

			collected = append(collected, m)
		}

		rows.Close()

		export.RemittanceRateAlerts = collected
	} else {
		slog.Warn("failed to collect remittance_rate_alerts for export", "user_id", userID.String(), "error", err)
	}

	// §60 BUG-013: referral data — referral_codes owned by the user, referrals
	// where they are the referrer, and referrals where they are the referee.
	// Referee/referrer PII is excluded from the opposite-side export per §60.
	if rows, err := s.store.gdprExportReferralCodes(ctx, userID); err == nil {
		export.ReferralCodes = rows
	} else {
		slog.Warn("failed to collect referral_codes for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportReferralsAsReferrer(ctx, userID); err == nil {
		export.ReferralsAsReferrer = rows
	} else {
		slog.Warn("failed to collect referrals_as_referrer for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportReferralsAsReferee(ctx, userID); err == nil {
		export.ReferralsAsReferee = rows
	} else {
		slog.Warn("failed to collect referrals_as_referee for export", "user_id", userID.String(), "error", err)
	}

	// BUG-022: export wiki_pages created by the user and wiki_revisions
	// submitted/reviewed by the user so the GDPR data export includes their
	// Destination Wiki contributions (§80, ADR-088).
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, space_id, slug, title, category,
		       is_active, is_featured, is_locked, created_at, updated_at
		FROM wiki_pages
		WHERE created_by = $1
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.WikiPages = append(export.WikiPages, row)
		}
	} else {
		slog.Warn("failed to collect wiki_pages for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.db.Reader.Query(ctx, `
		SELECT id, wiki_page_id, summary, status,
		       ai_generated, ai_model, created_at
		FROM wiki_revisions
		WHERE (submitted_by = $1 OR reviewed_by = $1 OR hitl_reviewed_by = $1)
		ORDER BY created_at DESC
	`, userID); err == nil {
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			cols, _ := rows.Values()

			for i, col := range rows.FieldDescriptions() {
				row[string(col.Name)] = cols[i]
			}

			export.WikiRevisions = append(export.WikiRevisions, row)
		}
	} else {
		slog.Warn("failed to collect wiki_revisions for export", "user_id", userID.String(), "error", err)
	}

	// BUG-012: export AI subsystem data so the GDPR data export covers
	// ai_usage_log, ai_agent_sessions, ai_agent_tool_calls, ai_hitl_reviews,
	// and the user's Honcho AI context. Each call is best-effort (logs on
	// failure) so a failing external Honcho call does not break the export.
	// Ref: japaconnect-engineering/sections/20-ai-core.md
	if rows, err := s.store.gdprExportAIUsageLog(ctx, userID); err == nil {
		export.AIUsageLog = rows
	} else {
		slog.Warn("failed to collect ai_usage_log for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportAIAgentSessions(ctx, userID); err == nil {
		export.AIAgentSessions = rows
	} else {
		slog.Warn("failed to collect ai_agent_sessions for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportAIAgentToolCalls(ctx, userID); err == nil {
		export.AIAgentToolCalls = rows
	} else {
		slog.Warn("failed to collect ai_agent_tool_calls for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportAIHITLReviews(ctx, userID); err == nil {
		export.AIHITLReviews = rows
	} else {
		slog.Warn("failed to collect ai_hitl_reviews for export", "user_id", userID.String(), "error", err)
	}

	if s.honchoFetcher != nil {
		if rows, err := s.honchoFetcher.GetContext(ctx, userID.String()); err == nil {
			export.HonchoContext = rows
		} else {
			slog.Warn("failed to collect honcho context for export", "user_id", userID.String(), "error", err)
		}
	}

	return export, nil
}
