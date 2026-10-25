package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// Service implements the verification-request business logic.
type Service struct {
	store   *Store
	webhooks WebhookEmitter
	asynq   *asynq.Client
	siteURL string
}

// NewService creates a Service backed by the given database pools.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
	}
}

// SetWebhooks injects the webhook event fan-out (Section 12). Nil-safe.
func (s *Service) SetWebhooks(w WebhookEmitter) { s.webhooks = w }

// SetAsynqClient injects the Asynq client used to enqueue email:send tasks.
func (s *Service) SetAsynqClient(c *asynq.Client) { s.asynq = c }

// SetSiteURL injects the public site base URL (JC_SITE_URL) used to build
// absolute frontend links in notification emails. Never derived from the
// request Host (CWE-601).
func (s *Service) SetSiteURL(u string) { s.siteURL = strings.TrimRight(u, "/") }

// emitWebhook fires a verification_request.* / user.verified event
// fire-and-forget (Rule 8). Errors are logged inside the emitter, never
// returned to the caller.
func (s *Service) emitWebhook(ctx context.Context, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}
	s.webhooks.EmitEvent(ctx, eventType, payload)
}

// SubmitRequest files a new verification request (POST /verification-requests).
//
// Eligibility:
//   - entity_type 'organization': org exists+active, not already verified, the
//     requester is the org owner or an owner/admin member, and no pending
//     request exists for that org.
//   - entity_type 'user': always the caller's own profile; caller not already
//     verified; no pending request exists for the caller.
//
// The linked form submission must exist, belong to the requester, and still be
// status='new' (BUG-009 equivalent — a submission is consumed by exactly one
// request).
func (s *Service) SubmitRequest(ctx context.Context, requesterID pgtype.UUID, input SubmitInput) (gen.VerificationRequest, error) {
	if input.EntityType != EntityTypeOrganization && input.EntityType != EntityTypeUser {
		return gen.VerificationRequest{}, fmt.Errorf("%w: entity_type must be 'organization' or 'user'", errpkg.ErrInvalid)
	}

	if input.RequestedType != nil && *input.RequestedType != "" && !IsValidType(*input.RequestedType) {
		return gen.VerificationRequest{}, fmt.Errorf("%w: requested_type must be one of %s", errpkg.ErrInvalid, strings.Join(VerificationTypes, "|"))
	}

	var orgID pgtype.UUID
	var org *gen.Organization

	switch input.EntityType {
	case EntityTypeOrganization:
		if input.OrganizationID == nil || *input.OrganizationID == "" {
			return gen.VerificationRequest{}, fmt.Errorf("%w: organization_id is required for entity_type 'organization'", errpkg.ErrInvalid)
		}
		if err := orgID.Scan(*input.OrganizationID); err != nil {
			return gen.VerificationRequest{}, fmt.Errorf("%w: invalid organization_id", errpkg.ErrInvalid)
		}

		active, err := s.store.ActiveOrgExistsByID(ctx, orgID)
		if err != nil {
			return gen.VerificationRequest{}, fmt.Errorf("checking organization: %w", err)
		}
		if !active {
			return gen.VerificationRequest{}, fmt.Errorf("%w: organization not found or inactive", errpkg.ErrNotFound)
		}

		row, err := s.store.GetOrganizationByIDOnly(ctx, orgID)
		if err != nil {
			return gen.VerificationRequest{}, fmt.Errorf("fetching organization: %w", err)
		}
		org = &row
		if org.Verified {
			return gen.VerificationRequest{}, fmt.Errorf("%w: organization is already verified", errpkg.ErrInvalidStatusTransition)
		}

		// Requester must be the org owner or an owner/admin member.
		allowed := org.OwnerUserID == requesterID
		if !allowed {
			role, err := s.store.GetOrganizationMemberRole(ctx, orgID, requesterID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return gen.VerificationRequest{}, fmt.Errorf("%w: only organization owners and admins may request verification", errpkg.ErrForbidden)
				}
				return gen.VerificationRequest{}, fmt.Errorf("checking membership: %w", err)
			}
			allowed = role == "owner" || role == "admin"
		}
		if !allowed {
			return gen.VerificationRequest{}, fmt.Errorf("%w: only organization owners and admins may request verification", errpkg.ErrForbidden)
		}

		if _, err := s.store.GetPendingForOrg(ctx, orgID); err == nil {
			return gen.VerificationRequest{}, fmt.Errorf("%w: a pending verification request already exists for this organization", errpkg.ErrDuplicate)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return gen.VerificationRequest{}, fmt.Errorf("checking pending requests: %w", err)
		}

	case EntityTypeUser:
		if input.OrganizationID != nil && *input.OrganizationID != "" {
			return gen.VerificationRequest{}, fmt.Errorf("%w: organization_id must be omitted for entity_type 'user'", errpkg.ErrInvalid)
		}

		verified, err := s.store.GetUserVerified(ctx, requesterID)
		if err != nil {
			return gen.VerificationRequest{}, fmt.Errorf("checking user: %w", err)
		}
		if verified {
			return gen.VerificationRequest{}, fmt.Errorf("%w: profile is already verified", errpkg.ErrInvalidStatusTransition)
		}

		if _, err := s.store.GetPendingForUser(ctx, requesterID); err == nil {
			return gen.VerificationRequest{}, fmt.Errorf("%w: you already have a pending verification request", errpkg.ErrDuplicate)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return gen.VerificationRequest{}, fmt.Errorf("checking pending requests: %w", err)
		}
	}

	// The linked submission must be fresh and owned by the requester.
	var submissionID pgtype.UUID
	if err := submissionID.Scan(input.FormSubmissionID); err != nil {
		return gen.VerificationRequest{}, fmt.Errorf("%w: invalid form_submission_id", errpkg.ErrInvalid)
	}
	submission, err := s.store.GetFormSubmissionByID(ctx, submissionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VerificationRequest{}, fmt.Errorf("%w: form submission not found", errpkg.ErrNotFound)
		}
		return gen.VerificationRequest{}, fmt.Errorf("fetching form submission: %w", err)
	}
	if submission.Status != "new" {
		return gen.VerificationRequest{}, fmt.Errorf("%w: form submission has already been consumed", errpkg.ErrInvalidStatusTransition)
	}
	if submission.UserID.Valid && submission.UserID != requesterID {
		return gen.VerificationRequest{}, fmt.Errorf("%w: form submission belongs to another user", errpkg.ErrForbidden)
	}

	return s.store.CreateVerificationRequest(ctx, gen.CreateVerificationRequestParams{
		RequesterUserID:  requesterID,
		EntityType:       input.EntityType,
		OrganizationID:   orgID,
		RequestedType:    strPtr(input.RequestedType),
		FormSubmissionID: submissionID,
	})
}

