package federation

// Remote actor resolution: remote_actors is the durable cache, Valkey
// jc:fed:remote:{sha256(actor_url)} the 6h hot cache, and fetchAP the
// SSRF-guarded fetch behind both. Signature verification resolves the
// signer through keyId → remote actor — the resolved actor's
// publicKey.id must equal the keyId presented, else a signer could
// borrow another actor's identity.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/db/gen"
)

// remoteCacheTTL matches the §126 jc:fed:remote:* catalog entry (6h).
const remoteCacheTTL = 6 * time.Hour

var (
	// ErrRemoteBlocked is a permanent failure — the target instance is
	// administratively blocked (or the mode is allowlist and the domain
	// is not on it). Tasks map it to SkipRetry.
	ErrRemoteBlocked = errors.New("federation: remote instance blocked")
	// ErrRemoteFetch is a transient fetch/resolution failure — retryable.
	ErrRemoteFetch = errors.New("federation: remote fetch failed")
)

// remoteActorDoc is the subset of a remote Person/Application/Service
// document the cache row needs.
type remoteActorDoc struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	PreferredUsername string `json:"preferredUsername"`
	Name              string `json:"name"`
	Inbox             string `json:"inbox"`
	Outbox            string `json:"outbox"`
	Endpoints         struct {
		SharedInbox string `json:"sharedInbox"`
	} `json:"endpoints"`
	PublicKey struct {
		ID           string `json:"id"`
		Owner        string `json:"owner"`
		PublicKeyPem string `json:"publicKeyPem"`
	} `json:"publicKey"`
	Icon struct {
		URL string `json:"url"`
	} `json:"icon"`
}

// remoteActorTypes are document types we accept as actors — Person is
// the common case, Service/Application cover instance bots and bridges.
var remoteActorTypes = map[string]bool{
	"Person": true, "Service": true, "Application": true,
	"Organization": true, "Group": true,
}

// cachedRemoteRow is the Valkey payload — the full remote_actors row so
// a hot hit costs no DB round-trip.
func (s *Service) cacheGetRemote(ctx context.Context, actorURL string) *gen.RemoteActor {
	if s.vk == nil {
		return nil
	}
	raw, err := s.vk.Get(ctx, RemoteCacheKey(actorURL)).Result()
	if err != nil {
		return nil
	}
	var row gen.RemoteActor
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return nil
	}
	return &row
}

func (s *Service) cacheSetRemote(ctx context.Context, row *gen.RemoteActor) {
	if s.vk == nil || row == nil {
		return
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return
	}
	s.vk.Set(ctx, RemoteCacheKey(row.ActorUrl), raw, remoteCacheTTL)
}

// cacheDelRemote drops the hot-cache copy of a remote actor — a row
// cached while 'active' must stop serving the moment a Delete{actor}
// tombstones it, or resolveRemoteActor's hit path returns the stale
// pre-delete snapshot and keeps dispatching for a dead actor.
func (s *Service) cacheDelRemote(ctx context.Context, actorURL string) {
	if s.vk == nil || actorURL == "" {
		return
	}
	_ = s.vk.Del(ctx, RemoteCacheKey(actorURL)).Err()
}

// instanceAllowed applies the §126 inbound gate: 'open' mode consults the
// blocklist; 'allowlist' inverts — only domains present in
// federation_instance_blocks (the operator-curated domain list) are
// permitted. Both inbound processing and outbound delivery share it.
func (s *Service) instanceAllowed(ctx context.Context, domain string) bool {
	if s.store == nil || domain == "" {
		return false
	}
	mode := "open"
	if s.settings != nil {
		if v, err := s.settings.GetSetting(ctx, "federation.instances.mode"); err == nil && v != "" {
			mode = v
		}
	}
	_, err := s.store.instanceBlock(ctx, domain)
	listed := err == nil
	if mode == "allowlist" {
		return listed
	}
	return !listed
}

