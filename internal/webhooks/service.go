package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// MaxDeliveryAttempts is the roadmap-mandated retry count for webhook delivery.
const MaxDeliveryAttempts = 5

// secretByteLength is the number of random bytes used to generate a
// per-subscription HMAC-SHA256 signing secret. 32 bytes = 256 bits.
const secretByteLength = 32

// challengeTokenByteLength is the number of random bytes used for the
// challenge/response verification token (§12). 16 bytes = 128 bits.
const challengeTokenByteLength = 16

// webhooksSettingsReader is the subset of config.Settings webhooks needs.
type webhooksSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetStringOr(ctx context.Context, key, fallback string) string
}

// Service coordinates webhook endpoint lookup, signed delivery, and delivery
// status tracking. It implements the worker.WebhookProcessor interface.
type Service struct {
	store    *Store
	settings webhooksSettingsReader
	siteName string
	enqueuer webhookEnqueuer
}

// webhookEnqueuer is the subset of worker.Enqueuer that webhooks needs.
type webhookEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SetSettings injects the hot-reloadable settings reader (AUDIT-BE-009).
func (s *Service) SetSettings(r webhooksSettingsReader) { s.settings = r }

// SetEnqueuer injects the Asynq enqueuer for webhook delivery tasks.
func (s *Service) SetEnqueuer(e webhookEnqueuer) { s.enqueuer = e }

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, siteName string) *Service {
	return &Service{store: NewStore(database), siteName: siteName}
}

// generateSecret generates a cryptographically random hex-encoded signing
// secret for a new webhook endpoint (§12: server-generated, shown once).
func generateSecret() (string, error) {
	b := make([]byte, secretByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating webhook secret: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// generateChallengeToken generates a cryptographically random hex-encoded
// challenge token for the §12 challenge/response verification.
func generateChallengeToken() (string, error) {
	b := make([]byte, challengeTokenByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating challenge token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// generateEventID generates a unique UUID for each webhook event occurrence
// (BUG-022). The §12 envelope's event_id must be unique per event so
// consumers can deduplicate and trace deliveries. Previously all deliveries
// shared the zero UUID, making the field useless.
func generateEventID() (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		return pgtype.UUID{}, fmt.Errorf("generating event ID: %w", err)
	}

	return id, nil
}

// ---------------------------------------------------------------------------
// Admin CRUD — Webhook Endpoints
// ---------------------------------------------------------------------------

// ListEndpoints returns all webhook endpoints for the given platform.
func (s *Service) ListEndpoints(ctx context.Context) ([]WebhookEndpointResponse, error) {
	rows, err := s.store.ListEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	var endpoints []WebhookEndpointResponse
	for _, ep := range rows {
		endpoints = append(endpoints, ep.ToResponse())
	}

	return endpoints, nil
}

// GetEndpoint returns a single webhook endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, id pgtype.UUID) (WebhookEndpointResponse, error) {
	ep, err := s.store.GetEndpoint(ctx, id)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	return ep.ToResponse(), nil
}

// CreateEndpoint registers a new webhook endpoint. The secret is
// server-generated and returned in the WebhookEndpointWithSecret so the
// handler can include it in the create response exactly once (§12). New
// endpoints default to is_active=false until challenge/response verification
// (BUG-002). createdBy is the admin user creating the endpoint.
func (s *Service) CreateEndpoint(ctx context.Context, createdBy pgtype.UUID, input CreateWebhookRequest) (WebhookEndpointWithSecret, error) {
	secret, err := generateSecret()
	if err != nil {
		return WebhookEndpointWithSecret{}, err
	}

	ep, err := s.store.CreateEndpoint(ctx, input.Name, input.URL, secret, input.EventTypes, createdBy)
	if err != nil {
		return WebhookEndpointWithSecret{}, err
	}

	return ep, nil
}

// UpdateEndpoint updates an existing webhook endpoint. The signing secret is
// never changed by this method — §12 PATCH only accepts { name, url,
// event_types, is_active } (BUG-023). The current secret is preserved
// as-is (already encrypted at rest from the initial create).
func (s *Service) UpdateEndpoint(ctx context.Context, id pgtype.UUID, input UpdateWebhookRequest) (WebhookEndpointResponse, error) {
	// Fetch current values for merge.
	current, err := s.store.GetEndpoint(ctx, id)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}

	url := current.URL
	if input.URL != nil {
		url = *input.URL
	}

	// BUG-023: preserve the current secret — the client cannot supply or
	// rotate the signing secret via PATCH. Secret rotation requires a
	// dedicated endpoint that generates a new server-side secret.
	secret := current.Secret

	eventTypes := current.EventTypes
	if input.EventTypes != nil {
		eventTypes = input.EventTypes
	}

	status := current.Status

	isActive := current.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
		if isActive {
			// §12: an endpoint may only be activated after challenge/response
			// verification. Reject attempts to set is_active=true when the
			// endpoint has not been verified (verified_at IS NULL). The
			// admin must call POST /admin/webhooks/{id}/verify first.
			if !current.VerifiedAt.Valid {
				return WebhookEndpointResponse{}, errors.New("webhook endpoint not verified: complete challenge/response verification before activating")
			}

			status = "active"
		} else {
			status = "disabled"
		}
	}

	updated, err := s.store.UpdateEndpoint(ctx, id, name, url, secret, eventTypes, status, isActive)
	if err != nil {
		return WebhookEndpointResponse{}, err
	}

	return updated.ToResponse(), nil
}

