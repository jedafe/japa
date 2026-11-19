package atproto

// Media → blob projection (§127 A4): stored media bytes become raw-codec
// sha256 blobs in atproto_blobs, referenced from feed.post embeds and the
// profile record's avatar/banner. Blob puts run outside the repo-write tx —
// blobs aren't signed repo data (see InsertAtprotoBlob's account-status
// gate note). Fetch failures and oversize bytes skip the single image;
// they never fail the projection.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Content pipeline)

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/ipfs/go-cid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// embedImageMaxBytes is the byte cap per embedded image — 1MB, the
// conservative bound across lexicon generations (current app.bsky
// embed.images allows 2MB; older appviews reject >1MB).
const embedImageMaxBytes = 1_000_000

// MediaReader fetches stored media bytes for blob projection.
// media.Service satisfies it; the interface keeps atproto out of the
// media package's import graph and lets tests fake the store.
type MediaReader interface {
	// ReadObject returns the object's bytes, at most maxBytes+1 — the +1
	// lets the caller distinguish an oversize object (skip) from an exact
	// fit without a second size read.
	ReadObject(ctx context.Context, bucket, path string, maxBytes int64) ([]byte, error)
}

// SetMediaReader wires the media byte source. Optional — nil leaves
// embeds/avatar/banner out of records (tests, media-less dev runs).
func (s *Service) SetMediaReader(r MediaReader) { s.media = r }

// blobForMedia fetches one media row's bytes (capped) and stores them as
// a repo-owned blob. A nil return means "skip this image" — unconfirmed,
// oversize, missing object, or unreadable store — never an error worth
// failing a projection over.
func (s *Service) blobForMedia(ctx context.Context, accountID pgtype.UUID, bucket, path, mime string, sizeBytes int64) *lexutil.LexBlob {
	if s.media == nil {
		return nil
	}
	if sizeBytes > embedImageMaxBytes {
		return nil // pre-check the stored size; the read caps below anyway
	}
	data, err := s.media.ReadObject(ctx, bucket, path, embedImageMaxBytes+1)
	if err != nil {
		slog.Warn("atproto: media fetch for blob", "path", path, "error", err)
		return nil
	}
	if int64(len(data)) > embedImageMaxBytes {
		return nil // stored size lied — trust the bytes
	}
	blobCid, err := s.PutBlob(ctx, accountID, mime, data)
	if err != nil {
		slog.Warn("atproto: blob put", "path", path, "error", err)
		return nil
	}
	c, err := cid.Parse(blobCid)
	if err != nil {
		slog.Warn("atproto: blob cid parse", "cid", blobCid, "error", err)
		return nil
	}
	return &lexutil.LexBlob{Ref: lexutil.LexLink(c), MimeType: mime, Size: int64(len(data))}
}

// imageEmbed builds the feed.post app.bsky.embed.images from a content
// row's image media — embed order and eligibility come from
// ListAtprotoContentImages (featured/header first, deduped, max 4).
// Returns nil when there's nothing to embed.
func (s *Service) imageEmbed(ctx context.Context, accountID, contentID pgtype.UUID) *bsky.FeedPost_Embed {
	if s.media == nil {
		return nil
	}
	rows, err := s.store.q.ListAtprotoContentImages(ctx, contentID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("atproto: content images", "content_id", contentID.String(), "error", err)
		}
		return nil
	}
	images := make([]*bsky.EmbedImages_Image, 0, len(rows))
	for _, m := range rows {
		blob := s.blobForMedia(ctx, accountID, m.StorageBucket, m.StoragePath, m.MimeType, m.FileSizeBytes)
		if blob == nil {
			continue
		}
		img := &bsky.EmbedImages_Image{Image: blob}
		if m.AltText != nil {
			img.Alt = *m.AltText
		}
		if m.WidthPx != nil && m.HeightPx != nil && *m.WidthPx > 0 && *m.HeightPx > 0 {
			img.AspectRatio = &bsky.EmbedDefs_AspectRatio{
				Width:  int64(*m.WidthPx),
				Height: int64(*m.HeightPx),
			}
		}
		images = append(images, img)
	}
	if len(images) == 0 {
		return nil
	}
	return &bsky.FeedPost_Embed{EmbedImages: &bsky.EmbedImages{Images: images}}
}

// enrichProfileBlobs attaches avatar/banner LexBlob refs to a profile
// record at write time — both profile paths (opt-in publishProfile and
// the pre-marshaled publish_record payload) run through here so the
// task JSON never has to carry blob material. Member removals converge:
// a profile with no avatar_media_id leaves Avatar nil, and the record
// write drops the old ref.
func (s *Service) enrichProfileBlobs(ctx context.Context, accountID, avatarMediaID, bannerMediaID pgtype.UUID, rec *bsky.ActorProfile) {
	if s.media == nil {
		return
	}
	ids := make([]pgtype.UUID, 0, 2)
	for _, id := range []pgtype.UUID{avatarMediaID, bannerMediaID} {
		if id.Valid {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	mediaByID := make(map[string]gen.Medium, len(ids))
	if rows, err := s.store.q.GetMediaByIDs(ctx, ids); err == nil {
		for _, m := range rows {
			mediaByID[m.ID.String()] = m
		}
	} else {
		slog.Warn("atproto: profile media rows", "account_id", accountID.String(), "error", err)
		return
	}
	put := func(id pgtype.UUID) *lexutil.LexBlob {
		m, ok := mediaByID[id.String()]
		if !ok || !m.Confirmed || !strings.HasPrefix(m.MimeType, "image/") || m.MimeType == "image/svg+xml" {
			return nil
		}
		return s.blobForMedia(ctx, accountID, m.StorageBucket, m.StoragePath, m.MimeType, m.FileSizeBytes)
	}
	if avatarMediaID.Valid {
		rec.Avatar = put(avatarMediaID)
	}
	if bannerMediaID.Valid {
		rec.Banner = put(bannerMediaID)
	}
}
