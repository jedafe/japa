// Package mcp — oauth.go implements the OAuth 2.1 authorization server that
// lets web-hosted agents (claude.ai custom connectors, ChatGPT connectors)
// obtain per-user bearer tokens for the §119 MCP endpoint.
//
// The server supports dynamic client registration (RFC 7591), the
// authorization-code grant with mandatory PKCE S256, rotating refresh
// tokens, and revocation (RFC 7009). Discovery follows RFC 8414
// (authorization-server metadata) and RFC 9728 (protected-resource
// metadata). Only public clients are supported — no client secrets are
// issued, matching how MCP hosts authenticate.
//
// All bearer material (authorization codes, access tokens, refresh tokens)
// is stored as SHA-256 hashes, matching the api_keys posture (§47).
//
// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
// Ref: japaconnect-engineering/adr/ADR-126-mcp-agent-distribution.md
package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	// Token value prefixes — the prefix is part of the bearer string so
	// middleware can cheaply route to the right validator.
	oauthAccessTokenPrefix  = "jcpat_"
	oauthRefreshTokenPrefix = "jcprt_"
	oauthAuthCodePrefix     = "jcac_"
	oauthClientIDPrefix     = "jcmc_"

	// oauthScopeOpenID is the OAuth-only pseudo-scope (§120 v1.4) that
	// unlocks GET /oauth/userinfo. It is not a ScopeCatalog entry — it is
	// inert to every publicapi.RequireScope gate.
	oauthScopeOpenID = "openid"

	// authorizationCodeTTL bounds the window in which a code can be
	// exchanged. 10 minutes per common OAuth practice.
	authorizationCodeTTL = 10 * time.Minute
)

// oauthSettingsReader is the subset of config.Settings the OAuth server
// needs for hot-reloadable TTLs (Rule 3). *config.Settings satisfies it.
type oauthSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// OAuthClient is a registered RFC 7591 client.
type OAuthClient struct {
	ID                      pgtype.UUID
	ClientID                string
	ClientName              string
	RedirectURIs            []string
	GrantTypes              []string
	Scopes                  []string
	TokenEndpointAuthMethod string
	CreatedAt               pgtype.Timestamptz
}

// OAuthAuthorizationCode is a pending authorization-code grant.
type OAuthAuthorizationCode struct {
	ID                  pgtype.UUID
	ClientID            string
	UserID              pgtype.UUID
	RedirectURI         string
	Scope               string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           pgtype.Timestamptz
}

// OAuthGrant is a connected-app row for the §120 grant-management
// surface: a client holding at least one live (unrevoked, unexpired)
// token for the user.
type OAuthGrant struct {
	ClientID       string
	ClientName     string
	Scopes         []string
	FirstGrantedAt pgtype.Timestamptz
	LastGrantedAt  pgtype.Timestamptz
}

// OAuthToken is an issued access or refresh token (hash-stored).
type OAuthToken struct {
	ID        pgtype.UUID
	ClientID  string
	UserID    pgtype.UUID
	Scope     string
	TokenType string
	ExpiresAt pgtype.Timestamptz
	RevokedAt *pgtype.Timestamptz
}

// OAuthUserIdentity is the userinfo (§120 v1.4) claims source — the
// identity fields a client holding an openid-scoped token may read.
type OAuthUserIdentity struct {
	UserID        pgtype.UUID
	Email         string
	Username      string
	EmailVerified bool
	DisplayName   *string
	AvatarURL     *string
}

