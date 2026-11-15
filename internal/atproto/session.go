// External-client sessions + app passwords (§127 A6-4).
//
// A JapaConnect member's atproto account has no main PDS password —
// external clients (bsky.app) log in with a per-client APP PASSWORD
// against com.atproto.server.createSession, and hold an HS256
// access/refresh pair signed with the platform JWT secret (scope
// claims keep this domain disjoint from the site's own JWTs).
// Refresh tokens are single-use: the session row's refresh_hash
// rotates with every pair, so a replayed stale token is a theft
// signal — the whole session is revoked.
//
// Sessions never authorize repo writes: com.atproto.repo.* answers
// 501 on this PDS — records stay canonical in contents.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-4)
package atproto

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	jauth "github.com/jedafe/japa/internal/auth"
	gen "github.com/jedafe/japa/internal/db/gen"
)

const (
	sessionScopeAccess  = "com.atproto.access"
	sessionScopeRefresh = "com.atproto.refresh"
	accessTokenTTL      = 2 * time.Hour
	refreshTokenTTL     = 90 * 24 * time.Hour
)

// ErrAuthRequired maps to the lexicon AuthenticationRequired /
// InvalidToken names — an unknown identifier, wrong password, or a
// dead/revoked/expired session all wear it (no oracle between them).
var ErrAuthRequired = errors.New("atproto: authentication required")

// jwtSecret resolves the HS256 secret captured at service construction —
// cfg.JWTSecret, or the ephemeral 32-byte random NewService minted when
// it was empty (dev: sessions die on restart, mirroring the platform's
// empty-secret behavior).
func (s *Service) jwtSecret() []byte { return s.jwtKey }

// mintAccessJWT issues the short-lived access token — scope + aud are
// what verifyAccessJWT re-checks (the claims are the whole contract:
// no per-request session lookup, matching real-PDS token semantics).
func (s *Service) mintAccessJWT(did string) (string, error) {
	now := time.Now()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"scope": sessionScopeAccess,
		"sub":   did,
		"aud":   s.hostDID(),
		"iat":   now.Unix(),
		"exp":   now.Add(accessTokenTTL).Unix(),
		"jti":   hex.EncodeToString(jti),
	}).SignedString(s.jwtSecret())
}

// mintRefreshJWT issues the refresh token whose jti IS the session
// row's id — rotation swaps refresh_hash so a replayed old token
// fails the CAS. Single-use enforced server-side, not by exp alone.
// The nonce carries real entropy: every other claim is deterministic,
// so two rotations in the same second would otherwise mint identical
// JWTs (identical hash → the "burned" token stays live).
func (s *Service) mintRefreshJWT(did string, sessionID pgtype.UUID) (string, error) {
	now := time.Now()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"scope": sessionScopeRefresh,
		"sub":   did,
		"aud":   s.hostDID(),
		"iat":   now.Unix(),
		"exp":   now.Add(refreshTokenTTL).Unix(),
		"jti":   uuidString(sessionID),
		"nonce": hex.EncodeToString(nonce),
	}).SignedString(s.jwtSecret())
}

// parseSessionJWT verifies signature+aud+expiry and returns the claims.
// Callers re-check scope — a refresh token must never serve as access.
func (s *Service) parseSessionJWT(raw, wantScope string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("atproto: unexpected jwt alg %v", t.Method.Alg())
		}
		return s.jwtSecret(), nil
	}, jwt.WithAudience(s.hostDID()), jwt.WithExpirationRequired())
	if err != nil {
		return nil, ErrAuthRequired
	}
	if scope, _ := claims["scope"].(string); scope != wantScope {
		return nil, ErrAuthRequired
	}
	return claims, nil
}

// bearerToken extracts the JWT from an Authorization: Bearer header.
func bearerToken(header string) string {
	const p = "Bearer "
	if !strings.HasPrefix(header, p) {
		return ""
	}
	return strings.TrimSpace(header[len(p):])
}

func refreshHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SessionAccount is the authenticated-account view every authed XRPC
// endpoint resolves once — did/handle/status plus the member email
// getSession echoes back.
type SessionAccount struct {
	AccountID      pgtype.UUID
	UserID         pgtype.UUID
	DID            string
	Handle         string
	Status         string
	DisabledReason *string
	Email          string
}

// activeStatus maps account state to the lexicon {active,status?} pair
// session outputs carry: self-disable reads 'deactivated', admin
// force-disable reads 'takendown' (the lexicon's two inert states —
// tombstoned accounts mint no sessions at all).
func (a *SessionAccount) activeStatus() (bool, string) {
	if a.Status != AccountStatusDisabled {
		return true, ""
	}
	if a.DisabledReason != nil && *a.DisabledReason == DisableReasonAdmin {
		return false, "takendown"
	}
	return false, "deactivated"
}

