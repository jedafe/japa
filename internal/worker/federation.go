package worker

// Asynq handlers for ActivityPub member provisioning (§126 auto opt-in).
// The service is registered by server setup — the handler is a thin
// decode+gate+dispatch shell; all actor semantics live in
// internal/federation.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// federationProvisioner is the seam the auto-opt-in handler provisions
// actors through — *federation.Service satisfies it in production. A
// local interface keeps worker out of federation's import graph (same
// shape as atprotoRepoWriter above). InvalidateActorIdentityForUser
// joins it for the lifecycle path: transitions out of 'active' must
// drop cached WebFinger/actor docs, not just freeze provisioning.
type federationProvisioner interface {
	AutoProvision(ctx context.Context, userID pgtype.UUID) error
	InvalidateActorIdentityForUser(ctx context.Context, userID pgtype.UUID, extraHandles ...string)
	// §126 M2 — inbound dispatch + outbound delivery seams.
	ProcessInbound(ctx context.Context, inboundID pgtype.UUID) error
	Fanout(ctx context.Context, contentID pgtype.UUID) error
	FanoutActorDelete(ctx context.Context, actorID pgtype.UUID) error
	Deliver(ctx context.Context, deliveryID pgtype.UUID) error
	ResolveRemoteActorTask(ctx context.Context, actorURL string) error
	PruneDeliveries(ctx context.Context, retentionDays int) error
}

var federationSvc federationProvisioner

// RegisterFederationService wires the AP service for federation:* task
// handlers. Called once during server setup.
func RegisterFederationService(s federationProvisioner) { federationSvc = s }

// EnqueueFederationAutoOptin fires federation:auto_optin after a member
// account is created — the §126 auto opt-in path. The handler re-checks
// the federation.auto_optin setting and each feature flag at run time, so
// enqueueing unconditionally at signup is correct: the task is a no-op
// whenever auto opt-in is off. Fire-and-forget (Rule 8); a nil enqueuer
// is a no-op.
func EnqueueFederationAutoOptin(ctx context.Context, userID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"user_id": userID.String()})
	if err != nil {
		slog.Warn("worker: federation auto-optin enqueue marshal failed", "user_id", userID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeFederationAutoOptin, payload); err != nil {
		slog.Warn("worker: federation auto-optin enqueue failed", "user_id", userID.String(), "error", err)
	}
}

// fireFederationAutoOptin is the indirection handlers use to enqueue the
// auto-opt-in task — a var so tests can observe the trigger.
var fireFederationAutoOptin = EnqueueFederationAutoOptin

// HandleFederationAutoOptin processes federation:auto_optin (§126 auto
// opt-in): provision the new member's federated identity — AP actor under
// feature.federation, atproto account under feature.atproto — when
// federation.auto_optin is on. AutoProvision only creates absent rows, so
// retries and re-enqueues converge and a member's explicit opt-out is
// never resurrected.
func HandleFederationAutoOptin(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:auto_optin")
	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("federation auto-optin: decode payload: %w: %w", err, asynq.SkipRetry)
	}
	var userID pgtype.UUID
	if err := userID.Scan(payload.UserID); err != nil || !userID.Valid {
		return fmt.Errorf("federation auto-optin: bad user_id %q: %w", payload.UserID, asynq.SkipRetry)
	}

	// Runtime gate — the setting is the whole point of the switch: off
	// means explicit member opt-in and the task exits cheaply.
	if appSettings == nil || !appSettings.GetBoolOr(ctx, "federation.auto_optin", true) {
		return nil
	}

	if featureChecker != nil && featureChecker.IsEnabled(ctx, "feature.federation") {
		if federationSvc == nil {
			return fmt.Errorf("federation auto-optin: service not registered")
		}
		if err := federationSvc.AutoProvision(ctx, userID); err != nil {
			return skipIfPermanent(fmt.Errorf("federation auto-optin: provision actor: %w", err))
		}
	}

	if featureChecker != nil && featureChecker.IsEnabled(ctx, "feature.atproto") {
		if atprotoSvc == nil {
			return fmt.Errorf("federation auto-optin: atproto service not registered")
		}
		if err := atprotoSvc.AutoProvision(ctx, userID); err != nil {
			return skipIfPermanent(fmt.Errorf("federation auto-optin: provision atproto: %w", err))
		}
	}

	return nil
}

// Backfill defaults — bound each task's work; the handler self-continues
// with a keyset cursor until the users table is exhausted.
const (
	federationBackfillDefaultBatch = 100
	federationBackfillMaxBatch     = 500
)

