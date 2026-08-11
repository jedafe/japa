package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
)

// JWTKeyRow represents an active JWT signing key loaded from the database.
type JWTKeyRow struct {
	KID       string
	Secret    []byte
	CreatedAt time.Time
}

// JWTKeyStore provides DB-backed persistence for JWT signing keys.
// It uses raw SQL (same pattern as store_totp_oauth.go) rather than
// sqlc-generated code because the jwt_signing_keys table is global
// (not tenant-scoped) and managed outside the normal sqlc query set.
type JWTKeyStore struct {
	db *db.DB
}

// NewJWTKeyStore creates a JWTKeyStore backed by the given database.
func NewJWTKeyStore(database *db.DB) *JWTKeyStore {
	return &JWTKeyStore{db: database}
}

// GetActiveKeys loads all active signing keys from the database, ordered
// by created_at descending so the most recently added key is first.
// Secrets are decrypted using internal/crypto (AES-256-GCM via JC_ENCRYPTION_KEY).
func (s *JWTKeyStore) GetActiveKeys(ctx context.Context) ([]JWTKeyRow, error) {
	query := `
		SELECT kid, secret, created_at
		FROM jwt_signing_keys
		WHERE active = true
		ORDER BY created_at DESC
	`

	rows, err := s.db.Reader.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying active JWT signing keys: %w", err)
	}
	defer rows.Close()

	var keys []JWTKeyRow

	for rows.Next() {
		var kid, encryptedSecret string
		var createdAt time.Time

		if err := rows.Scan(&kid, &encryptedSecret, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning JWT signing key row: %w", err)
		}

		plaintext, err := crypto.Decrypt(encryptedSecret)
		if err != nil {
			slog.Error("decrypting JWT signing key secret", "kid", kid, "error", err)
			continue
		}

		keys = append(keys, JWTKeyRow{
			KID:       kid,
			Secret:    []byte(plaintext),
			CreatedAt: createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating JWT signing key rows: %w", err)
	}

	return keys, nil
}

// InsertKey persists a new signing key to the database. The secret is
// encrypted before storage. If a key with the same kid already exists,
// the insert is skipped (upsert semantics via ON CONFLICT).
func (s *JWTKeyStore) InsertKey(ctx context.Context, kid string, secret []byte) error {
	encrypted, err := crypto.Encrypt(string(secret))
	if err != nil {
		return fmt.Errorf("encrypting JWT signing key: %w", err)
	}

	query := `
		INSERT INTO jwt_signing_keys (kid, secret, active, created_at)
		VALUES ($1, $2, true, now())
		ON CONFLICT (kid) DO NOTHING
	`

	_, err = s.db.Writer.Exec(ctx, query, kid, encrypted)
	if err != nil {
		return fmt.Errorf("inserting JWT signing key: %w", err)
	}

	return nil
}

// RetireKey marks a signing key as inactive. Tokens signed with this kid
// will no longer validate after this call. Only call this after all tokens
// using this kid have expired (wait at least AccessTokenTTL after stopping use).
func (s *JWTKeyStore) RetireKey(ctx context.Context, kid string) error {
	query := `
		UPDATE jwt_signing_keys
		SET active = false, retired_at = now()
		WHERE kid = $1 AND active = true
	`

	_, err := s.db.Writer.Exec(ctx, query, kid)
	if err != nil {
		return fmt.Errorf("retiring JWT signing key: %w", err)
	}

	return nil
}
