// did:plc provisioning + migration service core (§127 A6-2).
// Three entry points share the op machinery in plc.go:
//   - provisionNewPLC — opt-in path: generate the rotation keypair,
//     submit a genesis op, return the derived DID for the account row.
//   - MigratePLC      — worker/admin path: the in-place did:web→did:plc
//     swap (account.did + record URIs, one tx under the repo lock).
//   - plcUpdateHandle — syncHandle arm for did:plc accounts: submit an
//     update op before the local handle write so the directory never
//     trails our own #identity.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md
package atproto

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	jdb "github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// provisionNewPLC provisions a did:plc for a member opting in: rotation
// keypair → signed genesis op → directory submit → derived DID. On
// directory failure the rotation ciphertext still returns — storing it
// on the did:web fallback row makes the atproto:plc_migrate retry
// deterministic (same keys → same op → same DID).
func (s *Service) provisionNewPLC(ctx context.Context, handle, signingPubMB string) (did string, rotationKeyEnc string, err error) {
	rotPrivHex, rotPubMB, err := generateSigningKey()
	if err != nil {
		return "", "", err
	}
	rotationKeyEnc, err = jcrypto.Encrypt(rotPrivHex)
	if err != nil {
		return "", "", fmt.Errorf("atproto: encrypt rotation key: %w", err)
	}
	op := newPLCOperation(rotPubMB, signingPubMB, handle, s.cfg.pdsURL(), nil)
	if err := signPLCOp(op, rotPrivHex); err != nil {
		return "", rotationKeyEnc, err
	}
	did, err = op.did()
	if err != nil {
		return "", rotationKeyEnc, err
	}
	if err := s.submitPLCOp(ctx, did, op); err != nil {
		return "", rotationKeyEnc, err
	}
	return did, rotationKeyEnc, nil
}