// OAuthStore is the persistence surface the OAuth handlers and token
// validator need. Defined as an interface so handlers are unit-testable.
type OAuthStore interface {
	CreateClient(ctx context.Context, c OAuthClient) error
	GetClientByClientID(ctx context.Context, clientID string) (*OAuthClient, error)
	// CountClients returns the live oauth_clients row count — enforces the
	// mcp.oauth_max_clients registration cap (§120 DCR spam bound).
	CountClients(ctx context.Context) (int64, error)
	CreateAuthorizationCode(ctx context.Context, codeHash string, c OAuthAuthorizationCode) error
	// ConsumeAuthorizationCode atomically marks a code used; returns
	// pgx.ErrNoRows when the code is unknown, already used, or expired.
	ConsumeAuthorizationCode(ctx context.Context, codeHash, clientID string) (*OAuthAuthorizationCode, error)
	// CreateTokenPair inserts the access + refresh rows of a grant in one
	// transaction — a half-written pair can never surface. Returns the new
	// refresh token's row ID for the rotation chain.
	CreateTokenPair(ctx context.Context, accessHash string, access OAuthToken, refreshHash string, refresh OAuthToken) (refreshID pgtype.UUID, err error)
	GetTokenByHash(ctx context.Context, tokenType, tokenHash string) (*OAuthToken, error)
	// ConsumeRefreshToken atomically revokes and returns a refresh token;
	// pgx.ErrNoRows when unknown, already revoked, or consumed. Atomicity
	// matters: two concurrent rotations must not both pass the
	// not-yet-revoked check.
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (*OAuthToken, error)
	RevokeTokenByHash(ctx context.Context, tokenHash string) error
	// RevokeTokenFamily revokes every refresh + access token for a
	// client/user pair — used on refresh-token reuse detection and on
	// user-initiated grant revocation (§120 connected apps).
	RevokeTokenFamily(ctx context.Context, clientID string, userID pgtype.UUID) error
	// ListUserGrants returns the distinct clients holding at least one
	// unrevoked, unexpired token for the user — the connected-apps list.
	ListUserGrants(ctx context.Context, userID pgtype.UUID) ([]OAuthGrant, error)
	MarkTokenReplaced(ctx context.Context, oldID, newID pgtype.UUID) error
	// AuditOAuthEvent writes an audit_logs row for grant lifecycle events.
	// Best-effort — callers log-and-continue on error. actorUserID may be
	// invalid (client-initiated events like DCR).
	AuditOAuthEvent(ctx context.Context, action string, actorUserID pgtype.UUID, metadata map[string]any, ip string) error
	// GetUserIdentity returns the userinfo claims row for an openid-scoped
	// token owner.
	GetUserIdentity(ctx context.Context, userID pgtype.UUID) (*OAuthUserIdentity, error)
}

// oauthPGXStore is the production OAuthStore backed by pgx (same style as
// publicapi.Store — handwritten SQL, no sqlc dependency).
type oauthPGXStore struct {
	db *pgxpool.Pool
}

// NewOAuthStore creates the pgx-backed OAuth store.
func NewOAuthStore(db *pgxpool.Pool) OAuthStore {
	return &oauthPGXStore{db: db}
}

func (s *oauthPGXStore) CreateClient(ctx context.Context, c OAuthClient) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, client_name, redirect_uris, grant_types, scopes, token_endpoint_auth_method)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, c.ClientID, c.ClientName, c.RedirectURIs, c.GrantTypes, c.Scopes, c.TokenEndpointAuthMethod)

	return err
}

func (s *oauthPGXStore) CountClients(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM oauth_clients`).Scan(&n)
	return n, err
}

func (s *oauthPGXStore) GetClientByClientID(ctx context.Context, clientID string) (*OAuthClient, error) {
	var c OAuthClient
	err := s.db.QueryRow(ctx, `
		SELECT id, client_id, client_name, redirect_uris, grant_types, scopes, token_endpoint_auth_method, created_at
		FROM oauth_clients WHERE client_id = $1
	`, clientID).Scan(&c.ID, &c.ClientID, &c.ClientName, &c.RedirectURIs, &c.GrantTypes, &c.Scopes, &c.TokenEndpointAuthMethod, &c.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s *oauthPGXStore) CreateAuthorizationCode(ctx context.Context, codeHash string, c OAuthAuthorizationCode) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_authorization_codes
		    (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, codeHash, c.ClientID, c.UserID, c.RedirectURI, c.Scope, c.CodeChallenge, c.CodeChallengeMethod, c.ExpiresAt)

	return err
}

func (s *oauthPGXStore) ConsumeAuthorizationCode(ctx context.Context, codeHash, clientID string) (*OAuthAuthorizationCode, error) {
	var c OAuthAuthorizationCode
	err := s.db.QueryRow(ctx, `
		UPDATE oauth_authorization_codes
		SET used_at = now()
		WHERE code_hash = $1 AND client_id = $2 AND used_at IS NULL AND expires_at > now()
		RETURNING id, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, expires_at
	`, codeHash, clientID).Scan(&c.ID, &c.ClientID, &c.UserID, &c.RedirectURI, &c.Scope, &c.CodeChallenge, &c.CodeChallengeMethod, &c.ExpiresAt)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s *oauthPGXStore) CreateTokenPair(ctx context.Context, accessHash string, access OAuthToken, refreshHash string, refresh OAuthToken) (pgtype.UUID, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.Exec(ctx, `
		INSERT INTO oauth_tokens (token_hash, client_id, user_id, scope, token_type, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, accessHash, access.ClientID, access.UserID, access.Scope, "access", access.ExpiresAt); err != nil {
		return pgtype.UUID{}, err
	}

	var refreshID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO oauth_tokens (token_hash, client_id, user_id, scope, token_type, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, refreshHash, refresh.ClientID, refresh.UserID, refresh.Scope, "refresh", refresh.ExpiresAt).Scan(&refreshID); err != nil {
		return pgtype.UUID{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, err
	}

	return refreshID, nil
}

func (s *oauthPGXStore) GetTokenByHash(ctx context.Context, tokenType, tokenHash string) (*OAuthToken, error) {
	var t OAuthToken
	var revoked pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		SELECT id, client_id, user_id, scope, token_type, expires_at, revoked_at
		FROM oauth_tokens
		WHERE token_hash = $1 AND token_type = $2
	`, tokenHash, tokenType).Scan(&t.ID, &t.ClientID, &t.UserID, &t.Scope, &t.TokenType, &t.ExpiresAt, &revoked)
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		t.RevokedAt = &revoked
	}

	return &t, nil
}

func (s *oauthPGXStore) ConsumeRefreshToken(ctx context.Context, tokenHash string) (*OAuthToken, error) {
	var t OAuthToken
	var revoked pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		UPDATE oauth_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND token_type = 'refresh' AND revoked_at IS NULL
		RETURNING id, client_id, user_id, scope, token_type, expires_at, revoked_at
	`, tokenHash).Scan(&t.ID, &t.ClientID, &t.UserID, &t.Scope, &t.TokenType, &t.ExpiresAt, &revoked)
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		t.RevokedAt = &revoked
	}

	return &t, nil
}

