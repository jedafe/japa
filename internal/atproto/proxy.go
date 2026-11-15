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
// pin our request handler.
var appviewHTTPClient = &http.Client{Timeout: 15 * time.Second}

// ProxyAppView forwards GET /xrpc/app.bsky.{nsid}?{query} to the
// configured appview and mirrors status + content-type + body back.
// Everything else on /xrpc/app.bsky.* — plus any com.atproto.repo.*
// write an external client attempts — answers 501 so the client sees
// a lexicon error, not a silent black hole.
func (h *Handlers) ProxyAppView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		xrpcError(w, http.StatusNotImplemented, "MethodNotImplemented",
			"writes originate on japa.to — this PDS is read-mostly for external clients")
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
		if lk == "content-type" || lk == "cache-control" || lk == "atproto-content-labelers" {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
