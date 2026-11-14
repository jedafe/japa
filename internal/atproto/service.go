package atproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/worker"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Service holds the mini-PDS identity business logic.
type Service struct {
	cfg   Config
	db    *db.DB
	store *store
	vk    valkeycompat.Cmdable
	flags features.FlagEvaluator
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, cfg Config) *Service {
	return &Service{
		cfg:   cfg,
		db:    database,
		store: newStore(gen.New(database.Writer)),
	}
}

// SetValkey wires the Valkey client for DID doc caching.
// Optional — nil disables caching (tests/development).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// SetFlags wires the feature-flag evaluator used to prune flag-gated
// content types (§42) from record projections — the same eligibility set
// federation applies (§127: "same eligibility as AP").
func (s *Service) SetFlags(c features.FlagEvaluator) { s.flags = c }

// ---------------------------------------------------------------------------
// Caching (jc:atp:* — valkey-keys-catalog)
// ---------------------------------------------------------------------------

func (s *Service) cacheGet(ctx context.Context, key string) (map[string]any, bool) {
	if s.vk == nil {
		return nil, false
	}
	raw, err := s.vk.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return nil, false
	}
	return doc, true
}

func (s *Service) cacheSet(ctx context.Context, key string, doc any, ttl time.Duration) {
	if s.vk == nil {
		return
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return
	}
	_ = s.vk.Set(ctx, key, raw, ttl).Err()
}

// invalidateAccount drops the account's cached resolution keys — the
// did:web doc under its u/{uuid} path key and the handle→DID proof.
// One DEL per key: valkey-go rejects multi-key commands spanning
// different hash slots (panics), so keys are deleted individually —
// same convention as internal/users/handlers.go.
func (s *Service) invalidateAccount(ctx context.Context, handle, didPathID string) {
	if s.vk == nil {
		return
	}
	for _, k := range [2]string{
		"jc:atp:did:" + strings.ToLower(didPathID),
		"jc:atp:account:" + strings.ToLower(handle),
	} {
		_ = s.vk.Del(ctx, k).Err()
	}
}

// ---------------------------------------------------------------------------
// Public identity resolution
// ---------------------------------------------------------------------------

// HandleFromHost extracts the member handle from an inbound Host header:
// alice.japa.to → alice.japa.to; japa.to / api.japa.to → "" (bare domain
// and the PDS host itself are not member handles).
func (s *Service) HandleFromHost(host string) string {
	host = strings.ToLower(strings.Split(host, ":")[0])
	suffix := "." + s.cfg.handleDomain()
	if !strings.HasSuffix(host, suffix) || host == s.cfg.handleDomain() {
		return ""
	}
	if host == strings.Split(s.cfg.pdsHost(), ":")[0] {
		return ""
	}
	return host
}

// handleUsername extracts the username localpart from a member handle —
// alice.japa.to → alice. Anything nested or malformed yields "".
func (s *Service) handleUsername(handle string) string {
	suffix := "." + s.cfg.handleDomain()
	local := strings.TrimSuffix(strings.ToLower(handle), suffix)
	if local == "" || local == strings.ToLower(handle) || strings.Contains(local, ".") {
		return ""
	}
	return local
}

// handleLocalRe matches a single atproto handle label — lowercase alnum
// with interior hyphens (no leading/trailing hyphen, no underscore).
// This is the §50 username charset minus '_': usernames like "alice_w"
// are legal platform-side but can never verify as atproto handles.
var handleLocalRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// validHandle reports whether a username's derived handle can ever verify
// on the atproto network: the username-derived label must be DNS-legal,
// and the handle must not collide with the PDS host itself (HandleFromHost
// excludes it — an account named after the PDS host mints a handle that
// permanently 404s).
func (s *Service) validHandle(username, handle string) bool {
	if !handleLocalRe.MatchString(strings.ToLower(username)) {
		return false
	}
	if handle == strings.Split(s.cfg.pdsHost(), ":")[0] {
		return false
	}
	return true
}