func (s *oauthPGXStore) RevokeTokenByHash(ctx context.Context, tokenHash string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE oauth_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)

	return err
}

func (s *oauthPGXStore) RevokeTokenFamily(ctx context.Context, clientID string, userID pgtype.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE oauth_tokens SET revoked_at = now()
		WHERE client_id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, clientID, userID)

	return err
}

func (s *oauthPGXStore) ListUserGrants(ctx context.Context, userID pgtype.UUID) ([]OAuthGrant, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.client_id, c.client_name,
		       array_agg(t.scope) AS scope_rows,
		       min(t.created_at) AS first_granted_at,
		       max(t.created_at) AS last_granted_at
		FROM oauth_tokens t
		JOIN oauth_clients c ON c.client_id = t.client_id
		WHERE t.user_id = $1 AND t.revoked_at IS NULL AND t.expires_at > now()
		GROUP BY t.client_id, c.client_name
		ORDER BY last_granted_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OAuthGrant
	for rows.Next() {
		var g OAuthGrant
		var scopeRows []string
		if err := rows.Scan(&g.ClientID, &g.ClientName, &scopeRows, &g.FirstGrantedAt, &g.LastGrantedAt); err != nil {
			return nil, err
		}
		// Each token row carries a space-separated scope string; the grant's
		// scope set is the deduped union across the family's rows.
		set := map[string]struct{}{}
		for _, row := range scopeRows {
			for _, sc := range strings.Fields(row) {
				set[sc] = struct{}{}
			}
		}
		for sc := range set {
			g.Scopes = append(g.Scopes, sc)
		}
		sort.Strings(g.Scopes)
		out = append(out, g)
	}

	return out, rows.Err()
}

func (s *oauthPGXStore) MarkTokenReplaced(ctx context.Context, oldID, newID pgtype.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE oauth_tokens SET replaced_by = $2 WHERE id = $1
	`, oldID, newID)

	return err
}

func (s *oauthPGXStore) AuditOAuthEvent(ctx context.Context, action string, actorUserID pgtype.UUID, metadata map[string]any, ip string) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte("{}")
	}

	var actor any
	if actorUserID.Valid {
		actor = actorUserID
	}
	var ipArg any
	if ip != "" {
		ipArg = ip
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
		VALUES ($1, $2, 'oauth', $3::jsonb, $4::inet)
	`, actor, action, string(payload), ipArg)

	return err
}

