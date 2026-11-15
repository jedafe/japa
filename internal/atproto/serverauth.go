// com.atproto.server.* authed endpoints (§127 A6-3 portability).
//
// These run under the A6-4 access JWT (AuthenticateAccess) and give
// external tooling the account-migration + lifecycle surface the
// lexicon expects: service-auth tokens for the destination PDS,
// account status, reversible activate/deactivate, and the repo
// export artifact.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-3)
package atproto

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// maxServiceAuthTTL bounds getServiceAuth exp — a migration credential
// should live seconds, not hours (the destination uses it immediately).
const maxServiceAuthTTL = 5 * time.Minute

// ServiceAuth mints the ES256 JWT an external service verifies against
// the account's DID doc — signed by the REPO signing key (the
// verificationMethod every did:web doc and plc directory entry
// publishes). Claims: {iss:did, aud, lxm?, exp} — exp defaults to
// +60s and clamps to maxServiceAuthTTL either side of the default.
func (s *Service) ServiceAuth(ctx context.Context, acct *SessionAccount, aud, lxm string, exp int64) (string, error) {
	if !s.featureOn(ctx) {
		return "", ErrAuthRequired
	}
	// aud is the destination's DID — the lexicon parameter type; a
	// non-did aud mints a token no service can accept anyway.
	if !strings.HasPrefix(aud, "did:") {
		return "", ErrInvalidID
	}
	// lxm, when present, is the lexicon method the token authorizes —
	// must parse as an NSID.
	if lxm != "" {
		if _, err := syntax.ParseNSID(lxm); err != nil {
			return "", ErrInvalidID
		}
	}
	// A deactivated (disabled) account still mints service auth — the
	// standard migration flow deactivates then exports, and the
	// destination's importRepo presents this token. Tombstone never
	// reaches here: AuthenticateAccess already refused it.
	now := time.Now()
	expiry := now.Add(time.Minute)
	if exp > 0 {
		expiry = time.Unix(exp, 0)
		// A past exp is a client bug, not a clamp — answer the error
		// rather than hand back an already-dead token.
		if !expiry.After(now) {
			return "", ErrInvalidID
		}
		if expiry.After(now.Add(maxServiceAuthTTL)) {
			expiry = now.Add(maxServiceAuthTTL)
		}
	}

	row, err := s.store.q.GetAtprotoAccountByID(ctx, acct.AccountID)
	if err != nil {
		return "", fmt.Errorf("atproto: service auth account: %w", err)
	}
	privHex, err := jcrypto.Decrypt(row.SigningKeyEnc)
	if err != nil {
		return "", fmt.Errorf("atproto: service auth key: %w", err)
	}
	priv, err := parsePrivateKeyHex(privHex)
	if err != nil {
		return "", err
	}
	key, err := atcrypto.ParsePrivateBytesK256(priv.Serialize())
	if err != nil {
		return "", fmt.Errorf("atproto: service auth k256: %w", err)
	}

	header, err := b64urlJSON(map[string]string{"alg": "ES256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims := map[string]any{
		"iss": acct.DID,
		"aud": aud,
		"iat": now.Unix(),
		"exp": expiry.Unix(),
	}
	if lxm != "" {
		claims["lxm"] = lxm
	}
	payload, err := b64urlJSON(claims)
	if err != nil {
		return "", err
	}
	input := header + "." + payload
	// ES256 = raw R||S over sha256(input) — HashAndSign's exact output.
	sig, err := key.HashAndSign([]byte(input))
	if err != nil {
		return "", fmt.Errorf("atproto: service auth sign: %w", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// AccountStatusStats is checkAccountStatus's payload — activated +
// validDid plus the repo material migration tooling diffs between
// source and destination.
func (s *Service) AccountStatusStats(ctx context.Context, acct *SessionAccount) (*gen.GetAtprotoAccountStatusStatsRow, bool, error) {
	row, err := s.store.q.GetAtprotoAccountStatusStats(ctx, acct.AccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrNotFound
		}
		return nil, false, fmt.Errorf("atproto: account status stats: %w", err)
	}
	// validDid: the account's DID is structurally well-formed and this
	// PDS still serves/mints it — a did:web resolves via the doc route,
	// a did:plc via the directory (we don't round-trip the directory
	// here; a submitted op is the precondition of the did existing).
	valid := strings.HasPrefix(acct.DID, "did:plc:") || strings.HasPrefix(acct.DID, "did:web:")
	return &row, valid, nil
}

// DeactivateAccount — the XRPC arm of the member opt-out: identical
// semantics to DELETE /me/atproto (reversible 'member' disable +
// #account{false,deactivated}). deleteAfter is advisory and ignored —
// retention is PDS policy, not client instruction.
func (s *Service) DeactivateAccount(ctx context.Context, acct *SessionAccount) error {
	if acct.Status == AccountStatusTombstone {
		return ErrNotFound
	}
	_, err := s.DisableAccount(ctx, acct.UserID)
	return err
}

// ActivateAccount — re-enable through the same gates the member
// endpoint honors: admin-disabled rows refuse (ErrAdminDisabled),
// tombstones are terminal, everything else reactivates + reconciles.
func (s *Service) ActivateAccount(ctx context.Context, acct *SessionAccount) error {
	if acct.Status == AccountStatusTombstone {
		return ErrNotFound
	}
	_, err := s.EnableAccount(ctx, acct.UserID)
	return err
}

// ExportRepo streams the member's full repo CAR — the migration/
// backup artifact. Same bytes public getRepo serves for an active
// account — but it must NOT go through RepoCARSince: that path's
// availability gate answers RepoDeactivated on a disabled account,
// and the standard migration flow deactivates-then-exports. The
// member's own data is always exportable; tombstone is the only
// terminal state.
func (s *Service) ExportRepo(ctx context.Context, userID pgtype.UUID) ([]byte, string, error) {
	acct, err := s.store.q.GetAtprotoAccountAnyUserStatus(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("atproto: export account: %w", err)
	}
	if acct.Status == AccountStatusTombstone {
		return nil, "", ErrNotFound
	}
	car, err := s.RepoCAR(ctx, acct.ID)
	if err != nil {
		return nil, "", err
	}
	return car, acct.Did, nil
}

// revokeExternalCredentials kills every live session + app password —
// called by the tombstone core so erasure can't leave an external
// client holding a working credential against a deleted identity.
func (s *Service) revokeExternalCredentials(ctx context.Context, accountID pgtype.UUID) {
	if err := s.store.q.RevokeAtprotoSessionsForAccount(ctx, accountID); err != nil {
		slog.Warn("atproto: session revoke on tombstone failed", "account_id", accountID.String(), "error", err)
	}
	rows, err := s.store.q.ListAtprotoAppPasswords(ctx, accountID)
	if err != nil {
		slog.Warn("atproto: app password list on tombstone failed", "account_id", accountID.String(), "error", err)
		return
	}
	for i := range rows {
		if _, err := s.store.q.RevokeAtprotoAppPassword(ctx, gen.RevokeAtprotoAppPasswordParams{
			ID: rows[i].ID, AccountID: accountID,
		}); err != nil {
			slog.Warn("atproto: app password revoke on tombstone failed", "id", rows[i].ID.String(), "error", err)
		}
	}
}
