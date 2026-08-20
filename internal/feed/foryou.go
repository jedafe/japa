package feed

import (
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
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

	// Journey-aware recommendation weights.
	wJourneyDestination = 1.5 // target country matches visitor destination
	wJourneyStage       = 1.2 // journey stage matches
	wJourneyVisa        = 1.0 // visa route matches
	wJourneySource      = 0.8 // source/origin country matches
	wJourneyRoadmap     = 1.4 // content slug appears in readiness roadmap
	journeyBoostCap     = 1.8 // max combined journey multiplier
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
	// Journey-aware recommendation metadata.
	JourneyStage  *string
	VisaRoute     *string
	SourceCountry *string
	TargetCountry *string
}

// recencyDecay returns an exponential decay in [0,1]; fresher content scores higher.
func recencyDecay(ageHours float64) float64 {
	if ageHours < 0 {
		ageHours = 0
	}

	return math.Exp(-ageHours / halfLifeHours)
}

// ptrStrEquals reports whether a nullable candidate value matches a context value.
func ptrStrEquals(ptr *string, v string) bool {
	if ptr == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*ptr), strings.TrimSpace(v))
}

// journeyBoost computes a bounded multiplier for journey metadata matching.
// Destination country is the strongest signal, followed by stage, visa route,
// and source country. Roadmap content gets a separate additive bonus.
func journeyBoost(c Candidate, vc *visitor.VisitorContext, roadmapSlugs map[string]bool) float64 {
	if vc == nil {
		return 1.0
	}

	boost := 1.0

	if vc.DestinationCountry != "" && ptrStrEquals(c.TargetCountry, vc.DestinationCountry) {
		boost += wJourneyDestination
	}
	if vc.JourneyStage != "" && ptrStrEquals(c.JourneyStage, vc.JourneyStage) {
		boost += wJourneyStage
	}
	if vc.VisaRoute != "" && ptrStrEquals(c.VisaRoute, vc.VisaRoute) {
		boost += wJourneyVisa
	}
	if vc.OriginCountry != "" && ptrStrEquals(c.SourceCountry, vc.OriginCountry) {
		boost += wJourneySource
	}
	if len(roadmapSlugs) > 0 && c.Item.Slug != "" && roadmapSlugs[c.Item.Slug] {
		boost += wJourneyRoadmap
	}

	if boost > journeyBoostCap {
		return journeyBoostCap
	}
	return boost
}

// computeForYouScore blends the personalization signals into a single score.
// maxVelocity normalizes velocity relative to the fetched candidate pool.
// The geo boost is applied from the content's best linked primary geo space
// (city > state > country > region > global). vc provides journey-aware
// context (destination, stage, visa route, roadmap); nil disables journey
// scoring and global content still competes on other signals.
func computeForYouScore(c Candidate, now time.Time, maxVelocity float64, vc *visitor.VisitorContext, roadmapSlugs map[string]bool) float64 {
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

	// Journey-aware boost: destination-first, then stage/visa/source/roadmap.
	// General content (no journey metadata) receives a neutral 1.0 and still
	// competes via velocity/recency/follows/space geo.
	baseScore *= journeyBoost(c, vc, roadmapSlugs)

	return baseScore
}

// rankCandidates orders the pool in place per sortMode and returns it.
// vc is the visitor's resolved context (ADR-064 / journey-aware); nil disables
// personalization and uses the existing follow/geo/velocity signals only.
func rankCandidates(cands []Candidate, sortMode string, now time.Time, vc *visitor.VisitorContext, roadmapSlugs map[string]bool) []Candidate {
	maxVel := 0.0
	for i := range cands {
		if cands[i].VelocityScore > maxVel {
			maxVel = cands[i].VelocityScore
		}
	}

	// Pre-compute roadmap lookup for O(1) slug checks.
	if roadmapSlugs == nil {
		roadmapSlugs = make(map[string]bool)
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
			return computeForYouScore(cands[i], now, maxVel, vc, roadmapSlugs) > computeForYouScore(cands[j], now, maxVel, vc, roadmapSlugs)
		}
	})

	return cands
}

// journeyCacheToken builds a compact, stable token from the journey context
// so that ranking changes when origin, destination, stage, visa, purpose,
// travel date, or roadmap change without leaking PII into the Valkey key.
func journeyCacheToken(vc *visitor.VisitorContext) string {
	if vc == nil {
		return "0"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToUpper(vc.OriginCountry)))
	_, _ = h.Write([]byte(strings.ToUpper(vc.DestinationCountry)))
	_, _ = h.Write([]byte(vc.JourneyStage))
	_, _ = h.Write([]byte(vc.VisaRoute))
	_, _ = h.Write([]byte(vc.Purpose))
	_, _ = h.Write([]byte(vc.TravelDate))
	slugs := make([]string, len(vc.RoadmapSlugs))
	copy(slugs, vc.RoadmapSlugs)
	sort.Strings(slugs)
	for _, s := range slugs {
		_, _ = h.Write([]byte(s))
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// forYouCacheKey builds the per-user, per-sort, per-content-country,
// per-content-region, and per-journey-context Valkey key. The journey token is
// included because ranking now depends on destination, stage, visa, and roadmap
// signals; without it a context change would serve a stale ranking.
func forYouCacheKey(tenantID, userID pgtype.UUID, sortMode string, vc *visitor.VisitorContext) string {
	contentCountry := ""
	contentRegion := ""
	if vc != nil {
		contentCountry = vc.ContentCountry
		contentRegion = vc.ContentRegion
	}
	return valkey.MustKey("cache:feed:foryou:%s:%s:%s:%s:%s:%s",
		uuidToString(tenantID), uuidToString(userID), sortMode,
		strings.ToUpper(contentCountry), strings.ToUpper(contentRegion),
		journeyCacheToken(vc))
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
			JourneyStage:       r.JourneyStage,
			VisaRoute:          r.VisaRoute,
			SourceCountry:      r.SourceCountry,
			TargetCountry:      r.TargetCountry,
		},
		CreatedAt:          r.CreatedAt.Time,
		VoteScore:          derefInt32(r.VoteScore),
		VelocityScore:      pgNumericToFloat64(r.VelocityScore),
		IsAuthorFollowed:   r.IsAuthorFollowed,
		IsSpaceFollowed:    r.IsSpaceFollowed,
		FollowedTagOverlap: int(r.FollowedTagOverlap),
		SpaceGeoCode:       derefStr(r.SpaceGeoCode),
		GeoProximity:       interfaceToFloat64(r.GeoProximity),
		JourneyStage:       r.JourneyStage,
		VisaRoute:          r.VisaRoute,
		SourceCountry:      r.SourceCountry,
		TargetCountry:      r.TargetCountry,
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
