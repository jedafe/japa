package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// consumeBackupCode checks a candidate code against stored hashed backup codes,
// removes it if matched, and persists the updated set. Returns (matched, codes_remaining, error).
func consumeBackupCode(ctx context.Context, dbConn *db.DB, cred *gen.TotpCredential, code string, userID pgtype.UUID) (bool, int, error) {
	code = sanitizeCode(code)

	stored := parseBackupHashes(cred.BackupCodes)

	target := hashBackupCode(code)

	for i, h := range stored {
		if h == target {
			stored = append(stored[:i], stored[i+1:]...)

			encoded, err := encodeBackupHashes(stored)
			if err != nil {
				return false, 0, err
			}

			_, err = dbConn.Writer.Exec(ctx, `UPDATE totp_credentials SET backup_codes = $1, updated_at = now() WHERE user_id = $2`, encoded, userID)
			if err != nil {
				return false, 0, fmt.Errorf("persisting consumed backup code: %w", err)
			}

			return true, len(stored), nil
		}
	}

	return false, 0, nil
}

// hashBackupCode hashes a raw backup code for storage.
func hashBackupCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// parseBackupHashes deserializes stored backup code hashes from JSON.
func parseBackupHashes(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}

	var codes []string
	if err := json.Unmarshal(raw, &codes); err != nil {
		return nil
	}

	return codes
}

// encodeBackupHashes serializes backup code hashes to JSON.
func encodeBackupHashes(codes []string) ([]byte, error) {
	if len(codes) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(codes)
}
