package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M2 — outbound)
// Fanout expands one content lifecycle event into per-inbox
// federation_deliveries rows (sharedInbox coalescing collapses same-instance
// followers into a single POST); Deliver performs one signed attempt per row
// and moves it through pending → delivered | failed → dead. Delivery state
// is ledger-first: the task only stamps transitions, so replays and
// multi-worker claims converge on the row, never on task bookkeeping.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// TaskEnqueuer kicks ledgered work into asynq — the worker package's
// enqueue helpers, injected so the service test-suite can leave it nil.
type TaskEnqueuer interface {
	EnqueueDelivery(ctx context.Context, deliveryID pgtype.UUID) error
	// EnqueueDeliveryAfter schedules a retry attempt — the ledger's
	// next_retry_at gate keeps premature arrivals honest.
	EnqueueDeliveryAfter(ctx context.Context, deliveryID pgtype.UUID, delay time.Duration) error
	EnqueueInbound(ctx context.Context, inboundID pgtype.UUID) error
}

// SetEnqueuer wires the delivery/inbound enqueue seam (production: worker).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueue = e }

// enqueueDelivery kicks one delivery row — nil enqueuer is a no-op (the
// row stays pending until a sweep or admin kick; honest either way).
func (s *Service) enqueueDelivery(ctx context.Context, deliveryID pgtype.UUID) error {
	if s.enqueue == nil {
		return nil
	}
	return s.enqueue.EnqueueDelivery(ctx, deliveryID)
}

// deliveryMaxAttempts reads the §126 retry ceiling (default 8).
func (s *Service) deliveryMaxAttempts(ctx context.Context) int {
	if s.settings != nil {
		if n, err := s.settings.GetSettingInt(ctx, "federation.delivery.max_attempts"); err == nil && n > 0 {
			return n
		}
	}
	return 8
}

// ---------------------------------------------------------------------------
// Fanout — content lifecycle → delivery rows
// ---------------------------------------------------------------------------

// Fanout expands a content publish/update/unpublish/delete into delivery
// rows, one per distinct remote-follower inbox. The task re-checks
// eligibility at run time: content that became ineligible between enqueue
// and execution fanouts Delete instead of the stale Create/Update — the
// same converge-on-current-state posture as §127's publish task.
func (s *Service) Fanout(ctx context.Context, contentID pgtype.UUID) error {
	if s.store == nil {
		return nil
	}
	content, err := s.store.federatableContentByID(ctx, contentID, s.federatableTypesFor(ctx))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("federation fanout: load content: %w", err)
		}
		// Ineligible now — if the object was delivered before, remote
		// followers need Delete{tombstone}. Look up the content's author
		// without the federatable gate so the tombstone can fan out.
		return s.fanoutDeleteIfDelivered(ctx, contentID)
	}

	author, err := s.store.actorByUserID(ctx, content.AuthorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // author never federated — nothing to deliver to
		}
		return fmt.Errorf("federation fanout: load actor: %w", err)
	}
	if author.Status != "active" {
		return nil // actor disabled — remote followers get Delete via the lifecycle path
	}

	inboxes, err := s.store.remoteFollowerInboxes(ctx, author.ID)
	if err != nil {
		return fmt.Errorf("federation fanout: expand inboxes: %w", err)
	}
	if len(inboxes) == 0 {
		return nil // no remote followers — nothing to fan out
	}

	object := objectDocument(content, author.ActorUrl, author.FollowersUrl, s.siteBase(), contentPageURL(s.siteBase(), content))
	objID, _ := object["id"].(string)

	// First fanout for this object → Create; re-fanout after a delivered
	// Create/Update → Update (remote servers update in place).
	activityType := "Create"
	if s.store.hasDeliveredObject(ctx, author.ID, objID) {
		activityType = "Update"
	}
	var activity map[string]any
	if activityType == "Create" {
		activity = createActivity(author.ActorUrl, author.FollowersUrl, object)
	} else {
		activity = map[string]any{
			"id":     objID + "#update",
			"type":   "Update",
			"actor":  author.ActorUrl,
			"to":     []string{asPublic},
			"cc":     []string{author.FollowersUrl},
			"object": object,
		}
	}
	return s.insertDeliveries(ctx, author.ID, activity, objID+"#"+lowerType(activityType), inboxes)
}

