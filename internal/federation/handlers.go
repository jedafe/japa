package federation

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	case errors.Is(err, ErrNotFederated):
		// §126: a disabled/opted-out actor answers 404 with the
		// not_federated code — never-found handles keep "not_found".
		errpkg.WriteError(w, http.StatusNotFound, "not_federated", "Federation is not enabled for this identity")
	case errors.Is(err, ErrRotationCooldown):
		errpkg.WriteError(w, http.StatusTooManyRequests, "rotation_too_soon", "Key was rotated recently; try again later")
	case errors.Is(err, ErrHandleDomain):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_resource", "Unsupported resource")
	case errors.Is(err, ErrInvalidDomain):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_domain", "Invalid instance domain")
	case errors.Is(err, ErrConflict):
		errpkg.WriteError(w, http.StatusConflict, "conflict", "The target does not meet the required state")
	case errors.Is(err, ErrSignatureRequired):
		errpkg.WriteError(w, http.StatusUnauthorized, "signature_required", "A valid HTTP signature is required")
	case errors.Is(err, ErrRemoteBlocked):
		errpkg.WriteError(w, http.StatusForbidden, "instance_blocked", "Instance is not permitted")
	case errors.Is(err, ErrRemoteFetch):
		// Upstream AP doc unreachable — a remote-side condition, not
		// our internal error (relay subscribe hits this path).
		errpkg.WriteError(w, http.StatusBadGateway, "remote_fetch_failed", "Remote actor document could not be fetched")
	case errors.Is(err, ErrInvalidTarget):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_target", "Target does not resolve to a remote actor")
	case errors.Is(err, ErrMoveTooSoon):
		errpkg.WriteError(w, http.StatusTooManyRequests, "move_too_soon", "A migration already ran inside the cooldown window")
	case errors.Is(err, ErrInvalidTag):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_tag", "Tag must be 1-64 letters, digits or underscore")
	case errors.Is(err, ErrInvalidCursor):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor", "Malformed pagination cursor")
	case errors.Is(err, ErrTagSubscriptionLimit):
		errpkg.WriteError(w, http.StatusTooManyRequests, "tag_subscription_limit", "Tag subscription limit reached")
	case errors.Is(err, ErrMoveNotConfirmed):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "move_not_confirmed", "Target actor does not confirm the migration")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

