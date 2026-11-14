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

import "errors"

// ErrNotFound is returned when no active federated actor, federatable
// content object, or remote entity exists for the requested identifier.
var ErrNotFound = errors.New("federation: not found")

// ErrHandleDomain is returned when a WebFinger resource targets a domain
// other than the configured handle domain.
var ErrHandleDomain = errors.New("federation: foreign handle domain")

// ErrInvalidDomain is returned when an admin submits a malformed instance
// block domain.
var ErrInvalidDomain = errors.New("federation: invalid domain")

// Federatable content types (§126 Object Mapping) — real contents
// content_type values only. blog_post and guide serialize as Article;
// everything else as Note.
var federatableTypes = []string{"blog_post", "guide", "discussion", "story"}

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