// lowerType keeps activity_ap_id derivation boring — ids ride a fragment.
func lowerType(t string) string {
	b := []byte(t)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// fanoutDeleteIfDelivered delivers Delete{object} when a previously
// delivered object left the federatable set — unpublished, made private,
// deleted, opted out, or content-type un-gated. No prior delivery → no-op.
func (s *Service) fanoutDeleteIfDelivered(ctx context.Context, contentID pgtype.UUID) error {
	objID := s.apiBase() + "/federation/objects/contents/" + contentID.String()
	localActorID, delivered := s.store.deliveredObjectActor(ctx, objID)
	if !delivered {
		return nil
	}
	author, err := s.store.actorByID(ctx, localActorID)
	if err != nil || author.Status != "active" {
		return err // disabled actors propagate Delete via the lifecycle path
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, author.ID)
	if err != nil || len(inboxes) == 0 {
		return err
	}
	activity := map[string]any{
		"id":    objID + "#delete",
		"type":  "Delete",
		"actor": author.ActorUrl,
		"to":    []string{asPublic},
		"object": map[string]any{
			"id":   objID,
			"type": "Tombstone",
		},
	}
	return s.insertDeliveries(ctx, author.ID, activity, objID+"#delete", inboxes)
}

// FanoutActorDelete delivers Delete{actor} to every remote-follower
// inbox — the M2 propagation path for member disable/erasure (§126).
// Follower rows stay untouched here; the lifecycle write that calls this
// owns the status transition.
func (s *Service) FanoutActorDelete(ctx context.Context, actorID pgtype.UUID) error {
	author, err := s.store.actorByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("federation actor delete: load actor: %w", err)
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, author.ID)
	if err != nil {
		return fmt.Errorf("federation actor delete: expand inboxes: %w", err)
	}
	if len(inboxes) == 0 {
		return nil
	}
	activity := map[string]any{
		"id":    author.ActorUrl + "#delete",
		"type":  "Delete",
		"actor": author.ActorUrl,
		"to":    []string{asPublic},
		"object": map[string]any{
			"id":   author.ActorUrl,
			"type": "Tombstone",
		},
	}
	return s.insertDeliveries(ctx, author.ID, activity, author.ActorUrl+"#delete", inboxes)
}

// insertDeliveries ledgers one row per distinct inbox and kicks each into
// the queue post-insert. Insert errors fail the whole expansion — the
// caller's retry replays a clean expansion (activity_ap_id dedup keeps
// retries honest on rows that did land).
func (s *Service) insertDeliveries(ctx context.Context, localActorID pgtype.UUID, activity map[string]any, apID string, inboxes []gen.ListRemoteFollowerInboxesByActorRow) error {
	payload, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("federation fanout: marshal activity: %w", err)
	}
	typ, _ := activity["type"].(string)
	for _, inbox := range inboxes {
		if inbox.InboxUrl == nil || *inbox.InboxUrl == "" {
			continue
		}
		inboxURL := *inbox.InboxUrl
		// Blocked/unpermitted instances get no delivery row at all — the
		// worker would dead-letter it anyway; cheaper to skip here.
		if u, uerr := url.Parse(inboxURL); uerr == nil && !s.instanceAllowed(ctx, u.Hostname()) {
			continue
		}
		row, err := s.store.insertDelivery(ctx, gen.InsertFederationDeliveryParams{
			ActivityApID:  apID,
			ActivityType:  typ,
			ActivityJson:  payload,
			LocalActorID:  localActorID,
			RemoteActorID: inbox.RemoteActorID,
			InboxUrl:      inboxURL,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // replayed fanout — the row's task is already queued
		}
		if err != nil {
			return fmt.Errorf("federation fanout: insert delivery: %w", err)
		}
		if err := s.enqueueDelivery(ctx, row.ID); err != nil {
			slog.Warn("federation: delivery enqueue failed — sweeper picks up pending rows", "delivery_id", row.ID, "error", err)
		}
	}
	return nil
}

// contentPageURL is the member-facing permalink the AP object's "url"
// field points at — same shape the atproto projection uses.
func contentPageURL(siteBase string, c gen.GetFederatableContentByIDRow) string {
	if c.Slug != "" {
		return siteBase + "/content/" + c.Slug
	}
	return siteBase + "/content/" + c.ID.String()
}

// ---------------------------------------------------------------------------
// Deliver — one signed attempt
// ---------------------------------------------------------------------------