// AuthenticateAccess verifies the access JWT and resolves its sub DID
// to the account — did or previous_did both resolve (a pre-migration
// token stays valid across the did:web→did:plc swap: sub names the OLD
// did which previous_did still matches). Tombstoned/missing accounts
// fail AuthRequired; disabled accounts authenticate (the client needs
// the session to call activateAccount).
func (s *Service) AuthenticateAccess(ctx context.Context, authHeader string) (*SessionAccount, error) {
	raw := bearerToken(authHeader)
	if raw == "" {
		return nil, ErrAuthRequired
	}
	claims, err := s.parseSessionJWT(raw, sessionScopeAccess)
	if err != nil {
		return nil, err
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, ErrAuthRequired
	}
	return s.sessionAccountByDID(ctx, sub)
}

func (s *Service) sessionAccountByDID(ctx context.Context, did string) (*SessionAccount, error) {
	acct, err := s.store.q.GetAtprotoSessionAccount(ctx, did)
	if err != nil {
		return nil, ErrAuthRequired
	}
	if acct.Status == AccountStatusTombstone {
		return nil, ErrAuthRequired
	}
	return &SessionAccount{
		AccountID:      acct.ID,
		UserID:         acct.UserID,
		DID:            acct.Did,
		Handle:         acct.Handle,
		Status:         acct.Status,
		DisabledReason: acct.DisabledReason,
		Email:          acct.UserEmail,
	}, nil
}

// CreateSession authenticates identifier+app-password → the token pair.
// Identifier accepts handle, live username, did, or previous_did; the
// password verifies against every live app-password hash on the account
// (the wire carries no name — any live password admits). Timing stays
// flat: a bcrypt verify runs on every failure path (dummy hash on
// no-account, all-live-rows on no-match). A disabled account still
// mints — the response's active:false+status reports the state and the
// client needs the session to activateAccount. Tombstone/missing →
// AuthenticationRequired (indistinguishable from bad credentials).
func (s *Service) CreateSession(ctx context.Context, identifier, password, userAgent, ip string) (*SessionAccount, string, string, error) {
	if !s.featureOn(ctx) {
		return nil, "", "", ErrAuthRequired
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || password == "" {
		return nil, "", "", ErrAuthRequired
	}
	acct, err := s.store.q.GetAtprotoSessionAccount(ctx, identifier)
	if errors.Is(err, pgx.ErrNoRows) {
		// Flat timing: the bcrypt cost is the dominant term — run the
		// same verify a real account would so a missing account isn't
		// distinguishable from a wrong password.
		_ = jauth.VerifyPassword(sessionDummyHash, password)
		return nil, "", "", ErrAuthRequired
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: session account: %w", err)
	}
	if acct.Status == AccountStatusTombstone {
		_ = jauth.VerifyPassword(sessionDummyHash, password)
		return nil, "", "", ErrAuthRequired
	}

	pws, err := s.store.q.ListAtprotoLiveAppPasswords(ctx, acct.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: app passwords: %w", err)
	}
	var matched *gen.AtprotoAppPassword
	for i := range pws {
		if jauth.VerifyPassword(pws[i].PasswordHash, password) == nil {
			matched = &pws[i]
			break
		}
	}
	if matched == nil {
		if len(pws) == 0 {
			_ = jauth.VerifyPassword(sessionDummyHash, password)
		}
		return nil, "", "", ErrAuthRequired
	}
	_ = s.store.q.TouchAtprotoAppPassword(ctx, matched.ID)

	// Session row first — its id is the refresh jti.
	sess, err := s.store.q.CreateAtprotoSession(ctx, gen.CreateAtprotoSessionParams{
		AccountID:        acct.ID,
		AppPasswordID:    matched.ID,
		RefreshHash:      "", // rewritten below once the JWT exists
		UserAgent:        strPtr(userAgent),
		LastIp:           strPtr(ip),
		RefreshExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(refreshTokenTTL), Valid: true},
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: create session: %w", err)
	}
	access, err := s.mintAccessJWT(acct.Did)
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: access jwt: %w", err)
	}
	refresh, err := s.mintRefreshJWT(acct.Did, sess.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: refresh jwt: %w", err)
	}
	if n, err := s.store.q.RotateAtprotoSession(ctx, gen.RotateAtprotoSessionParams{
		ID:      sess.ID,
		NewHash: refreshHash(refresh),
		OldHash: "",
	}); err != nil || n == 0 {
		// Don't strand an empty-hash row — it can never refresh anyway.
		_, _ = s.store.q.RevokeAtprotoSession(ctx, sess.ID)
		return nil, "", "", fmt.Errorf("atproto: session hash seed: %w", err)
	}
	return &SessionAccount{
		AccountID:      acct.ID,
		UserID:         acct.UserID,
		DID:            acct.Did,
		Handle:         acct.Handle,
		Status:         acct.Status,
		DisabledReason: acct.DisabledReason,
		Email:          acct.UserEmail,
	}, access, refresh, nil
}

