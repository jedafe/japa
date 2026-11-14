package atproto

// Blob store (§127 A2): content-addressed binary storage for media
// referenced from records (A4's uploadBlob/getBlob surface serves it).
// Blob CIDs are raw-codec SHA2-256 — the multicodec atproto blob refs use.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/multiformats/go-multihash"
)

// PutBlob stores media bytes under their raw-codec sha256 CID for the
// account. Content-addressing makes calls idempotent — re-uploading the
// same bytes returns the same CID. Returns the CID string records
// reference via LexBlob.Ref.
func (s *Service) PutBlob(ctx context.Context, accountID pgtype.UUID, mime string, data []byte) (string, error) {
	sum, err := multihash.Sum(data, multihash.SHA2_256, -1)
	if err != nil {
		return "", fmt.Errorf("atproto: blob hash: %w", err)
	}
	c := cid.NewCidV1(cid.Raw, sum)
	if _, err := s.store.q.InsertAtprotoBlob(ctx, gen.InsertAtprotoBlobParams{
		AccountID: accountID,
		Cid:       c.String(),
		Mime:      mime,
		Size:      int64(len(data)),
		Data:      data,
	}); err != nil {
		return "", fmt.Errorf("atproto: store blob: %w", err)
	}
	return c.String(), nil
}

// GetBlob fetches a stored blob — A4's getBlob XRPC serves this.
func (s *Service) GetBlob(ctx context.Context, accountID pgtype.UUID, blobCid string) (mime string, data []byte, err error) {
	row, err := s.store.q.GetAtprotoBlob(ctx, gen.GetAtprotoBlobParams{
		AccountID: accountID,
		Cid:       blobCid,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("atproto: get blob: %w", err)
	}
	return row.Mime, row.Data, nil
}