func (s *oauthPGXStore) GetUserIdentity(ctx context.Context, userID pgtype.UUID) (*OAuthUserIdentity, error) {
	var id OAuthUserIdentity
	var emailVerifiedAt pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.username, u.email_verified_at, up.display_name, up.avatar_url
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = $1
	`, userID).Scan(&id.UserID, &id.Email, &id.Username, &emailVerifiedAt, &id.DisplayName, &id.AvatarURL)
	if err != nil {
		return nil, err
	}
	id.EmailVerified = emailVerifiedAt.Valid

	return &id, nil
}

// ---------------------------------------------------------------------------
// Service — token issuance, PKCE, rotation, and the middleware lookup path
// ---------------------------------------------------------------------------

// OAuthService wraps the store with token issuance, PKCE verification, and
// the Valkey-cached access-token lookup the MCP auth middleware uses.
type OAuthService struct {
	store    OAuthStore
	settings oauthSettingsReader
	cache    valkeycompat.Cmdable
}

// NewOAuthService creates the OAuth service. cache may be nil (lookups then
// always hit Postgres).
func NewOAuthService(store OAuthStore, settings oauthSettingsReader, cache valkeycompat.Cmdable) *OAuthService {
	return &OAuthService{store: store, settings: settings, cache: cache}
}

func newRandomToken(prefix string, bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return prefix + hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *OAuthService) accessTokenTTL(ctx context.Context) time.Duration {
	sec := 3600
	if s.settings != nil {
		sec = s.settings.GetIntOr(ctx, "mcp.oauth_access_token_ttl_seconds", 3600)
	}
	if sec <= 0 {
		sec = 3600
	}

	return time.Duration(sec) * time.Second
}

func (s *OAuthService) refreshTokenTTL(ctx context.Context) time.Duration {
	sec := 2592000
	if s.settings != nil {
		sec = s.settings.GetIntOr(ctx, "mcp.oauth_refresh_token_ttl_seconds", 2592000)
	}
	if sec <= 0 {
		sec = 2592000
	}

	return time.Duration(sec) * time.Second
}

func (s *OAuthService) tokenCacheTTL(ctx context.Context) time.Duration {
	sec := 60
	if s.settings != nil {
		sec = s.settings.GetIntOr(ctx, "mcp.oauth_token_cache_ttl_seconds", 60)
	}
	if sec <= 0 {
		sec = 60
	}

	return time.Duration(sec) * time.Second
}

// reuseGraceWindow bounds how recently a refresh token must have been
// rotated for a second presentation to count as a benign lost-response retry
// rather than token-reuse compromise. Covers the concurrent-refresh race and
// clients that retry after the rotation response never arrived.
func (s *OAuthService) reuseGraceWindow(ctx context.Context) time.Duration {
	sec := 30
	if s.settings != nil {
		sec = s.settings.GetIntOr(ctx, "mcp.oauth_reuse_grace_seconds", 30)
	}
	if sec < 0 {
		sec = 0
	}

	return time.Duration(sec) * time.Second
}

// maxClients bounds oauth_clients growth — RFC 7591 registration is public
// and rate-limited per-IP, so a hard global cap plus the idle-client sweep
// keeps distributed registration spam from growing the table (§120).
// Returns <= 0 when the cap is disabled (mcp.oauth_max_clients <= 0).
func (s *OAuthService) maxClients(ctx context.Context) int64 {
	n := int64(500)
	if s.settings != nil {
		n = int64(s.settings.GetIntOr(ctx, "mcp.oauth_max_clients", 500))
	}
	return n
}

// famrevCacheKey is the tombstone written when a client/user token family is
// revoked on refresh-token reuse. Cached access-token lookups check it so
// family revocation cannot be outlived by a cached entry.
func famrevCacheKey(clientID string, userID pgtype.UUID) string {
	return valkey.MustKey("oauth_famrev:%s:%s", clientID, userID.String())
}

// oauthCacheEntry is the JSON payload cached at jc:oauth_atok:{hash}.
type oauthCacheEntry struct {
	UserID   string   `json:"user_id"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
	Expires  int64    `json:"exp"`
}