// listActiveUserIDs pages active, non-deleted users by keyset cursor —
// a var so tests can substitute a fake; dbPool is the worker-wide seam.
var listActiveUserIDs = func(ctx context.Context, after pgtype.UUID, limit int) ([]pgtype.UUID, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("db pool not registered")
	}
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT id FROM users WHERE status = 'active' AND id > $1 ORDER BY id LIMIT $2`,
		after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// enqueueBackfillContinuation self-enqueues the next page of
// federation:backfill_optin — a var so tests can assert the cursor.
var enqueueBackfillContinuation = func(cursor string, batch int) error {
	if taskEnqueuer == nil {
		return fmt.Errorf("enqueuer not registered")
	}
	payload, err := json.Marshal(map[string]any{"cursor": cursor, "batch": batch})
	if err != nil {
		return err
	}
	return taskEnqueuer.Enqueue(TypeFederationBackfillOptin, payload)
}

// HandleFederationBackfillOptin processes federation:backfill_optin (§126
// backfill): keyset-page active users and run the same AutoProvision calls
// as the signup task. An explicit operator decision — deliberately NOT
// gated on federation.auto_optin (that setting controls implicit signup
// provisioning; the backfill converges members who predate it or raced
// it). Feature flags still gate each surface and AutoProvision keeps every
// guarantee: absent-row only, ineligible members skip clean, opt-outs are
// never resurrected.
func HandleFederationBackfillOptin(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:backfill_optin")

	var payload struct {
		Cursor string `json:"cursor"`
		Batch  int    `json:"batch"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	after := pgtype.UUID{Valid: true} // zero uuid — first page
	if payload.Cursor != "" {
		if err := after.Scan(payload.Cursor); err != nil || !after.Valid {
			return fmt.Errorf("federation backfill: bad cursor %q: %w", payload.Cursor, asynq.SkipRetry)
		}
	}
	batch := payload.Batch
	if batch <= 0 || batch > federationBackfillMaxBatch {
		batch = federationBackfillDefaultBatch
	}

	fedOn := featureChecker != nil && featureChecker.IsEnabled(ctx, "feature.federation")
	atpOn := featureChecker != nil && featureChecker.IsEnabled(ctx, "feature.atproto")
	if !fedOn && !atpOn {
		return nil // both surfaces dark — nothing to converge
	}
	if fedOn && federationSvc == nil {
		return fmt.Errorf("federation backfill: service not registered")
	}
	if atpOn && atprotoSvc == nil {
		return fmt.Errorf("federation backfill: atproto service not registered")
	}

	ids, err := listActiveUserIDs(ctx, after, batch)
	if err != nil {
		return fmt.Errorf("federation backfill: page users: %w", err)
	}

	var last pgtype.UUID
	for _, id := range ids {
		last = id
		if fedOn {
			if err := federationSvc.AutoProvision(ctx, id); err != nil {
				return skipIfPermanent(fmt.Errorf("federation backfill: actor %s: %w", id.String(), err))
			}
		}
		if atpOn {
			if err := atprotoSvc.AutoProvision(ctx, id); err != nil {
				return skipIfPermanent(fmt.Errorf("federation backfill: atproto %s: %w", id.String(), err))
			}
		}
	}

	// A full page means more users may follow — continue from the last id.
	// A retry of THIS task re-pages the same cursor, so continuation is
	// safe to enqueue only on success.
	if len(ids) == batch {
		if err := enqueueBackfillContinuation(last.String(), batch); err != nil {
			return fmt.Errorf("federation backfill: enqueue next page: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// §126 M2 — inbound dispatch + outbound delivery
// ---------------------------------------------------------------------------

// EnqueueFederationFanout fires federation:fanout for one content id —
// called by content's post-commit emit sites (publish/update/unpublish/
// delete). The task re-checks eligibility at run time, so enqueueing on
// every transition is correct: ineligible-at-run-time fanouts a Delete or
// no-ops. Fire-and-forget (Rule 8); a nil enqueuer is a no-op.
func EnqueueFederationFanout(ctx context.Context, contentID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"content_id": contentID.String()})
	if err != nil {
		slog.Warn("worker: federation fanout enqueue marshal failed", "content_id", contentID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeFederationFanout, payload); err != nil {
		slog.Warn("worker: federation fanout enqueue failed", "content_id", contentID.String(), "error", err)
	}
}

// EnqueueFederationActorDelete fires federation:fanout for an actor tombstone
// — disable/erasure paths propagate Delete{actor} to remote followers.
func EnqueueFederationActorDelete(ctx context.Context, actorID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"actor_id": actorID.String()})
	if err != nil {
		return
	}
	if err := taskEnqueuer.Enqueue(TypeFederationFanout, payload); err != nil {
		slog.Warn("worker: federation actor-delete enqueue failed", "actor_id", actorID.String(), "error", err)
	}
}

// enqueueFederationDelivery kicks one delivery row — the production
// TaskEnqueuer seam for *federation.Service.
func enqueueFederationDelivery(ctx context.Context, deliveryID pgtype.UUID) error {
	if taskEnqueuer == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
	if err != nil {
		return err
	}
	return taskEnqueuer.Enqueue(TypeFederationDeliverActivity, payload)
}

