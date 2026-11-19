package worker

// Asynq handlers for the native AT Protocol mini-PDS (§127, ADR-139).
// The service is registered by server setup — the handlers are thin
// decode+dispatch shells; all repo semantics live in internal/atproto.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// atprotoRepoWriter is the seam the publish/delete handlers write
// through — *atproto.Service satisfies it in production. A local
// interface (not the concrete type) keeps worker out of atproto's
// auth-heavy import graph — internal/auth itself imports this package.
type atprotoRepoWriter interface {
	PublishRecordJSON(ctx context.Context, accountID pgtype.UUID, collection, rkey string, raw json.RawMessage, contentID pgtype.UUID) (string, error)
	PublishContent(ctx context.Context, contentID pgtype.UUID) (string, error)
	DeleteRecordURI(ctx context.Context, accountID pgtype.UUID, uri string) error
	RequestCrawl(ctx context.Context, accountID pgtype.UUID) error
	SyncUserLifecycle(ctx context.Context, userID pgtype.UUID) (bool, error)
	AutoProvision(ctx context.Context, userID pgtype.UUID) error
	ReconcileRepo(ctx context.Context, accountID pgtype.UUID) error
	MigratePLC(ctx context.Context, accountID pgtype.UUID) error
	EnqueueReconcileBelowPostFloor(ctx context.Context, accountID pgtype.UUID)
	ReconcileSweepPage(ctx context.Context, cursor pgtype.UUID, limit int32) (string, error)
}

var atprotoSvc atprotoRepoWriter

// RegisterAtprotoService wires the mini-PDS service for atproto:* task
// handlers. Called once during server setup.
func RegisterAtprotoService(s atprotoRepoWriter) { atprotoSvc = s }

// permanenter is the structural contract service errors use to flag
// retry-proof failures — internal/atproto wraps its terminal cases
// (bad payload, ineligible content, inactive account, unparseable URI)
// in an error implementing Permanent() bool, so worker can append
// asynq.SkipRetry without importing the package (import cycle).
type permanenter interface{ Permanent() bool }

// skipIfPermanent appends asynq.SkipRetry when the service marks the
// failure terminal; transient errors pass through for normal retry.
func skipIfPermanent(err error) error {
	var p permanenter
	if errors.As(err, &p) && p.Permanent() {
		return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
	}
	return err
}

// EnqueueAtprotoPublishContent fires atproto:publish_record for a
// contents-derived feed.post — the service resolves the author's repo
// and builds the record from the canonical row at task time (freshness
// + eligibility + account pairing at write, not enqueue). Fire-and-
// forget (Rule 8); a nil enqueuer is a no-op.
func EnqueueAtprotoPublishContent(ctx context.Context, contentID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"collection": "app.bsky.feed.post",
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("worker: atproto publish enqueue marshal failed", "content_id", contentID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoPublishRecord, payload); err != nil {
		slog.Warn("worker: atproto publish enqueue failed", "content_id", contentID.String(), "error", err)
	}
}

// EnqueueAtprotoDeleteRecord fires atproto:delete_record for an
// at:// URI — unpublish/tombstone path (§11). Fire-and-forget.
func EnqueueAtprotoDeleteRecord(ctx context.Context, accountID pgtype.UUID, uri string) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"account_id": accountID.String(),
		"uri":        uri,
	})
	if err != nil {
		slog.Warn("worker: atproto delete enqueue marshal failed", "uri", uri, "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoDeleteRecord, payload); err != nil {
		slog.Warn("worker: atproto delete enqueue failed", "uri", uri, "error", err)
	}
}

// EnqueueAtprotoRequestCrawl fires atproto:request_crawl after account
// activation — the relay learns our PDS host exists and starts crawling
// subscribeRepos/getRepo (§127 A3). Fire-and-forget (Rule 8); a nil
// enqueuer is a no-op.
func EnqueueAtprotoRequestCrawl(ctx context.Context, accountID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"account_id": accountID.String()})
	if err != nil {
		slog.Warn("worker: atproto crawl enqueue marshal failed", "account_id", accountID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoRequestCrawl, payload); err != nil {
		slog.Warn("worker: atproto crawl enqueue failed", "account_id", accountID.String(), "error", err)
	}
}

