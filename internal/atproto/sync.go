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

// repoCARByDID resolves the repo param then returns RepoCAR — shared by
// getRepo and getRecord (a repo without a head commit doesn't exist on
// the network).
func (s *Service) repoCARByDID(ctx context.Context, did string) ([]byte, error) {
	account, err := s.accountForDID(ctx, did)
	if err != nil {
		return nil, err
	}
	return s.RepoCAR(ctx, account.ID)
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
	valid := make([]cid.Cid, 0, len(cids))
	for _, raw := range cids {
		c, err := cid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("atproto: bad cid %q", raw)
		}
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
