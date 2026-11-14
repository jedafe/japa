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
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Service holds the mini-PDS identity business logic.
type Service struct {
	cfg   Config
	store *store
	vk    valkeycompat.Cmdable
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, cfg Config) *Service {
	return &Service{
		cfg:   cfg,
		store: newStore(gen.New(database.Writer)),
	}
}

// SetValkey wires the Valkey client for DID doc caching.
// Optional — nil disables caching (tests/development).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

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
func (s *Service) invalidateAccount(ctx context.Context, handle, didPathID string) {
	if s.vk == nil {
		return
	}
	_ = s.vk.Del(ctx,
		"jc:atp:did:"+strings.ToLower(didPathID),
		"jc:atp:account:"+strings.ToLower(handle),
	).Err()
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

// syncHandle lazily reconciles the stored handle with the member's live
// username after a rename — the DID itself is immutable
// (did:web:…:u:{user_uuid} never changes). Returns the handle to
// emit in docs. Idempotent; on contention the stale value is served.
func (s *Service) syncHandle(ctx context.Context, accountID pgtype.UUID, storedHandle, liveUsername string) string {
	if liveUsername == "" {
		return storedHandle
	}
	want := s.cfg.handleFor(liveUsername)
	if strings.EqualFold(storedHandle, want) {
		return storedHandle
	}
	if err := s.store.setAccountHandle(ctx, accountID, want); err != nil {
		return storedHandle
	}
	id := uuidString(accountID)
	s.invalidateAccount(ctx, storedHandle, id)
	s.invalidateAccount(ctx, want, id)
	return want
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
	if !strings.EqualFold(s.syncHandle(ctx, account.ID, account.Handle, account.LiveUsername), handle) {
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
	doc := didDocument(account.Did, s.syncHandle(ctx, account.ID, account.Handle, account.LiveUsername),
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
	account.Handle = s.syncHandle(ctx, account.ID, account.Handle, user.Username)
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
	handle := s.cfg.handleFor(user.Username)

	existing, err := s.store.accountByUserID(ctx, userID)
	if err == nil {
		if existing.Status != AccountStatusActive {
			if uerr := s.store.setAccountStatus(ctx, existing.ID, AccountStatusActive); uerr != nil {
				return AccountStatus{}, fmt.Errorf("atproto: reactivate: %w", uerr)
			}
			existing.Status = AccountStatusActive
			s.invalidateAccount(ctx, existing.Handle, uuidString(existing.ID))
		}
		existing.Handle = s.syncHandle(ctx, existing.ID, existing.Handle, user.Username)
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
		s.syncHandle(ctx, claimed.ID, claimed.Handle, claimed.LiveUsername)
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
	return s.statusFor(account), nil
}

// DisableAccount deactivates the caller's account. The row is kept so a
// later re-enable preserves the DID + keypair (identity is permanent);
// A6 additionally emits #tombstone for full erasure.
func (s *Service) DisableAccount(ctx context.Context, userID pgtype.UUID) (AccountStatus, error) {
	account, err := s.store.accountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountStatus{}, ErrNotFound
		}
		return AccountStatus{}, fmt.Errorf("atproto: lookup account: %w", err)
	}
	if account.Status != AccountStatusDisabled {
		if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusDisabled); err != nil {
			return AccountStatus{}, fmt.Errorf("atproto: disable: %w", err)
		}
	}
	s.invalidateAccount(ctx, account.Handle, uuidString(account.ID))
	account.Status = AccountStatusDisabled
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