// MigratePLC runs the atproto:plc_migrate task core — the in-place
// did:web→did:plc swap for one account. Idempotent end-to-end: already-
// did:plc rows no-op, a re-run after a crash derives the same op
// (rotation key + handle are stable) and the directory-registered check
// skips the resubmit. Signing key, MST, blocks, blobs and commit
// history are untouched — the repo is the same identity, renamed.
func (s *Service) MigratePLC(ctx context.Context, accountID pgtype.UUID) error {
	if !s.featureOn(ctx) {
		return permanent(ErrNotFound)
	}
	acct, err := s.store.q.GetAtprotoAccountPLCMaterial(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return permanent(ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("atproto: plc material: %w", err)
	}
	if strings.HasPrefix(acct.Did, plcDIDPrefix) {
		return nil // already migrated — the task is idempotent
	}
	if acct.Status == AccountStatusTombstone {
		return permanent(ErrNotFound)
	}

	// The op's alsoKnownAs carries the CURRENT desired handle — the
	// stored handle can lag a rename; converge it before the swap so the
	// genesis advertises the live identity (the did:web doc keeps serving
	// under previous_did and reflects the same handle on its next read).
	handle := acct.Handle
	if want, ok := s.desiredHandle(acct.UserID, acct.LiveUsername); ok && !strings.EqualFold(acct.Handle, want) {
		err := s.store.setAccountHandle(ctx, accountID, want)
		if err != nil {
			s.releaseStaleClaim(ctx, want, accountID)
			err = s.store.setAccountHandle(ctx, accountID, want)
		}
		if err == nil {
			handle = want
		} else {
			slog.Warn("atproto: pre-migration handle sync failed; op carries stored handle",
				"account_id", accountID.String(), "handle", acct.Handle)
		}
	}

	// Rotation key custody — first-write-wins; a racer's key must never
	// be clobbered once any op might already reference it. Re-read after
	// the conditional write so the op signs with the row's key.
	var rotEnc string
	if acct.PlcRotationKeyEnc != nil {
		rotEnc = *acct.PlcRotationKeyEnc
	} else {
		privHex, _, err := generateSigningKey()
		if err != nil {
			return err
		}
		enc, err := jcrypto.Encrypt(privHex)
		if err != nil {
			return fmt.Errorf("atproto: encrypt rotation key: %w", err)
		}
		if _, err := s.store.q.SetAtprotoAccountRotationKey(ctx, gen.SetAtprotoAccountRotationKeyParams{
			ID:                accountID,
			PlcRotationKeyEnc: &enc,
		}); err != nil {
			return fmt.Errorf("atproto: set rotation key: %w", err)
		}
		rotEnc = enc
	}
	rotPrivHex, rotPubMB, err := rotationKeyMaterial(rotEnc)
	if err != nil {
		return err
	}

	op := newPLCOperation(rotPubMB, acct.PublicKeyMultibase, handle, s.cfg.pdsURL(), nil)
	if err := signPLCOp(op, rotPrivHex); err != nil {
		return err
	}
	newDID, err := op.did()
	if err != nil {
		return err
	}

	// Crash-safe submit: a registered DID means an earlier attempt
	// already landed the op — skip straight to the swap.
	registered, err := s.plcDIDRegistered(ctx, newDID)
	if err != nil {
		return err
	}
	if !registered {
		if err := s.submitPLCOp(ctx, newDID, op); err != nil {
			if errors.Is(err, ErrPLCRejected) {
				return permanent(err)
			}
			return err
		}
	}

	oldDID := acct.Did
	// The swap and the record-URI rewrite share one tx under the same
	// account lock repo writes take — a racing commit either lands fully
	// before (its index rows are rewritten here) or fully after (it
	// writes new-DID URIs). CAS on the expected DID guards a double run.
	if err := jdb.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if _, err := q.LockAtprotoAccountForRepo(ctx, accountID); err != nil {
			return fmt.Errorf("atproto: lock account: %w", err)
		}
		n, err := q.SwapAtprotoAccountDID(ctx, gen.SwapAtprotoAccountDIDParams{
			ID:     accountID,
			NewDid: newDID,
			OldDid: &oldDID,
		})
		if err != nil {
			return fmt.Errorf("atproto: did swap: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("atproto: did swap raced — %s no longer current", oldDID)
		}
		if _, err := q.RewriteAtprotoRecordDIDPrefix(ctx, gen.RewriteAtprotoRecordDIDPrefixParams{
			AccountID: accountID,
			OldDid:    &oldDID,
			NewDid:    &newDID,
		}); err != nil {
			return fmt.Errorf("atproto: record uri rewrite: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	s.invalidateAccount(ctx, handle, uuidString(acct.UserID))
	s.invalidateAccount(ctx, acct.Handle, uuidString(acct.UserID))
	// §127: #identity announces the new DID; requestCrawl points relays
	// at it. The old DID goes silent — its did:web doc keeps resolving
	// via previous_did for embedded provenance.
	s.emitIdentityEvent(ctx, newDID, handle)
	worker.EnqueueAtprotoRequestCrawl(ctx, accountID)
	slog.Info("atproto account migrated to did:plc",
		"account_id", accountID.String(), "old_did", oldDID, "did", newDID)
	return nil
}

// plcUpdateHandle submits the directory update op a handle change on a
// did:plc account requires — alsoKnownAs lives in the signed op log, so
// it can only change under rotation-key signature. The op chains onto
// the audit-log head; a directory failure leaves the stored handle in
// place (doc + #identity stay consistent; the next lazy sync retries).
func (s *Service) plcUpdateHandle(ctx context.Context, accountID pgtype.UUID, newHandle string) error {
	acct, err := s.store.q.GetAtprotoAccountPLCMaterial(ctx, accountID)
	if err != nil {
		return fmt.Errorf("atproto: plc material: %w", err)
	}
	if acct.PlcRotationKeyEnc == nil {
		return fmt.Errorf("atproto: did:plc account %s has no rotation key", accountID)
	}
	head, err := s.plcAuditHead(ctx, acct.Did)
	if err != nil {
		return err
	}
	rotPrivHex, rotPubMB, err := rotationKeyMaterial(*acct.PlcRotationKeyEnc)
	if err != nil {
		return err
	}
	op := newPLCOperation(rotPubMB, acct.PublicKeyMultibase, newHandle, s.cfg.pdsURL(), &head)
	if err := signPLCOp(op, rotPrivHex); err != nil {
		return err
	}
	return s.submitPLCOp(ctx, acct.Did, op)
}

// desiredHandle computes the handle a member's live username should
// hold — the {username}.{domain} form when it's claimable-legal, the
// permanent u-{uuid} fallback when it isn't (the same rule syncHandle
// applies). ok=false means no sync can run (missing username/domain).
func (s *Service) desiredHandle(userID pgtype.UUID, liveUsername string) (want string, ok bool) {
	if liveUsername == "" || s.cfg.handleDomain() == "" {
		return "", false
	}
	want = s.cfg.handleFor(liveUsername)
	// §50 admits '_' — a rename into a handle-illegal name can't mint a
	// verifiable {username}.{domain}; fall back to the permanent
	// uuid-derived handle (see syncHandle for the full rationale).
	if !s.validHandle(liveUsername, want) {
		want = s.cfg.handleFor("u-" + uuidString(userID))
	}
	return want, true
}