// ResolveAccessToken validates an opaque OAuth access token and returns the
// synthetic key context the MCP middleware treats as an API-key caller.
// Fail-closed on every ambiguity.
func (s *OAuthService) ResolveAccessToken(ctx context.Context, token string) (userID pgtype.UUID, scopes []string, clientID string, err error) {
	hash := hashToken(token)
	cacheKey := valkey.MustKey("oauth_atok:%s", hash)

	// Cache hit: a "revoked" tombstone or a full entry. Fail closed when the
	// entry is malformed.
	if s.cache != nil {
		if cached, cerr := s.cache.Get(ctx, cacheKey).Result(); cerr == nil {
			if cached == "revoked" {
				return pgtype.UUID{}, nil, "", errors.New("token revoked")
			}
			var e oauthCacheEntry
			if json.Unmarshal([]byte(cached), &e) == nil && e.Expires > time.Now().Unix() {
				var uid pgtype.UUID
				if uerr := uid.Scan(e.UserID); uerr != nil {
					return pgtype.UUID{}, nil, "", errors.New("invalid cached user id")
				}
				// A refresh-token reuse event revokes the whole family; the
				// famrev tombstone keeps cached access tokens from outliving it.
				if n, xerr := s.cache.Exists(ctx, famrevCacheKey(e.ClientID, uid)).Result(); xerr == nil && n > 0 {
					return pgtype.UUID{}, nil, "", errors.New("token family revoked")
				}
				return uid, e.Scopes, e.ClientID, nil
			}
		}
	}

	tok, err := s.store.GetTokenByHash(ctx, "access", hash)
	if err != nil {
		return pgtype.UUID{}, nil, "", errors.New("invalid token")
	}
	if tok.RevokedAt != nil {
		return pgtype.UUID{}, nil, "", errors.New("token revoked")
	}
	if !tok.ExpiresAt.Valid || tok.ExpiresAt.Time.Before(time.Now()) {
		return pgtype.UUID{}, nil, "", errors.New("token expired")
	}

	// Populate the cache — bounded by the shorter of the cache TTL and the
	// token's remaining life so an expiring token is never served stale.
	if s.cache != nil {
		ttl := s.tokenCacheTTL(ctx)
		if rem := time.Until(tok.ExpiresAt.Time); rem < ttl {
			ttl = rem
		}
		if ttl > 0 {
			entry, _ := json.Marshal(oauthCacheEntry{
				UserID:   tok.UserID.String(),
				ClientID: tok.ClientID,
				Scopes:   strings.Fields(tok.Scope),
				Expires:  tok.ExpiresAt.Time.Unix(),
			})
			_ = s.cache.Set(ctx, cacheKey, entry, ttl).Err()
		}
	}

	return tok.UserID, strings.Fields(tok.Scope), tok.ClientID, nil
}

// RevokeToken invalidates a bearer token (RFC 7009) and tombstones its cache
// entry so revocation takes effect within the cache window.
func (s *OAuthService) RevokeToken(ctx context.Context, token string) error {
	hash := hashToken(token)

	if err := s.store.RevokeTokenByHash(ctx, hash); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, valkey.MustKey("oauth_atok:%s", hash), "revoked", s.tokenCacheTTL(ctx)).Err()
	}

	return nil
}

// revokeTokenFamily revokes a client/user token family and drops the famrev
// tombstone so cached access tokens die within the cache window. Callers
// write the audit row.
func (s *OAuthService) revokeTokenFamily(ctx context.Context, clientID string, userID pgtype.UUID) {
	_ = s.store.RevokeTokenFamily(ctx, clientID, userID)
	if s.cache != nil {
		// Tombstone outlives the access-token cache window.
		_ = s.cache.Set(ctx, famrevCacheKey(clientID, userID), "1", s.tokenCacheTTL(ctx)).Err()
	}
}

// ListGrants returns the caller's connected-app rows (§120).
func (s *OAuthService) ListGrants(ctx context.Context, userID pgtype.UUID) ([]OAuthGrant, error) {
	return s.store.ListUserGrants(ctx, userID)
}

// RevokeUserGrant revokes every token a client holds for the user — the
// connected-apps "disconnect" path (§120). Returns the store error so the
// handler can distinguish a failure from an idempotent no-op.
func (s *OAuthService) RevokeUserGrant(ctx context.Context, clientID string, userID pgtype.UUID) error {
	if err := s.store.RevokeTokenFamily(ctx, clientID, userID); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, famrevCacheKey(clientID, userID), "1", s.tokenCacheTTL(ctx)).Err()
	}

	return nil
}

// verifyPKCE checks a code_verifier against a stored S256 challenge.
// Constant-time compare — the verifier is the only proof of possession.
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))

	return hmac.Equal([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge))
}

// tokenResponse is the RFC 6749 token-endpoint response body. userID is
// unexported — carried for audit logging, never serialized.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`

	userID pgtype.UUID
}