// resolveRemoteActor returns the cached remote_actors row for actorURL,
// fetching + upserting when absent or stale (fetched_at older than the
// Valkey TTL). Instance blocks are honored before any fetch.
func (s *Service) resolveRemoteActor(ctx context.Context, actorURL string) (*gen.RemoteActor, error) {
	u, err := url.Parse(actorURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: bad actor url %q", ErrRemoteFetch, actorURL)
	}
	if !s.instanceAllowed(ctx, u.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrRemoteBlocked, u.Hostname())
	}

	if row := s.cacheGetRemote(ctx, actorURL); row != nil && row.Status == "active" {
		return row, nil
	}
	if s.store != nil {
		if row, err := s.store.remoteActorByURL(ctx, actorURL); err == nil {
			fresh := row.FetchedAt.Valid && time.Since(row.FetchedAt.Time) < remoteCacheTTL
			if fresh && row.Status == "active" {
				s.cacheSetRemote(ctx, &row)
				return &row, nil
			}
			if row.Status == "tombstone" || row.Status == "deleted" || row.Status == "blocked" {
				return nil, fmt.Errorf("%w: %s", ErrRemoteBlocked, u.Hostname())
			}
		}
	}

	doc, err := s.fetchRemoteActorDoc(ctx, actorURL)
	if err != nil {
		return nil, err
	}
	row, err := s.upsertRemoteActor(ctx, actorURL, doc)
	if err != nil {
		return nil, fmt.Errorf("%w: upsert: %v", ErrRemoteFetch, err)
	}
	s.cacheSetRemote(ctx, row)
	return row, nil
}

// sameURLHost reports whether two URL strings share a host — the
// provenance rule for activity/object ids (an activity's id must live
// on its signer's domain, per the dispatchCreate object-id rule).
func sameURLHost(a, b string) bool {
	au, aerr := url.Parse(a)
	bu, berr := url.Parse(b)
	if aerr != nil || berr != nil || au.Hostname() == "" || bu.Hostname() == "" {
		return false
	}
	return strings.EqualFold(au.Hostname(), bu.Hostname())
}

// fetchRemoteActorDoc fetches and minimally parses an actor document.
func (s *Service) fetchRemoteActorDoc(ctx context.Context, actorURL string) (*remoteActorDoc, error) {
	body, err := s.fetchAP(ctx, actorURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteFetch, err)
	}
	var doc remoteActorDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: actor doc parse: %v", ErrRemoteFetch, err)
	}
	if !remoteActorTypes[doc.Type] {
		return nil, fmt.Errorf("%w: %q is not an actor document (type %q)", ErrRemoteFetch, actorURL, doc.Type)
	}
	return &doc, nil
}