// DeleteEndpoint removes a webhook endpoint by ID.
func (s *Service) DeleteEndpoint(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteEndpoint(ctx, id)
}

// ---------------------------------------------------------------------------
// Personal webhook endpoints (scope='user') — Phase 3
// API-key holders subscribe to content.published; deliveries are filtered to
// content the owner can see at emit time.
// ---------------------------------------------------------------------------

// ErrChallengeFailed marks a failed challenge/response verification so
// handlers can map it to 502 without string-matching.
var ErrChallengeFailed = errors.New("webhook challenge/response verification failed")

// CreateUserEndpoint registers a personal webhook endpoint. The secret is
// server-generated and returned in the row for the one-time create response.
func (s *Service) CreateUserEndpoint(ctx context.Context, ownerID pgtype.UUID, input CreateWebhookRequest) (WebhookEndpointWithSecret, error) {
	secret, err := generateSecret()
	if err != nil {
		return WebhookEndpointWithSecret{}, err
	}

	return s.store.CreateUserEndpoint(ctx, ownerID, input.Name, input.URL, secret, input.EventTypes)
}

// ListUserEndpoints returns the owner's personal endpoints (secrets withheld).
func (s *Service) ListUserEndpoints(ctx context.Context, ownerID pgtype.UUID) ([]WebhookEndpointResponse, error) {
	return s.store.ListUserEndpoints(ctx, ownerID)
}

// CountUserEndpoints returns the owner's personal-endpoint count for the
// api.webhooks.max_per_key cap.
func (s *Service) CountUserEndpoints(ctx context.Context, ownerID pgtype.UUID) (int64, error) {
	return s.store.CountUserEndpoints(ctx, ownerID)
}

// DeleteUserEndpoint removes a personal endpoint owned by the caller.
// Returns ErrNotFound when absent or owned by someone else.
func (s *Service) DeleteUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error {
	n, err := s.store.DeleteUserEndpoint(ctx, ownerID, id)
	if err != nil {
		return err
	}

	if n == 0 {
		return fmt.Errorf("%w: webhook endpoint", errpkg.ErrNotFound)
	}

	return nil
}

// VerifyUserEndpoint runs the §12 challenge/response on an owner-scoped
// personal endpoint and activates it on success.
func (s *Service) VerifyUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error {
	ep, err := s.store.GetUserEndpoint(ctx, ownerID, id)
	if err != nil {
		return fmt.Errorf("%w: webhook endpoint", errpkg.ErrNotFound)
	}

	token, err := generateChallengeToken()
	if err != nil {
		return err
	}

	challengeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := PerformChallenge(challengeCtx, ep.URL, token); err != nil {
		slog.Warn("user webhook challenge/response failed", "endpoint_id", id, "url", ep.URL, "error", err)
		return fmt.Errorf("%w: %w", ErrChallengeFailed, err)
	}

	if n, err := s.store.VerifyUserEndpoint(ctx, ownerID, id); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: webhook endpoint", errpkg.ErrNotFound)
	}

	slog.Info("user webhook endpoint verified", "endpoint_id", id, "url", ep.URL)

	return nil
}

// ListUserDeliveries returns deliveries for an owner-scoped endpoint after
// verifying ownership — a foreign endpoint ID yields ErrNotFound.
func (s *Service) ListUserDeliveries(ctx context.Context, ownerID, endpointID pgtype.UUID, limit, offset int32) ([]WebhookDeliveryResponse, error) {
	if _, err := s.store.GetUserEndpoint(ctx, ownerID, endpointID); err != nil {
		return nil, fmt.Errorf("%w: webhook endpoint", errpkg.ErrNotFound)
	}

	return s.ListDeliveries(ctx, endpointID, limit, offset)
}

