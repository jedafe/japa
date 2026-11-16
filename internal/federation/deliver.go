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
	"sync"
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
	// Create/Update → Update (remote servers update in place). The
	// activity id carries an updated_at discriminator: distinct edits are
	// distinct events — both our (activity_ap_id, inbox) dedup and the
	// remote's own activity-id dedup must not swallow a second Update.
	// Retries of THIS event re-read the same updated_at and converge.
	activityType := "Create"
	if s.store.hasDeliveredObject(ctx, author.ID, objID) {
		activityType = "Update"
	}
	var activity map[string]any
	var apID string
	if activityType == "Create" {
		activity = createActivity(author.ActorUrl, author.FollowersUrl, object)
		apID = objID + "#create"
	} else {
		apID = fmt.Sprintf("%s#update/%d", objID, content.UpdatedAt.Time.Unix())
		activity = map[string]any{
			"id":     apID,
			"type":   "Update",
			"actor":  author.ActorUrl,
			"to":     []string{asPublic},
			"cc":     []string{author.FollowersUrl},
			"object": object,
		}
	}
	_, err = s.insertDeliveries(ctx, author.ID, activity, apID, inboxes, true)
	return err
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
// deleted, opted out, or content-type un-gated. The delivery ledger is
// proof a remote copy exists; when the 30-day prune already dropped it,
// a non-NULL published_at is the durable signal the object once fanned
// out. Over-approximating is protocol-safe: Delete for an object the
// remote never saw is a no-op there.
func (s *Service) fanoutDeleteIfDelivered(ctx context.Context, contentID pgtype.UUID) error {
	objID := s.apiBase() + "/federation/objects/contents/" + contentID.String()
	var author gen.FederatedActor
	if localActorID, delivered := s.store.deliveredObjectActor(ctx, objID); delivered {
		a, err := s.store.actorByID(ctx, localActorID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // actor row erased — nothing left to sign with
			}
			return err
		}
		author = a
	} else {
		lite, err := s.store.contentLiteByID(ctx, contentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // row gone entirely — nothing ever fanned out
			}
			return err
		}
		if !wasPublished(lite) {
			return nil // never published → no Create ever reached remotes
		}
		a, err := s.store.actorByUserID(ctx, lite.AuthorID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		author = a
	}
	if author.Status != "active" {
		return nil // disabled actors propagate Delete via the lifecycle path
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, author.ID)
	if err != nil || len(inboxes) == 0 {
		return err
	}
	// Per-event discriminator: a publish→unpublish→republish→unpublish
	// cycle produces TWO distinct Deletes — identical ap_ids would collapse
	// in the (activity_ap_id, inbox) dedup and the remote would keep
	// serving the resurrected copy. updated_at bumps on every lifecycle
	// transition, so the id is stable across retries of THIS event and
	// distinct across events. When the lite row is gone (hard delete),
	// the actor's own updated_at is the fallback discriminator — any
	// earlier same-stamped delete for this object already reached remotes.
	lite, lerr := s.store.contentLiteByID(ctx, contentID)
	disc := author.UpdatedAt
	if lerr == nil && lite.UpdatedAt.Valid {
		disc = lite.UpdatedAt
	}
	apID := fmt.Sprintf("%s#delete/%d", objID, disc.Time.Unix())
	activity := map[string]any{
		"id":    apID,
		"type":  "Delete",
		"actor": author.ActorUrl,
		"to":    []string{asPublic},
		"object": map[string]any{
			"id":   objID,
			"type": "Tombstone",
		},
	}
	_, err = s.insertDeliveries(ctx, author.ID, activity, apID, inboxes, true)
	return err
}

// FanoutActorDelete delivers Delete{actor} to every remote-follower
// inbox — the M2 propagation path for member disable/erasure (§126).
// Follower rows stay untouched here; the lifecycle write that calls this
// owns the status transition.
func (s *Service) FanoutActorDelete(ctx context.Context, actorID pgtype.UUID) error {
	_, err := s.fanoutActorDelete(ctx, actorID, true)
	return err
}

// fanoutActorDelete ledgers the Delete{actor} rows and returns the ids it
// created. enqueue=false leaves them pending for a synchronous drive —
// the erasure path must attempt them while the actor's key still exists.
// The activity id keys on the actor's updated_at: disable→re-enable→
// disable are distinct events whose deliveries must not dedup-collapse,
// while a disable→erasure sequence (no interim write) reuses the earlier
// stamp — a remote that already got that Delete needs no second copy.
func (s *Service) fanoutActorDelete(ctx context.Context, actorID pgtype.UUID, enqueue bool) ([]pgtype.UUID, error) {
	author, err := s.store.actorByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("federation actor delete: load actor: %w", err)
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, author.ID)
	if err != nil {
		return nil, fmt.Errorf("federation actor delete: expand inboxes: %w", err)
	}
	if len(inboxes) == 0 {
		return nil, nil
	}
	apID := fmt.Sprintf("%s#delete/%d", author.ActorUrl, author.UpdatedAt.Time.Unix())
	activity := map[string]any{
		"id":    apID,
		"type":  "Delete",
		"actor": author.ActorUrl,
		"to":    []string{asPublic},
		"object": map[string]any{
			"id":   author.ActorUrl,
			"type": "Tombstone",
		},
	}
	return s.insertDeliveries(ctx, author.ID, activity, apID, inboxes, enqueue)
}