// enqueueFederationDeliveryAfter schedules a retry attempt — asynq
// ProcessIn matches the ledger's next_retry_at.
func enqueueFederationDeliveryAfter(ctx context.Context, deliveryID pgtype.UUID, delay time.Duration) error {
	if taskEnqueuer == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"delivery_id": deliveryID.String()})
	if err != nil {
		return err
	}
	return taskEnqueuer.Enqueue(TypeFederationDeliverActivity, payload, asynq.ProcessIn(delay))
}

// enqueueFederationInbound kicks a ledgered inbound row — the production
// TaskEnqueuer seam for the inbox handlers.
func enqueueFederationInbound(ctx context.Context, inboundID pgtype.UUID) error {
	if taskEnqueuer == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"inbound_id": inboundID.String()})
	if err != nil {
		return err
	}
	return taskEnqueuer.Enqueue(TypeFederationProcessInbound, payload)
}

// federationTaskEnqueuer adapts the package enqueue helpers to
// federation.Service's TaskEnqueuer seam — wired at server setup because
// the federation package can't import worker.
type federationTaskEnqueuer struct{}

// FederationEnqueuer is the value server setup hands to SetEnqueuer.
func FederationEnqueuer() federationTaskEnqueuer { return federationTaskEnqueuer{} }

func (federationTaskEnqueuer) EnqueueDelivery(ctx context.Context, id pgtype.UUID) error {
	return enqueueFederationDelivery(ctx, id)
}
func (federationTaskEnqueuer) EnqueueDeliveryAfter(ctx context.Context, id pgtype.UUID, d time.Duration) error {
	return enqueueFederationDeliveryAfter(ctx, id, d)
}
func (federationTaskEnqueuer) EnqueueInbound(ctx context.Context, id pgtype.UUID) error {
	return enqueueFederationInbound(ctx, id)
}

// HandleFederationFanout processes federation:fanout — expands one
// content lifecycle event (or actor delete) into per-inbox delivery rows.
func HandleFederationFanout(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:fanout")
	var payload struct {
		ContentID string `json:"content_id"`
		ActorID   string `json:"actor_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	if federationSvc == nil {
		return fmt.Errorf("federation fanout: service not registered")
	}
	if payload.ActorID != "" {
		var actorID pgtype.UUID
		if err := actorID.Scan(payload.ActorID); err != nil || !actorID.Valid {
			return fmt.Errorf("federation fanout: bad actor_id %q: %w", payload.ActorID, asynq.SkipRetry)
		}
		return federationSvc.FanoutActorDelete(ctx, actorID)
	}
	var contentID pgtype.UUID
	if err := contentID.Scan(payload.ContentID); err != nil || !contentID.Valid {
		return fmt.Errorf("federation fanout: bad content_id %q: %w", payload.ContentID, asynq.SkipRetry)
	}
	return federationSvc.Fanout(ctx, contentID)
}

// HandleFederationDeliverActivity processes federation:deliver_activity —
// one signed POST attempt. The ledger owns attempts/backoff; the handler
// is a thin dispatch.
func HandleFederationDeliverActivity(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:deliver_activity")
	var payload struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	var id pgtype.UUID
	if err := id.Scan(payload.DeliveryID); err != nil || !id.Valid {
		return fmt.Errorf("federation deliver: bad delivery_id %q: %w", payload.DeliveryID, asynq.SkipRetry)
	}
	if federationSvc == nil {
		return fmt.Errorf("federation deliver: service not registered")
	}
	return federationSvc.Deliver(ctx, id)
}

// HandleFederationProcessInbound processes federation:process_inbound —
// dispatch a signature-verified, ledgered activity.
func HandleFederationProcessInbound(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:process_inbound")
	var payload struct {
		InboundID string `json:"inbound_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	var id pgtype.UUID
	if err := id.Scan(payload.InboundID); err != nil || !id.Valid {
		return fmt.Errorf("federation inbound: bad inbound_id %q: %w", payload.InboundID, asynq.SkipRetry)
	}
	if federationSvc == nil {
		return fmt.Errorf("federation inbound: service not registered")
	}
	return federationSvc.ProcessInbound(ctx, id)
}

// HandleFederationResolveActor processes federation:resolve_actor —
// fetch + upsert one remote_actors row (low tier).
func HandleFederationResolveActor(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:resolve_actor")
	var payload struct {
		ActorURL string `json:"actor_url"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	if payload.ActorURL == "" {
		return fmt.Errorf("federation resolve: empty actor_url: %w", asynq.SkipRetry)
	}
	if federationSvc == nil {
		return fmt.Errorf("federation resolve: service not registered")
	}
	return federationSvc.ResolveRemoteActorTask(ctx, payload.ActorURL)
}

// HandleFederationPruneDeliveries processes federation:prune_deliveries —
// daily sweep of terminal ledger rows past §126's 30-day retention.
func HandleFederationPruneDeliveries(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:federation:prune_deliveries")
	if federationSvc == nil {
		return fmt.Errorf("federation prune: service not registered")
	}
	return federationSvc.PruneDeliveries(ctx, 30)
}
