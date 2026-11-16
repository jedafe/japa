package federation

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	gen "github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for the federation HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// writeTypedJSON writes an AP/JRD document with its protocol content type
// (the public federation surface does not use the API error envelope for
// 2xx — remote servers parse by content type).
func writeTypedJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Body may be partially written; log path — the handler that calls
		// this already committed the status.
		_ = err
	}
}

// handleServiceErr maps service sentinels to the standard error envelope.
func handleServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ErrHandleDomain):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_resource", "Unsupported resource")
	case errors.Is(err, ErrInvalidDomain):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_domain", "Invalid instance domain")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

// ---------------------------------------------------------------------------
// Public surface (§126 URL Layout — unversioned /federation/* + /.well-known)
// ---------------------------------------------------------------------------

// GetWebfinger serves GET /.well-known/webfinger?resource=acct:{u}@{domain}.
func (h *Handlers) GetWebfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_resource", "resource parameter is required")
		return
	}
	doc, err := h.service.Webfinger(r.Context(), resource)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeJRD, doc)
}

// GetWellKnownNodeInfo serves GET /.well-known/nodeinfo.
func (h *Handlers) GetWellKnownNodeInfo(w http.ResponseWriter, r *http.Request) {
	writeTypedJSON(w, http.StatusOK, "application/json", h.service.WellKnownNodeInfo())
}

// GetNodeInfo serves GET /federation/nodeinfo/2.1.
func (h *Handlers) GetNodeInfo(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.NodeInfo(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeNodeInfo, doc)
}

// GetActor serves GET /federation/actors/{username}. M2: a non-active
// actor row answers 410 + Tombstone — the terminal signal remotes stop
// retrying on (404 stays for handles that never federated).
func (h *Handlers) GetActor(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	doc, err := h.service.ActorDocument(r.Context(), username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if tomb := h.service.ActorTombstone(r.Context(), username); tomb != nil {
				writeTypedJSON(w, http.StatusGone, ContentTypeActivity, tomb)
				return
			}
		}
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetOutbox serves GET /federation/actors/{username}/outbox[?page=N].
func (h *Handlers) GetOutbox(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Outbox(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowers serves GET /federation/actors/{username}/followers[?page=N].
func (h *Handlers) GetFollowers(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Followers(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowing serves GET /federation/actors/{username}/following[?page=N].
func (h *Handlers) GetFollowing(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Following(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetObject serves GET /federation/objects/contents/{id}.
func (h *Handlers) GetObject(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid object id")
		return
	}
	doc, err := h.service.ObjectDocument(r.Context(), id)
	if err != nil {
		// §126: once-live objects answer 410 + Tombstone — remotes
		// distinguish "deleted" from "never existed" and drop the copy.
		if errors.Is(err, ErrGone) {
			writeTypedJSON(w, http.StatusGone, ContentTypeActivity,
				tombstoneDocument(h.service.objectURL(id.String())))
			return
		}
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// ---------------------------------------------------------------------------
// Member opt-in (/api/v1/me/federation — auth + federation.use)
// ---------------------------------------------------------------------------

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// GetMyFederation serves GET /api/v1/me/federation.
func (h *Handlers) GetMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetMyFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// EnableMyFederation serves POST /api/v1/me/federation.
func (h *Handlers) EnableMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.EnableFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// DisableMyFederation serves DELETE /api/v1/me/federation.
func (h *Handlers) DisableMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.DisableFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// ListFollowRequests serves GET /api/v1/me/federation/follow-requests —
// the pending remote Follow queue for manually_approves_followers members.
func (h *Handlers) ListFollowRequests(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := h.service.ListFollowRequests(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"requests": rows})
}

// AcceptFollowRequest serves POST /api/v1/me/federation/follow-requests/{remote_actor_id}/accept.
func (h *Handlers) AcceptFollowRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveFollowRequest(w, r, true)
}

// RejectFollowRequest serves POST /api/v1/me/federation/follow-requests/{remote_actor_id}/reject.
func (h *Handlers) RejectFollowRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveFollowRequest(w, r, false)
}

func (h *Handlers) resolveFollowRequest(w http.ResponseWriter, r *http.Request, accept bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var remoteID pgtype.UUID
	if err := remoteID.Scan(chi.URLParam(r, "remote_actor_id")); err != nil || !remoteID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid remote actor id")
		return
	}
	if err := h.service.ResolveFollowRequest(r.Context(), user.UserID, remoteID, accept); err != nil {
		handleServiceErr(w, err)
		return
	}
	status := "rejected"
	if accept {
		status = "accepted"
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": status})
}

// ---------------------------------------------------------------------------
// Admin (/api/v1/admin/federation/*)
// ---------------------------------------------------------------------------

// actorSortKeys is the DataTable sort whitelist for the admin actors list —
// each key has a CASE arm in ListFederatedActors (unlisted sorts fall back
// to created_at DESC server-side).
var actorSortKeys = map[string]bool{
	"created_at": true,
	"username":   true,
	"status":     true,
	"actor_url":  true,
}

// ListActors serves GET /api/v1/admin/federation/actors.
func (h *Handlers) ListActors(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	sortKey := r.URL.Query().Get("sort")
	if !actorSortKeys[sortKey] {
		sortKey = "created_at"
	}
	sortDir := r.URL.Query().Get("dir")
	if sortDir == "" {
		sortDir = r.URL.Query().Get("sort_dir")
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}
	rows, total, err := h.service.ListActors(r.Context(), r.URL.Query().Get("q"), sortKey, sortDir, limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "total_pages": totalPages(total, limit)},
	})
}

// GetStats serves GET /api/v1/admin/federation/stats.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, stats)
}

// ListBlocks serves GET /api/v1/admin/federation/instances.
func (h *Handlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	rows, total, err := h.service.ListBlocks(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "total_pages": totalPages(total, limit)},
	})
}

