package atproto

// Lifecycle sync + GDPR erasure (§127 lifecycle, ADR-139): platform
// user status is canonical — the atproto account follows it. Every
// users.status write enqueues atproto:lifecycle_sync, so this file must
// converge idempotently under retries and races: a freeze emits
// #account{active:false} exactly once, a member's own opt-out is never
// silently reactivated by moderation cycles, and tombstone is terminal.
//
// Erasure runs BEFORE the user row's physical delete — retraction
// commits are signed while the key custody row still exists, which is
// precisely what 001143's cascade warning protects.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Lifecycle)

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	indmst "github.com/bluesky-social/indigo/mst"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// SyncUserLifecycle reconciles one member's atproto account with their
// live users.status — the atproto:lifecycle_sync task's entry point.
// Idempotent: repeat enqueues and out-of-order delivery both converge.
// The bool reports whether the member is 'active' at run time — the
// worker handler uses it to gate the §126 auto-opt-in re-evaluation,
// so transitions to suspended/banned/deleted never produce the task.
func (s *Service) SyncUserLifecycle(ctx context.Context, userID pgtype.UUID) (bool, error) {
	ustatus, err := s.store.q.GetUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// User row gone — only GDPR's hard delete removes it, and that
		// path tombstones first; a racing lifecycle read treats the
		// absence as deleted rather than resurrecting a ghost.
		ustatus = "deleted"
	} else if err != nil {
		return false, fmt.Errorf("atproto: lifecycle user status: %w", err)
	}
	userActive := ustatus == "active"

	account, err := s.store.q.GetAtprotoAccountAnyUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return userActive, nil // never opted in — nothing to converge
	}
	if err != nil {
		return userActive, fmt.Errorf("atproto: lifecycle account: %w", err)
	}
	if account.Status == AccountStatusTombstone {
		return userActive, nil // terminal — nothing downgrades or resurrects it
	}
	if userActive {
		return true, s.reactivateFromLifecycle(ctx, account)
	}
	return false, s.freezeFromLifecycle(ctx, account, ustatus)
}

// lifecycleAccountStatusWord maps a users.status to the #account
// frame's status enum — "suspended", "takendown", or "deleted".
func lifecycleAccountStatusWord(ustatus string) string {
	switch ustatus {
	case "banned":
		return "takendown"
	case "deleted":
		return "deleted"
	default:
		// suspended/restricted/pending and any future status.
		return "suspended"
	}
}

// freezeFromLifecycle disables an active account because the platform
// user left 'active' — disabled_reason 'lifecycle' marks the origin so
// reactivation knows it may undo it. Emits #account{active:false} once:
// an already-disabled account (either reason) is left untouched.
func (s *Service) freezeFromLifecycle(ctx context.Context, account gen.AtprotoAccount, ustatus string) error {
	if account.Status != AccountStatusActive {
		return nil
	}
	reason := DisableReasonLifecycle
	if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusDisabled, &reason); err != nil {
		return fmt.Errorf("atproto: lifecycle freeze: %w", err)
	}
	s.emitAccountEvent(ctx, account.Did, false, lifecycleAccountStatusWord(ustatus))
	s.invalidateAccount(ctx, account.Handle, uuidString(account.UserID))
	slog.Info("atproto account lifecycle-frozen",
		"did", account.Did, "user_status", ustatus)
	return nil
}

// reactivateFromLifecycle restores ONLY lifecycle-frozen accounts when
// the user returns to 'active'. A member-initiated disable — explicit
// 'member' or a historical NULL reason — survives: moderation must not
// silently re-enable an opt-out the member chose.
func (s *Service) reactivateFromLifecycle(ctx context.Context, account gen.AtprotoAccount) error {
	if account.Status != AccountStatusDisabled {
		return nil // active already, or tombstone — both terminal here
	}
	if account.DisabledReason == nil || *account.DisabledReason != DisableReasonLifecycle {
		return nil
	}
	if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusActive, nil); err != nil {
		return fmt.Errorf("atproto: lifecycle reactivate: %w", err)
	}
	s.emitAccountEvent(ctx, account.Did, true, "")
	s.invalidateAccount(ctx, account.Handle, uuidString(account.UserID))
	worker.EnqueueAtprotoRequestCrawl(ctx, account.ID)
	slog.Info("atproto account lifecycle-reactivated", "did", account.Did)
	return nil
}

// EraseAccount is the GDPR erasure path — called by the gdpr service
// before the user row's physical delete, while key custody still
// exists: every indexed record retracts in one signed commit (the
// #commit frame carries the delete ops, so the network sees the
// content leave), then the account tombstones and #account{deleted}
// announces the end-state. Erasure-mode repo writes are what make this
// reachable: the lifecycle freeze may already have disabled the
// account, and only tombstone is past signing.
func (s *Service) EraseAccount(ctx context.Context, userID pgtype.UUID) error {
	account, err := s.store.q.GetAtprotoAccountAnyUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // never opted in
	}
	if err != nil {
		return fmt.Errorf("atproto: erase account lookup: %w", err)
	}
	if account.Status == AccountStatusTombstone {
		return nil
	}

	err = s.withRepoWrite(ctx, account.ID, repoWriteErasure, func(rep *atrepo.Repo, q *gen.Queries) ([]indexWrite, error) {
		recs, err := q.ListAtprotoRecords(ctx, account.ID)
		if err != nil {
			return nil, fmt.Errorf("atproto: erase records list: %w", err)
		}
		writes := make([]indexWrite, 0, len(recs))
		for _, rec := range recs {
			rpath := rec.Collection + "/" + rec.Rkey
			if derr := rep.DeleteRecord(ctx, rpath); derr != nil && !errors.Is(derr, indmst.ErrNotFound) {
				return nil, fmt.Errorf("atproto: erase %s: %w", rpath, derr)
			}
			// MST-absent but index-present rows still unindex — that
			// repairs drift rather than leaving a ghost pointer.
			writes = append(writes, indexWrite{del: true, collection: rec.Collection, rkey: rec.Rkey})
		}
		if len(writes) == 0 {
			return nil, errRepoNoop
		}
		return writes, nil
	})
	if err != nil && !errors.Is(err, errRepoNoop) {
		return fmt.Errorf("atproto: erase records: %w", err)
	}

	// A4/GDPR: blob bytes are member media — they must not outlive the
	// tombstone. Purge before the status flip so a failure stays retryable
	// (a tombstoned account early-returns above).
	if _, err := s.store.q.DeleteAtprotoBlobsForAccount(ctx, account.ID); err != nil {
		return fmt.Errorf("atproto: erase blobs: %w", err)
	}

	if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusTombstone, nil); err != nil {
		return fmt.Errorf("atproto: tombstone: %w", err)
	}
	s.emitAccountEvent(ctx, account.Did, false, "deleted")
	s.invalidateAccount(ctx, account.Handle, uuidString(account.UserID))
	slog.Info("atproto account erased", "did", account.Did)
	return nil
}