// EnqueueAtprotoLifecycleSync fires atproto:lifecycle_sync after a
// users.status write — the service reconciles the member's account
// (freeze on suspension/deletion, reactivate only lifecycle-frozen
// accounts). Fire-and-forget (Rule 8); safe to call unconditionally —
// members without an account converge to a no-op.
func EnqueueAtprotoLifecycleSync(ctx context.Context, userID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"user_id": userID.String()})
	if err != nil {
		slog.Warn("worker: atproto lifecycle enqueue marshal failed", "user_id", userID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoLifecycleSync, payload); err != nil {
		slog.Warn("worker: atproto lifecycle enqueue failed", "user_id", userID.String(), "error", err)
	}
}

// EnqueueAtprotoReconcileRepo fires atproto:reconcile_repo on every
// path back to 'active' (lifecycle reactivation, member re-enable —
// §127 A4/Lifecycle). Emits skipped while the repo was frozen leave
// stale records that would otherwise resurface; the task converges
// index rows to canonical eligibility. Fire-and-forget (Rule 8); a nil
// enqueuer is a no-op.
func EnqueueAtprotoReconcileRepo(ctx context.Context, accountID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"account_id": accountID.String()})
	if err != nil {
		slog.Warn("worker: atproto reconcile enqueue marshal failed", "account_id", accountID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoReconcileRepo, payload); err != nil {
		slog.Warn("worker: atproto reconcile enqueue failed", "account_id", accountID.String(), "error", err)
	}
}

// HandleAtprotoLifecycleSync processes atproto:lifecycle_sync (§127
// lifecycle): reconcile the member's atproto account with their live
// users.status. Idempotent — retries and out-of-order delivery converge.
func HandleAtprotoLifecycleSync(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:lifecycle_sync")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:lifecycle_sync: service not registered, dropping task")
		return nil
	}

	var payload struct {
		UserID string `json:"user_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:lifecycle_sync: invalid payload", "error", err)
		return err
	}
	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:atproto:lifecycle_sync: %w: %w", err, asynq.SkipRetry)
	}
	userActive, err := atprotoSvc.SyncUserLifecycle(ctx, userID)
	if err != nil {
		slog.Error("task:atproto:lifecycle_sync: sync failed", "user_id", payload.UserID, "error", err)
		return skipIfPermanent(err)
	}
	// §126 auto opt-in: a transition INTO active re-evaluates
	// auto-provisioning — a member suspended between signup and the
	// task run receives their identity on reactivation. Gated on the
	// run-time status (not the enqueue-time intent), so transitions to
	// suspended/banned/deleted never produce the task at all.
	if userActive {
		fireFederationAutoOptin(ctx, userID)
	} else if federationSvc != nil {
		// A transition OUT of active hides the actor behind the
		// status-gated lookups — drop its cached WebFinger/actor docs so
		// suspension stops serving immediately rather than at TTL expiry.
		federationSvc.InvalidateActorIdentityForUser(ctx, userID)
	}
	return nil
}

// HandleAtprotoRequestCrawl processes atproto:request_crawl (§127 A3):
// POST com.atproto.sync.requestCrawl to JC_ATPROTO_RELAY_URL for one
// activated account. Relay/network failures retry; a missing or
// inactive account is permanent.
func HandleAtprotoRequestCrawl(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:request_crawl")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:request_crawl: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:request_crawl: invalid payload", "error", err)
		return err
	}
	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:request_crawl: %w: %w", err, asynq.SkipRetry)
	}
	if err := atprotoSvc.RequestCrawl(ctx, accountID); err != nil {
		slog.Error("task:atproto:request_crawl: crawl failed", "account_id", payload.AccountID, "error", err)
		return skipIfPermanent(err)
	}
	slog.Info("task:atproto:request_crawl: sent", "account_id", payload.AccountID)
	return nil
}

// HandleAtprotoReconcileRepo processes atproto:reconcile_repo (§127
// A4/Lifecycle): the account's indexed records converge to canonical
// eligibility — owned+eligible feed.post rows re-project, ineligible
// or author-moved rows tombstone, the self profile republishes. Run on
// every path back to 'active' since emits skip while a repo is frozen.
// Transient per-row failures retry the whole task; a gone or frozen
// account is terminal.
func HandleAtprotoReconcileRepo(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:reconcile_repo")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:reconcile_repo: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:reconcile_repo: invalid payload", "error", err)
		return err
	}
	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:reconcile_repo: %w: %w", err, asynq.SkipRetry)
	}
	if err := atprotoSvc.ReconcileRepo(ctx, accountID); err != nil {
		slog.Error("task:atproto:reconcile_repo: reconcile failed", "account_id", payload.AccountID, "error", err)
		return skipIfPermanent(err)
	}
	slog.Info("task:atproto:reconcile_repo: done", "account_id", payload.AccountID)
	return nil
}

// HandleAtprotoPublishRecord processes atproto:publish_record (§127 A2).
// Two payload shapes: an embedded lexicon `record` (profile upsert via
// rkey, or TID create when rkey is empty), or a bare `content_id` —
// then the service loads + eligibility-checks + projects the contents
// row itself, so a stale or unpublishable row is never written.
func HandleAtprotoPublishRecord(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:publish_record")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:publish_record: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID  string          `json:"account_id"`
		Collection string          `json:"collection"`
		Rkey       string          `json:"rkey,omitempty"`
		ContentID  string          `json:"content_id,omitempty"`
		Record     json.RawMessage `json:"record,omitempty"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:publish_record: invalid payload", "error", err)
		return err
	}
	contentID, err := scanOptionalUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:atproto:publish_record: %w: %w", err, asynq.SkipRetry)
	}

	// Content-derived post: no embedded record — the service resolves the
	// author's repo from the contents row, builds the record, and re-
	// checks eligibility at write time, so an unpublished/deleted row is
	// never projected.
	if len(payload.Record) == 0 {
		if !contentID.Valid {
			slog.Error("task:atproto:publish_record: payload has neither record nor content_id")
			return fmt.Errorf("task:atproto:publish_record: incomplete payload: %w", asynq.SkipRetry)
		}
		if payload.Collection != "" && payload.Collection != "app.bsky.feed.post" {
			slog.Error("task:atproto:publish_record: content task with non-post collection", "collection", payload.Collection)
			return fmt.Errorf("task:atproto:publish_record: collection %q cannot carry content_id: %w", payload.Collection, asynq.SkipRetry)
		}
		uri, err := atprotoSvc.PublishContent(ctx, contentID)
		if err != nil {
			slog.Error("task:atproto:publish_record: publish failed",
				"content_id", payload.ContentID, "error", err)
			return skipIfPermanent(err)
		}
		slog.Info("task:atproto:publish_record: committed", "uri", uri)
		return nil
	}

	if payload.AccountID == "" {
		slog.Error("task:atproto:publish_record: payload missing account_id")
		return fmt.Errorf("task:atproto:publish_record: incomplete payload: %w", asynq.SkipRetry)
	}
	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:publish_record: %w: %w", err, asynq.SkipRetry)
	}
	if payload.Collection == "" {
		slog.Error("task:atproto:publish_record: record payload missing collection")
		return fmt.Errorf("task:atproto:publish_record: incomplete payload: %w", asynq.SkipRetry)
	}
	uri, err := atprotoSvc.PublishRecordJSON(ctx, accountID, payload.Collection, payload.Rkey, payload.Record, contentID)
	if err != nil {
		slog.Error("task:atproto:publish_record: publish failed",
			"account_id", payload.AccountID, "collection", payload.Collection, "error", err)
		return skipIfPermanent(err)
	}
	slog.Info("task:atproto:publish_record: committed", "uri", uri)
	return nil
}