// syncHandle lazily reconciles the stored handle with the member's live
// username after a rename — the DID itself is immutable
// (did:web:…:u:{user_uuid} never changes). Returns the handle to
// emit in docs. Idempotent; on contention the stale value is served.
// userID (the account's owner — NOT the row PK) keys the jc:atp:did:*
// doc cache, which is addressed by the did:web u/{user_uuid} path.
func (s *Service) syncHandle(ctx context.Context, accountID, userID pgtype.UUID, storedHandle, liveUsername string) string {
	if liveUsername == "" || s.cfg.handleDomain() == "" {
		return storedHandle
	}
	want := s.cfg.handleFor(liveUsername)
	if strings.EqualFold(storedHandle, want) {
		return storedHandle
	}
	if err := s.store.setAccountHandle(ctx, accountID, want); err != nil {
		// The wanted handle is claimed by another row. When that claim is
		// stale — the holder renamed away — release it and retry once.
		// Without the release, accountByHandle's live-username preference
		// always picks the claimant-shadowed row and the stale row is
		// never synced: the claim never frees and the true holder is
		// locked out of the handle permanently.
		s.releaseStaleClaim(ctx, want, accountID)
		if rerr := s.store.setAccountHandle(ctx, accountID, want); rerr != nil {
			slog.Warn("atproto: handle sync blocked", "account_id", accountID.String(), "handle", want, "error", rerr)
			return storedHandle
		}
	}
	id := uuidString(userID)
	s.invalidateAccount(ctx, storedHandle, id)
	s.invalidateAccount(ctx, want, id)
	// §127 A3: handle changes ride #identity — relays/appviews refresh
	// their handle→DID caches off it.
	s.emitIdentityEvent(ctx, s.cfg.didFor(id), want)
	return want
}

// releaseStaleClaim frees a contested handle when its holder's live
// username no longer derives to it (a rename left the stored claim
// behind). A live-aligned claim is left alone — the handle is genuinely
// taken. accountID is the account currently syncing; the claim row is
// never itself. One level deep by design — chained stale claims resolve
// through each row's own read path.
func (s *Service) releaseStaleClaim(ctx context.Context, handle string, accountID pgtype.UUID) {
	claimed, err := s.store.handleClaim(ctx, handle)
	if err != nil || claimed.ID == accountID || claimed.LiveUsername == "" {
		return
	}
	live := s.cfg.handleFor(claimed.LiveUsername)
	if strings.EqualFold(live, handle) {
		return // live-aligned claim — the handle is genuinely held
	}
	if err := s.store.setAccountHandle(ctx, claimed.ID, live); err != nil {
		slog.Warn("atproto: stale claim release failed", "account_id", claimed.ID.String(), "handle", handle, "error", err)
		return
	}
	id := uuidString(claimed.UserID)
	s.invalidateAccount(ctx, claimed.Handle, id)
	s.invalidateAccount(ctx, live, id)
	s.emitIdentityEvent(ctx, s.cfg.didFor(id), live)
}

// uuidString renders a pgtype.UUID in canonical lowercase hyphenated
// form — the did:web u/{id} path component and jc:atp:did:* key suffix.
func uuidString(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ResolveHandleDID returns the member's DID string for the atproto-did
// well-known (text/plain body). Active accounts of active users only.
// Cached 10m at jc:atp:account:* — the appview hits this endpoint on
// every handle verification.
func (s *Service) ResolveHandleDID(ctx context.Context, handle string) (string, error) {
	cacheKey := "jc:atp:account:" + strings.ToLower(handle)
	if s.vk != nil {
		if did, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && did != "" {
			return did, nil
		}
	}
	account, err := s.store.accountByHandle(ctx, handle, s.handleUsername(handle))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("atproto: handle lookup: %w", err)
	}
	// Post-sync check: if the row matched via a stale stored handle, the
	// claim just migrated to the member's live username — the requested
	// handle is now unclaimed. Returning the row's DID here would cache
	// (and prove) the wrong account for a freed handle.
	if !strings.EqualFold(s.syncHandle(ctx, account.ID, account.UserID, account.Handle, account.LiveUsername), handle) {
		return "", ErrNotFound
	}
	if s.vk != nil {
		_ = s.vk.Set(ctx, cacheKey, account.Did, 10*time.Minute).Err()
	}
	return account.Did, nil
}

// idRe matches the did:web path component — a canonical UUID (the
// account's user_id). Path params outside it can't name an account.
var idRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DIDDocument serves the did:web resolution document for
// {PDSURL}/atproto/u/{id}/did.json — id is the account's user UUID.
// Cached 10m at jc:atp:did:{id}.
func (s *Service) DIDDocument(ctx context.Context, id string) (map[string]any, error) {
	if !idRe.MatchString(id) {
		return nil, ErrInvalidID
	}
	cacheKey := "jc:atp:did:" + strings.ToLower(id)
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
	}

	// Resolve by DID — the path component is the DID encoding, immutable
	// by construction (keyed on user_id, never the mutable username).
	// UUIDs are canonically lowercase; normalize so a case-variant path
	// still resolves the same document.
	account, err := s.store.accountByDID(ctx, s.cfg.didFor(strings.ToLower(id)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("atproto: account lookup: %w", err)
	}

	// alsoKnownAs emits the live handle (synced on read after renames).
	doc := didDocument(account.Did, s.syncHandle(ctx, account.ID, account.UserID, account.Handle, account.LiveUsername),
		account.PublicKeyMultibase, s.cfg.pdsURL())
	s.cacheSet(ctx, cacheKey, doc, 10*time.Minute)
	return doc, nil
}