// VerifyEndpoint performs the §12 challenge/response verification: generates a
// random token, sends a GET to {url}?challenge={token}, and verifies the
// endpoint echoes the token back. On success, marks the endpoint as verified
// and active (BUG-001). Returns an error if the challenge fails.
func (s *Service) VerifyEndpoint(ctx context.Context, id pgtype.UUID) error {
	endpoint, err := s.store.GetEndpoint(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching endpoint for verification: %w", err)
	}

	token, err := generateChallengeToken()
	if err != nil {
		return err
	}

	challengeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := PerformChallenge(challengeCtx, endpoint.URL, token); err != nil {
		slog.Warn("webhook challenge/response failed", "endpoint_id", id, "url", endpoint.URL, "error", err)
		return fmt.Errorf("challenge/response verification failed: %w", err)
	}

	if err := s.store.VerifyEndpoint(ctx, id); err != nil {
		return err
	}

	slog.Info("webhook endpoint verified", "endpoint_id", id, "url", endpoint.URL)

	return nil
}

// TestEndpoint sends a test event to a webhook endpoint and records the delivery.
func (s *Service) TestEndpoint(ctx context.Context, endpointID pgtype.UUID, eventType string) (pgtype.UUID, error) {
	// Verify the endpoint exists and belongs to this platform.
	endpoint, err := s.store.GetEndpoint(ctx, endpointID)
	if err != nil {
		return pgtype.UUID{}, err
	}

	testPayload := map[string]any{
		"type":    "test",
		"message": "This is a test webhook delivery from " + s.siteName,
	}

	// BUG-022: generate a unique event ID per event occurrence so the
	// envelope's event_id is meaningful for deduplication and tracing.
	eventID, err := generateEventID()
	if err != nil {
		return pgtype.UUID{}, err
	}

	deliveryID, err := s.CreateDelivery(ctx, endpoint.ID, eventID, eventType, testPayload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating test delivery: %w", err)
	}

	// Enqueue the async delivery task.
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
		// Without this task the delivery row stays 'pending' forever — surface a failed enqueue.
		if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, payload); err != nil {
			slog.Error("webhooks: failed to enqueue test delivery", "delivery_id", deliveryID.String(), "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
		}
	}

	slog.Info("webhook test delivery created", "delivery_id", deliveryID, "endpoint_id", endpointID, "event_type", eventType)

	return deliveryID, nil
}

