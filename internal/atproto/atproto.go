// Package atproto implements JapaConnect's native AT Protocol identity
// layer — the A1 milestone of Section 127's mini-PDS: per-member opt-in
// accounts holding a K-256 (secp256k1) signing keypair, a did:web
// identifier, and a {username}.{handle_domain} handle. Public surface:
// the atproto-did well-known (handle→DID proof) and did:web resolution
// documents. Repo storage, XRPC sync, and Jetstream inbound arrive in
// milestones A2–A5.
//
// Account/DID/handle identifiers are permanent — the public PDS surface
// is unversioned at the PDS host, never under /api/v1.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md
// Ref: japaconnect-engineering/adr/ADR-139-atproto-native.md
package atproto

import (
	"errors"
	"net"
	"strings"
)

// Account lifecycle statuses — mirror federated_actors semantics.
const (
	AccountStatusActive    = "active"
	AccountStatusDisabled  = "disabled"
	AccountStatusTombstone = "tombstone"
)

// DisableReason values for atproto_accounts.disabled_reason — WHO froze
// the account (001147). 'member' re-enables only via member opt-in;
// 'lifecycle' re-enables when the user returns to 'active'. A NULL
// reason reads as member-initiated — every pre-column disable was.
const (
	DisableReasonMember    = "member"
	DisableReasonLifecycle = "lifecycle"
)

// repoInactiveError names the lexicon error for a repo that exists but
// isn't servable — the sync surface distinguishes RepoDeactivated /
// RepoSuspended / RepoTakendown so relays can tell "not hosted"
// (RepoNotFound) from a present-but-frozen account.
type repoInactiveError struct{ name string }

func (e *repoInactiveError) Error() string { return "atproto repo inactive: " + e.name }

var (
	ErrNotFound      = errors.New("atproto account not found")
	ErrInvalidHandle = errors.New("invalid atproto handle")
	ErrInvalidID     = errors.New("invalid atproto account id")
	ErrHandleTaken   = errors.New("atproto handle already claimed")
	ErrMisconfigured = errors.New("atproto pds url or handle domain not configured")
)

// Config carries the JC_ATPROTO_* deployment values (internal/config),
// with the same derivation fallbacks as the federation Config.
type Config struct {
	// PDSURL is the public base serving DID docs + XRPC sync —
	// JC_ATPROTO_PDS_URL (default: JC_API_PUBLIC_URL). The `service`
	// endpoint embedded in DID documents.
	PDSURL string
	// APIBase is JC_API_PUBLIC_URL — PDSURL fallback.
	APIBase string
	// SiteBase is JC_SITE_URL — handle-domain fallback.
	SiteBase string
	// HandleDomain — JC_ATPROTO_HANDLE_DOMAIN (default: host of
	// JC_SITE_URL). Member handles are {username}.{HandleDomain}.
	HandleDomain string
	// RelayURL — JC_ATPROTO_RELAY_URL (default https://bsky.network).
	// requestCrawl target (A3).
	RelayURL string
	// PLCURL — JC_ATPROTO_PLC_URL (default https://plc.directory).
	// did:plc directory (A6).
	PLCURL string
	// JetstreamURL — JC_ATPROTO_JETSTREAM_URL; filtered inbound
	// consumer endpoint (A5).
	JetstreamURL string
}

// pdsURL resolves the PDS public base: configured PDS URL wins, else the
// API public base.
func (c Config) pdsURL() string {
	if c.PDSURL != "" {
		return strings.TrimRight(c.PDSURL, "/")
	}
	return strings.TrimRight(c.APIBase, "/")
}

// handleDomain resolves the handle domain: configured domain wins, else
// the SiteBase host (mirrors federation's derivation). Ports are stripped —
// handles are DNS names and can't carry one (dev JC_SITE_URL values like
// localhost:3000 would otherwise mint unresolvable "alice.localhost:3000"
// handles). The pdsHost derivation keeps its port deliberately — did:web
// encodes it (%3A).
func (c Config) handleDomain() string {
	if c.HandleDomain != "" {
		return stripPort(strings.ToLower(strings.TrimSpace(c.HandleDomain)))
	}
	u := strings.TrimPrefix(c.SiteBase, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return stripPort(strings.ToLower(strings.TrimSuffix(u, "/")))
}

// stripPort drops the :port suffix from a host[:port] string, tolerating
// bracketed IPv6; a bare hostname passes through unchanged.
func stripPort(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(h, "[]")
	}
	return hostport
}

// siteBase returns the canonical site origin — page links in records
// (post text, canonical_url anchors) hang off it.
func (c Config) siteBase() string { return strings.TrimRight(c.SiteBase, "/") }

// pdsHost returns the host component of the PDS URL — the did:web
// method-specific-id's leading segment.
func (c Config) pdsHost() string {
	u := strings.TrimPrefix(c.pdsURL(), "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(strings.TrimSuffix(u, "/"))
}

// didFor derives the did:web identifier for an account:
// did:web:{pds_host}:atproto:u:{user_uuid} — resolves at
// {PDSURL}/atproto/u/{uuid}/did.json per the did:web spec.
// The path component is the user UUID, NOT the username: usernames are
// mutable and re-claimable, so a username-derived DID would collide
// across different members over time (rename → new registrant mints the
// identical DID under a different keypair). UUIDs are permanent.
// did:web requires percent-encoding of ':' in the host segment —
// dev PDS hosts like localhost:8080 produce did:web:localhost%3A8080:…
func (c Config) didFor(id string) string {
	host := strings.ReplaceAll(c.pdsHost(), ":", "%3A")
	return "did:web:" + host + ":atproto:u:" + id
}

// handleFor derives the member handle: {username}.{handle_domain},
// lowercased — atproto handles are case-insensitive and normalized
// lowercase; storing canonical-lowercase prevents 'Alice' and 'alice'
// (distinct users under case-sensitive users.username) colliding into
// ambiguous lower() matches.
func (c Config) handleFor(username string) string {
	return strings.ToLower(username + "." + c.handleDomain())
}
