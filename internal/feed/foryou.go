package feed

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// For-You ranking weights (v1 constants; later tunable via system_settings).
const (
	wVelocity     = 1.0
	wAuthor       = 3.0
	wSpace        = 2.5
	wTag          = 1.5
	wRecency      = 2.0
	tagOverlapCap = 3
	halfLifeHours = 48.0
)

// Candidate is one scored row in the For-You candidate pool.
type Candidate struct {
	Item               FeedItem
	CreatedAt          time.Time
	VoteScore          int32
	VelocityScore      float64
	IsAuthorFollowed   bool
	IsSpaceFollowed    bool
	FollowedTagOverlap int
}

// recencyDecay returns an exponential decay in [0,1]; fresher content scores higher.
func recencyDecay(ageHours float64) float64 {
	if ageHours < 0 {
		ageHours = 0
	}

	return math.Exp(-ageHours / halfLifeHours)
}

// computeForYouScore blends the personalization signals into a single score.
// maxVelocity normalizes velocity relative to the fetched candidate pool.
func computeForYouScore(c Candidate, now time.Time, maxVelocity float64) float64 {
	velNorm := 0.0
	if maxVelocity > 0 {
		velNorm = c.VelocityScore / maxVelocity
	}

	author := 0.0
	if c.IsAuthorFollowed {
		author = 1.0
	}

	space := 0.0
	if c.IsSpaceFollowed {
		space = 1.0
	}

	overlap := min(c.FollowedTagOverlap, tagOverlapCap)

	tag := float64(overlap) / float64(tagOverlapCap)
	rec := recencyDecay(now.Sub(c.CreatedAt).Hours())

	return wVelocity*velNorm + wAuthor*author + wSpace*space + wTag*tag + wRecency*rec
}

// rankCandidates orders the pool in place per sortMode and returns it.
func rankCandidates(cands []Candidate, sortMode string, now time.Time) []Candidate {
	maxVel := 0.0
	for i := range cands {
		if cands[i].VelocityScore > maxVel {
			maxVel = cands[i].VelocityScore
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		switch sortMode {
		case SortLatest:
			return cands[i].CreatedAt.After(cands[j].CreatedAt)
		case SortTop:
			return cands[i].VoteScore > cands[j].VoteScore
		case SortRising:
			return cands[i].VelocityScore > cands[j].VelocityScore
		default: // SortRelevant
			return computeForYouScore(cands[i], now, maxVel) > computeForYouScore(cands[j], now, maxVel)
		}
	})

	return cands
}

// forYouCacheKey builds the per-user, per-sort Valkey key.
func forYouCacheKey(tenantID, userID pgtype.UUID, sortMode string) string {
	return fmt.Sprintf("feed:foryou:%s:%s:%s",
		uuidToString(tenantID), uuidToString(userID), sortMode)
}

// candidateFromRow maps a generated candidate row into a scored Candidate.
// vote_score and bookmark_count are nullable (*int32) here because the outer
// SELECT LEFT JOINs content_scores, so they are dereferenced via derefInt32.
func candidateFromRow(r gen.GetForYouCandidatesRow) Candidate {
	return Candidate{
		Item: FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          derefInt32(r.VoteScore),
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      derefInt32(r.BookmarkCount),
		},
		CreatedAt:          r.CreatedAt.Time,
		VoteScore:          derefInt32(r.VoteScore),
		VelocityScore:      pgNumericToFloat64(r.VelocityScore),
		IsAuthorFollowed:   r.IsAuthorFollowed,
		IsSpaceFollowed:    r.IsSpaceFollowed,
		FollowedTagOverlap: int(r.FollowedTagOverlap),
	}
}

// pageSlice returns a bounded page of items; out-of-range offsets yield empty.
func pageSlice(items []FeedItem, offset, limit int32) []FeedItem {
	if offset >= int32(len(items)) {
		return []FeedItem{}
	}

	end := min(offset+limit, int32(len(items)))

	return items[offset:end]
}

// dedupeUUIDs removes duplicate UUIDs while preserving order.
func dedupeUUIDs(in []pgtype.UUID) []pgtype.UUID {
	seen := make(map[string]struct{}, len(in))
	out := make([]pgtype.UUID, 0, len(in))

	for _, id := range in {
		k := uuidToString(id)
		if _, ok := seen[k]; ok {
			continue
		}

		seen[k] = struct{}{}
		out = append(out, id)
	}

	return out
}
