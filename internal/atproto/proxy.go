// app.bsky.* read proxy (§127 A6-4).
//
// An external client logged into this PDS calls app.bsky.* on US, not
// on the AppView — the PDS forwards each GET to the configured
// JC_ATPROTO_APPVIEW_URL verbatim (same path + query) and streams the
// answer back. No credential forwarding: the member's federated data
// is public, so public-appview reads suffice for timeline/profile/
// thread. Mutating verbs and com.atproto.repo.* answer 501 — writes
// stay canonical in contents.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-4)
package atproto

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// appviewHTTPClient bounds the upstream hop — a hung appview must not
// pin our request handler. Redirects are NOT followed: a 3xx could
// bounce the fetch to an arbitrary origin (SSRF by redirect chain) —
// we mirror the status + Location verbatim and let the client decide.
var appviewHTTPClient = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// ProxyRepo answers every /xrpc/com.atproto.repo.* call 501 — that
// namespace is PDS-LOCAL lexicon space (repo.getRecord/listRecords/
// describeRepo plus every write): an AppView can't serve it, so
// proxying would let upstream answer for our names. Read-mostly
// external login leans on app.bsky.* instead.
func (h *Handlers) ProxyRepo(w http.ResponseWriter, r *http.Request) {
	xrpcError(w, http.StatusNotImplemented, "MethodNotImplemented",
		"writes originate on japa.to — this PDS is read-mostly for external clients")
}

// ProxyAppView forwards GET /xrpc/app.bsky.{nsid}?{query} to the
// configured appview and mirrors status + content-type + body back.
// Non-GET app.bsky verbs answer 501.
func (h *Handlers) ProxyAppView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.ProxyRepo(w, r)
		return
	}
	base := strings.TrimRight(h.service.cfg.AppViewURL, "/")
	if base == "" {
		xrpcError(w, http.StatusInternalServerError, "InternalError", "appview not configured")
		return
	}
	// Stay inside /xrpc/ on the appview origin — a crafted `..` suffix
	// must not widen the proxy into other upstream namespaces.
	path := r.URL.Path
	if !strings.HasPrefix(path, "/xrpc/") || strings.Contains(path, "..") {
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "bad proxy path")
		return
	}
	target := base + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		xrpcError(w, http.StatusInternalServerError, "InternalError", "proxy build failed")
		return
	}
	resp, err := appviewHTTPClient.Do(upstream)
	if err != nil {
		xrpcError(w, http.StatusBadGateway, "UpstreamUnreachable", "appview request failed")
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "content-type" || lk == "cache-control" ||
			lk == "atproto-content-labelers" || lk == "location" {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
