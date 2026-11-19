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
	worker.EnqueueAtprotoReconcileRepo(ctx, account.ID)
	slog.Info("atproto account lifecycle-reactivated", "did", account.Did)
	return nil
}

// ReconcileRepo converges the account's indexed records to canonical
// eligibility — the atproto:reconcile_repo task path (§127 A4/
// Lifecycle). Emits skip while a repo is frozen (suspension or member
// disable), so records that went stale inside a disabled window would
// otherwise resurface on reactivation; every path to 'active' —
// reactivation, member re-enable, first-time provisioning — enqueues
// this instead. Per row: a feed.post still eligible AND still
// owned re-projects (refresh — pre-A4 records gain facets/embeds); an
// ineligible row, or one whose author moved to another repo, tombstones
// via the normal delete path; a `self` profile republishes (avatar/
// display-name drift has the same window). A non-active account at
// task time no-ops — the next reactivation re-enqueues. Per-row
// permanent failures skip (e.g. a delete landing after a concurrent
// tombstone); transient failures join so the task retries.
func (s *Service) ReconcileRepo(ctx context.Context, accountID pgtype.UUID) error {
	account, err := s.store.q.GetAtprotoAccountByID(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return permanent(ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("atproto: reconcile account: %w", err)
	}
	if account.Status != AccountStatusActive {
		return nil // frozen repos reconcile on their next activation
	}
	recs, err := s.store.q.ListAtprotoRecords(ctx, accountID)
	if err != nil {
		return fmt.Errorf("atproto: reconcile list: %w", err)
	}
	minPosts := s.minPostsDiscoverable(ctx)
	var errs error
	perRow := func(err error) {
		// Permanent per-row failures are skipped — the row already
		// converged (concurrent tombstone/rewrite). Transient ones join
		// so the task retries the remainder.
		if err != nil && !isPermanentErr(err) {
			errs = errors.Join(errs, err)
		}
	}
	profileIndexed := false
	for _, rec := range recs {
		switch rec.Collection {
		case CollectionProfile:
			profileIndexed = true
			perRow(s.publishProfile(ctx, &account))
		case CollectionPost:
			if !rec.ContentID.Valid {
				// content_id ON DELETE SET NULL — an admin hard delete
				// orphaned the attribution; a post can only exist via
				// content projection, so this copy is provably stale.
				perRow(s.DeleteRecordURI(ctx, accountID, rec.Uri))
				continue
			}
			if _, err := s.store.q.GetAtprotoProjectableContent(ctx, gen.GetAtprotoProjectableContentParams{
				ID:           rec.ContentID,
				ContentTypes: s.federatableTypesFor(ctx),
				MinPosts:     minPosts,
			}); errors.Is(err, pgx.ErrNoRows) {
				// Unpublished, private, deleted, opted-out, or a type that
				// left the allowlist — the projected copy is stale.
				perRow(s.DeleteRecordURI(ctx, accountID, rec.Uri))
			} else if err != nil {
				perRow(err)
			} else if owner, oerr := s.store.q.GetAtprotoAccountIDByContent(ctx, rec.ContentID); oerr != nil && !errors.Is(oerr, pgx.ErrNoRows) {
				perRow(oerr)
			} else if errors.Is(oerr, pgx.ErrNoRows) || owner != accountID {
				// The author moved to another repo (or left federation
				// entirely) — this repo's copy is stale regardless of
				// content eligibility.
				perRow(s.DeleteRecordURI(ctx, accountID, rec.Uri))
			} else {
				// Still eligible and still owned — re-upsert; the refresh
				// also repairs drift (pre-A4 records gain facets/embeds).
				_, perr := s.publishContent(ctx, rec.ContentID, false)
				perRow(perr)
			}
		}
	}
	// Eligible content with no index row — posts published inside the
	// disabled window never emitted, so the index walk above can't see
	// them. PublishContent's in-tx re-check keeps this self-correcting.
	missing, err := s.store.q.ListAtprotoReconcileMissingPosts(ctx, gen.ListAtprotoReconcileMissingPostsParams{
		UserID:       account.UserID,
		AccountID:    accountID,
		ContentTypes: s.federatableTypesFor(ctx),
		MinPosts:     minPosts,
	})
	if err != nil {
		return errors.Join(errs, fmt.Errorf("atproto: reconcile missing: %w", err))
	}
	for _, contentID := range missing {
		_, perr := s.publishContent(ctx, contentID, false)
		perRow(perr)
	}
	if !profileIndexed {
		// A profile never projected — the account opted in below the
		// §127 floor, or predates profiles — leaves no index row for the
		// walk to see. publishProfile resolves the floor either way:
		// met → upsert, unmet → no-op delete.
		perRow(s.publishProfile(ctx, &account))
	}
	return errs
}

// ReconcileSweepPage walks one keyset page of active accounts for
// atproto:reconcile_sweep (§127 projection floor / §11), probing each
// for projection divergence — indexed rows below the floor, or
// withheld backlog above it — and enqueuing atproto:reconcile_repo
// only for divergent repos. Returns the next page cursor ("" at the
// end). Probing is read-only: converged repos cost one query, not a
// commit — the sweep can run daily without rewriting healthy repos.
func (s *Service) ReconcileSweepPage(ctx context.Context, cursor pgtype.UUID, limit int32) (string, error) {
	if !s.featureOn(ctx) || limit <= 0 {
		// limit <= 0 would page nothing yet read "full" — guard the
		// rows[-1] cursor read, not just the semantics.
		return "", nil
	}
	rows, err := s.store.q.ListAtprotoSweepAccounts(ctx, gen.ListAtprotoSweepAccountsParams{
		Cursor:    cursor,
		PageLimit: limit,
	})
	if err != nil {
		return "", fmt.Errorf("atproto: sweep page: %w", err)
	}
	// The probe inputs are page-invariant — resolve once rather than
	// per account (flag eval and the settings read are remote lookups).
	minPosts := s.minPostsDiscoverable(ctx)
	contentTypes := s.federatableTypesFor(ctx)
	var errs error
	for _, a := range rows {
		divergent, err := s.store.q.GetAtprotoRepoDivergent(ctx, gen.GetAtprotoRepoDivergentParams{
			UserID:       a.UserID,
			AccountID:    a.ID,
			ContentTypes: contentTypes,
			MinPosts:     minPosts,
		})
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("atproto: sweep probe %s: %w", a.ID.String(), err))
			continue
		}
		if divergent != nil && *divergent {
			worker.EnqueueAtprotoReconcileRepo(ctx, a.ID)
		}
	}
	next := ""
	if int32(len(rows)) == limit {
		next = uuidString(rows[len(rows)-1].ID)
	}
	return next, errs
}

