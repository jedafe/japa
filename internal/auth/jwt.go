// Package auth provides authentication and session management for JapaConnect.
//
// jwt.go implements JWT access token minting and validation with multi-kid
// rotation support. Multiple signing secrets can be active simultaneously,
// identified by a kid (key ID) header. This allows zero-disruption secret
// rotation: a new secret is added, old tokens validate against the old kid,
// and after one access token TTL (15 minutes) the old secret is retired.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccessTokenTTL is the validity period for access tokens.
// Short lifetime limits the damage window if a token is leaked.
const AccessTokenTTL = 15 * time.Minute

// JWTClaims defines the structure of our access token claims.
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
}

// KeyRotation holds the active signing keys for JWT rotation.
// CurrentKeyID is used to sign new tokens. All KeyIDs in Keys
// are accepted for validation, allowing overlap during rotation.
type KeyRotation struct {
	CurrentKeyID string
	Keys         map[string][]byte // kid -> secret
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
// The token expires after AccessTokenTTL (15 minutes).
func (kr *KeyRotation) MintAccessToken(userID, tenantID pgtype.UUID) (string, error) {
	if !userID.Valid {
		return "", errors.New("cannot mint token: user ID is not valid")
	}
	if !tenantID.Valid {
		return "", errors.New("cannot mint token: tenant ID is not valid")
	}

	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID:   userID.String(),
		TenantID: tenantID.String(),
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
// the signature with the corresponding secret.
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

	return claims, nil
}