// HandleAtprotoDeleteRecord processes atproto:delete_record (§11:
// tombstone/unpublish) — drops the record addressed by the URI and
// commits the tombstoned repo. Deleting an already-gone record
// short-circuits without a commit, so retries are safe and free.
func HandleAtprotoDeleteRecord(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:delete_record")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:delete_record: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID string `json:"account_id"`
		URI       string `json:"uri"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:delete_record: invalid payload", "error", err)
		return err
	}
	if payload.AccountID == "" || payload.URI == "" {
		slog.Error("task:atproto:delete_record: payload missing account_id or uri")
		return fmt.Errorf("task:atproto:delete_record: incomplete payload: %w", asynq.SkipRetry)
	}

	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:delete_record: %w: %w", err, asynq.SkipRetry)
	}

	if err := atprotoSvc.DeleteRecordURI(ctx, accountID, payload.URI); err != nil {
		slog.Error("task:atproto:delete_record: delete failed",
			"account_id", payload.AccountID, "uri", payload.URI, "error", err)
		return skipIfPermanent(err)
	}
	// A tombstone can drop the author under the §127 projection floor —
	// the emit path only removes the deleted content's own record, so
	// the profile (and any still-indexed siblings) need the reconcile
	// walk to converge. Above the floor this no-ops.
	atprotoSvc.EnqueueReconcileBelowPostFloor(ctx, accountID)
	slog.Info("task:atproto:delete_record: committed", "uri", payload.URI)
	return nil
}

// EnqueueAtprotoReconcileSweep fires atproto:reconcile_sweep — the
// daily drift probe (§127 projection floor / §11). An empty cursor
// starts the keyset walk; each page self-continues until the table is
// exhausted. The caller decides fire-and-forget vs propagation — the
// page handler propagates so a dropped continuation retries the task
// (same convention as federation:backfill_optin's page walk).
func EnqueueAtprotoReconcileSweep(ctx context.Context, cursor string) error {
	if taskEnqueuer == nil {
		return fmt.Errorf("atproto reconcile_sweep: enqueuer not registered")
	}
	payload, err := json.Marshal(map[string]string{"cursor": cursor})
	if err != nil {
		return fmt.Errorf("atproto reconcile_sweep: marshal: %w", err)
	}
	return taskEnqueuer.Enqueue(TypeAtprotoReconcileSweep, payload)
}

// HandleAtprotoReconcileSweep processes one atproto:reconcile_sweep
// page (§127 projection floor / §11): the service walks a keyset page
// of active accounts, probes each for projection divergence, and
// enqueues reconcile_repo only where the probe disagrees. A non-empty
// next cursor self-continues with the following page.
func HandleAtprotoReconcileSweep(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:reconcile_sweep")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:reconcile_sweep: service not registered, dropping task")
		return nil
	}

	var payload struct {
		Cursor string `json:"cursor"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:reconcile_sweep: invalid payload", "error", err)
		return err
	}
	var cursor pgtype.UUID
	if payload.Cursor != "" {
		c, err := scanUUID(payload.Cursor)
		if err != nil {
			return fmt.Errorf("task:atproto:reconcile_sweep: %w: %w", err, asynq.SkipRetry)
		}
		cursor = c
	} else {
		// Zero uuid — the keyset predicate (id > cursor) admits every
		// row, so page one needs no special case.
		_ = cursor.Scan("00000000-0000-0000-0000-000000000000")
	}

	next, err := atprotoSvc.ReconcileSweepPage(ctx, cursor, reconcileSweepPageSize)
	if err != nil {
		slog.Error("task:atproto:reconcile_sweep: page failed", "error", err)
		return err
	}
	// A retry of THIS task re-pages the same cursor, so continuation is
	// safe to enqueue only on success — a swallowed failure would
	// truncate the walk silently until tomorrow's scheduled run.
	if next != "" {
		if err := EnqueueAtprotoReconcileSweep(ctx, next); err != nil {
			return fmt.Errorf("task:atproto:reconcile_sweep: enqueue next page: %w", err)
		}
	}
	return nil
}