// DeliverWebhook fetches the delivery and endpoint rows, builds a signed
// envelope, attempts a single delivery, and updates the delivery status.
// If the delivery fails and retries remain, it records the 'retrying' state
// with next_attempt_at and returns an error so Asynq re-enqueues the task
// with native retry (BUG-017). If all retries are exhausted, it records
// 'failed' and returns nil so Asynq does not re-enqueue indefinitely.
func (s *Service) DeliverWebhook(ctx context.Context, deliveryID [16]byte) error {
	id := pgtype.UUID{Bytes: deliveryID, Valid: true}

	delivery, err := s.store.GetWebhookDelivery(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching webhook delivery: %w", err)
	}

	endpoint, err := s.store.GetWebhookEndpoint(ctx, delivery.EndpointID)
	if err != nil {
		return fmt.Errorf("fetching webhook endpoint: %w", err)
	}

	// §12: only active (verified) endpoints receive deliveries. An inactive
	// endpoint is marked failed so the delivery log reflects the skip reason.
	if !endpoint.IsActive {
		slog.Info("webhook endpoint inactive, skipping delivery", "delivery_id", delivery.ID, "endpoint_id", endpoint.ID)
		return s.store.MarkWebhookFailed(ctx, id, delivery.Attempts, "endpoint inactive or not verified")
	}

	var data any
	if err := json.Unmarshal(delivery.Payload, &data); err != nil {
		return fmt.Errorf("unmarshaling webhook payload: %w", err)
	}

	envelope := WebhookEnvelope{
		EventID:       delivery.EventID.String(),
		EventType:     delivery.EventType,
		OccurredAt:    delivery.CreatedAt.Time,
		SchemaVersion: SchemaVersion,
		Data:          data,
	}

	maxAttempts := MaxDeliveryAttempts
	if s.settings != nil {
		maxAttempts = s.settings.GetIntOr(ctx, "notifications.max_delivery_attempts", MaxDeliveryAttempts)
	}

	// Single attempt per task invocation (BUG-017: async retry via Asynq
	// native retry instead of blocking the worker with synchronous sleeps).
	result, deliverErr := Deliver(ctx, endpoint.URL, endpoint.Secret, envelope)
	attempt := delivery.Attempts + 1

	// Capture the response code and body for the delivery log (§12).
	var respCode *int32

	var respBody *string

	if result.StatusCode > 0 {
		// #nosec G115 // value bounded by caller
		c := int32(result.StatusCode)
		respCode = &c
	}

	if result.ResponseBody != "" {
		b := result.ResponseBody
		respBody = &b
	}

	if deliverErr == nil {
		if err := s.store.MarkWebhookDelivered(ctx, id, attempt, respCode, respBody); err != nil {
			slog.Error("failed to mark webhook delivery as delivered", "delivery_id", id, "error", err)
		}

		slog.Info("webhook delivered successfully", "delivery_id", id, "endpoint", endpoint.URL, "event_type", delivery.EventType, "attempt", attempt)

		return nil
	}

	// Delivery failed. If retries remain, record 'retrying' with next_attempt_at
	// and return an error so Asynq re-enqueues (BUG-017).
	//nolint:gosec // G115: bounded
	if attempt < int32(maxAttempts) { // #nosec //nolint:gosec // G115: bounded
		nextAttempt := time.Now().Add(backoffDuration(int(attempt)))
		errMsg := deliverErr.Error()

		if markErr := s.store.MarkWebhookRetrying(ctx, id, attempt, errMsg, nextAttempt, respCode, respBody); markErr != nil {
			slog.Error("failed to mark webhook delivery as retrying", "delivery_id", id, "error", markErr)
		}

		slog.Warn("webhook delivery failed, will retry", "delivery_id", id, "endpoint", endpoint.URL, "attempt", attempt, "max_attempts", maxAttempts, "next_attempt_at", nextAttempt, "error", deliverErr)

		return fmt.Errorf("webhook delivery attempt %d failed: %w", attempt, deliverErr)
	}

	// All retries exhausted — mark as failed and return nil so Asynq stops.
	errMsg := deliverErr.Error()
	if markErr := s.store.MarkWebhookFailed(ctx, id, attempt, errMsg); markErr != nil {
		slog.Error("failed to mark webhook delivery as failed", "delivery_id", id, "error", markErr)
	}

	slog.Error("webhook delivery failed after all retries", "delivery_id", id, "endpoint", endpoint.URL, "attempts", attempt, "error", deliverErr)

	return nil
}

// backoffDuration returns the delay before the next retry attempt. The schedule
// is exponential: 1s, 2s, 4s, 8s, 16s, ... for attempts 1, 2, 3, 4, 5, ...
func backoffDuration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	if attempt > 30 {
		attempt = 30
	}

	return time.Duration(1<<uint(attempt-1)) * time.Second //nolint:gosec // G115: attempt bounded to 30
}

// ---------------------------------------------------------------------------
// Delivery Logs
// ---------------------------------------------------------------------------

// ListDeliveries returns delivery records for a specific endpoint.
func (s *Service) ListDeliveries(ctx context.Context, endpointID pgtype.UUID, limit, offset int32) ([]WebhookDeliveryResponse, error) {
	rows, err := s.store.ListDeliveries(ctx, endpointID, limit, offset)
	if err != nil {
		return nil, err
	}

	var deliveries []WebhookDeliveryResponse
	for _, d := range rows {
		deliveries = append(deliveries, d.ToResponse())
	}

	return deliveries, nil
}

// GetDelivery returns a specific delivery record.
func (s *Service) GetDelivery(ctx context.Context, deliveryID pgtype.UUID) (*WebhookDelivery, error) {
	d, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}

	return &d, nil
}

// RetryDelivery reschedules a failed delivery for immediate retry. It
// verifies that the delivery belongs to the given endpointID (BUG-025:
// previously did not verify ownership, allowing cross-webhook retry).
func (s *Service) RetryDelivery(ctx context.Context, endpointID, deliveryID pgtype.UUID) error {
	// Verify the delivery belongs to this endpoint before retrying.
	delivery, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("fetching delivery for retry: %w", err)
	}

	if delivery.EndpointID != endpointID {
		return fmt.Errorf("delivery %s does not belong to webhook endpoint %s", deliveryID, endpointID)
	}

	if err := s.store.RetryDelivery(ctx, deliveryID); err != nil {
		return err
	}

	// Enqueue the retry delivery task.
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
		// The row was just reset to 'pending'; a lost enqueue would strand it — surface the failure.
		if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, payload); err != nil {
			slog.Error("webhooks: failed to enqueue retry delivery", "delivery_id", deliveryID.String(), "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
		}
	}

	return nil
}

