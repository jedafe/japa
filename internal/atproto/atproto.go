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
	"strings"
)

// Account lifecycle statuses — mirror federated_actors semantics.
const (
	AccountStatusActive    = "active"
	AccountStatusDisabled  = "disabled"
	AccountStatusTombstone = "tombstone"
)

var (
	ErrNotFound      = errors.New("atproto account not found")
	ErrInvalidHandle = errors.New("invalid atproto handle")
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
// the SiteBase host (mirrors federation's derivation).
func (c Config) handleDomain() string {
	if c.HandleDomain != "" {
		return strings.ToLower(strings.TrimSpace(c.HandleDomain))
	}
	u := strings.TrimPrefix(c.SiteBase, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(strings.TrimSuffix(u, "/"))
}

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

// relayURL resolves the requestCrawl target (A3).
func (c Config) relayURL() string {
	if c.RelayURL != "" {
		return c.RelayURL
	}
	return "https://bsky.network"
}

// didFor derives the did:web identifier for a username:
// did:web:{pds_host}:atproto:u:{username} — resolves at
// {PDSURL}/atproto/u/{username}/did.json per the did:web spec.
func (c Config) didFor(username string) string {
	return "did:web:" + c.pdsHost() + ":atproto:u:" + username
}

// handleFor derives the member handle: {username}.{handle_domain}.
func (c Config) handleFor(username string) string {
	return username + "." + c.handleDomain()
}
