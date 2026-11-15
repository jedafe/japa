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

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// federationProvisioner is the seam the auto-opt-in handler provisions
// actors through — *federation.Service satisfies it in production. A
// local interface keeps worker out of federation's import graph (same
// shape as atprotoRepoWriter above).
type federationProvisioner interface {
	AutoProvision(ctx context.Context, userID pgtype.UUID) error
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