// issueTokenPair creates a new access + refresh token pair for a grant and
// returns the response plus the new refresh token's row ID (for the
// rotation chain's replaced_by link).
func (s *OAuthService) issueTokenPair(ctx context.Context, clientID string, userID pgtype.UUID, scope string) (*tokenResponse, pgtype.UUID, error) {
	access, err := newRandomToken(oauthAccessTokenPrefix, 32)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	refresh, err := newRandomToken(oauthRefreshTokenPrefix, 32)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}

	accessTTL := s.accessTokenTTL(ctx)
	now := time.Now()

	refreshID, err := s.store.CreateTokenPair(ctx,
		hashToken(access), OAuthToken{
			ClientID:  clientID,
			UserID:    userID,
			Scope:     scope,
			ExpiresAt: pgtype.Timestamptz{Time: now.Add(accessTTL), Valid: true},
		},
		hashToken(refresh), OAuthToken{
			ClientID:  clientID,
			UserID:    userID,
			Scope:     scope,
			ExpiresAt: pgtype.Timestamptz{Time: now.Add(s.refreshTokenTTL(ctx)), Valid: true},
		})
	if err != nil {
		return nil, pgtype.UUID{}, err
	}

	return &tokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int64(accessTTL.Seconds()),
		RefreshToken: refresh,
		Scope:        scope,
		userID:       userID,
	}, refreshID, nil
}

var errRefreshReuse = errors.New("refresh token reuse detected")

// ExchangeAuthorizationCode consumes an authorization code, verifies the
// PKCE verifier, and issues a token pair.
func (s *OAuthService) ExchangeAuthorizationCode(ctx context.Context, code, clientID, redirectURI, codeVerifier string) (*tokenResponse, error) {
	if !strings.HasPrefix(code, oauthAuthCodePrefix) {
		return nil, errors.New("invalid grant")
	}

	grant, err := s.store.ConsumeAuthorizationCode(ctx, hashToken(code), clientID)
	if err != nil {
		return nil, errors.New("invalid grant")
	}
	if grant.RedirectURI != redirectURI {
		return nil, errors.New("invalid grant")
	}
	if !verifyPKCE(codeVerifier, grant.CodeChallenge) {
		return nil, errors.New("invalid grant")
	}

	resp, _, err := s.issueTokenPair(ctx, grant.ClientID, grant.UserID, grant.Scope)
	return resp, err
}

// RotateRefreshToken exchanges a refresh token for a new pair. The consume is
// atomic — concurrent rotations of the same token cannot both succeed. Reuse
// of a revoked token revokes the whole client/user token family (RFC 6819
// §5.2.2.3) and drops a famrev tombstone so cached access tokens die too.
//
// The client binding is checked BEFORE the consume: presenting another
// client's live refresh token must neither burn the victim's token nor
// reach the reuse-detection path (the token is simply rejected).
func (s *OAuthService) RotateRefreshToken(ctx context.Context, refreshToken, clientID string) (*tokenResponse, error) {
	if !strings.HasPrefix(refreshToken, oauthRefreshTokenPrefix) {
		return nil, errors.New("invalid grant")
	}

	hash := hashToken(refreshToken)

	prev, lerr := s.store.GetTokenByHash(ctx, "refresh", hash)
	if lerr != nil {
		return nil, errors.New("invalid grant")
	}
	if prev.ClientID != clientID {
		return nil, errors.New("invalid grant")
	}
	if prev.RevokedAt != nil {
		// Distinguish "already rotated" from unknown misses. Within the grace
		// window a second presentation is most likely a lost-response retry
		// (or a concurrent rotation) — plain invalid_grant, no family revoke.
		// Older revocations replayed later are treated as compromise.
		if time.Since(prev.RevokedAt.Time) <= s.reuseGraceWindow(ctx) {
			return nil, errors.New("invalid grant")
		}
		s.revokeTokenFamily(ctx, prev.ClientID, prev.UserID)
		_ = s.store.AuditOAuthEvent(ctx, "oauth.token_family_revoked", prev.UserID, map[string]any{
			"client_id": prev.ClientID,
			"reason":    "refresh_token_reuse",
		}, "")
		return nil, errRefreshReuse
	}
	if !prev.ExpiresAt.Valid || prev.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("invalid grant")
	}

	tok, err := s.store.ConsumeRefreshToken(ctx, hash)
	if err != nil {
		// Lost the race to a concurrent rotation — the token went from live
		// to revoked between our read and consume. Re-read once to catch the
		// boundary and treat it as a grace-window retry.
		return nil, errors.New("invalid grant")
	}

	pair, newRefreshID, err := s.issueTokenPair(ctx, tok.ClientID, tok.UserID, tok.Scope)
	if err != nil {
		return nil, err
	}

	_ = s.store.MarkTokenReplaced(ctx, tok.ID, newRefreshID)

	return pair, nil
}