// Deliver performs one delivery attempt on a ledgered row. Terminal rows
// are skipped (replay-safe); failures stamp attempts/next_retry_at and
// return an asynq-visible error only for transient causes so the queue's
// own retry cooperates with the ledger's backoff.
func (s *Service) Deliver(ctx context.Context, deliveryID pgtype.UUID) error {
	row, err := s.store.deliveryByID(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	switch row.Status {
	case "delivered", "dead":
		return nil // terminal — replay no-op
	}
	// Backoff gate: a task firing before next_retry_at (asynq retry, a
	// duplicate enqueue, a raced sweep) must not re-POST early — the row's
	// own schedule is authoritative.
	if row.Status == "failed" && row.NextRetryAt.Valid && row.NextRetryAt.Time.After(time.Now()) {
		return nil
	}

	// Instance gate re-checked at send time — a domain blocked between
	// fanout and delivery must not receive the POST.
	if u, uerr := url.Parse(row.InboxUrl); uerr != nil || !s.instanceAllowed(ctx, u.Hostname()) {
		return s.stampDelivery(ctx, row, "dead", "destination instance blocked")
	}

	author, err := s.store.actorByID(ctx, row.LocalActorID)
	if err != nil {
		return s.stampDelivery(ctx, row, "dead", "local actor missing")
	}
	privPEM, err := crypto.Decrypt(author.PrivateKeyEnc)
	if err != nil {
		return s.stampDelivery(ctx, row, "dead", "private key undecryptable")
	}
	priv, err := parsePrivateKeyPEM(privPEM)
	if err != nil {
		return s.stampDelivery(ctx, row, "dead", "private key unparseable")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, row.InboxUrl, bytes.NewReader(row.ActivityJson))
	if err != nil {
		return s.stampDelivery(ctx, row, "dead", "bad inbox url")
	}
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("User-Agent", "JapaConnect/"+s.cfg.APIBase)
	if err := SignRequest(req, author.PublicKeyID, priv, row.ActivityJson); err != nil {
		return s.stampDelivery(ctx, row, "dead", "signing failed")
	}

	resp, err := s.deliverAP(ctx, req)
	if err != nil {
		return s.failDelivery(ctx, row, "http: "+err.Error())
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return s.stampDelivery(ctx, row, "delivered", "")
	case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
		// Remote account/instance removed — retrying can never succeed.
		return s.stampDelivery(ctx, row, "dead", fmt.Sprintf("remote %d", resp.StatusCode))
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// 4xx is a refusal — dead-letter rather than burn the retry budget.
		return s.stampDelivery(ctx, row, "dead", fmt.Sprintf("remote %d", resp.StatusCode))
	default:
		return s.failDelivery(ctx, row, fmt.Sprintf("remote %d", resp.StatusCode))
	}
}

// failDelivery stamps a transient failure: attempts++ and either
// 'failed' with exponential next_retry_at or 'dead' at the configured
// ceiling. On 'failed' the task self-re-enqueues with ProcessIn(delay) —
// the ledger's backoff is authoritative and the next_retry_at gate makes
// duplicate tasks converge rather than double-POST.
func (s *Service) failDelivery(ctx context.Context, row gen.FederationDelivery, reason string) error {
	attempts := row.Attempts + 1
	if int(attempts) >= s.deliveryMaxAttempts(ctx) {
		return s.stampDeliveryAttempts(ctx, row, "dead", reason, attempts, pgtype.Timestamptz{})
	}
	// Exponential backoff: 2m, 4m, 8m, … capped at 1h.
	shift := attempts
	if shift > 9 {
		shift = 9
	}
	delay := time.Duration(1<<shift) * time.Minute
	if delay > time.Hour {
		delay = time.Hour
	}
	next := pgtype.Timestamptz{Time: time.Now().Add(delay), Valid: true}
	if err := s.stampDeliveryAttempts(ctx, row, "failed", reason, attempts, next); err != nil {
		return err
	}
	if s.enqueue != nil {
		if err := s.enqueue.EnqueueDeliveryAfter(ctx, row.ID, delay); err != nil {
			slog.Warn("federation: retry re-enqueue failed", "delivery_id", row.ID, "error", err)
		}
	}
	return nil
}

// stampDelivery transitions to a terminal/non-retry state, preserving
// attempts and delivered_at (the update query is unconditional — every
// field needs an honest value, not a zero clobber).
func (s *Service) stampDelivery(ctx context.Context, row gen.FederationDelivery, status, errMsg string) error {
	return s.stampDeliveryAttempts(ctx, row, status, errMsg, row.Attempts, pgtype.Timestamptz{})
}

// stampDeliveryAttempts is the single write path for delivery transitions.
func (s *Service) stampDeliveryAttempts(ctx context.Context, row gen.FederationDelivery, status, errMsg string, attempts int32, nextRetry pgtype.Timestamptz) error {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	deliveredAt := row.DeliveredAt
	if status == "delivered" {
		deliveredAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	return s.store.updateDeliveryStatus(ctx, gen.UpdateFederationDeliveryStatusParams{
		ID:          row.ID,
		Status:      status,
		Attempts:    attempts,
		LastError:   errPtr,
		NextRetryAt: nextRetry,
		DeliveredAt: deliveredAt,
	})
}

// ResolveRemoteActorTask is the worker seam for federation:resolve_actor —
// a low-priority fetch+upsert for rows the inbound path couldn't resolve
// inline (or scheduled refreshes).
func (s *Service) ResolveRemoteActorTask(ctx context.Context, actorURL string) error {
	_, err := s.resolveRemoteActor(ctx, actorURL)
	return err
}

// PruneDeliveries sweeps delivered/dead rows past retention — §126's
// 30-day ledger hygiene task.
func (s *Service) PruneDeliveries(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	return s.store.pruneDeliveries(ctx, int32(retentionDays))
}
