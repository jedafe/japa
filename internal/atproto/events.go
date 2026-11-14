package atproto

// Firehose events (§127 A3): atproto_repo_events is the global
// sequencer behind com.atproto.sync.subscribeRepos. #commit rows insert
// inside the repo-write transaction — seq order == commit order across
// all accounts, so a committed rev can never be skipped by the stream.
// #identity (handle changes) and #account (activation/deactivation —
// the modern lexicon's replacement for the deprecated #handle frame)
// insert best-effort on lifecycle transitions.
//
// payload holds the frame body minus seq/time, which are stamped from
// the row itself at emit — one identity counter is the only clock the
// stream orders on.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Sync + discovery)

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/ipfs/go-cid"
	car "github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// Event kinds — atproto_repo_events.kind CHECK constraint.
const (
	EventKindCommit   = "commit"
	EventKindIdentity = "identity"
	EventKindAccount  = "account"
)

// Frame header ops — subscribeRepos's two-part frame: {op,t} map then
// the message body. -1 is the error frame, 1 a message carrying t.
const (
	frameOpError   int64 = -1
	frameOpMessage int64 = 1
)

// insertEvent writes one sequencer row inside the caller's transaction
// (commit events) or on the pool (lifecycle events). payload is the
// frame body with seq/time left zero — both stamp from the row at emit.
func insertEvent(ctx context.Context, q *gen.Queries, did, kind string, payload []byte) error {
	_, err := q.InsertAtprotoRepoEvent(ctx, gen.InsertAtprotoRepoEventParams{
		Did:     did,
		Kind:    kind,
		Payload: payload,
	})
	return err
}

// emitEvent best-effort inserts a lifecycle frame (identity/account).
// These ride the pool, not a tx — a lost frame costs one stale identity
// cache downstream (the next change re-emits), which is cheap compared
// to aborting the caller's operation.
func (s *Service) emitEvent(ctx context.Context, did, kind string, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		slog.Warn("atproto: event marshal failed", "kind", kind, "did", did, "error", err)
		return
	}
	if err := insertEvent(ctx, s.store.q, did, kind, payload); err != nil {
		slog.Warn("atproto: event insert failed", "kind", kind, "did", did, "error", err)
	}
}

// emitIdentityEvent announces a handle change — the #identity frame
// nudges relays/appviews to refresh their handle→DID caches.
func (s *Service) emitIdentityEvent(ctx context.Context, did, handle string) {
	s.emitEvent(ctx, did, EventKindIdentity, &comatproto.SyncSubscribeRepos_Identity{
		Did:    did,
		Handle: &handle,
	})
}

// emitAccountEvent announces an activation/deactivation — status is the
// lexicon reason when active=false ("deactivated", "deleted").
func (s *Service) emitAccountEvent(ctx context.Context, did string, active bool, status string) {
	body := &comatproto.SyncSubscribeRepos_Account{Did: did, Active: active}
	if status != "" {
		body.Status = &status
	}
	s.emitEvent(ctx, did, EventKindAccount, body)
}

// emitCommitEvent appends the #commit frame for a just-signed repo
// commit. Called inside the repo-write tx: the event's identity-seq is
// allocated in commit order so the stream can't skip or reorder revs.
// ops are built from the write set BEFORE the index writes apply — the
// still-stale index rows supply each update/delete op's prev CID.
func (s *Service) emitCommitEvent(ctx context.Context, q *gen.Queries, rep *atrepo.Repo, accountID pgtype.UUID, commitCid cid.Cid, rev string, head gen.AtprotoRepoCommit, hasHead bool, bs *repoBlockstore, writes []indexWrite) error {
	ops, err := commitOps(ctx, q, accountID, writes)
	if err != nil {
		return err
	}
	blocks, err := diffCAR(commitCid, bs)
	if err != nil {
		return err
	}
	body := &comatproto.SyncSubscribeRepos_Commit{
		Repo:   rep.RepoDid(),
		Commit: lexutil.LexLink(commitCid),
		Rev:    rev,
		Ops:    ops,
		Blobs:  []lexutil.LexLink{},
		Blocks: blocks,
	}
	if hasHead {
		body.Since = &head.Rev
		prevData, err := commitDataCid(head.Data)
		if err != nil {
			return fmt.Errorf("atproto: prev commit data: %w", err)
		}
		link := lexutil.LexLink(prevData)
		body.PrevData = &link
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("atproto: commit event marshal: %w", err)
	}
	if err := insertEvent(ctx, q, rep.RepoDid(), EventKindCommit, payload); err != nil {
		return fmt.Errorf("atproto: commit event insert: %w", err)
	}
	return nil
}