// EraseAccount is the GDPR erasure path — called by the gdpr service
// before the user row's physical delete, while key custody still
// exists. It delegates to the shared tombstone core; erasure-mode repo
// writes are what make this reachable: the lifecycle freeze may already
// have disabled the account, and only tombstone is past signing.
func (s *Service) EraseAccount(ctx context.Context, userID pgtype.UUID) error {
	account, err := s.store.q.GetAtprotoAccountAnyUserStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // never opted in
	}
	if err != nil {
		return fmt.Errorf("atproto: erase account lookup: %w", err)
	}
	return s.TombstoneAccount(ctx, account)
}

// TombstoneAccount is the shared A6-1 erasure core (§127) — GDPR
// EraseAccount and admin force-tombstone both run it. Order: a silent
// transitional disable write-gates the repo (no frame — the network
// learns only the 'deleted' end-state), one signed commit retracts
// every indexed record (the #commit frame carries the delete ops while
// signing-key custody still exists), blob bytes purge BEFORE the
// tombstone flip so a failure stays retryable (a tombstoned account
// early-returns forever — orphaned media would be unrecoverable), then
// the end-state announces as #account{deleted} → #tombstone: relays drop
// the repo's records wholesale on the tombstone frame rather than
// replaying per-record deletes. Terminal — no path downgrades or
// resurrects it.
func (s *Service) TombstoneAccount(ctx context.Context, account gen.AtprotoAccount) error {
	if account.Status == AccountStatusTombstone {
		return nil
	}

	// Write-gate before the retraction commit: a queued publish still
	// holding the repo lock commits first and lands in the retraction
	// set (SELECT FOR UPDATE serializes the tx against this UPDATE), a
	// publish starting after sees status != active and refuses, and a
	// member re-enable hitting 'admin' is refused — erasure is not
	// interruptible. Silent: no #account beat for a state that exists
	// only to fence writes.
	if account.Status != AccountStatusDisabled || account.DisabledReason == nil ||
		*account.DisabledReason != DisableReasonAdmin {
		reason := DisableReasonAdmin
		if err := s.store.setAccountStatus(ctx, account.ID, AccountStatusDisabled, &reason); err != nil {
			return fmt.Errorf("atproto: tombstone write-gate: %w", err)
		}
	}

	err := s.withRepoWrite(ctx, account.ID, repoWriteErasure, func(rep *atrepo.Repo, q *gen.Queries) ([]indexWrite, error) {
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
	// A6-4: external credentials can't outlive the identity — revoke
	// every session + app password. Outstanding access tokens expire
	// naturally (2h ceiling); refreshes die immediately.
	s.revokeExternalCredentials(ctx, account.ID)
	s.emitAccountEvent(ctx, account.Did, false, "deleted")
	s.emitTombstoneEvent(ctx, account.Did)
	s.invalidateAccount(ctx, account.Handle, uuidString(account.UserID))
	slog.Info("atproto account tombstoned", "did", account.Did)
	return nil
}