func strPtr(t *string) *string {
	if t != nil && *t != "" {
		return t
	}
	return nil
}

// ListMyRequests returns the requester's own requests (GET /me/verification-requests).
func (s *Service) ListMyRequests(ctx context.Context, requesterID pgtype.UUID) ([]gen.ListVerificationRequestsByRequesterRow, error) {
	return s.store.ListByRequester(ctx, requesterID)
}

// GetRequest returns a single request, scoped to the requester — a requester
// can only read their own submissions.
func (s *Service) GetRequest(ctx context.Context, id, requesterID pgtype.UUID) (gen.GetVerificationRequestByIDRow, error) {
	row, err := s.store.GetVerificationRequestByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row, fmt.Errorf("%w: verification request not found", errpkg.ErrNotFound)
		}
		return row, fmt.Errorf("fetching request: %w", err)
	}
	if row.RequesterUserID != requesterID {
		return row, fmt.Errorf("%w: verification request not found", errpkg.ErrNotFound)
	}
	return row, nil
}

// WithdrawRequest marks a pending request withdrawn. Only the requester may
// withdraw; reviewed requests are terminal.
func (s *Service) WithdrawRequest(ctx context.Context, id, requesterID pgtype.UUID) error {
	n, err := s.store.Withdraw(ctx, id, requesterID)
	if err != nil {
		return fmt.Errorf("withdrawing request: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: no pending verification request found for withdrawal", errpkg.ErrNotFound)
	}
	return nil
}