// RefreshSession rotates the pair — the presented refresh token must
// verify AND match the session row's stored hash (single-use). A
// stale-hash presentation on a LIVE session is a theft signal: the
// whole session is revoked and the refresh fails either way, so the
// attacker and the legitimate client both fall off.
func (s *Service) RefreshSession(ctx context.Context, authHeader string) (*SessionAccount, string, string, error) {
	raw := bearerToken(authHeader)
	if raw == "" {
		return nil, "", "", ErrAuthRequired
	}
	claims, err := s.parseSessionJWT(raw, sessionScopeRefresh)
	if err != nil {
		return nil, "", "", err
	}
	jtiStr, _ := claims["jti"].(string)
	var jti pgtype.UUID
	if jtiStr == "" || jti.Scan(jtiStr) != nil {
		return nil, "", "", ErrAuthRequired
	}
	sess, err := s.store.q.GetAtprotoSessionByID(ctx, jti)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", "", ErrAuthRequired
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: session: %w", err)
	}
	if sess.RevokedAt.Valid || sess.RefreshExpiresAt.Time.Before(time.Now()) {
		return nil, "", "", ErrAuthRequired
	}
	// The token's sub must name the session's account — a valid JWT
	// carrying a mismatched pair is malformed, never a refresh.
	sub, _ := claims["sub"].(string)
	prevDid := ""
	if sess.PreviousDid != nil {
		prevDid = *sess.PreviousDid
	}
	if sub == "" || (sub != sess.Did && sub != prevDid) {
		return nil, "", "", ErrAuthRequired
	}
	// A session minted mid-erasure (createSession in-flight while the
	// tombstone CAS landed) must die at first refresh, not live on.
	if sess.AccountStatus == AccountStatusTombstone {
		_, _ = s.store.q.RevokeAtprotoSession(ctx, sess.ID)
		return nil, "", "", ErrAuthRequired
	}

	presented := refreshHash(raw)
	if sess.RefreshHash != presented {
		// Stale-hash replay on a live session — revoke everything.
		_, _ = s.store.q.RevokeAtprotoSession(ctx, sess.ID)
		return nil, "", "", ErrAuthRequired
	}

	sub = sess.Did // mint on the account's CURRENT did, not the token's
	access, err := s.mintAccessJWT(sub)
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: access jwt: %w", err)
	}
	refresh, err := s.mintRefreshJWT(sub, sess.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: refresh jwt: %w", err)
	}
	n, err := s.store.q.RotateAtprotoSession(ctx, gen.RotateAtprotoSessionParams{
		ID:      sess.ID,
		NewHash: refreshHash(refresh),
		OldHash: presented,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("atproto: rotate session: %w", err)
	}
	if n == 0 {
		// A concurrent rotation won the CAS — this token already burned.
		_, _ = s.store.q.RevokeAtprotoSession(ctx, sess.ID)
		return nil, "", "", ErrAuthRequired
	}
	return &SessionAccount{
		AccountID:      sess.AccountID,
		UserID:         sess.UserID,
		DID:            sess.Did,
		Handle:         sess.Handle,
		Status:         sess.AccountStatus,
		DisabledReason: sess.DisabledReason,
		Email:          sess.UserEmail,
	}, access, refresh, nil
}

// DeleteSession revokes the session a refresh JWT names. Idempotent —
// a second call on the same row still 200s (real-PDS semantics: the
// client can't tell revoked from dead).
func (s *Service) DeleteSession(ctx context.Context, authHeader string) error {
	raw := bearerToken(authHeader)
	if raw == "" {
		return ErrAuthRequired
	}
	claims, err := s.parseSessionJWT(raw, sessionScopeRefresh)
	if err != nil {
		return err
	}
	jtiStr, _ := claims["jti"].(string)
	var jti pgtype.UUID
	if jtiStr == "" || jti.Scan(jtiStr) != nil {
		return ErrAuthRequired
	}
	_, _ = s.store.q.RevokeAtprotoSession(ctx, jti)
	return nil
}

// ---------------------------------------------------------------------------
// App passwords (A6-4) — mint/list/revoke shared by the XRPC surface and
// the /api/v1 member routes.
// ---------------------------------------------------------------------------

