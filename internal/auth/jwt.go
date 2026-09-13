// Package auth provides authentication and session management for JapaConnect.
// // jwt.go implements JWT access token minting and validation with multi-kid
// rotation support. Multiple signing secrets can be active simultaneously,
// identified by a kid (key ID) header. This allows zero-disruption secret
// rotation: a new secret is added, old tokens validate against the old kid,
// and after one access token TTL (15 minutes) the old secret is retired.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// AccessTokenTTL is the validity period for access tokens.
// Short lifetime limits the damage window if a token is leaked.
// Now read from system_settings 'auth.session.access_token_ttl_minutes' with
// a 15-minute fallback. See settings.go for the implementation.
const AccessTokenTTL = 15 * time.Minute

// JWTClaims defines the structure of our access token claims.
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	JTI      string `json:"jti"`
	Scope    string `json:"scope,omitempty"` // "totp_verify" for challenge tokens
}

// KeyRotation holds the active signing keys for JWT rotation.
// CurrentKeyID is used to sign new tokens. All KeyIDs in Keys
// are accepted for validation, allowing overlap during rotation.
type KeyRotation struct {
	CurrentKeyID string
	Keys         map[string][]byte // kid -> secret
	Settings     SettingsReader    // Hot-reloadable system settings for access token TTL
}

// NewKeyRotation creates a KeyRotation with a single key.
func NewKeyRotation(kid string, secret []byte) *KeyRotation {
	return &KeyRotation{
		CurrentKeyID: kid,
		Keys:         map[string][]byte{kid: secret},
	}
}

// AddKey adds a new signing key. Tokens signed with any key in the
// rotation are still valid. Use this during rotation: add the new key,
// set it as current, then after one TTL remove the old key.
func (kr *KeyRotation) AddKey(kid string, secret []byte) {
	kr.Keys[kid] = secret
}

// RemoveKey removes a signing key. Tokens signed with this kid will
// no longer validate. Only call this after all tokens using this kid
// have expired (wait at least AccessTokenTTL after stopping its use).
func (kr *KeyRotation) RemoveKey(kid string) {
	delete(kr.Keys, kid)
}

// LoadFromDB loads all active signing keys from the database and populates
// the in-memory rotation map. The most recently created key becomes the
// current signing key. If the database has active keys, they replace any
// keys already in the rotation. Ref: §02 "jwt_signing_keys table".
func (kr *KeyRotation) LoadFromDB(ctx context.Context, store *JWTKeyStore) error {
	keys, err := store.GetActiveKeys(ctx)
	if err != nil {
		return fmt.Errorf("loading JWT signing keys from DB: %w", err)
	}

	if len(keys) == 0 {
		return nil // No keys in DB — caller should seed from env
	}

	// Replace in-memory keys with DB-loaded keys.
	kr.Keys = make(map[string][]byte, len(keys))
	for _, k := range keys {
		kr.Keys[k.KID] = k.Secret
	}

	// The first key (most recently created) is the current signing key.
	kr.CurrentKeyID = keys[0].KID

	return nil
}

// PersistKey encrypts and stores a signing key to the database so it
// survives server restarts. Called during bootstrap when seeding from
// the JC_JWT_SECRET env var, or during key rotation when adding a new key.
func (kr *KeyRotation) PersistKey(ctx context.Context, store *JWTKeyStore, kid string, secret []byte) error {
	if err := store.InsertKey(ctx, kid, secret); err != nil {
		return fmt.Errorf("persisting JWT signing key %s: %w", kid, err)
	}

	kr.AddKey(kid, secret)

	return nil
}

// SetCurrent sets the key ID used to sign new tokens.
func (kr *KeyRotation) SetCurrent(kid string) error {
	if _, ok := kr.Keys[kid]; !ok {
		return fmt.Errorf("key ID %s not in rotation", kid)
	}

	kr.CurrentKeyID = kid

	return nil
}

// MintAccessToken creates a signed JWT for the given user and tenant.
// The token expires after the access token TTL configured in system_settings
// 'auth.session.access_token_ttl_minutes' (default: 15 minutes). Each token gets a
// unique JTI (JWT ID) for individual revocation support.
func (kr *KeyRotation) MintAccessToken(userID, tenantID pgtype.UUID) (string, error) {
	return kr.MintAccessTokenWithContext(context.Background(), userID, tenantID)
}

