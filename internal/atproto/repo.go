package atproto

// Repo engine (§127 A2, ADR-139): per-account signed atproto repos.
// Every write is one transaction — LockAtprotoAccountForRepo serializes
// same-repo writers, then blocks + commit + records index + repo_seq
// persist together. All MST/CAR/CBOR/K-256 primitives come from indigo;
// we never hand-roll them (ADR-139's primary conformance risk).
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Repo engine)

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	indmst "github.com/bluesky-social/indigo/mst"
	atrepo "github.com/bluesky-social/indigo/repo"
	blockformat "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	car "github.com/ipld/go-car"
	_ "github.com/ipld/go-ipld-prime/codec/dagcbor" // register dag-cbor in the multicodec registry for CAR traversal
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	jdb "github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// repoBlockstore adapts a repo's committed blocks to the IPLD blockstore
// interfaces indigo consumes (Get/Put are all cbor.IpldBlockstore needs;
// the wider go-ipfs-blockstore shape is kept so CAR writers can use the
// same adapter). New blocks land in dirty — flush persists only those.
type repoBlockstore struct {
	blocks map[cid.Cid]blockformat.Block
	dirty  map[cid.Cid]struct{}
}

func newRepoBlockstore() *repoBlockstore {
	return &repoBlockstore{
		blocks: make(map[cid.Cid]blockformat.Block),
		dirty:  make(map[cid.Cid]struct{}),
	}
}

func (bs *repoBlockstore) Get(_ context.Context, c cid.Cid) (blockformat.Block, error) {
	b, ok := bs.blocks[c]
	if !ok {
		return nil, ipld.ErrNotFound{Cid: c}
	}
	return b, nil
}

func (bs *repoBlockstore) Put(_ context.Context, b blockformat.Block) error {
	bs.blocks[b.Cid()] = b
	bs.dirty[b.Cid()] = struct{}{}
	return nil
}

func (bs *repoBlockstore) Has(_ context.Context, c cid.Cid) (bool, error) {
	_, ok := bs.blocks[c]
	return ok, nil
}

func (bs *repoBlockstore) GetSize(_ context.Context, c cid.Cid) (int, error) {
	b, ok := bs.blocks[c]
	if !ok {
		return -1, ipld.ErrNotFound{Cid: c}
	}
	return len(b.RawData()), nil
}