// commitOps shapes the writes' record mutations into lexicon ops. The
// index row read still sees pre-write state, so an existing row means
// "update" (or "delete") and supplies prev — inductive-firehose
// consumers apply ops against it.
func commitOps(ctx context.Context, q *gen.Queries, accountID pgtype.UUID, writes []indexWrite) ([]*comatproto.SyncSubscribeRepos_RepoOp, error) {
	ops := make([]*comatproto.SyncSubscribeRepos_RepoOp, 0, len(writes))
	for _, w := range writes {
		path := w.collection + "/" + w.rkey
		op := &comatproto.SyncSubscribeRepos_RepoOp{Path: path}
		prev, perr := q.GetAtprotoRecordByPath(ctx, gen.GetAtprotoRecordByPathParams{
			AccountID:  accountID,
			Collection: w.collection,
			Rkey:       w.rkey,
		})
		if perr != nil && !errors.Is(perr, pgx.ErrNoRows) {
			slog.Warn("atproto: op prev lookup failed", "path", path, "error", perr)
		}
		havePrev := perr == nil && prev.Cid != ""
		if w.del {
			op.Action = "delete"
			if havePrev {
				link, err := lexLinkString(prev.Cid)
				if err != nil {
					return nil, err
				}
				op.Prev = &link
			}
		} else {
			link, err := lexLinkString(w.cid)
			if err != nil {
				return nil, err
			}
			op.Cid = &link
			if havePrev {
				op.Action = "update"
				prevLink, err := lexLinkString(prev.Cid)
				if err != nil {
					return nil, err
				}
				op.Prev = &prevLink
			} else {
				op.Action = "create"
			}
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// lexLinkString parses a stored CID string into a LexLink.
func lexLinkString(s string) (lexutil.LexLink, error) {
	c, err := cid.Parse(s)
	if err != nil {
		return lexutil.LexLink(cid.Undef), fmt.Errorf("atproto: cid %q: %w", s, err)
	}
	return lexutil.LexLink(c), nil
}

// commitDataCid reads a persisted commit block back into SignedCommit
// and returns its `data` CID — the MST root the commit covered, which is
// what a #commit frame's prevData points at.
func commitDataCid(commitBlock []byte) (cid.Cid, error) {
	var sc atrepo.SignedCommit
	if err := sc.UnmarshalCBOR(bytes.NewReader(commitBlock)); err != nil {
		return cid.Undef, err
	}
	return sc.Data, nil
}

// diffCAR frames the commit's dirty blockset as CAR v1 — the #commit
// frame's `blocks`: commit block CID first in the roots list (lexicon
// requirement), the commit block first in the section stream, then the
// remaining new MST/record blocks. Not a DAG traversal — exactly the
// blocks this commit introduced, which is what "a diff since the
// previous repo state" means.
func diffCAR(root cid.Cid, bs *repoBlockstore) (lexutil.LexBytes, error) {
	var buf bytes.Buffer
	if err := car.WriteHeader(&car.CarHeader{Roots: []cid.Cid{root}, Version: 1}, &buf); err != nil {
		return nil, fmt.Errorf("atproto: diff car header: %w", err)
	}
	if b, ok := bs.blocks[root]; ok {
		if err := carutil.LdWrite(&buf, root.Bytes(), b.RawData()); err != nil {
			return nil, fmt.Errorf("atproto: diff car commit block: %w", err)
		}
	}
	rest := make([]cid.Cid, 0, len(bs.dirty))
	for c := range bs.dirty {
		if c != root {
			rest = append(rest, c)
		}
	}
	// Deterministic section order — map iteration is random, and a
	// reproducible diff stream is cheaper to diff/debug downstream.
	sort.Slice(rest, func(i, j int) bool { return rest[i].String() < rest[j].String() })
	for _, c := range rest {
		if err := carutil.LdWrite(&buf, c.Bytes(), bs.blocks[c].RawData()); err != nil {
			return nil, fmt.Errorf("atproto: diff car block %s: %w", c, err)
		}
	}
	return buf.Bytes(), nil
}

// writeFrameHeader emits the two-part frame's first half — the {op,t}
// CBOR map — with the same cbor-gen primitives indigo's generated
// marshalers use (the events package itself is avoided: it drags in
// gorm via XRPCStreamEvent's private fields).
func writeFrameHeader(w io.Writer, msgType string, op int64) error {
	cw := cbg.NewCborWriter(w)
	fieldCount := uint64(2)
	if msgType == "" {
		fieldCount--
	}
	if err := cw.WriteMajorTypeHeader(cbg.MajMap, fieldCount); err != nil {
		return err
	}
	if msgType != "" {
		if err := cw.WriteMajorTypeHeader(cbg.MajTextString, 1); err != nil {
			return err
		}
		if _, err := cw.WriteString("t"); err != nil {
			return err
		}
		if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(msgType))); err != nil {
			return err
		}
		if _, err := cw.WriteString(msgType); err != nil {
			return err
		}
	}
	if err := cw.WriteMajorTypeHeader(cbg.MajTextString, 2); err != nil {
		return err
	}
	if _, err := cw.WriteString("op"); err != nil {
		return err
	}
	if op >= 0 {
		return cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, uint64(op))
	}
	return cw.WriteMajorTypeHeader(cbg.MajNegativeInt, uint64(-op-1))
}