// reconcileSweepPageSize bounds each sweep page — large enough that a
// fleet walks in a handful of tasks, small enough that one page's probe
// round-trips stay cheap.
const reconcileSweepPageSize = 200

// EnqueueAtprotoPlcMigrate fires atproto:plc_migrate for one account
// (§127 A6-2): provision a did:plc — rotation-key custody, signed
// genesis op, directory submit, in-place did swap. Idempotent; a
// re-enqueue on an already-did:plc account no-ops. Fire-and-forget
// (Rule 8); a nil enqueuer is a no-op.
func EnqueueAtprotoPlcMigrate(ctx context.Context, accountID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"account_id": accountID.String()})
	if err != nil {
		slog.Warn("worker: atproto plc_migrate enqueue marshal failed", "account_id", accountID.String(), "error", err)
		return
	}
	if err := taskEnqueuer.Enqueue(TypeAtprotoPlcMigrate, payload); err != nil {
		slog.Warn("worker: atproto plc_migrate enqueue failed", "account_id", accountID.String(), "error", err)
	}
}

// HandleAtprotoPlcMigrate processes atproto:plc_migrate (§127 A6-2):
// run the idempotent provisioning core for one account. Transient
// directory errors retry; a tombstoned/gone account or a rejected op
// skips permanently.
func HandleAtprotoPlcMigrate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:plc_migrate")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:plc_migrate: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:plc_migrate: invalid payload", "error", err)
		return err
	}
	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:plc_migrate: %w: %w", err, asynq.SkipRetry)
	}
	if err := atprotoSvc.MigratePLC(ctx, accountID); err != nil {
		slog.Error("task:atproto:plc_migrate: migrate failed", "account_id", payload.AccountID, "error", err)
		return skipIfPermanent(err)
	}
	slog.Info("task:atproto:plc_migrate: done", "account_id", payload.AccountID)
	return nil
}