func (bs *repoBlockstore) PutMany(ctx context.Context, blks []blockformat.Block) error {
	for _, b := range blks {
		if err := bs.Put(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

func (bs *repoBlockstore) DeleteBlock(_ context.Context, c cid.Cid) error {
	delete(bs.blocks, c)
	delete(bs.dirty, c)
	return nil
}

func (bs *repoBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	ch := make(chan cid.Cid, len(bs.blocks))
	go func() {
		defer close(ch)
		for c := range bs.blocks {
			select {
			case ch <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (bs *repoBlockstore) HashOnRead(bool) {}

// errRepoNoop aborts a repo write that changed nothing — the tx rolls
// back untouched (no commit row, no seq bump, no index writes) and the
// caller maps it to nil. Deletes of absent rpaths use it so retried
// unpublishes stay free (indigo's DeleteRecord errors on missing keys).
var errRepoNoop = errors.New("atproto: repo write noop")

// indexWrite is one atproto_records index mutation produced by a repo op.
type indexWrite struct {
	del        bool
	uri        string
	collection string
	rkey       string
	cid        string
	contentID  pgtype.UUID
}

// commitSigner decrypts the account's K-256 scalar and returns the signer
// indigo's Repo.Commit invokes on the dag-cbor commit bytes. HashAndSign
// produces the 64-byte low-S signature the network expects — the same
// path atproto verification uses (atcrypto, not hand-rolled secp256k1).
func commitSigner(signingKeyEnc string) (func(context.Context, string, []byte) ([]byte, error), error) {
	privHex, err := jcrypto.Decrypt(signingKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("atproto: signing key decrypt: %w", err)
	}
	priv, err := parsePrivateKeyHex(privHex)
	if err != nil {
		return nil, err
	}
	key, err := atcrypto.ParsePrivateBytesK256(priv.Serialize())
	if err != nil {
		return nil, fmt.Errorf("atproto: k256 key: %w", err)
	}
	return func(_ context.Context, _ string, b []byte) ([]byte, error) {
		return key.HashAndSign(b)
	}, nil
}

// loadBlockstore hydrates an account's committed blocks into an
// in-memory blockstore indigo can open the repo over.
func loadBlockstore(ctx context.Context, q *gen.Queries, accountID pgtype.UUID) (*repoBlockstore, error) {
	rows, err := q.ListAtprotoRepoBlocks(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("atproto: load blocks: %w", err)
	}
	bs := newRepoBlockstore()
	for _, row := range rows {
		c, err := cid.Parse(row.Cid)
		if err != nil {
			return nil, fmt.Errorf("atproto: stored block cid %q: %w", row.Cid, err)
		}
		b, err := blockformat.NewBlockWithCid(row.Data, c)
		if err != nil {
			return nil, fmt.Errorf("atproto: stored block %s: %w", row.Cid, err)
		}
		bs.blocks[c] = b
	}
	return bs, nil
}

// openRepo opens the account's repo over the hydrated blockstore, or
// initializes an empty one when no commit exists yet (opt-in bootstrap).
func openRepo(ctx context.Context, bs *repoBlockstore, did string, head gen.AtprotoRepoCommit, hasHead bool) (*atrepo.Repo, error) {
	if !hasHead {
		return atrepo.NewRepo(ctx, did, bs), nil
	}
	root, err := cid.Parse(head.Cid)
	if err != nil {
		return nil, fmt.Errorf("atproto: head commit cid %q: %w", head.Cid, err)
	}
	return atrepo.OpenRepo(ctx, bs, root)
}

// withRepoWrite runs fn inside the repo write transaction: the account
// row is locked FOR UPDATE (serializing same-repo writers), blocks are
// hydrated, fn mutates the repo and returns index writes, then the new
// signed commit + dirty blocks + index rows + repo_seq bump persist
// atomically. Disabled/tombstone accounts cannot write (§127 lifecycle).
// fn receives the tx's queries so a caller can re-validate canonical
// state on the same connection right before the commit — READ COMMITTED
// means it observes anything committed up to that statement, which is
// what closes the publish-vs-unpublish interleave.
func (s *Service) withRepoWrite(ctx context.Context, accountID pgtype.UUID, fn func(rep *atrepo.Repo, q *gen.Queries) ([]indexWrite, error)) error {
	return jdb.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		acct, err := q.LockAtprotoAccountForRepo(ctx, accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("atproto: lock account: %w", err)
		}
		if acct.Status != AccountStatusActive {
			return ErrNotFound
		}

		bs, err := loadBlockstore(ctx, q, accountID)
		if err != nil {
			return err
		}
		head, err := q.GetAtprotoRepoHead(ctx, accountID)
		hasHead := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("atproto: repo head: %w", err)
		}
		rep, err := openRepo(ctx, bs, acct.Did, head, hasHead)
		if err != nil {
			return fmt.Errorf("atproto: open repo: %w", err)
		}

		writes, err := fn(rep, q)
		if err != nil {
			return err
		}

		signer, err := commitSigner(acct.SigningKeyEnc)
		if err != nil {
			return err
		}
		commitCid, rev, err := rep.Commit(ctx, signer)
		if err != nil {
			return fmt.Errorf("atproto: commit: %w", err)
		}

		// indigo's TIDClock starts at zero on every OpenRepo — it is
		// monotonic only within a process, never seeded from the head
		// rev. A wall-clock regression (NTP step-back, VM snapshot)
		// would mint rev <= head's, breaking the strictly-increasing
		// ordering relays require. TIDs are fixed-width base32sort so
		// lexicographic compare is exact. Refuse to persist — the error
		// stays transient so the retry lands once the clock recovers.
		if hasHead && rev <= head.Rev {
			return fmt.Errorf("atproto: rev regression %s <= %s", rev, head.Rev)
		}

		for c := range bs.dirty {
			if err := q.InsertAtprotoRepoBlock(ctx, gen.InsertAtprotoRepoBlockParams{
				AccountID: accountID,
				Cid:       c.String(),
				Data:      bs.blocks[c].RawData(),
			}); err != nil {
				return fmt.Errorf("atproto: persist block %s: %w", c, err)
			}
		}

		var prev *string
		if p := rep.SignedCommit().Prev; p != nil {
			s := p.String()
			prev = &s
		}
		if _, err := q.InsertAtprotoRepoCommit(ctx, gen.InsertAtprotoRepoCommitParams{
			AccountID: accountID,
			Seq:       acct.RepoSeq + 1,
			Cid:       commitCid.String(),
			Rev:       rev,
			PrevCid:   prev,
			Data:      bs.blocks[commitCid].RawData(),
		}); err != nil {
			return fmt.Errorf("atproto: persist commit: %w", err)
		}

		// §127 A3: the #commit firehose frame lands in the same tx — the
		// events identity-seq is allocated in commit order, so the stream
		// can never skip or reorder a committed rev. Built before the
		// index writes apply: ops read the still-stale index rows for
		// their prev CIDs.
		if err := s.emitCommitEvent(ctx, q, rep, accountID, commitCid, rev, head, hasHead, bs, writes); err != nil {
			return fmt.Errorf("atproto: emit commit event: %w", err)
		}

		for _, w := range writes {
			if w.del {
				if err := q.DeleteAtprotoRecord(ctx, gen.DeleteAtprotoRecordParams{
					AccountID:  accountID,
					Collection: w.collection,
					Rkey:       w.rkey,
				}); err != nil {
					return fmt.Errorf("atproto: unindex record: %w", err)
				}
				continue
			}
			if _, err := q.UpsertAtprotoRecord(ctx, gen.UpsertAtprotoRecordParams{
				Uri:        w.uri,
				AccountID:  accountID,
				Collection: w.collection,
				Rkey:       w.rkey,
				Cid:        w.cid,
				ContentID:  w.contentID,
			}); err != nil {
				return fmt.Errorf("atproto: index record: %w", err)
			}
		}

		return q.BumpAtprotoRepoSeq(ctx, gen.BumpAtprotoRepoSeqParams{
			ID:      accountID,
			RepoSeq: acct.RepoSeq + 1,
		})
	})
}

// upsertRecord is the Add-vs-Update upsert indigo lacks — its PutRecord
// fails on existing keys and UpdateRecord fails on missing ones, so the
// pick rides on whether the rpath already resolves.
func upsertRecord(ctx context.Context, rep *atrepo.Repo, rpath string, rec atrepo.CborMarshaler) (cid.Cid, error) {
	if _, _, gerr := rep.GetRecordBytes(ctx, rpath); gerr == nil {
		return rep.UpdateRecord(ctx, rpath, rec)
	} else if errors.Is(gerr, indmst.ErrNotFound) {
		return rep.PutRecord(ctx, rpath, rec)
	} else {
		return cid.Undef, gerr
	}
}

// PutRecord upserts collection/rkey, commits the repo, and indexes the
// at:// URI. rkey must satisfy atproto record-key rules (TID or literal
// like "self"). Returns the record's at:// URI.
func (s *Service) PutRecord(ctx context.Context, accountID pgtype.UUID, collection, rkey string, rec atrepo.CborMarshaler, contentID pgtype.UUID) (string, error) {
	var uri string
	err := s.withRepoWrite(ctx, accountID, func(rep *atrepo.Repo, _ *gen.Queries) ([]indexWrite, error) {
		rpath := collection + "/" + rkey
		recCid, err := upsertRecord(ctx, rep, rpath, rec)
		if err != nil {
			return nil, fmt.Errorf("atproto: put %s: %w", rpath, err)
		}
		uri = fmt.Sprintf("at://%s/%s", rep.RepoDid(), rpath)
		return []indexWrite{{uri: uri, collection: collection, rkey: rkey, cid: recCid.String(), contentID: contentID}}, nil
	})
	if err != nil {
		return "", err
	}
	return uri, nil
}

// CreateRecord writes a record under a fresh TID rkey (indigo's
// CreateRecord allocates it and returns it) — the feed.post path.
// Returns uri + rkey.
func (s *Service) CreateRecord(ctx context.Context, accountID pgtype.UUID, collection string, rec atrepo.CborMarshaler, contentID pgtype.UUID) (uri, rkey string, err error) {
	err = s.withRepoWrite(ctx, accountID, func(rep *atrepo.Repo, _ *gen.Queries) ([]indexWrite, error) {
		recCid, tid, err := rep.CreateRecord(ctx, collection, rec)
		if err != nil {
			return nil, fmt.Errorf("atproto: create %s: %w", collection, err)
		}
		rkey = tid
		uri = fmt.Sprintf("at://%s/%s/%s", rep.RepoDid(), collection, rkey)
		return []indexWrite{{uri: uri, collection: collection, rkey: rkey, cid: recCid.String(), contentID: contentID}}, nil
	})
	if err != nil {
		return "", "", err
	}
	return uri, rkey, nil
}

// DeleteRecord removes collection/rkey, commits the tombstoned repo, and
// drops the index row. An absent rpath is a true no-op — no commit, no
// seq bump — so retried unpublishes stay idempotent (indigo's
// DeleteRecord errors on missing keys, so existence is checked first).
func (s *Service) DeleteRecord(ctx context.Context, accountID pgtype.UUID, collection, rkey string) error {
	err := s.withRepoWrite(ctx, accountID, func(rep *atrepo.Repo, _ *gen.Queries) ([]indexWrite, error) {
		rpath := collection + "/" + rkey
		if _, _, gerr := rep.GetRecordBytes(ctx, rpath); errors.Is(gerr, indmst.ErrNotFound) {
			return nil, errRepoNoop
		} else if gerr != nil {
			return nil, fmt.Errorf("atproto: stat %s: %w", rpath, gerr)
		}
		if err := rep.DeleteRecord(ctx, rpath); err != nil {
			return nil, fmt.Errorf("atproto: delete %s/%s: %w", collection, rkey, err)
		}
		return []indexWrite{{del: true, collection: collection, rkey: rkey}}, nil
	})
	if errors.Is(err, errRepoNoop) {
		return nil
	}
	return err
}

// RepoCAR serializes the account's repo (commit root + every block) to a
// CAR v1 stream — the payload A3's com.atproto.sync.getRepo serves and
// the conformance check replays. Read-only; no account lock needed since
// blocks are immutable and the head commit anchors the snapshot.
func (s *Service) RepoCAR(ctx context.Context, accountID pgtype.UUID) ([]byte, error) {
	q := s.store.q
	head, err := q.GetAtprotoRepoHead(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("atproto: repo head: %w", err)
	}
	root, err := cid.Parse(head.Cid)
	if err != nil {
		return nil, fmt.Errorf("atproto: head commit cid %q: %w", head.Cid, err)
	}
	bs, err := loadBlockstore(ctx, q, accountID)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	dsvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
	if err := car.WriteCar(ctx, dsvc, []cid.Cid{root}, &buf); err != nil {
		return nil, fmt.Errorf("atproto: write car: %w", err)
	}
	return buf.Bytes(), nil
}
