package atproto

// XRPC sync handlers (§127 A3, unversioned public surface). These speak
// the XRPC error contract — {"error": <name>, "message": <str>} at HTTP
// 400 — not the /api/v1 envelope: relays and appviews parse lexicon
// error names, so the internal envelope's code shape doesn't apply here.

import (
	"errors"
	"net/http"
	"strconv"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// xrpcError writes the XRPC error contract for the public sync surface.
func xrpcError(w http.ResponseWriter, status int, name, message string) {
	errpkg.WriteJSON(w, status, map[string]string{
		"error":   name,
		"message": message,
	})
}

// xrpcNotFound maps service errors to the endpoint's lexicon error name —
// lexicon/param errors are 400, anything else is 500: a 4xx reads as a
// permanent condition to a relay, so transient failures must never wear it.
func xrpcNotFound(w http.ResponseWriter, err error, name string) {
	var inactive *repoInactiveError
	switch {
	case errors.As(err, &inactive):
		xrpcError(w, http.StatusBadRequest, inactive.name, "repository is not active")
	case errors.Is(err, ErrNotFound):
		xrpcError(w, http.StatusBadRequest, name, "repository not found")
	case errors.Is(err, ErrInvalidID):
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid parameter")
	default:
		xrpcError(w, http.StatusInternalServerError, "InternalError", "internal error")
	}
}

// GetHostDIDDoc serves GET /.well-known/did.json — the PDS host's own
// did:web document, which com.atproto.server.describeServer's `did`
// field resolves against.
func (h *Handlers) GetHostDIDDoc(w http.ResponseWriter, r *http.Request) {
	if h.service.cfg.pdsURL() == "" {
		handleServiceErr(w, ErrMisconfigured)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/did+ld+json", h.service.hostDIDDocument())
}

// DescribeServer serves GET /xrpc/com.atproto.server.describeServer —
// the PDS's self-description relays read after requestCrawl.
func (h *Handlers) DescribeServer(w http.ResponseWriter, r *http.Request) {
	if h.service.cfg.pdsURL() == "" || h.service.cfg.handleDomain() == "" {
		xrpcError(w, http.StatusInternalServerError, "InternalError", "server misconfigured")
		return
	}
	false_ := false
	out := &comatproto.ServerDescribeServer_Output{
		AvailableUserDomains:      []string{"." + h.service.cfg.handleDomain()},
		Did:                       h.service.hostDID(),
		InviteCodeRequired:        &false_,
		PhoneVerificationRequired: &false_,
	}
	writeTypedJSON(w, http.StatusOK, "application/json", out)
}

// GetRepo serves GET /xrpc/com.atproto.sync.getRepo?did=[&since=] —
// the full repo CAR without since; a diff CAR (head commit + the block
// set introduced by newer commits, per atproto_commit_blocks) with it.
func (h *Handlers) GetRepo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := h.service.RepoCARSince(r.Context(), q.Get("did"), q.Get("since"))
	if err != nil {
		xrpcNotFound(w, err, "RepoNotFound")
		return
	}
	w.Header().Set("Content-Type", carMediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetBlocks serves GET /xrpc/com.atproto.sync.getBlocks?did=&cids= —
// a CAR slice of the named blocks (repeated cids params, XRPC array).
func (h *Handlers) GetBlocks(w http.ResponseWriter, r *http.Request) {
	did := r.URL.Query().Get("did")
	cids := r.URL.Query()["cids"]
	data, err := h.service.BlocksCAR(r.Context(), did, cids)
	if err != nil {
		xrpcNotFound(w, err, "RepoNotFound")
		return
	}
	w.Header().Set("Content-Type", carMediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetLatestCommit serves GET /xrpc/com.atproto.sync.getLatestCommit?did=
// — head commit CID + rev.
func (h *Handlers) GetLatestCommit(w http.ResponseWriter, r *http.Request) {
	did := r.URL.Query().Get("did")
	c, rev, err := h.service.HeadCommit(r.Context(), did)
	if err != nil {
		xrpcNotFound(w, err, "RepoNotFound")
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.SyncGetLatestCommit_Output{
		Cid: c,
		Rev: rev,
	})
}

// GetRecord serves GET /xrpc/com.atproto.sync.getRecord?did=&collection=
// &rkey= — a CAR carrying the record and the MST material to verify it.
// The lexicon's required param is `did`; legacy `repo` is accepted for
// pre-rename callers (both name the same repo DID).
func (h *Handlers) GetRecord(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	did := q.Get("did")
	if did == "" {
		did = q.Get("repo")
	}
	data, err := h.service.RecordCAR(r.Context(), did, q.Get("collection"), q.Get("rkey"))
	if err != nil {
		xrpcNotFound(w, err, "RecordNotFound")
		return
	}
	w.Header().Set("Content-Type", carMediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetRepoStatus serves GET /xrpc/com.atproto.sync.getRepoStatus?did= —
// the relay's inactive-recheck: reports whether a hosted repo is
// servable and, when not, which inactive state it's in.
func (h *Handlers) GetRepoStatus(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.RepoStatus(r.Context(), r.URL.Query().Get("did"))
	if err != nil {
		xrpcNotFound(w, err, "RepoNotFound")
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", out)
}

// ListRepos serves GET /xrpc/com.atproto.sync.listRepos?limit=&cursor=
// — the hosted-repo census ecosystem resync tooling reads.
func (h *Handlers) ListRepos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var limit int32
	if raw := q.Get("limit"); raw != "" {
		if n, perr := strconv.ParseInt(raw, 10, 32); perr == nil {
			limit = int32(n)
		} else {
			xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid limit")
			return
		}
	}
	repos, cursor, err := h.service.ListRepos(r.Context(), q.Get("cursor"), limit)
	if err != nil {
		xrpcNotFound(w, err, "RepoNotFound")
		return
	}
	out := map[string]any{"repos": repos}
	if cursor != "" {
		out["cursor"] = cursor
	}
	writeTypedJSON(w, http.StatusOK, "application/json", out)
}