// insertDeliveries ledgers one row per distinct inbox and kicks each into
// the queue post-insert (unless enqueue=false — the erasure path drives
// rows synchronously). Insert errors fail the whole expansion — the
// caller's retry replays a clean expansion (activity_ap_id dedup keeps
// retries honest on rows that did land). Returns the created row ids.
func (s *Service) insertDeliveries(ctx context.Context, localActorID pgtype.UUID, activity map[string]any, apID string, inboxes []gen.ListRemoteFollowerInboxesByActorRow, enqueue bool) ([]pgtype.UUID, error) {
	payload, err := json.Marshal(activity)
	if err != nil {
		return nil, fmt.Errorf("federation fanout: marshal activity: %w", err)
	}
	typ, _ := activity["type"].(string)
	var ids []pgtype.UUID
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
			return nil, fmt.Errorf("federation fanout: insert delivery: %w", err)
		}
		ids = append(ids, row.ID)
		if enqueue {
			if err := s.enqueueDelivery(ctx, row.ID); err != nil {
				slog.Warn("federation: delivery enqueue failed — sweeper picks up pending rows", "delivery_id", row.ID, "error", err)
			}
		}
	}
	return ids, nil
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
	return s.attemptDelivery(ctx, row)
}

// attemptDelivery performs one signed POST for the row and stamps the
// outcome. Split from Deliver so the erasure flush can drive rows without
// the backoff gate — key custody is about to burn, a last attempt beats
// a schedule the actor row will not survive to honor.
func (s *Service) attemptDelivery(ctx context.Context, row gen.FederationDelivery) error {
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
	case resp.StatusCode == http.StatusTooManyRequests:
		// A rate-limited inbox is transient — the ledger's backoff retries
		// later rather than dead-lettering on one throttled attempt.
		return s.failDelivery(ctx, row, "remote 429")
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

// flushDeliveriesInline drives delivery rows synchronously — the erasure
// path's only window to sign before the user cascade drops
// federated_actors. Bounded concurrency keeps a large follower set from
// serializing into minutes; outcomes still land on the ledger rows, so
// failures dead-letter honestly once custody is gone.
func (s *Service) flushDeliveriesInline(ctx context.Context, ids []pgtype.UUID) {
	if len(ids) == 0 {
		return
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, id := range ids {
		row, err := s.store.deliveryByID(ctx, id)
		if err != nil {
			continue
		}
		if row.Status == "delivered" || row.Status == "dead" {
			continue
		}
		wg.Add(1)
		go func(row gen.FederationDelivery) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := s.attemptDelivery(ctx, row); err != nil {
				slog.Warn("federation: inline delivery failed", "delivery_id", row.ID.String(), "error", err)
			}
		}(row)
	}
	wg.Wait()
}

// ResolveRemoteActorTask is the worker seam for federation:resolve_actor —
// a low-priority fetch+upsert for rows the inbound path couldn't resolve
// inline (or scheduled refreshes).
func (s *Service) ResolveRemoteActorTask(ctx context.Context, actorURL string) error {
	_, err := s.resolveRemoteActor(ctx, actorURL)
	return err
}

// PruneDeliveries is the daily ledger hygiene task (§126, §11). It first
// re-arms stranded work — 'failed' rows whose scheduled retry was lost to
// an enqueue failure, and 'pending' rows whose initial task never fired —
// then sweeps delivered/dead rows past retention. Re-enqueue is cheap:
// the ledger gates make a duplicate kick converge in Deliver.
func (s *Service) PruneDeliveries(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if s.enqueue != nil {
		due, err := s.store.dueDeliveries(ctx, 200)
		if err != nil {
			return fmt.Errorf("federation delivery sweep: %w", err)
		}
		for _, d := range due {
			if err := s.enqueueDelivery(ctx, d.ID); err != nil {
				slog.Warn("federation: sweep re-enqueue failed — next run retries", "delivery_id", d.ID, "error", err)
			}
		}
		// Same stranded-work class on the inbound side: a lost
		// EnqueueInbound leaves 'received' rows with no driver.
		stale, err := s.store.staleInbound(ctx, 200)
		if err != nil {
			return fmt.Errorf("federation inbound sweep: %w", err)
		}
		for _, id := range stale {
			if err := s.enqueue.EnqueueInbound(ctx, id); err != nil {
				slog.Warn("federation: inbound sweep re-enqueue failed", "inbound_id", id, "error", err)
			}
		}
	}
	if err := s.store.pruneInbound(ctx, int32(retentionDays)); err != nil {
		return fmt.Errorf("federation inbound prune: %w", err)
	}
	return s.store.pruneDeliveries(ctx, int32(retentionDays))
}