// AdminList returns the review queue with total count for pagination.
func (s *Service) AdminList(ctx context.Context, status, entityType string, page, pageSize int32) ([]gen.ListVerificationRequestsRow, int64, error) {
	if pageSize <= 0 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}
	if page <= 0 {
		page = 1
	}
	if status != "" && status != StatusPending && status != StatusApproved && status != StatusRejected && status != StatusWithdrawn {
		return nil, 0, fmt.Errorf("%w: status must be one of pending|approved|rejected|withdrawn", errpkg.ErrInvalid)
	}
	if entityType != "" && entityType != EntityTypeOrganization && entityType != EntityTypeUser {
		return nil, 0, fmt.Errorf("%w: entity_type must be organization|user", errpkg.ErrInvalid)
	}
	return s.store.List(ctx, status, entityType, pageSize, (page-1)*pageSize)
}

// ReviewRequest approves or rejects a pending request (admin, two-step — the
// badge grant is a separate POST /admin/{entity}/{id}/verify call). On a
// successful review the requester is notified by email (fire-and-forget,
// Rule 8) and a verification_request.* webhook event is emitted.
func (s *Service) ReviewRequest(ctx context.Context, id, reviewerID pgtype.UUID, input ReviewInput) error {
	if input.Status != StatusApproved && input.Status != StatusRejected {
		return fmt.Errorf("%w: status must be 'approved' or 'rejected'", errpkg.ErrInvalid)
	}

	// Fetch first — needed for the notification payload and to distinguish
	// "not found" from "not pending" for the execrows path.
	row, err := s.store.GetVerificationRequestByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: verification request not found", errpkg.ErrNotFound)
		}
		return fmt.Errorf("fetching request for review: %w", err)
	}
	if row.Status != StatusPending {
		return fmt.Errorf("%w: only pending requests can be reviewed", errpkg.ErrInvalidStatusTransition)
	}

	n, err := s.store.Review(ctx, gen.ReviewVerificationRequestParams{
		ID:             id,
		Status:         input.Status,
		ReviewedBy:     reviewerID,
		ResolutionNote: notePtr(input.ResolutionNote),
	})
	if err != nil {
		return fmt.Errorf("reviewing request: %w", err)
	}
	if n == 0 {
		// Lost a status race between fetch and update.
		return fmt.Errorf("%w: only pending requests can be reviewed", errpkg.ErrInvalidStatusTransition)
	}

	eventType := "verification_request." + input.Status + "d"
	if input.Status == StatusRejected {
		eventType = "verification_request.rejected"
	}
	s.emitWebhook(ctx, eventType, map[string]any{
		"request_id":  id,
		"entity_type": row.EntityType,
		"entity_id":   entityID(row),
		"status":      input.Status,
	})

	s.enqueueReviewEmail(ctx, row, input)

	return nil
}

func notePtr(note string) *string {
	if note == "" {
		return nil
	}
	return &note
}

func entityID(row gen.GetVerificationRequestByIDRow) any {
	if row.OrganizationID.Valid {
		return row.OrganizationID
	}
	return row.RequesterUserID
}

// enqueueReviewEmail notifies the requester of the review outcome via an
// email:send task. Fire-and-forget (Rule 8) — the review is already persisted.
func (s *Service) enqueueReviewEmail(ctx context.Context, row gen.GetVerificationRequestByIDRow, input ReviewInput) {
	if s.asynq == nil {
		return
	}

	email, err := s.store.GetUserEmail(ctx, row.RequesterUserID)
	if err != nil || email == "" {
		slog.Warn("verification: fetch requester email for review notify",
			"requester_user_id", row.RequesterUserID.String(), "error", err)
		return
	}

	subject := "Your verification request was " + input.Status
	link := "/verification-request"
	if s.siteURL != "" {
		link = s.siteURL + link
	}
	body := fmt.Sprintf(
		"Your verification request has been %s by our review team.\n\n"+
			"You can track the outcome here:\n%s\n",
		input.Status, link,
	)
	if input.ResolutionNote != "" {
		body += "\nNote from the reviewer: " + input.ResolutionNote + "\n"
	}

	payload, err := json.Marshal(map[string]any{
		"to":      email,
		"subject": subject,
		"body":    body,
		"type":    "verification_review",
	})
	if err != nil {
		slog.Error("verification: marshal review email payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeEmailSend, payload)
	if _, err := s.asynq.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeEmailSend))); err != nil {
		slog.Error("verification: enqueue review email", "to", email, "error", err)
	}
}