// ---------------------------------------------------------------------------
// Member opt-in
// ---------------------------------------------------------------------------

// AccountStatus is the member-facing account view — the signing key
// has no field here by design (it never leaves the store/crypto layer).
type AccountStatus struct {
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	DID       string `json:"did,omitempty"`
	Handle    string `json:"handle,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (s *Service) statusFor(a gen.AtprotoAccount) AccountStatus {
	st := AccountStatus{
		Enabled: a.Status == AccountStatusActive,
		Status:  a.Status,
		DID:     a.Did,
		Handle:  a.Handle,
	}
	if a.CreatedAt.Valid {
		st.CreatedAt = a.CreatedAt.Time.UTC().Format(time.RFC3339)
	}
	return st
}

// GetMyAccount returns the caller's atproto account state.
func (s *Service) GetMyAccount(ctx context.Context, userID pgtype.UUID) (AccountStatus, error) {
	account, err := s.store.accountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountStatus{Enabled: false, Status: "none"}, nil
		}
		return AccountStatus{}, fmt.Errorf("atproto: my account: %w", err)
	}
	user, err := s.store.userByID(ctx, userID)
	if err != nil {
		return AccountStatus{}, fmt.Errorf("atproto: my user: %w", err)
	}
	account.Handle = s.syncHandle(ctx, account.ID, account.UserID, account.Handle, user.Username)
	return s.statusFor(account), nil
}

// EnableAccount mints (or reactivates) the caller's atproto account:
// K-256 keypair, did:web identifier, {username}.{handle_domain} handle.
// The private key persists only as internal/crypto ciphertext.
func (s *Service) EnableAccount(ctx context.Context, userID pgtype.UUID) (AccountStatus, error) {
	user, err := s.store.userByID(ctx, userID)
	if err != nil {
		return AccountStatus{}, fmt.Errorf("atproto: lookup user: %w", err)
	}
	if user.Status != "active" {
		return AccountStatus{}, ErrNotFound
	}
	// Refuse to mint or mutate identity on an unconfigured PDS — an empty
	// pdsURL yields "did:web::atproto:u:…" and an empty domain writes
	// "{username}." handles. Both persist permanently.
	if s.cfg.pdsURL() == "" || s.cfg.handleDomain() == "" {
		return AccountStatus{}, ErrMisconfigured
	}
	handle := s.cfg.handleFor(user.Username)
	// The §50 username charset allows '_' — atproto handle labels don't.
	// Minting anyway produces an account that verifies as handle.invalid
	// on the network (ADR-139's silent-failure mode); reject up front.
	// Also rejects handles colliding with the PDS host, which
	// HandleFromHost deliberately never resolves.
	if !s.validHandle(user.Username, handle) {
		return AccountStatus{}, ErrInvalidHandle
	}

	existing, err := s.store.accountByUserID(ctx, userID)
	if err == nil {
		// Tombstone is terminal — A6's erasure end-state. The member
		// endpoints must never resurrect or downgrade it.
		if existing.Status == AccountStatusTombstone {
			return AccountStatus{}, ErrNotFound
		}
		wasActive := existing.Status == AccountStatusActive
		if !wasActive {
			if uerr := s.store.setAccountStatus(ctx, existing.ID, AccountStatusActive); uerr != nil {
				return AccountStatus{}, fmt.Errorf("atproto: reactivate: %w", uerr)
			}
			existing.Status = AccountStatusActive
			s.invalidateAccount(ctx, existing.Handle, uuidString(existing.UserID))
			s.emitAccountEvent(ctx, existing.Did, true, "")
		}
		existing.Handle = s.syncHandle(ctx, existing.ID, existing.UserID, existing.Handle, user.Username)
		if err := s.ensureRepoInitialized(ctx, &existing); err != nil {
			return AccountStatus{}, err
		}
		if !wasActive {
			worker.EnqueueAtprotoRequestCrawl(ctx, existing.ID)
		}
		return s.statusFor(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AccountStatus{}, fmt.Errorf("atproto: lookup account: %w", err)
	}

	privHex, pubMultibase, err := generateSigningKey()
	if err != nil {
		return AccountStatus{}, err
	}
	privEnc, err := jcrypto.Encrypt(privHex)
	if err != nil {
		return AccountStatus{}, fmt.Errorf("atproto: encrypt key: %w", err)
	}

	// First-come-first-served on the case-insensitive handle — but only a
	// LIVE-ALIGNED claimant blocks. A stale row (holder renamed away,
	// account dormant) gets its handle synced to the live username first,
	// freeing the contested handle for its legitimate owner.
	if claimed, err := s.store.handleClaim(ctx, handle); err == nil && claimed.UserID != userID {
		if strings.EqualFold(s.cfg.handleFor(claimed.LiveUsername), handle) {
			return AccountStatus{}, ErrHandleTaken
		}
		s.syncHandle(ctx, claimed.ID, claimed.UserID, claimed.Handle, claimed.LiveUsername)
	}

	account, err := s.store.createAccount(ctx, gen.CreateAtprotoAccountParams{
		UserID:             userID,
		Handle:             handle,
		Did:                s.cfg.didFor(uuidString(userID)),
		SigningKeyEnc:      privEnc,
		PublicKeyMultibase: pubMultibase,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return AccountStatus{}, ErrHandleTaken
		}
		return AccountStatus{}, fmt.Errorf("atproto: create account: %w", err)
	}

	slog.Info("atproto account enabled", "user_id", userID.String(), "did", account.Did, "handle", handle)
	// §127 A3: announce activation + identity before the first commit so
	// the firehose reads account → identity → commit in seq order.
	s.emitAccountEvent(ctx, account.Did, true, "")
	s.emitIdentityEvent(ctx, account.Did, handle)
	// §127: opt-in mints the repo root commit — repo init + the
	// app.bsky.actor.profile record land in one signed commit.
	if err := s.ensureRepoInitialized(ctx, &account); err != nil {
		return AccountStatus{}, err
	}
	worker.EnqueueAtprotoRequestCrawl(ctx, account.ID)
	return s.statusFor(account), nil
}

// ensureRepoInitialized writes the member's first repo commit — an
// app.bsky.actor.profile record — when the account has none yet. A
// repo_seq of 0 marks either a fresh opt-in or a previously-interrupted
// bootstrap; the write is idempotent (same record, same MST) and runs
// the normal locked commit path.
func (s *Service) ensureRepoInitialized(ctx context.Context, account *gen.AtprotoAccount) error {
	if account.RepoSeq > 0 {
		return nil
	}
	return s.publishProfile(ctx, account)
}

// DisableAccount deactivates the caller's account. The row is kept so a
// later re-enable preserves the DID + keypair (identity is permanent);
// A6 additionally emits #tombstone for full erasure — and tombstone is
// terminal: this path must never downgrade it back to 'disabled'.
func (s *Service) DisableAccount(ctx context.Context, userID pgtype.UUID) (AccountStatus, error) {
	account, err := s.store.accountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountStatus{}, ErrNotFound
		}
		return AccountStatus{}, fmt.Errorf("atproto: lookup account: %w", err)
	}
	if account.Status == AccountStatusActive {
		if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusDisabled); err != nil {
			return AccountStatus{}, fmt.Errorf("atproto: disable: %w", err)
		}
		account.Status = AccountStatusDisabled
		s.emitAccountEvent(ctx, account.Did, false, "deactivated")
	}
	s.invalidateAccount(ctx, account.Handle, uuidString(account.UserID))
	return s.statusFor(account), nil
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

// AdminAccount is the admin-facing row — signing key zeroed before
// marshaling (never leaves the API, same contract as AP admin actors).
type AdminAccount struct {
	ID                 pgtype.UUID `json:"id"`
	UserID             pgtype.UUID `json:"user_id"`
	Username           string      `json:"username"`
	Handle             string      `json:"handle"`
	DID                string      `json:"did"`
	PublicKeyMultibase string      `json:"public_key_multibase"`
	SigningKeyEnc      string      `json:"-"`
	Status             string      `json:"status"`
	RepoSeq            int64       `json:"repo_seq"`
	CreatedAt          string      `json:"created_at"`
	DisabledAt         string      `json:"disabled_at,omitempty"`
}

// ListAccounts returns the paginated admin account list.
func (s *Service) ListAccounts(ctx context.Context, q string, limit, offset int32) ([]AdminAccount, int64, error) {
	rows, err := s.store.listAccounts(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("atproto: list accounts: %w", err)
	}
	total, err := s.store.countAccounts(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("atproto: count accounts: %w", err)
	}
	out := make([]AdminAccount, 0, len(rows))
	for _, r := range rows {
		a := AdminAccount{
			ID:                 r.ID,
			UserID:             r.UserID,
			Username:           r.Username,
			Handle:             r.Handle,
			DID:                r.Did,
			PublicKeyMultibase: r.PublicKeyMultibase,
			Status:             r.Status,
			RepoSeq:            r.RepoSeq,
		}
		if r.CreatedAt.Valid {
			a.CreatedAt = r.CreatedAt.Time.UTC().Format(time.RFC3339)
		}
		if r.DisabledAt.Valid {
			a.DisabledAt = r.DisabledAt.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, a)
	}
	return out, total, nil
}

// Stats returns account counters for the admin dashboard.
func (s *Service) Stats(ctx context.Context) (gen.GetAtprotoStatsRow, error) {
	return s.store.stats(ctx)
}
