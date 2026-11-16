// Package federation implements the ActivityPub identity surface for
// JapaConnect (Section 126, ADR-138): WebFinger discovery, Person actor
// documents, OrderedCollections (outbox/followers/following), NodeInfo,
// and the HTTP-signature primitives used by outbound delivery (M2).
//
// Actor and object @id URLs live on the API host under /federation/*
// (unversioned — AP ids are permanent). Member opt-in creates a
// federated_actors row holding an RSA-2048 keypair; the private key is
// AES-256-GCM-encrypted at rest via internal/crypto.
//
// Ref: japaconnect-engineering/sections/126-activitypub-federation.md
package federation

import (
	"errors"

	"github.com/jedafe/japa/internal/rss/model"
)

// ErrNotFound is returned when no active federated actor, federatable
// content object, or remote entity exists for the requested identifier.
var ErrNotFound = errors.New("federation: not found")

// ErrGone marks a once-live object that is now deleted or withdrawn —
// answered 410 + a Tombstone body so remotes can garbage-collect the
// copy they may hold (the same was-ever-live signal the Delete fanout
// uses: deleted_at or published_at on the lite row).
var ErrGone = errors.New("federation: object tombstoned")

// ErrHandleDomain is returned when a WebFinger resource targets a domain
// other than the configured handle domain.
var ErrHandleDomain = errors.New("federation: foreign handle domain")

// ErrInvalidDomain is returned when an admin submits a malformed instance
// block domain.
var ErrInvalidDomain = errors.New("federation: invalid domain")

// ErrRotationCooldown marks a member-triggered key rotation inside the
// §126 cooldown window (federation.key_rotation.cooldown_hours) — the
// handler maps it to 429. The admin path skips this gate.
var ErrRotationCooldown = errors.New("federation: rotation cooldown active")

// ErrNotFederated marks an identity that once federated but no longer
// does (disabled/opted-out actor) — §126 maps it to 404 `not_federated`,
// distinct from a never-federated handle's `not_found`.
var ErrNotFederated = errors.New("federation: not federated")

// Federatable content types (§126 Object Mapping) — real contents
// content_type values only. blog_post and guide serialize as Article;
// everything else as Note. Flag-gated types are pruned per request by
// features.GatedContentTypes — a disabled feature's content never
// federates (same rule RSS/search apply).
// federatableTypes re-exports the shared allowlist — canonical home is
// rss/model (leaf package) so both federation and the mini-PDS consume
// the same list without an import cycle (§127: same eligibility as AP).
var federatableTypes = model.FederatableTypes()

// apObjectType maps a contents.content_type to its ActivityStreams type.
func apObjectType(contentType string) string {
	switch contentType {
	case "blog_post", "guide":
		return "Article"
	default:
		return "Note"
	}
}

// Actor status values persisted on federated_actors.status.
const (
	ActorStatusActive    = "active"
	ActorStatusDisabled  = "disabled"
	ActorStatusTombstone = "tombstone"
)

// Content types served on the public federation surface.
const (
	ContentTypeActivity = `application/activity+json; profile="https://www.w3.org/ns/activitystreams"`
	ContentTypeJRD      = "application/jrd+json"
	ContentTypeNodeInfo = `application/json; profile="http://nodeinfo.diaspora.software/ns/schema/2.1#"`
)

// asContext is the JSON-LD context set on every emitted document.
var asContext = []any{
	"https://www.w3.org/ns/activitystreams",
	"https://w3id.org/security/v1",
}

// asPublic is the ActivityStreams Public addressing collection.
const asPublic = "https://www.w3.org/ns/activitystreams#Public"
