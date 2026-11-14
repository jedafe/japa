package atproto

// XRPC sync handlers (§127 A3, unversioned public surface). These speak
// the XRPC error contract — {"error": <name>, "message": <str>} at HTTP
// 400 — not the /api/v1 envelope: relays and appviews parse lexicon
// error names, so the internal envelope's code shape doesn't apply here.

import (
	"errors"
	"net/http"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// xrpcError writes the XRPC error contract for the public sync surface.
func xrpcError(w http.ResponseWriter, name, message string) {
	errpkg.WriteJSON(w, http.StatusBadRequest, map[string]string{
		"error":   name,
		"message": message,
	})
}

// xrpcNotFound maps ErrNotFound to the endpoint's lexicon error name.
func xrpcNotFound(w http.ResponseWriter, err error, name string) {
	switch {
	case errors.Is(err, ErrNotFound):
		xrpcError(w, name, "repository not found")
	case errors.Is(err, ErrInvalidID):
		xrpcError(w, "InvalidRequest", "invalid parameter")
	default:
		xrpcError(w, "InternalError", "internal error")
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
// the full repo CAR (commit root + live blocks). `since` is accepted
// but advisory: the full CAR is always a legal, self-sufficient answer
// (§127 documents it as such — per-commit diff CARs need a
// block-per-commit index that adds storage for marginal wire savings).
func (h *Handlers) GetRepo(w http.ResponseWriter, r *http.Request) {
	did := r.URL.Query().Get("did")
	data, err := h.service.repoCARByDID(r.Context(), did)
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

// GetRecord serves GET /xrpc/com.atproto.sync.getRecord?repo=&collection=
// &rkey= — a CAR carrying the record and the MST material to verify it.
func (h *Handlers) GetRecord(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := h.service.RecordCAR(r.Context(), q.Get("repo"), q.Get("collection"), q.Get("rkey"))
	if err != nil {
		xrpcNotFound(w, err, "RecordNotFound")
		return
	}
	w.Header().Set("Content-Type", carMediaType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
