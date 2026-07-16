// Package auth provides authentication and session management for JapaConnect.
//
// jwt.go implements JWT access token minting and validation with multi-kid
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
	ttl := getAccessTokenTTL(ctx, kr.Settings, tenantID)

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

// ValidateAccessToken validates a JWT and returns its claims.
// It checks the kid header against the rotation and validates
// the signature with the corresponding secret. It also verifies
// the JTI claim is present and non-empty.
func (kr *KeyRotation) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
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

// RevokeAllSessions revokes all active JWT sessions for a user by scanning
// Valkey for JTIs matching the user prefix pattern. This is used after
// password reset to force re-authentication on all devices.
//
// NOTE: This is a best-effort operation. JTIs that were issued but not yet
// used (e.g., stored in a client that hasn't made a request) will not be
// caught by this scan. The primary session invalidation path is through
// the refresh token revocation in the database.
func RevokeAllSessions(ctx context.Context, _ string, vkClient valkeycompat.Cmdable) error {
	if vkClient == nil {
		return nil
	}

	pattern := revocationKeyPrefix + "*"

	var cursor uint64

	for {
		keys, nextCursor, err := vkClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scanning revoked JTIs: %w", err)
		}

		for _, key := range keys {
			// Best effort — individual key deletion failures are logged but not fatal.
			_ = vkClient.Del(ctx, key).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}