// MintAccessTokenWithContext creates a signed JWT with explicit context for
// reading hot-reloadable settings. Use this when the caller has a request context
// available for settings lookup.
func (kr *KeyRotation) MintAccessTokenWithContext(ctx context.Context, userID, tenantID pgtype.UUID) (string, error) {
	if !userID.Valid {
		return "", errors.New("cannot mint token: user ID is not valid")
	}

	if !tenantID.Valid {
		return "", errors.New("cannot mint token: tenant ID is not valid")
	}

	jti := uuid.New().String()
	ttl := GetAccessTokenTTL(ctx, kr.Settings, tenantID)

	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
		UserID:   userID.String(),
		TenantID: tenantID.String(),
		JTI:      jti,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = kr.CurrentKeyID

	secret, ok := kr.Keys[kr.CurrentKeyID]
	if !ok {
		return "", fmt.Errorf("current key ID %s not found in rotation", kr.CurrentKeyID)
	}

	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}

	return signed, nil
}

// TOTPChallengeTTL is the lifetime of a 2FA challenge token (5 minutes).
const TOTPChallengeTTL = 5 * time.Minute

// MintTOTPChallengeToken creates a short-lived challenge token with scope "totp_verify".
// The challenge token is issued after successful password validation when TOTP is
// enabled, and must be presented to complete the 2FA step.
func (kr *KeyRotation) MintTOTPChallengeToken(userID, tenantID pgtype.UUID) (string, error) {
	if !userID.Valid || !tenantID.Valid {
		return "", errors.New("cannot mint challenge token: user or tenant ID invalid")
	}

	jti := uuid.New().String()
	now := time.Now()

	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(TOTPChallengeTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
		UserID: userID.String(),

		JTI:   jti,
		Scope: "totp_verify",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = kr.CurrentKeyID

	secret, ok := kr.Keys[kr.CurrentKeyID]
	if !ok {
		return "", fmt.Errorf("current key ID %s not found in rotation", kr.CurrentKeyID)
	}

	return token.SignedString(secret)
}

// ValidateChallengeToken validates a TOTP challenge token and returns its claims.
// It enforces that the token has scope "totp_verify".
func (kr *KeyRotation) ValidateChallengeToken(tokenString string) (*JWTClaims, error) {
	claims, err := kr.parseToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Scope != "totp_verify" {
		return nil, errors.New("token is not a TOTP challenge token")
	}

	return claims, nil
}

// ValidateAccessToken validates a full-session JWT and returns its claims.
// Scoped tokens (such as the "totp_verify" challenge issued before the 2FA
// step completes) are rejected: they only grant access to the endpoint that
// completes their flow, never to authenticated API routes.
func (kr *KeyRotation) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	claims, err := kr.parseToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Scope != "" {
		return nil, fmt.Errorf("token has restricted scope %q and is not a session token", claims.Scope)
	}

	return claims, nil
}

// parseToken validates a JWT signature and base claims without inspecting
// the scope. It checks the kid header against the rotation and validates
// the signature with the corresponding secret. It also verifies
// the JTI claim is present and non-empty.
func (kr *KeyRotation) parseToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid header in token")
		}

		secret, exists := kr.Keys[kid]
		if !exists {
			return nil, fmt.Errorf("unknown key ID: %s", kid)
		}

		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// JTI must be present and non-empty for revocation support.
	if claims.JTI == "" {
		return nil, errors.New("token missing JTI claim")
	}

	return claims, nil
}

// revocationKeyPrefix is the Valkey key prefix for revoked JTIs.
const revocationKeyPrefix = "jc:revoked_jti:"

// IsTokenRevoked checks whether a specific token (by JTI) has been revoked.
// Returns false if Valkey is unreachable (fail-open for availability).
func IsTokenRevoked(ctx context.Context, jti string, vkClient valkeycompat.Cmdable) bool {
	if vkClient == nil {
		return false
	}

	exists, err := vkClient.Exists(ctx, revocationKeyPrefix+jti).Result()
	if err != nil {
		// Valkey unreachable — fail open, don't block legitimate requests.
		return false
	}

	return exists > 0
}

// RevokeToken adds a JTI to the revoked set in Valkey with a TTL matching
// the remaining token lifetime. The key automatically expires so revoked
// JTIs don't accumulate forever.
func RevokeToken(ctx context.Context, jti string, expiresAt time.Time, vkClient valkeycompat.Cmdable) error {
	if vkClient == nil || jti == "" {
		return nil
	}

	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		// Token already expired, no need to store revocation.
		return nil
	}

	key := revocationKeyPrefix + jti

	return vkClient.Set(ctx, key, "1", ttl).Err()
}

// Note: per-user session revocation is handled by revoking refresh tokens in
// the database (RevokeAllUserRefreshTokens) plus revoking the caller's
// current access-token JTI via RevokeToken. There is no Valkey-side
// "revoke all sessions by user" scan because JTIs are not indexed by user —
// only revoked JTIs are stored (jc:revoked_jti:{jti}), and deleting those
// keys would *un-revoke* already-revoked tokens.