// CreateDelivery records a pending webhook delivery in the database. Callers
// enqueue the Asynq webhook_delivery task with the returned delivery ID.
func (s *Service) CreateDelivery(ctx context.Context, endpointID, eventID pgtype.UUID, eventType string, payload any) (pgtype.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshaling webhook payload: %w", err)
	}

	return s.store.CreateDelivery(ctx, endpointID, eventID, eventType, payloadJSON)
}

// EmitEvent dispatches a webhook event to all active endpoints for the platform
// that subscribe to the given event type. For each matching endpoint it
// creates a pending delivery row and enqueues the Asynq webhook_delivery
// task. This is the canonical entry point for feature services to fire
// outgoing webhooks (Section 12).
// // Ref: japaconnect-engineering/sections/12-webhooks.md.
func (s *Service) EmitEvent(ctx context.Context, eventType string, payload any) {
	endpoints, err := s.store.ListEndpoints(ctx)
	if err != nil {
		slog.Warn("webhooks: failed to list endpoints for emit", "error", err)
		return
	}

	// BUG-022: generate a unique event ID per event occurrence so the
	// envelope's event_id is meaningful for deduplication and tracing.
	// All deliveries for this event share the same event_id, which is
	// correct — they are fan-out copies of the same logical event.
	eventID, err := generateEventID()
	if err != nil {
		slog.Warn("webhooks: failed to generate event ID", "error", err)
		return
	}

	for _, ep := range endpoints {
		// §12: only verified, active endpoints receive deliveries.
		if !ep.IsActive {
			continue
		}

		// Check if the endpoint subscribes to this event type.
		if !endpointMatches(ep.EventTypes, eventType) {
			continue
		}

		// Phase 3: personal endpoints (scope='user') only receive events
		// for content the owner can see. Fail closed — an unresolvable
		// content reference or a failed check means no delivery row at
		// all, so a private publish can never produce a phantom signed
		// payload for a subscriber who shouldn't see it.
		if ep.Scope == "user" {
			if !s.userEndpointMayReceive(ctx, ep, eventType, payload) {
				continue
			}
		}

		deliveryID, err := s.CreateDelivery(ctx, ep.ID, eventID, eventType, payload)
		if err != nil {
			slog.Error("webhooks: failed to create delivery for emit", "endpoint_id", ep.ID, "event_type", eventType, "error", err)
			errpkg.CaptureException(fmt.Errorf("emit create delivery %s: %w", eventType, err))

			continue
		}

		if s.enqueuer != nil {
			taskPayload, _ := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
			if err := s.enqueuer.Enqueue(worker.TypeWebhookDelivery, taskPayload); err != nil {
				slog.Error("webhooks: failed to enqueue delivery for emit", "delivery_id", deliveryID.String(), "error", err)
				errpkg.CaptureException(fmt.Errorf("enqueue %s (delivery %s): %w", worker.TypeWebhookDelivery, deliveryID.String(), err))
			}
		}
	}
}

// userEndpointMayReceive reports whether a personal endpoint should receive
// this event: the event must be content.*, carry a resolvable content_id,
// and that content must be visible to the endpoint's owner (created_by) per
// the shared visibility predicate (status/visibility/space membership).
func (s *Service) userEndpointMayReceive(ctx context.Context, ep WebhookEndpointWithSecret, eventType string, payload any) bool {
	if !ep.CreatedBy.Valid {
		return false
	}

	if !strings.HasPrefix(eventType, "content.") {
		return false
	}

	// Payloads are map[string]any from content emitters; marshal/unmarshal
	// keeps this tolerant of any structured payload shape.
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	var body struct {
		ContentID string `json:"content_id"`
	}

	if err := json.Unmarshal(raw, &body); err != nil || body.ContentID == "" {
		return false
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(body.ContentID); err != nil {
		return false
	}

	visible, err := s.store.IsContentVisibleToUser(ctx, contentID, ep.CreatedBy)
	if err != nil {
		slog.Warn("webhooks: visibility check failed for user endpoint",
			"endpoint_id", ep.ID, "content_id", body.ContentID, "error", err)
		return false
	}

	return visible
}

// endpointMatches reports whether the endpoint's event_types list includes
// the event (or a "*" wildcard).
func endpointMatches(eventTypes []string, eventType string) bool {
	for _, t := range eventTypes {
		if t == "*" || t == eventType {
			return true
		}
	}

	return false
}
