package atproto

// Sync surface (§127 A3): the read side relays/appviews crawl — repo
// CARs, block slices, head commits, single records, and the
// subscribeRepos event log — plus the outbound requestCrawl call to the
// relay (JC_ATPROTO_RELAY_URL, config-pinned per §127's SSRF rule).
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Sync + discovery)

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	car "github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// carMediaType is the content type all CAR-producing XRPC endpoints use.
const carMediaType = "application/vnd.ipld.car"

// accountForDID resolves a sync-endpoint `did`/`repo` param to its
// account row — active accounts of active users only (suspended members
// and disabled accounts don't exist on the network).
func (s *Service) accountForDID(ctx context.Context, did string) (gen.GetAtprotoAccountByDIDRow, error) {
	if did == "" {
		return gen.GetAtprotoAccountByDIDRow{}, ErrNotFound
	}
	account, err := s.store.accountByDID(ctx, did)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.GetAtprotoAccountByDIDRow{}, ErrNotFound
	}
	if err != nil {
		return gen.GetAtprotoAccountByDIDRow{}, fmt.Errorf("atproto: account by did: %w", err)
	}
	return account, nil
}

// RepoCARSince serves getRepo: the full CAR when since is empty; a diff
// CAR otherwise — head commit as root + every block introduced by the
// commits newer than the client's rev (the same block sets the #commit
// firehose frames carry, unioned). Revs are TIDs: lexicographic order
// is chronological. since at/after head answers with just the head
// commit block — the "you're current" no-op. A since range spanning a
// commit the block index doesn't cover (pre-001146 history) falls back
// to the full CAR — always a legal, self-sufficient answer.
func (s *Service) RepoCARSince(ctx context.Context, did, since string) ([]byte, error) {
	account, err := s.accountForDID(ctx, did)
	if err != nil {
		return nil, err
	}
	if since == "" {
		return s.RepoCAR(ctx, account.ID)
	}
	head, err := s.store.q.GetAtprotoRepoHead(ctx, account.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("atproto: repo head: %w", err)
	}
	headCid, err := cid.Parse(head.Cid)
	if err != nil {
		return nil, fmt.Errorf("atproto: head commit cid %q: %w", head.Cid, err)
	}
	if since >= head.Rev {
		return carFromBlockSet(headCid, map[string][]byte{head.Cid: head.Data}, nil)
	}
	// A since naming a rev we never minted can't tell us what the client
	// already holds — the diff could under-ship context. Full CAR is the
	// always-correct answer.
	knownRev, err := s.store.q.AtprotoCommitRevExists(ctx, gen.AtprotoCommitRevExistsParams{
		AccountID: account.ID,
		Rev:       since,
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: rev lookup: %w", err)
	}
	if !knownRev {
		return s.RepoCAR(ctx, account.ID)
	}
	commits, err := s.store.q.ListAtprotoCommitsAfterRev(ctx, gen.ListAtprotoCommitsAfterRevParams{
		AccountID: account.ID,
		Rev:       since,
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: commits after rev: %w", err)
	}
	seqs := make([]int64, len(commits))
	for i, c := range commits {
		seqs[i] = c.Seq
	}
	rows, err := s.store.q.ListAtprotoCommitBlocks(ctx, gen.ListAtprotoCommitBlocksParams{
		AccountID: account.ID,
		Seqs:      seqs,
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: commit blocks: %w", err)
	}
	covered := make(map[int64]struct{}, len(commits))
	cids := make([]string, 0, len(rows))
	for _, r := range rows {
		covered[r.Seq] = struct{}{}
		cids = append(cids, r.Cid)
	}
	if len(covered) != len(commits) {
		return s.RepoCAR(ctx, account.ID)
	}
	blocks, err := s.store.q.ListAtprotoRepoBlocksByCids(ctx, gen.ListAtprotoRepoBlocksByCidsParams{
		AccountID: account.ID,
		Cids:      cids,
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: slice blocks: %w", err)
	}
	blockData := make(map[string][]byte, len(blocks)+1)
	for _, b := range blocks {
		blockData[b.Cid] = b.Data
	}
	blockData[head.Cid] = head.Data
	// Commit blocks first in CAR order (head leads via carFromBlockSet's
	// root emit), then the introduced MST/record blocks — mirroring the
	// #commit frame's diff ordering relays already apply.
	isCommit := make(map[string]struct{}, len(commits))
	order := make([]cid.Cid, 0, len(cids))
	for _, c := range commits {
		isCommit[c.Cid] = struct{}{}
		cc, err := cid.Parse(c.Cid)
		if err != nil {
			return nil, fmt.Errorf("atproto: commit cid %q: %w", c.Cid, err)
		}
		order = append(order, cc)
	}
	for _, raw := range cids {
		if _, ok := isCommit[raw]; ok {
			continue
		}
		cc, err := cid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("atproto: slice cid %q: %w", raw, err)
		}
		order = append(order, cc)
	}
	return carFromBlockSet(headCid, blockData, order)
}

// carFromBlockSet frames a CAR v1: root first in the roots list AND the
// section stream (convention the #commit frame established), then each
// ordered CID present in blockData. Absent CIDs skip silently — callers
// decide what absence means before framing.
func carFromBlockSet(root cid.Cid, blockData map[string][]byte, order []cid.Cid) ([]byte, error) {
	var buf bytes.Buffer
	if err := car.WriteHeader(&car.CarHeader{Roots: []cid.Cid{root}, Version: 1}, &buf); err != nil {
		return nil, fmt.Errorf("atproto: car header: %w", err)
	}
	written := map[cid.Cid]struct{}{}
	emit := func(c cid.Cid) error {
		if _, ok := written[c]; ok {
			return nil
		}
		data, ok := blockData[c.String()]
		if !ok {
			return nil
		}
		if err := carutil.LdWrite(&buf, c.Bytes(), data); err != nil {
			return fmt.Errorf("atproto: car block %s: %w", c, err)
		}
		written[c] = struct{}{}
		return nil
	}
	if err := emit(root); err != nil {
		return nil, err
	}
	for _, c := range order {
		if err := emit(c); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// HeadCommit returns (commit CID, rev) for getLatestCommit.
func (s *Service) HeadCommit(ctx context.Context, did string) (string, string, error) {
	account, err := s.accountForDID(ctx, did)
	if err != nil {
		return "", "", err
	}
	head, err := s.store.q.GetAtprotoRepoHead(ctx, account.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("atproto: repo head: %w", err)
	}
	return head.Cid, head.Rev, nil
}

// BlocksCAR frames the caller-named CIDs of one repo as CAR v1 for
// getBlocks — unknown CIDs are simply absent from the section stream
// (content-addressed blocks are immutable; a missing one can't be
// fetched later).
func (s *Service) BlocksCAR(ctx context.Context, did string, cids []string) ([]byte, error) {
	account, err := s.accountForDID(ctx, did)
	if err != nil {
		return nil, err
	}
	if len(cids) == 0 {
		return nil, ErrInvalidID
	}
	seen := make(map[cid.Cid]struct{}, len(cids))
	valid := make([]cid.Cid, 0, len(cids))
	for _, raw := range cids {
		c, err := cid.Parse(raw)
		if err != nil {
			return nil, ErrInvalidID
		}
		if _, dup := seen[c]; dup {
			continue // duplicate section bytes add nothing — CIDs address content
		}
		seen[c] = struct{}{}
		valid = append(valid, c)
	}
	texts := make([]string, 0, len(valid))
	for _, c := range valid {
		texts = append(texts, c.String())
	}
	rows, err := s.store.q.ListAtprotoRepoBlocksByCids(ctx, gen.ListAtprotoRepoBlocksByCidsParams{
		AccountID: account.ID,
		Cids:      texts,
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: blocks: %w", err)
	}
	byCid := make(map[string]gen.AtprotoRepoBlock, len(rows))
	for _, r := range rows {
		byCid[r.Cid] = r
	}
	var buf bytes.Buffer
	if err := car.WriteHeader(&car.CarHeader{Roots: valid, Version: 1}, &buf); err != nil {
		return nil, fmt.Errorf("atproto: blocks car header: %w", err)
	}
	for _, c := range valid {
		row, ok := byCid[c.String()]
		if !ok {
			continue
		}
		if err := carutil.LdWrite(&buf, c.Bytes(), row.Data); err != nil {
			return nil, fmt.Errorf("atproto: blocks car write: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// RecordCAR serves getRecord: the record must exist in the index —
// otherwise RecordNotFound. The CAR carries the head commit plus the
// repo's live blockset (same traversal as RepoCAR): indigo has no MST
// proof-path extraction, so the slice is the full reachable set — a
// superset the verifier walks the same way, bounded by our small
// member-repo size.
func (s *Service) RecordCAR(ctx context.Context, did, collection, rkey string) ([]byte, error) {
	account, err := s.accountForDID(ctx, did)
	if err != nil {
		return nil, err
	}
	if collection == "" || rkey == "" {
		return nil, ErrInvalidID
	}
	if _, err := s.store.q.GetAtprotoRecordByPath(ctx, gen.GetAtprotoRecordByPathParams{
		AccountID:  account.ID,
		Collection: collection,
		Rkey:       rkey,
	}); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("atproto: record lookup: %w", err)
	}
	return s.RepoCAR(ctx, account.ID)
}

// EventsAfter returns the next page of firehose events after cursor
// (subscribeRepos replay + live tail).
func (s *Service) EventsAfter(ctx context.Context, after int64, limit int32) ([]gen.AtprotoRepoEvent, error) {
	return s.store.q.ListAtprotoRepoEventsAfter(ctx, gen.ListAtprotoRepoEventsAfterParams{
		After:     after,
		PageLimit: limit,
	})
}

// MaxEventSeq returns the firehose high-water mark — a subscribeRepos
// cursor beyond it is a FutureCursor error frame.
func (s *Service) MaxEventSeq(ctx context.Context) (int64, error) {
	return s.store.q.GetAtprotoRepoEventMaxSeq(ctx)
}

// hostDID is the PDS host's own did:web — describeServer's required
// `did` field, resolved by /.well-known/did.json.
func (s *Service) hostDID() string {
	return "did:web:" + strings.ReplaceAll(s.cfg.pdsHost(), ":", "%3A")
}

// hostDIDDocument is the host did:web document — id + the
// #atproto_pds service endpoint (no verificationMethod: the PDS host
// has no keypair — member keys sign commits, not service calls).
func (s *Service) hostDIDDocument() map[string]any {
	return map[string]any{
		"@context": []string{"https://www.w3.org/ns/did/v1"},
		"id":       s.hostDID(),
		"service": []map[string]string{{
			"id":              "#atproto_pds",
			"type":            "AtprotoPersonalDataServer",
			"serviceEndpoint": s.cfg.pdsURL(),
		}},
	}
}

// crawlHTTPClient posts requestCrawl — config-pinned relay target, no
// user-supplied hosts (§127 SSRF). 10s keeps the worker from pinning a
// slot on a hung relay.
var crawlHTTPClient = &http.Client{Timeout: 10 * time.Second}

// RequestCrawl posts com.atproto.sync.requestCrawl for this PDS to the
// relay — the relay then crawls subscribeRepos/getRepo. The task
// payload carries account_id, but the wire body names the PDS host
// itself: the account id validates the trigger (an inactive/missing
// account makes the crawl pointless — permanent), the relay only ever
// sees the config-pinned hostname.
func (s *Service) RequestCrawl(ctx context.Context, accountID pgtype.UUID) error {
	status, err := s.store.q.GetAtprotoAccountStatusByID(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return permanent(ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("atproto: crawl account status: %w", err)
	}
	if status != AccountStatusActive {
		return permanent(ErrNotFound)
	}
	relay := strings.TrimRight(s.cfg.RelayURL, "/")
	if relay == "" {
		return permanent(fmt.Errorf("atproto: JC_ATPROTO_RELAY_URL not configured"))
	}
	if !s.featureOn(ctx) {
		return permanent(fmt.Errorf("atproto: feature.atproto disabled"))
	}
	body, err := json.Marshal(map[string]string{"hostname": stripPort(s.cfg.pdsHost())})
	if err != nil {
		return permanent(fmt.Errorf("atproto: crawl payload: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		relay+"/xrpc/com.atproto.sync.requestCrawl", bytes.NewReader(body))
	if err != nil {
		return permanent(fmt.Errorf("atproto: crawl request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := crawlHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("atproto: requestCrawl: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("atproto: requestCrawl: relay %s returned %d", relay, resp.StatusCode)
	}
	return nil
}
