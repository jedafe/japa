package feed

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
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

	// Geo boost (ADR-064 §32): bounded multiplier so local content gets a
	// relevance boost without dominating engagement/velocity signals. The
	// cap ensures geo re-weights, never dominates.
	wGeo        = 1.0
	geoBoostCap = 1.5 // final score multiplied by at most 1.5 for geo-matched content
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
	SpaceGeoCode       string  // ISO country code of the content's space (empty = no geo)
	GeoProximity       float64 // 5=city,4=state,3=country,2=region,1=global/no match
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
// The geo boost is applied from the content's best linked primary geo space
// (city > state > country > region > global). contentCountry is kept for the
// cache key but the actual proximity value is now pre-computed per candidate.
func computeForYouScore(c Candidate, now time.Time, maxVelocity float64, _ string) float64 {
	velNorm := 0.0
	if maxVelocity > 0 {
		velNorm = c.VelocityScore / maxVelocity
	}

	author := 0.0
	if c.IsAuthorFollowed {
		author = 1.0
	}

	// Followed spaces get a strong affinity boost that is larger than the
	// generic geo fallback, matching the requested ranking order.
	space := 0.0
	if c.IsSpaceFollowed {
		space = 1.0
	}

	overlap := min(c.FollowedTagOverlap, tagOverlapCap)

	tag := float64(overlap) / float64(tagOverlapCap)
	rec := recencyDecay(now.Sub(c.CreatedAt).Hours())

	baseScore := wVelocity*velNorm + wAuthor*author + wSpace*space + wTag*tag + wRecency*rec

	// Bounded geo boost: city/state/country/region matches increase the score.
	// Proximity is 1 (none) to 5 (city). Scale the bonus linearly and cap it.
	if c.GeoProximity > 1 {
		boost := 1.0 + wGeo*(c.GeoProximity-1)/4.0
		if boost > geoBoostCap {
			boost = geoBoostCap
		}
		baseScore *= boost
	}

	return baseScore
}

// rankCandidates orders the pool in place per sortMode and returns it.
// contentCountry is the visitor's resolved ContentCountry (ADR-064) used for
// the bounded geo boost; empty string disables geo weighting.
func rankCandidates(cands []Candidate, sortMode string, now time.Time, contentCountry string) []Candidate {
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
			return computeForYouScore(cands[i], now, maxVel, contentCountry) > computeForYouScore(cands[j], now, maxVel, contentCountry)
		}
	})

	return cands
}

// forYouCacheKey builds the per-user, per-sort, per-content-country Valkey
// key. contentCountry is included because the ranking (and therefore the
// cached order) varies with the ADR-064 §32 geo boost — without it, a
// ?geo= override or a preference change could serve a stale, wrong-geo
// ranking for up to forYouCacheTTL. Uses the jc:cache:feed:foryou: prefix
// per Rule 4 (VALKEY-001/005).
func forYouCacheKey(tenantID, userID pgtype.UUID, sortMode, contentCountry string) string {
	return valkey.MustKey("cache:feed:foryou:%s:%s:%s:%s",
		uuidToString(tenantID), uuidToString(userID), sortMode, strings.ToUpper(contentCountry))
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
			SpaceName:          r.SpaceName,
			SpaceSlug:          r.SpaceSlug,
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
		SpaceGeoCode:       derefStr(r.SpaceGeoCode),
		GeoProximity:       interfaceToFloat64(r.GeoProximity),
	}
}

// pageSlice returns a bounded page of items; out-of-range offsets yield empty.
func pageSlice(items []FeedItem, offset, limit int32) []FeedItem {
	n := int32(len(items)) //nolint:gosec // G115: feed length is bounded by the candidate cap (<< int32 max)
	if offset >= n {
		return []FeedItem{}
	}

	end := min(offset+limit, n)

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