// upsertRemoteActor maps a fetched document onto remote_actors.
func (s *Service) upsertRemoteActor(ctx context.Context, actorURL string, doc *remoteActorDoc) (*gen.RemoteActor, error) {
	u, _ := url.Parse(actorURL)
	domain := strings.ToLower(u.Hostname())
	nz := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	// acct needs a localpart — a username-less actor stores no acct at all
	// rather than the degenerate "@domain".
	acct := ""
	if doc.PreferredUsername != "" {
		acct = doc.PreferredUsername + "@" + domain
	}
	row, err := s.store.upsertRemoteActor(ctx, gen.UpsertRemoteActorParams{
		ActorUrl:          actorURL,
		Acct:              nz(acct),
		Domain:            domain,
		InboxUrl:          nz(doc.Inbox),
		SharedInboxUrl:    nz(doc.Endpoints.SharedInbox),
		OutboxUrl:         nz(doc.Outbox),
		PublicKeyPem:      nz(doc.PublicKey.PublicKeyPem),
		PublicKeyID:       nz(doc.PublicKey.ID),
		PreferredUsername: nz(doc.PreferredUsername),
		DisplayName:       nz(doc.Name),
		AvatarUrl:         nz(doc.Icon.URL),
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// resolveKeyActor resolves the signing actor for an inbound request.
// Resolution order: the remote_actors row carrying public_key_id ==
// keyId; else fetch the keyId URL minus its fragment — an actor document
// there must advertise publicKey.id == keyId (ownership check), and a
// bare Key document is followed through its owner to the actor.
func (s *Service) resolveKeyActor(ctx context.Context, keyID string) (*gen.RemoteActor, error) {
	if s.store == nil {
		return nil, fmt.Errorf("%w: store not configured", ErrRemoteFetch)
	}
	if row, err := s.store.remoteActorByKeyID(ctx, keyID); err == nil {
		// A tombstoned/deleted/blocked actor's key must not verify — its
		// signed traffic stops being meaningful the moment the row leaves
		// 'active', the same posture resolveRemoteActor takes. The domain
		// gate is re-checked too: an instance blocked after the row was
		// cached must not keep verifying.
		if row.Status != "active" {
			return nil, fmt.Errorf("%w: signer actor status %s", ErrRemoteBlocked, row.Status)
		}
		if !s.instanceAllowed(ctx, row.Domain) {
			return nil, fmt.Errorf("%w: %s", ErrRemoteBlocked, row.Domain)
		}
		return &row, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: key lookup: %v", ErrRemoteFetch, err)
	}

	// keyId typically embeds the actor URL before the fragment
	// ({actor}#main-key). A remote that splits key hosting gets one extra
	// hop through the Key doc's owner — both paths verify the advertised
	// publicKey.id equals the presented keyId before trusting the key.
	base, _, _ := strings.Cut(keyID, "#")
	if base == "" {
		return nil, fmt.Errorf("%w: empty key id", ErrRemoteFetch)
	}
	if u, uerr := url.Parse(base); uerr == nil && !s.instanceAllowed(ctx, u.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrRemoteBlocked, u.Hostname())
	}
	doc, err := s.fetchRemoteActorDoc(ctx, base)
	if err != nil {
		return nil, err
	}
	actorURL := doc.ID
	if actorURL == "" {
		return nil, fmt.Errorf("%w: key document missing id", ErrRemoteFetch)
	}
	// fetchedFrom records the URL the accepted document actually came
	// from — the canonical-confirmation gate below decides on it.
	fetchedFrom := base
	if !remoteActorTypes[doc.Type] {
		// Bare Key document — resolve its owner as the actor.
		if doc.PublicKey.Owner == "" {
			return nil, fmt.Errorf("%w: key document without owner", ErrRemoteFetch)
		}
		owner, err := s.fetchRemoteActorDoc(ctx, doc.PublicKey.Owner)
		if err != nil {
			return nil, err
		}
		if !remoteActorTypes[owner.Type] {
			return nil, fmt.Errorf("%w: key owner is not an actor", ErrRemoteFetch)
		}
		if !strings.EqualFold(owner.PublicKey.ID, keyID) && owner.PublicKey.PublicKeyPem != doc.PublicKey.PublicKeyPem {
			return nil, fmt.Errorf("%w: owner does not advertise key %q", ErrRemoteFetch, keyID)
		}
		fetchedFrom = doc.PublicKey.Owner
		actorURL = owner.ID
		doc = owner
	} else if !strings.EqualFold(doc.PublicKey.ID, keyID) {
		return nil, fmt.Errorf("%w: actor does not advertise key %q", ErrRemoteFetch, keyID)
	}
	if doc.PublicKey.PublicKeyPem == "" {
		return nil, fmt.Errorf("%w: actor advertises no public key", ErrRemoteFetch)
	}
	au, aerr := url.Parse(actorURL)
	if aerr != nil || au.Hostname() == "" {
		return nil, fmt.Errorf("%w: actor id is not a URL: %q", ErrRemoteFetch, actorURL)
	}
	// The actor document's own id may point at a different host than the
	// keyId URL did (redirected identities, key proxies) — the instance
	// gate applies to the identity we actually cache, not only the hop.
	if !s.instanceAllowed(ctx, au.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrRemoteBlocked, au.Hostname())
	}
	// Canonical confirmation: a document served at X claiming to BE actor
	// Y only binds keyId to Y when Y's own address advertises the same
	// key — otherwise any host could mint remote_actors rows keyed under
	// someone else's actor URL, and their signature would then verify
	// against the minted key while dispatch trusts actor_url (full
	// impersonation). When the fetched URL already IS the claimed id the
	// self-claim is canonical — Mastodon's keyId layout lands here.
	if !strings.EqualFold(actorURL, fetchedFrom) {
		canonical, err := s.fetchRemoteActorDoc(ctx, actorURL)
		if err != nil {
			return nil, err
		}
		if !remoteActorTypes[canonical.Type] ||
			!strings.EqualFold(canonical.ID, actorURL) ||
			!strings.EqualFold(canonical.PublicKey.ID, keyID) {
			return nil, fmt.Errorf("%w: actor %q does not confirm key %q", ErrRemoteFetch, actorURL, keyID)
		}
		doc = canonical
	}
	// An advertised key owner must name the actor itself — a document
	// may not point its signing key at a different actor.
	if doc.PublicKey.Owner != "" && !strings.EqualFold(doc.PublicKey.Owner, actorURL) {
		return nil, fmt.Errorf("%w: key owner %q does not match actor %q", ErrRemoteFetch, doc.PublicKey.Owner, actorURL)
	}
	return s.upsertRemoteActor(ctx, actorURL, doc)
}