// authorizeFetch applies the §126 authorized-fetch gate to a public GET:
// when federation.authorized_fetch is on, the request must carry a valid
// HTTP signature. Returns false once a response has been written.
func (h *Handlers) authorizeFetch(w http.ResponseWriter, r *http.Request) bool {
	if err := h.service.VerifyFetchSignature(r); err != nil {
		handleServiceErr(w, err)
		return false
	}
	return true
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
	if !h.authorizeFetch(w, r) {
		return
	}
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
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.Outbox(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowers serves GET /federation/actors/{username}/followers[?page=N].
func (h *Handlers) GetFollowers(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.Followers(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowing serves GET /federation/actors/{username}/following[?page=N].
func (h *Handlers) GetFollowing(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.Following(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFeatured serves GET /federation/actors/{username}/collections/featured
// — the member's pinned federatable contents (§126 M4).
func (h *Handlers) GetFeatured(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.Featured(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFeaturedTags serves GET /federation/actors/{username}/collections/tags
// — the hashtags the member uses across their federatable contents (§126 M4).
func (h *Handlers) GetFeaturedTags(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.FeaturedTags(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetObject serves GET /federation/objects/contents/{id}.
func (h *Handlers) GetObject(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": st})
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"requests": rows}})
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": status}})
}

// RotateMyFederationKey serves POST /api/v1/me/federation/rotate-key
// (§126 M3a) — swaps the member's signing keypair and announces it via
// Update{Person} fanout. Member-triggered rotation is cooldown-gated.
func (h *Handlers) RotateMyFederationKey(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.service.RotateActorKey(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// FollowRemote serves POST /api/v1/me/federation/following (§126 M5) —
// resolves the target and delivers the signed Follow.
func (h *Handlers) FollowRemote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Target) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "target is required")
		return
	}
	res, err := h.service.FollowRemote(r.Context(), user.UserID, body.Target)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// ListOutboundFollowing serves GET /api/v1/me/federation/following (§126 M5).
func (h *Handlers) ListOutboundFollowing(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := h.service.ListOutboundFollowing(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": rows}})
}

// UnfollowRemote serves DELETE /api/v1/me/federation/following/{remote_actor_id}
// (§126 M5) — delivers Undo{Follow} and tears the edge down.
func (h *Handlers) UnfollowRemote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var remoteID pgtype.UUID
	if err := remoteID.Scan(chi.URLParam(r, "remote_actor_id")); err != nil || !remoteID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid remote actor id")
		return
	}
	if err := h.service.UnfollowRemote(r.Context(), user.UserID, remoteID); err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "undone"}})
}

// RemoteTimeline serves GET /api/v1/me/federation/timeline (§126 M5) —
// keyset-paginated remote posts from actors the member follows.
func (h *Handlers) RemoteTimeline(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var limit int32
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 32); err == nil && v > 0 {
			limit = int32(v)
		}
	}
	page, err := h.service.RemoteTimeline(r.Context(), user.UserID, r.URL.Query().Get("cursor"), limit, r.URL.Query().Get("tag"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// RemoteTagSearch serves GET /api/v1/me/federation/search?tag=&domain=
// (§126 M5 tag discovery) — live fan-out to remote tag timelines.
func (h *Handlers) RemoteTagSearch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	res, err := h.service.RemoteTagSearch(r.Context(), user.UserID,
		r.URL.Query().Get("tag"), r.URL.Query().Get("domain"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// ListTagSubscriptions serves GET /api/v1/me/federation/tag-subscriptions.
func (h *Handlers) ListTagSubscriptions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := h.service.ListTagSubscriptions(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscriptions": rows}})
}

// SubscribeTag serves POST /api/v1/me/federation/tag-subscriptions —
// {tag, domain?}: row is the ingest warrant; a domain-scoped sub also
// ledgers Follow{Hashtag} from the instance actor.
func (h *Handlers) SubscribeTag(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Tag    string `json:"tag"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "tag is required")
		return
	}
	sub, err := h.service.SubscribeTag(r.Context(), user.UserID, body.Tag, body.Domain)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sub})
}

// UnsubscribeTag serves DELETE /api/v1/me/federation/tag-subscriptions/{id}
// — Undo{Follow} for domain-scoped subs, then row delete.
func (h *Handlers) UnsubscribeTag(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var subID pgtype.UUID
	if err := subID.Scan(chi.URLParam(r, "id")); err != nil || !subID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid subscription id")
		return
	}
	if err := h.service.UnsubscribeTag(r.Context(), user.UserID, subID); err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "unsubscribed"}})
}

// RequestMove serves POST /api/v1/me/federation/move (§126 M5) —
// verifies the target's alsoKnownAs, stamps moved_to_url, fans the
// signed Move out to remote followers.
func (h *Handlers) RequestMove(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		TargetActorURL string `json:"target_actor_url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || strings.TrimSpace(body.TargetActorURL) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "target_actor_url is required")
		return
	}
	if err := h.service.RequestMove(r.Context(), user.UserID, body.TargetActorURL); err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "moved"}})
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

// RotateActorKey serves POST /api/v1/admin/federation/actors/{id}/rotate-key
// (§126 M3a) — the incident-response rotation: same swap as the member
// path but ungated by the cooldown, so a compromised key can be retired
// immediately.
func (h *Handlers) RotateActorKey(w http.ResponseWriter, r *http.Request) {
	var actorID pgtype.UUID
	if err := actorID.Scan(chi.URLParam(r, "id")); err != nil || !actorID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid actor id")
		return
	}
	res, err := h.service.AdminRotateActorKey(r.Context(), actorID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, res)
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
	// M3: a relay courier re-signs subscribers' activities — the id then
	// lives on the AUTHOR's domain, not the signer's. Only an accepted
	// relay subscription relaxes the check (and only to the actor's own
	// host; anything else is still a squat).
	foreignID := apID != "" && !sameURLHost(apID, signer.ActorUrl)
	if foreignID && h.service.isAcceptedRelay(ctx, signer.ActorUrl) {
		foreignID = !sameURLHost(apID, apActorID(act))
	}
	if foreignID {
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

// ---------------------------------------------------------------------------
// M3 — instance actor + group actors (public surface)
// ---------------------------------------------------------------------------

// GetInstance serves GET /federation/instance — the Application doc.
func (h *Handlers) GetInstance(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.InstanceDocument(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetInstanceOutbox serves GET /federation/instance/outbox[?page=N].
func (h *Handlers) GetInstanceOutbox(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.InstanceOutbox(r.Context(), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetInstanceFollowers serves GET /federation/instance/followers[?page=N].
func (h *Handlers) GetInstanceFollowers(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.InstanceFollowers(r.Context(), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetGroup serves GET /federation/groups/{slug} — 410 + Tombstone for a
// tombstoned group actor; 404 when the space is private or never federated.
func (h *Handlers) GetGroup(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	doc, err := h.service.GroupDocument(r.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrGone) {
			if tomb := h.service.GroupTombstone(r.Context(), slug); tomb != nil {
				writeTypedJSON(w, http.StatusGone, ContentTypeActivity, tomb)
				return
			}
		}
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetGroupOutbox serves GET /federation/groups/{slug}/outbox[?page=N].
func (h *Handlers) GetGroupOutbox(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.GroupOutbox(r.Context(), chi.URLParam(r, "slug"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetGroupFeatured serves GET /federation/groups/{slug}/collections/featured
// (§126 M5).
func (h *Handlers) GetGroupFeatured(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.GroupFeatured(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetGroupFeaturedTags serves GET /federation/groups/{slug}/collections/tags
// (§126 M5).
func (h *Handlers) GetGroupFeaturedTags(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.GroupFeaturedTags(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetGroupFollowers serves GET /federation/groups/{slug}/followers[?page=N].
func (h *Handlers) GetGroupFollowers(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeFetch(w, r) {
		return
	}
	doc, err := h.service.GroupFollowers(r.Context(), chi.URLParam(r, "slug"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// ---------------------------------------------------------------------------
// M3 — admin: relays + group actors
// ---------------------------------------------------------------------------

// ListRelays serves GET /admin/federation/relays[?page=&limit=].
func (h *Handlers) ListRelays(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	rows, total, err := h.service.ListRelays(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"relays": rows, "total": total, "page": page, "limit": limit,
		"total_pages": totalPages(total, limit),
	})
}

// CreateRelay serves POST /admin/federation/relays — body {actor_url}.
func (h *Handlers) CreateRelay(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		ActorURL string `json:"actor_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ActorURL) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "actor_url is required")
		return
	}
	relay, err := h.service.SubscribeRelay(r.Context(), strings.TrimSpace(body.ActorURL), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, relay)
}

// DeleteRelay serves DELETE /admin/federation/relays/{id} — Undo{Follow} + remove.
func (h *Handlers) DeleteRelay(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid relay id")
		return
	}
	if err := h.service.UnsubscribeRelay(r.Context(), id); err != nil {
		handleServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListGroups serves GET /admin/federation/groups[?page=&limit=].
func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	rows, total, err := h.service.ListGroupActors(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"groups": rows, "total": total, "page": page, "limit": limit,
		"total_pages": totalPages(total, limit),
	})
}

// CreateGroup serves POST /admin/federation/groups — body {space_id}.
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		SpaceID string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "space_id is required")
		return
	}
	var spaceID pgtype.UUID
	if err := spaceID.Scan(body.SpaceID); err != nil || !spaceID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
		return
	}
	actor, err := h.service.ProvisionGroupActor(r.Context(), spaceID, user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, actor)
}

// DeleteGroup serves DELETE /admin/federation/groups/{id} — tombstone +
// Delete{Group} fanout.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid actor id")
		return
	}
	if err := h.service.TombstoneGroupActor(r.Context(), id); err != nil {
		handleServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