// CreateBlock serves POST /api/v1/admin/federation/instances.
func (h *Handlers) CreateBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Domain string `json:"domain"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	block, err := h.service.CreateBlock(r.Context(), body.Domain, body.Reason, user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, block)
}

// DeleteBlock serves DELETE /api/v1/admin/federation/instances/{id}.
func (h *Handlers) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid block id")
		return
	}
	if err := h.service.DeleteBlock(r.Context(), id); err != nil {
		handleServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parsePage(r *http.Request) int {
	raw := r.URL.Query().Get("page")
	// Mastodon's outbox walker uses ?page=true for the first page.
	if raw == "true" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	// Bound page — (page-1)*size must not overflow int32 at the service
	// layer (outbox_page_size caps at 200, so 1e6 pages stays well under).
	if n > 1_000_000 {
		return 1_000_000
	}
	return n
}

// parseLimitOffset resolves the admin DataTable contract (?page=&limit=;
// ?offset= still honored for older callers) into limit + row offset + the
// 1-based page number echoed back in meta.
func parseLimitOffset(r *http.Request) (limit, offset, page int32) {
	limit = 20
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32); err == nil && v > 0 && v <= 100 {
		limit = int32(v)
	}
	// Explicit offset wins; page= maps to (page-1)*limit per the repo's
	// DataTable convention (page is 1-based).
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32); err == nil && v >= 0 {
		offset = int32(v)
		return limit, offset, offset/limit + 1
	}
	page = 1
	// Bound page — (page-1)*limit must not overflow int32.
	if v, err := strconv.ParseInt(r.URL.Query().Get("page"), 10, 32); err == nil && v > 1 {
		if v > 1_000_000 {
			v = 1_000_000
		}
		page = int32(v)
	}
	return limit, (page - 1) * limit, page
}

func totalPages(total int64, limit int32) int32 {
	if limit <= 0 || total <= 0 {
		return 1
	}
	return int32((total + int64(limit) - 1) / int64(limit))
}

// ---------------------------------------------------------------------------
// Inboxes (§126 M2 — POST /federation/inbox + /federation/actors/{u}/inbox)
// ---------------------------------------------------------------------------

const inboxMaxBody = 64 << 10 // §126: 64 KB payload cap

// PostInbox serves POST /federation/inbox — the shared inbox every remote
// instance's sharedInbox delivery targets.
func (h *Handlers) PostInbox(w http.ResponseWriter, r *http.Request) {
	h.postInbox(w, r, "")
}

// PostActorInbox serves POST /federation/actors/{username}/inbox. Unknown
// or non-federated actors 404 before any signature work — the shared
// inbox remains the catch-all for addressed activities.
func (h *Handlers) PostActorInbox(w http.ResponseWriter, r *http.Request) {
	h.postInbox(w, r, chi.URLParam(r, "username"))
}

// postInbox runs the §126 pipeline: 64 KB cap → parse → declared-actor
// block pre-check → keyId resolution → signature+digest verify →
// jc:fed:inbox:dedup SET NX → federation_inbound insert → process_inbound
// enqueue → 202. Signature failures → 401; blocked domain → 403 + a
// rejected ledger row; transient key-fetch failure → 503 (remotes
// redeliver on 5xx; 401/403 are read as permanent and would drop the
// activity on the floor).
func (h *Handlers) postInbox(w http.ResponseWriter, r *http.Request, username string) {
	ctx := r.Context()

	if username != "" {
		actor, err := h.service.ActorExists(ctx, username)
		if err != nil {
			handleServiceErr(w, err)
			return
		}
		if actor.Status != "active" {
			// §126 M2: disabled/tombstoned actors answer 410 Gone — a
			// terminal signal remotes can stop retrying on.
			writeTypedJSON(w, http.StatusGone, ContentTypeActivity, tombstoneDocument(actor.ActorUrl))
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, inboxMaxBody+1))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Unreadable request body")
		return
	}
	if len(body) > inboxMaxBody {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Activity exceeds 64 KiB")
		return
	}
	var act map[string]any
	if err := json.Unmarshal(body, &act); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_activity", "Body is not an ActivityStreams object")
		return
	}

	// Declared-actor block pre-check — cheap rejection before any key
	// resolution network work; the signer's domain is re-checked after
	// resolveKeyActor.
	if actorURL := apActorID(act); actorURL != "" {
		if u, uerr := url.Parse(actorURL); uerr == nil && !h.service.instanceAllowed(ctx, u.Hostname()) {
			h.service.rejectInbound(ctx, act, actorURL, "instance blocked", body)
			errpkg.WriteError(w, http.StatusForbidden, "instance_blocked", "Instance is not permitted")
			return
		}
	}

	params := ParseSignatureHeader(r.Header.Get("Signature"))
	keyID := params["keyId"]
	if keyID == "" {
		errpkg.WriteError(w, http.StatusUnauthorized, "missing_signature", "Signature keyId required")
		return
	}
	signer, err := h.service.resolveKeyActor(ctx, keyID)
	if err != nil {
		switch {
		case errors.Is(err, ErrRemoteBlocked):
			h.service.rejectInbound(ctx, act, apActorID(act), "signer instance blocked", body)
			errpkg.WriteError(w, http.StatusForbidden, "instance_blocked", "Instance is not permitted")
		case errors.Is(err, ErrRemoteFetch):
			errpkg.WriteError(w, http.StatusServiceUnavailable, "key_fetch_failed", "Signer key unavailable — retry")
		default:
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Signature key unresolved")
		}
		return
	}
	if signer.PublicKeyPem == nil || *signer.PublicKeyPem == "" {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Signer has no advertised key")
		return
	}
	pub, err := parsePublicKeyPEM(*signer.PublicKeyPem)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Signer key unparseable")
		return
	}
	verified := VerifySignature(r, pub, body) == nil
	if !verified {
		// A failed verify on a cached signer may be key rotation in
		// place — same keyId, fresh pem. One bounded re-fetch and retry
		// before rejecting; without it the remote is cut off until the
		// row's own refresh path happens to run.
		if refreshed, rerr := h.service.refreshRemoteKey(ctx, signer); rerr == nil &&
			refreshed.PublicKeyPem != nil && *refreshed.PublicKeyPem != "" {
			if pub2, perr := parsePublicKeyPEM(*refreshed.PublicKeyPem); perr == nil &&
				VerifySignature(r, pub2, body) == nil {
				signer = refreshed
				verified = true
			}
		}
	}
	if !verified {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Signature verification failed")
		return
	}

	// Activity-id provenance: the id must live on the signer's domain —
	// the same rule dispatchCreate applies to object ids. The global
	// (activity_ap_id) dedup would otherwise let any instance squat the
	// ids another instance legitimately owns: their real delivery then
	// dedups away unprocessed. Rejected BEFORE the id can touch the
	// marker or the ledger — storing it would itself be the squat.
	apID := apStr(act["id"])
	if apID != "" && !sameURLHost(apID, signer.ActorUrl) {
		h.service.rejectInbound(ctx, act, signer.ActorUrl, "activity id outside signer domain", body)
		errpkg.WriteError(w, http.StatusForbidden, "foreign_activity_id", "Activity id must live on the signer's domain")
		return
	}

	// Dedup — Valkey marker is the fast path; the partial unique on
	// federation_inbound is the backstop. Either short-circuits to 202.
	// On a marker hit the ledger row is verified before short-circuiting:
	// a first delivery whose insert failed AFTER the marker landed would
	// otherwise swallow the remote's redelivery without ever ledgering it.
	if apID != "" && h.service.vk != nil {
		if ok, serr := h.service.vk.SetNX(ctx, InboxDedupKey(apID), "1", 48*time.Hour).Result(); serr == nil && !ok {
			if _, derr := h.service.store.inboundByActivityID(ctx, apID); derr == nil {
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}
	}
	row, err := h.service.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   stringPtrOrNil(apID),
		ActivityType:   apStr(act["type"]),
		ActorUrl:       stringPtrOrNil(signer.ActorUrl),
		KeyID:          stringPtrOrNil(keyID),
		SignatureValid: true,
		Payload:        body,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusAccepted) // already ledgered
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	if h.service.enqueue != nil {
		if err := h.service.enqueue.EnqueueInbound(ctx, row.ID); err != nil {
			slog.Warn("federation: inbound enqueue failed", "inbound_id", row.ID, "error", err)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}