// appPasswordAlphabet is RFC 4648 base32 lowercase — no 0/1/8/9/b/l/o
// lookalikes — matching the real PDS's xxxx-xxxx-xxxx-xxxx shape.
const appPasswordAlphabet = "abcdefghijklmnopqrstuvwxyz234567"

// mintAppPassword generates the shown-once secret.
func mintAppPassword() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	groups := make([]string, 4)
	for g := 0; g < 4; g++ {
		var b strings.Builder
		for i := 0; i < 4; i++ {
			b.WriteByte(appPasswordAlphabet[raw[g*4+i]&31])
		}
		groups[g] = b.String()
	}
	return strings.Join(groups, "-"), nil
}

// AppPasswordView is the list shape — the hash never leaves the store.
type AppPasswordView struct {
	ID         pgtype.UUID `json:"id"`
	Name       string      `json:"name"`
	CreatedAt  time.Time   `json:"created_at"`
	LastUsedAt *time.Time  `json:"last_used_at,omitempty"`
}

// CreateAppPassword mints a named app password for the member's
// account — the plaintext returns once and only the bcrypt persists.
// A name collision with a live password is a conflict (real-PDS rule).
func (s *Service) CreateAppPassword(ctx context.Context, accountID pgtype.UUID, name string) (*AppPasswordView, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return nil, "", ErrInvalidID
	}
	plain, err := mintAppPassword()
	if err != nil {
		return nil, "", err
	}
	hash, err := jauth.HashPassword(plain)
	if err != nil {
		return nil, "", err
	}
	row, err := s.store.q.CreateAtprotoAppPassword(ctx, gen.CreateAtprotoAppPasswordParams{
		AccountID:    accountID,
		Name:         name,
		PasswordHash: hash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", ErrHandleTaken
		}
		return nil, "", fmt.Errorf("atproto: create app password: %w", err)
	}
	return appPasswordView(&row), plain, nil
}

// ListAppPasswords returns live passwords newest-first.
func (s *Service) ListAppPasswords(ctx context.Context, accountID pgtype.UUID) ([]AppPasswordView, error) {
	rows, err := s.store.q.ListAtprotoAppPasswords(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("atproto: list app passwords: %w", err)
	}
	out := make([]AppPasswordView, 0, len(rows))
	for i := range rows {
		out = append(out, *appPasswordView(&rows[i]))
	}
	return out, nil
}

// RevokeAppPassword — member-API path (id-keyed). Sessions the
// credential minted die with it — revoking IS the "log that client
// out" lever.
func (s *Service) RevokeAppPassword(ctx context.Context, accountID, id pgtype.UUID) error {
	n, err := s.store.q.RevokeAtprotoAppPassword(ctx, gen.RevokeAtprotoAppPasswordParams{
		ID:        id,
		AccountID: accountID,
	})
	if err != nil {
		return fmt.Errorf("atproto: revoke app password: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.store.q.RevokeAtprotoSessionsForAppPassword(ctx, id)
}

// RevokeAppPasswordByName — the XRPC lexicon contract (name-keyed).
func (s *Service) RevokeAppPasswordByName(ctx context.Context, accountID pgtype.UUID, name string) error {
	// Resolve the row first — the session cascade keys on its id.
	pws, err := s.store.q.ListAtprotoLiveAppPasswords(ctx, accountID)
	if err != nil {
		return fmt.Errorf("atproto: app passwords: %w", err)
	}
	var target *gen.AtprotoAppPassword
	for i := range pws {
		if pws[i].Name == name {
			target = &pws[i]
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}
	if _, err := s.store.q.RevokeAtprotoAppPasswordByName(ctx, gen.RevokeAtprotoAppPasswordByNameParams{
		AccountID: accountID,
		Name:      name,
	}); err != nil {
		return fmt.Errorf("atproto: revoke app password: %w", err)
	}
	return s.store.q.RevokeAtprotoSessionsForAppPassword(ctx, target.ID)
}

func appPasswordView(r *gen.AtprotoAppPassword) *AppPasswordView {
	v := &AppPasswordView{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt.Time}
	if r.LastUsedAt.Valid {
		t := r.LastUsedAt.Time
		v.LastUsedAt = &t
	}
	return v
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// sessionDummyHash is a cost-12 bcrypt of a fixed non-credential
// string — the same timing-oracle defense internal/auth applies to
// login (CWE-208): every failed createSession runs one verify.
const sessionDummyHash = "$2a$12$0UiF9DiW0OZpvvnVY8ZSou5FYJrmO3njq6a4hQaaeFhFQZr3nK6pK" // #nosec G101 -- fixed non-credential string for timing-oracle defense

// b64urlJSON marshals a JWT segment (header or claims map) — the
// service-auth token is hand-assembled because golang-jwt wants a key
// type our hex-held K-256 isn't.
func b64urlJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