// frameBytes rebuilds a stored event row into its two-part CBOR frame —
// header {op,t} + lexicon body. seq/time stamp from the row: seq is the
// stream's ordering, time is when the event committed.
func frameBytes(row gen.AtprotoRepoEvent) ([]byte, error) {
	stamp := row.CreatedAt.Time.UTC().Format(time.RFC3339)
	var msgType string
	var body atrepo.CborMarshaler
	switch row.Kind {
	case EventKindCommit:
		var b comatproto.SyncSubscribeRepos_Commit
		if err := json.Unmarshal(row.Payload, &b); err != nil {
			return nil, fmt.Errorf("atproto: commit payload: %w", err)
		}
		b.Seq, b.Time = row.Seq, stamp
		msgType, body = "#commit", &b
	case EventKindIdentity:
		var b comatproto.SyncSubscribeRepos_Identity
		if err := json.Unmarshal(row.Payload, &b); err != nil {
			return nil, fmt.Errorf("atproto: identity payload: %w", err)
		}
		b.Seq, b.Time = row.Seq, stamp
		msgType, body = "#identity", &b
	case EventKindAccount:
		var b comatproto.SyncSubscribeRepos_Account
		if err := json.Unmarshal(row.Payload, &b); err != nil {
			return nil, fmt.Errorf("atproto: account payload: %w", err)
		}
		b.Seq, b.Time = row.Seq, stamp
		msgType, body = "#account", &b
	default:
		return nil, fmt.Errorf("atproto: unknown event kind %q", row.Kind)
	}
	var buf bytes.Buffer
	if err := writeFrameHeader(&buf, msgType, frameOpMessage); err != nil {
		return nil, fmt.Errorf("atproto: frame header: %w", err)
	}
	if err := body.MarshalCBOR(&buf); err != nil {
		return nil, fmt.Errorf("atproto: frame body: %w", err)
	}
	return buf.Bytes(), nil
}

// errorFrameBytes serializes an error frame — subscribeRepos's in-band
// error channel (FutureCursor/OutdatedCursor). The body is the lexicon
// error object {error, message}.
func errorFrameBytes(name, message string) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeFrameHeader(&buf, "", frameOpError); err != nil {
		return nil, fmt.Errorf("atproto: error frame header: %w", err)
	}
	cw := cbg.NewCborWriter(&buf)
	if err := cw.WriteMajorTypeHeader(cbg.MajMap, 2); err != nil {
		return nil, err
	}
	for _, kv := range [][2]string{{"error", name}, {"message", message}} {
		if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(kv[0]))); err != nil {
			return nil, err
		}
		if _, err := cw.WriteString(kv[0]); err != nil {
			return nil, err
		}
		if err := cw.WriteMajorTypeHeader(cbg.MajTextString, uint64(len(kv[1]))); err != nil {
			return nil, err
		}
		if _, err := cw.WriteString(kv[1]); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
